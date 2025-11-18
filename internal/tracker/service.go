package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultTTL         = 120 * time.Second
	heartbeatHashKey   = "peers:heartbeat"
	segmentKeyPrefix   = "segment"
	peerSegmentsPrefix = "peer"
)

type Config struct {
	TTL           time.Duration
	TopologyURL   string
	RegionWeights map[string]int
}

type Service struct {
	cfg          Config
	rdb          *redis.Client
	httpClient   *http.Client
	mu           sync.RWMutex
	regionWeight map[string]int
}

func NewService(rdb *redis.Client, cfg Config) *Service {
	if cfg.TTL == 0 {
		cfg.TTL = defaultTTL
	}
	return &Service{
		cfg:        cfg,
		rdb:        rdb,
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

type AnnounceRequest struct {
	PeerID    string   `json:"peer_id"`
	Room      string   `json:"room"`
	Region    string   `json:"region"`
	RTTms     int      `json:"rtt_ms"`
	Segments  []string `json:"segments"`
	Neighbors []string `json:"neighbors"`
}

type HeartbeatRequest struct {
	PeerID    string   `json:"peer_id"`
	Segments  []string `json:"segments"`
	Neighbors []string `json:"neighbors"`
}

type LookupResponse struct {
	Segment string        `json:"segment"`
	Peers   []PeerSummary `json:"peers"`
}

type PeerSummary struct {
	PeerID string `json:"peer_id"`
	Region string `json:"region"`
	RTTms  int    `json:"rtt_ms"`
}

func (s *Service) HandleAnnounce(ctx context.Context, req AnnounceRequest) error {
	if req.PeerID == "" {
		return fmt.Errorf("peer_id required")
	}
	now := time.Now().Unix()
	if err := s.rdb.HSet(ctx, heartbeatHashKey, req.PeerID, now).Err(); err != nil {
		return err
	}
	if err := s.storeSegments(ctx, req.PeerID, req.Segments); err != nil {
		return err
	}
	metaKey := fmt.Sprintf("peer:%s:meta", req.PeerID)
	metaBytes, _ := json.Marshal(req)
	if err := s.rdb.Set(ctx, metaKey, metaBytes, s.cfg.TTL).Err(); err != nil {
		return err
	}
	if err := s.updateTopology(ctx, req.PeerID, req.Region, req.RTTms, req.Neighbors); err != nil {
		return err
	}
	return nil
}

func (s *Service) HandleHeartbeat(ctx context.Context, req HeartbeatRequest) error {
	if req.PeerID == "" {
		return fmt.Errorf("peer_id required")
	}
	now := time.Now().Unix()
	if err := s.rdb.HSet(ctx, heartbeatHashKey, req.PeerID, now).Err(); err != nil {
		return err
	}
	if len(req.Segments) > 0 {
		if err := s.storeSegments(ctx, req.PeerID, req.Segments); err != nil {
			return err
		}
	}
	if len(req.Neighbors) > 0 {
		if err := s.updateTopology(ctx, req.PeerID, "", 0, req.Neighbors); err != nil {
			return err
		}
	}
	peerKey := fmt.Sprintf("peer:%s:meta", req.PeerID)
	s.rdb.Expire(ctx, peerKey, s.cfg.TTL)
	return nil
}

func (s *Service) storeSegments(ctx context.Context, peer string, segments []string) error {
	peerSetKey := fmt.Sprintf("%s:%s:segments", peerSegmentsPrefix, peer)
	existing, err := s.rdb.SMembers(ctx, peerSetKey).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	existingSet := make(map[string]struct{}, len(existing))
	for _, seg := range existing {
		existingSet[seg] = struct{}{}
	}
	newSet := make(map[string]struct{}, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		newSet[seg] = struct{}{}
		segmentKey := fmt.Sprintf("%s:%s", segmentKeyPrefix, seg)
		if err := s.rdb.SAdd(ctx, segmentKey, peer).Err(); err != nil {
			return err
		}
	}
	for seg := range existingSet {
		if _, keep := newSet[seg]; !keep {
			segmentKey := fmt.Sprintf("%s:%s", segmentKeyPrefix, seg)
			s.rdb.SRem(ctx, segmentKey, peer)
		}
	}
	if err := s.rdb.Del(ctx, peerSetKey).Err(); err != nil {
		return err
	}
	if len(newSet) > 0 {
		values := make([]interface{}, 0, len(newSet))
		for seg := range newSet {
			values = append(values, seg)
		}
		if err := s.rdb.SAdd(ctx, peerSetKey, values...).Err(); err != nil {
			return err
		}
	}
	s.rdb.Expire(ctx, peerSetKey, s.cfg.TTL)
	return nil
}

func (s *Service) LookupSegment(ctx context.Context, segment string, preferredRegion string) (LookupResponse, error) {
	segmentKey := fmt.Sprintf("%s:%s", segmentKeyPrefix, segment)
	peerIDs, err := s.rdb.SMembers(ctx, segmentKey).Result()
	if err != nil && err != redis.Nil {
		return LookupResponse{}, err
	}
	summaries := make([]PeerSummary, 0, len(peerIDs))
	for _, id := range peerIDs {
		metaKey := fmt.Sprintf("peer:%s:meta", id)
		raw, err := s.rdb.Get(ctx, metaKey).Bytes()
		if err != nil {
			continue
		}
		var ann AnnounceRequest
		if err := json.Unmarshal(raw, &ann); err != nil {
			continue
		}
		summaries = append(summaries, PeerSummary{
			PeerID: id,
			Region: ann.Region,
			RTTms:  ann.RTTms,
		})
	}
	sortPeers(summaries, preferredRegion)
	return LookupResponse{
		Segment: segment,
		Peers:   summaries,
	}, nil
}

func sortPeers(peers []PeerSummary, preferredRegion string) {
	less := func(i, j int) bool {
		ri := peers[i].Region == preferredRegion
		rj := peers[j].Region == preferredRegion
		if ri != rj {
			return ri
		}
		return peers[i].RTTms < peers[j].RTTms
	}
	for i := 0; i < len(peers); i++ {
		for j := i + 1; j < len(peers); j++ {
			if !less(i, j) {
				peers[i], peers[j] = peers[j], peers[i]
			}
		}
	}
}

func (s *Service) StartReaper(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.reap(ctx)
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()
}

func (s *Service) reap(ctx context.Context) {
	entries, err := s.rdb.HGetAll(ctx, heartbeatHashKey).Result()
	if err != nil {
		return
	}
	now := time.Now().Unix()
	for peer, tsStr := range entries {
		ts := parseInt64(tsStr)
		if now-ts > int64(s.cfg.TTL.Seconds()) {
			s.removePeer(ctx, peer)
		}
	}
}

func parseInt64(val string) int64 {
	var out int64
	fmt.Sscanf(val, "%d", &out)
	return out
}

func (s *Service) removePeer(ctx context.Context, peer string) {
	peerSetKey := fmt.Sprintf("%s:%s:segments", peerSegmentsPrefix, peer)
	segments, _ := s.rdb.SMembers(ctx, peerSetKey).Result()
	for _, seg := range segments {
		segmentKey := fmt.Sprintf("%s:%s", segmentKeyPrefix, seg)
		s.rdb.SRem(ctx, segmentKey, peer)
	}
	s.rdb.Del(ctx, peerSetKey)
	s.rdb.HDel(ctx, heartbeatHashKey, peer)
	metaKey := fmt.Sprintf("peer:%s:meta", peer)
	s.rdb.Del(ctx, metaKey)
	if s.cfg.TopologyURL != "" {
		req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("%s/peers/%s", strings.TrimSuffix(s.cfg.TopologyURL, "/"), peer), nil)
		s.httpClient.Do(req)
	}
}

func (s *Service) updateTopology(ctx context.Context, peerID, region string, rtt int, neighbors []string) error {
	if s.cfg.TopologyURL == "" {
		return nil
	}
	payload := map[string]any{
		"peer_id":   peerID,
		"neighbors": neighbors,
	}
	if region != "" {
		payload["region"] = region
	}
	if rtt > 0 {
		payload["rtt_ms"] = rtt
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/peers", strings.TrimSuffix(s.cfg.TopologyURL, "/")), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
