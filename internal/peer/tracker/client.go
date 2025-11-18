package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

type AnnouncePayload struct {
	PeerID    string   `json:"peer_id"`
	Room      string   `json:"room"`
	Region    string   `json:"region"`
	RTTms     int      `json:"rtt_ms"`
	Segments  []string `json:"segments"`
	Neighbors []string `json:"neighbors"`
}

type HeartbeatPayload struct {
	PeerID    string   `json:"peer_id"`
	Segments  []string `json:"segments"`
	Neighbors []string `json:"neighbors"`
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		http: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *Client) Announce(ctx context.Context, payload AnnouncePayload) error {
	return c.post(ctx, "/announce", payload)
}

func (c *Client) Heartbeat(ctx context.Context, payload HeartbeatPayload) error {
	return c.post(ctx, "/heartbeat", payload)
}

func (c *Client) post(ctx context.Context, path string, body any) error {
	if c.baseURL == "" {
		return fmt.Errorf("tracker url not configured")
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("tracker error: %s", resp.Status)
	}
	return nil
}

// MeasureRTT measures the RTT to the tracker
func (c *Client) MeasureRTT(ctx context.Context) (int, error) {
	if c.baseURL == "" {
		return 0, fmt.Errorf("tracker url not configured")
	}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/healthz", nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	rtt := int(time.Since(start).Milliseconds())
	return rtt, nil
}