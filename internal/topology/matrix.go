package topology

import (
	"fmt"
	"math/rand"
)

// GenerateAdjacencyMatrix generates a random adjacency matrix for peer connections
// connectionProbability: probability that any two peers are connected (0.0 to 1.0)
// Returns a map where matrix[peerA][peerB] = true means peerA connects to peerB
func GenerateAdjacencyMatrix(peerCount int, connectionProbability float64) map[string]map[string]bool {
	matrix := make(map[string]map[string]bool, peerCount)
	
	// Initialize matrix
	for i := 1; i <= peerCount; i++ {
		peerID := fmt.Sprintf("peer-%d", i)
		matrix[peerID] = make(map[string]bool)
	}
	
	// Generate bidirectional connections
	rng := rand.New(rand.NewSource(rand.Int63()))
	
	for i := 1; i <= peerCount; i++ {
		peerA := fmt.Sprintf("peer-%d", i)
		
		for j := 1; j <= peerCount; j++ {
			if i == j {
				continue // Skip self-connections
			}
			
			peerB := fmt.Sprintf("peer-%d", j)
			
			// Generate connection based on probability
			if rng.Float64() < connectionProbability {
				// Bidirectional: if A connects to B, then B connects to A
				matrix[peerA][peerB] = true
				matrix[peerB][peerA] = true
			}
		}
	}
	
	return matrix
}

// GetNeighborsFromMatrix returns the list of neighbors for a peer from the adjacency matrix
func GetNeighborsFromMatrix(matrix map[string]map[string]bool, peerID string) []string {
	neighbors, exists := matrix[peerID]
	if !exists {
		return []string{}
	}
	
	result := make([]string, 0, len(neighbors))
	for neighbor := range neighbors {
		if neighbors[neighbor] {
			result = append(result, neighbor)
		}
	}
	
	return result
}

// MatrixToGraph converts an adjacency matrix to a graph structure
func MatrixToGraph(matrix map[string]map[string]bool) map[string][]string {
	graph := make(map[string][]string)
	
	for peerID, neighbors := range matrix {
		neighborList := make([]string, 0, len(neighbors))
		for neighbor := range neighbors {
			if neighbors[neighbor] {
				neighborList = append(neighborList, neighbor)
			}
		}
		graph[peerID] = neighborList
	}
	
	return graph
}

