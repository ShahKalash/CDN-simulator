package signalling

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type PeerID string

type Announcement struct {
	Peer      PeerID
	Room      string
	Neighbors []PeerID
	Metadata  map[string]any
}

type PathRequest struct {
	From   PeerID
	To     PeerID
	Room   string
	Reason string
}

type PathResponse struct {
	Path []PeerID
}

type Hub struct {
	mu        sync.RWMutex
	rooms     map[string]map[PeerID]*Connection
	graph     map[PeerID]map[PeerID]struct{}
	roomGraph map[string]map[PeerID]map[PeerID]struct{}
}

func NewHub() *Hub {
	return &Hub{
		rooms:     make(map[string]map[PeerID]*Connection),
		graph:     make(map[PeerID]map[PeerID]struct{}),
		roomGraph: make(map[string]map[PeerID]map[PeerID]struct{}),
	}
}

func (h *Hub) Register(room string, conn *Connection) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.rooms[room]; !ok {
		h.rooms[room] = make(map[PeerID]*Connection)
	}
	h.rooms[room][conn.Peer] = conn
}

func (h *Hub) Unregister(room string, peer PeerID) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if peers, ok := h.rooms[room]; ok {
		delete(peers, peer)
		if len(peers) == 0 {
			delete(h.rooms, room)
		}
	}
	delete(h.graph, peer)
	for _, neighbors := range h.graph {
		delete(neighbors, peer)
	}
	for _, roomGraph := range h.roomGraph {
		delete(roomGraph, peer)
		for _, neighbors := range roomGraph {
			delete(neighbors, peer)
		}
	}
}

func (h *Hub) Announce(room string, ann Announcement) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.graph[ann.Peer]; !ok {
		h.graph[ann.Peer] = make(map[PeerID]struct{})
	}
	targetGraph := h.graph[ann.Peer]
	for neighbor := range targetGraph {
		delete(h.graph[neighbor], ann.Peer)
	}
	h.graph[ann.Peer] = make(map[PeerID]struct{})
	for _, neighbor := range ann.Neighbors {
		h.addUndirectedEdge(h.graph, ann.Peer, neighbor)
	}

	if _, ok := h.roomGraph[room]; !ok {
		h.roomGraph[room] = make(map[PeerID]map[PeerID]struct{})
	}
	roomGraph := h.roomGraph[room]
	if _, ok := roomGraph[ann.Peer]; !ok {
		roomGraph[ann.Peer] = make(map[PeerID]struct{})
	}
	for neighbor := range roomGraph[ann.Peer] {
		delete(roomGraph[neighbor], ann.Peer)
	}
	roomGraph[ann.Peer] = make(map[PeerID]struct{})
	for _, neighbor := range ann.Neighbors {
		h.addUndirectedEdge(roomGraph, ann.Peer, neighbor)
	}
}

func (h *Hub) addUndirectedEdge(g map[PeerID]map[PeerID]struct{}, a, b PeerID) {
	if a == "" || b == "" || a == b {
		return
	}
	if _, ok := g[a]; !ok {
		g[a] = make(map[PeerID]struct{})
	}
	if _, ok := g[b]; !ok {
		g[b] = make(map[PeerID]struct{})
	}
	g[a][b] = struct{}{}
	g[b][a] = struct{}{}
}

func (h *Hub) ShortestPath(room string, from, to PeerID) (PathResponse, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	graph := h.graph
	if roomGraph, ok := h.roomGraph[room]; ok && len(roomGraph) > 0 {
		graph = roomGraph
	}

	_, okFrom := graph[from]
	_, okTo := graph[to]
	if !okFrom || !okTo {
		return PathResponse{}, fmt.Errorf("unknown peer(s) in room %s", room)
	}

	path := bfs(graph, from, to)
	if len(path) == 0 {
		return PathResponse{}, errors.New("no path available")
	}
	return PathResponse{Path: path}, nil
}

func bfs(graph map[PeerID]map[PeerID]struct{}, start, goal PeerID) []PeerID {
	type node struct {
		peer PeerID
		path []PeerID
	}
	visited := map[PeerID]struct{}{start: {}}
	queue := []node{{peer: start, path: []PeerID{start}}}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.peer == goal {
			return cur.path
		}
		for neighbor := range graph[cur.peer] {
			if _, seen := visited[neighbor]; seen {
				continue
			}
			visited[neighbor] = struct{}{}
			nextPath := append(append([]PeerID(nil), cur.path...), neighbor)
			queue = append(queue, node{peer: neighbor, path: nextPath})
		}
	}
	return nil
}

func (h *Hub) BroadcastPath(ctx context.Context, room string, path []PeerID) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	conns, ok := h.rooms[room]
	if !ok {
		return fmt.Errorf("room %s not found", room)
	}
	for _, peer := range path {
		conn, ok := conns[peer]
		if !ok {
			continue
		}
		conn.SendPath(ctx, path)
	}
	return nil
}

