package chat

import (
	"sync"
	"time"

	"9router/proxy/internal/models"
)

// ConnectionLoadTracker tracks active in-flight requests and recent latency
// per upstream connection to enable Least-Busy load balancing.
type ConnectionLoadTracker struct {
	mu          sync.RWMutex
	inFlight    map[string]int           // connectionID -> active count
	latencies   map[string]time.Duration // connectionID -> exponential moving average latency
	failures    map[string]int           // connectionID -> consecutive failures
	cooldowns   map[string]time.Time     // connectionID -> cooled down until
}

var globalLoadTracker = newConnectionLoadTracker()

func newConnectionLoadTracker() *ConnectionLoadTracker {
	return &ConnectionLoadTracker{
		inFlight:  make(map[string]int),
		latencies: make(map[string]time.Duration),
		failures:  make(map[string]int),
		cooldowns: make(map[string]time.Time),
	}
}

// Acquire marks a request in flight for a connection and returns a release callback.
func (t *ConnectionLoadTracker) Acquire(connID string) func(success bool, latency time.Duration) {
	if connID == "" {
		return func(bool, time.Duration) {}
	}

	t.mu.Lock()
	t.inFlight[connID]++
	t.mu.Unlock()

	var once sync.Once
	return func(success bool, latency time.Duration) {
		once.Do(func() {
			t.mu.Lock()
			defer t.mu.Unlock()

			if t.inFlight[connID] > 0 {
				t.inFlight[connID]--
			}
			if latency > 0 {
				prev, exists := t.latencies[connID]
				if !exists || prev == 0 {
					t.latencies[connID] = latency
				} else {
					// EWMA with alpha = 0.3 for responsive latency tracking
					t.latencies[connID] = time.Duration(float64(prev)*0.7 + float64(latency)*0.3)
				}
			}

			if success {
				t.failures[connID] = 0
			} else {
				t.failures[connID]++
				// Back off for 15s after 3 consecutive failures
				if t.failures[connID] >= 3 {
					t.cooldowns[connID] = time.Now().Add(15 * time.Second)
				}
			}
		})
	}
}

// InFlightCount returns the current concurrent requests running on a connection.
func (t *ConnectionLoadTracker) InFlightCount(connID string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.inFlight[connID]
}

// AverageLatency returns the EWMA latency for a connection.
func (t *ConnectionLoadTracker) AverageLatency(connID string) time.Duration {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.latencies[connID]
}

// IsCooledDown checks if connection is temporarily backed off.
func (t *ConnectionLoadTracker) IsCooledDown(connID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	until, exists := t.cooldowns[connID]
	if !exists {
		return false
	}
	return time.Now().Before(until)
}

// SortByLeastBusy orders connections prioritizing:
// 1. Not in temporary cooldown
// 2. Fewest in-flight requests (idle 0 in-flight accounts first)
// 3. Lowest recorded average latency
// 4. Fallback to recency / priority order
func (t *ConnectionLoadTracker) SortByLeastBusy(conns []*models.ProviderConnection) []*models.ProviderConnection {
	if len(conns) <= 1 {
		return conns
	}

	result := make([]*models.ProviderConnection, len(conns))
	copy(result, conns)

	t.mu.RLock()
	defer t.mu.RUnlock()

	now := time.Now()

	// Bubble/Insertion sort for stability and small slice sizes (typically 1-50 connections)
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			c1 := result[i]
			c2 := result[j]

			cooled1 := t.cooldowns[c1.ID].After(now)
			cooled2 := t.cooldowns[c2.ID].After(now)

			// Cooled down connections sort after non-cooled down
			if cooled1 != cooled2 {
				if cooled1 {
					result[i], result[j] = result[j], result[i]
				}
				continue
			}

			// Fewest in-flight requests first
			f1 := t.inFlight[c1.ID]
			f2 := t.inFlight[c2.ID]
			if f1 != f2 {
				if f1 > f2 {
					result[i], result[j] = result[j], result[i]
				}
				continue
			}

			// Lower latency first (if both have latency recorded)
			l1 := t.latencies[c1.ID]
			l2 := t.latencies[c2.ID]
			if l1 > 0 && l2 > 0 && l1 != l2 {
				if l1 > l2 {
					result[i], result[j] = result[j], result[i]
				}
				continue
			}
		}
	}

	return result
}

