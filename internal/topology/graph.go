package topology

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type Node struct {
	ID        string `json:"peer_id"`
	Region    string `json:"region,omitempty"`
	RTTms     int    `json:"rtt_ms,omitempty"`
	Neighbors map[string]struct{}
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type Graph struct {
	mu    sync.RWMutex
	nodes map[string]*Node
}

func NewGraph() *Graph {
	return &Graph{
		nodes: make(map[string]*Node),
	}
}

func (g *Graph) Upsert(nodeID string, region string, rtt int, neighbors []string, metadata map[string]any) {
	g.mu.Lock()
	defer g.mu.Unlock()
	node, ok := g.nodes[nodeID]
	if !ok {
		node = &Node{ID: nodeID, Neighbors: make(map[string]struct{})}
		g.nodes[nodeID] = node
	}
	if region != "" {
		node.Region = region
	}
	if rtt > 0 {
		node.RTTms = rtt
	}
	if metadata != nil {
		if node.Metadata == nil {
			node.Metadata = make(map[string]any)
		}
		for k, v := range metadata {
			node.Metadata[k] = v
		}
	}
	node.Neighbors = make(map[string]struct{})
	for _, neighbor := range neighbors {
		neighbor = strings.TrimSpace(neighbor)
		if neighbor == "" || neighbor == nodeID {
			continue
		}
		node.Neighbors[neighbor] = struct{}{}
		// Only add reverse edge if neighbor already exists in the graph.
		// This prevents "ghost" nodes being recreated solely from other peers'
		// neighbor lists after they have been removed.
		if other, ok := g.nodes[neighbor]; ok {
			if other.Neighbors == nil {
				other.Neighbors = make(map[string]struct{})
			}
			other.Neighbors[nodeID] = struct{}{}
		}
	}
}

func (g *Graph) Remove(peerID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.nodes, peerID)
	for _, node := range g.nodes {
		delete(node.Neighbors, peerID)
	}
}

func (g *Graph) Snapshot() map[string][]string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make(map[string][]string, len(g.nodes))
	for id, node := range g.nodes {
		list := make([]string, 0, len(node.Neighbors))
		for neighbor := range node.Neighbors {
			list = append(list, neighbor)
		}
		out[id] = list
	}
	return out
}

func (g *Graph) BFS(from, to string) ([]string, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	start, ok := g.nodes[from]
	if !ok {
		return nil, fmt.Errorf("unknown peer %s", from)
	}
	if _, ok := g.nodes[to]; !ok {
		return nil, fmt.Errorf("unknown peer %s", to)
	}
	type pathNode struct {
		id   string
		path []string
	}
	visited := map[string]struct{}{start.ID: {}}
	queue := []pathNode{{id: start.ID, path: []string{start.ID}}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur.id == to {
			return cur.path, nil
		}
		for neighbor := range g.nodes[cur.id].Neighbors {
			if _, seen := visited[neighbor]; seen {
				continue
			}
			visited[neighbor] = struct{}{}
			nextPath := append(append([]string(nil), cur.path...), neighbor)
			queue = append(queue, pathNode{id: neighbor, path: nextPath})
		}
	}
	return nil, fmt.Errorf("no path between %s and %s", from, to)
}

func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}
