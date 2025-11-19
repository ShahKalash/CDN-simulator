package rtt

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Measurer tracks RTT measurements for different peers/endpoints
type Measurer struct {
	mu    sync.RWMutex
	rtts  map[string]int // peer/endpoint -> RTT in milliseconds
	count map[string]int // peer/endpoint -> number of measurements
}

// NewMeasurer creates a new RTT measurer
func NewMeasurer() *Measurer {
	return &Measurer{
		rtts:  make(map[string]int),
		count: make(map[string]int),
	}
}

// MeasureHTTP measures the RTT for an HTTP request
// Returns the RTT in milliseconds and any error
func (m *Measurer) MeasureHTTP(ctx context.Context, client *http.Client, method, url string) (int, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	rtt := int(time.Since(start).Milliseconds())
	return rtt, nil
}

// Update updates the RTT for a peer/endpoint using exponential moving average
// This smooths out fluctuations while still being responsive to changes
func (m *Measurer) Update(peerID string, measuredRTT int) {
	if measuredRTT <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	
	oldRTT, exists := m.rtts[peerID]
	count := m.count[peerID]
	
	if !exists {
		// First measurement, use it directly
		m.rtts[peerID] = measuredRTT
		m.count[peerID] = 1
		return
	}
	
	// Exponential moving average: newRTT = alpha * measured + (1-alpha) * oldRTT
	// Using alpha = 0.3 for good balance between responsiveness and stability
	alpha := 0.3
	newRTT := int(alpha*float64(measuredRTT) + (1-alpha)*float64(oldRTT))
	m.rtts[peerID] = newRTT
	m.count[peerID] = count + 1
}

// Get returns the current RTT for a peer/endpoint
// Returns 0 if no measurement exists
func (m *Measurer) Get(peerID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rtts[peerID]
}

// GetAverage returns the average RTT across all measured peers
// Useful for determining a baseline RTT when no specific measurement exists
func (m *Measurer) GetAverage() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if len(m.rtts) == 0 {
		return 0
	}
	
	sum := 0
	for _, rtt := range m.rtts {
		sum += rtt
	}
	return sum / len(m.rtts)
}

// CalculatePathRTT calculates the total RTT for a multi-hop path
// Returns the sum of RTTs for each hop in the path
func (m *Measurer) CalculatePathRTT(path []string) int {
	if len(path) < 2 {
		return 0
	}
	
	totalRTT := 0
	for i := 0; i < len(path)-1; i++ {
		to := path[i+1]
		
		// Get RTT for this hop
		hopRTT := m.Get(to)
		if hopRTT == 0 {
			// If no measurement, estimate based on average
			avg := m.GetAverage()
			if avg == 0 {
				avg = 25 // Default fallback
			}
			hopRTT = avg
		}
		totalRTT += hopRTT
	}
	
	return totalRTT
}

// GetAll returns all RTT measurements
func (m *Measurer) GetAll() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	result := make(map[string]int, len(m.rtts))
	for k, v := range m.rtts {
		result[k] = v
	}
	return result
}

// GetCount returns the number of measurements for a peer
func (m *Measurer) GetCount(peerID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.count[peerID]
}

