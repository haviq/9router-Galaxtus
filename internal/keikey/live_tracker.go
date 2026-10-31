package keikey

import (
	"sync"
	"time"
)

// keyLiveState tracks in-memory live activity for a single API key.
type keyLiveState struct {
	activeCount int
	activeIPs   map[string]int // IP -> concurrent request count
	lastUsedIP  string
	lastUsedAt  time.Time
}

// LiveTracker tracks real-time in-flight requests and active client IPs per API key.
type LiveTracker struct {
	mu   sync.RWMutex
	keys map[string]*keyLiveState
}

var (
	defaultTracker     *LiveTracker
	defaultTrackerOnce sync.Once
)

// DefaultLiveTracker returns the process-wide API key live tracker.
func DefaultLiveTracker() *LiveTracker {
	defaultTrackerOnce.Do(func() {
		defaultTracker = NewLiveTracker()
	})
	return defaultTracker
}

// NewLiveTracker creates a new instance of LiveTracker.
func NewLiveTracker() *LiveTracker {
	return &LiveTracker{
		keys: make(map[string]*keyLiveState),
	}
}

// TrackRequest registers an in-flight request for the given API key and client IP.
// It returns a cleanup function that must be called when the request finishes.
func (t *LiveTracker) TrackRequest(keyID, clientIP string) func() {
	if t == nil || keyID == "" {
		return func() {}
	}
	if clientIP == "" {
		clientIP = "127.0.0.1"
	}

	t.mu.Lock()
	state, exists := t.keys[keyID]
	if !exists {
		state = &keyLiveState{
			activeIPs: make(map[string]int),
		}
		t.keys[keyID] = state
	}
	state.activeCount++
	state.activeIPs[clientIP]++
	state.lastUsedIP = clientIP
	state.lastUsedAt = time.Now().UTC()
	t.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			s, ok := t.keys[keyID]
			if !ok {
				return
			}
			s.activeCount--
			if s.activeCount < 0 {
				s.activeCount = 0
			}
			s.activeIPs[clientIP]--
			if s.activeIPs[clientIP] <= 0 {
				delete(s.activeIPs, clientIP)
			}
			// If idle for a while, we retain lastUsedIP and lastUsedAt for reporting
		})
	}
}

// GetStats returns the current active request count and list of active IPs for an API key.
func (t *LiveTracker) GetStats(keyID string) (activeRequests int, activeIPs []string, lastIP string, lastAt string) {
	if t == nil || keyID == "" {
		return 0, []string{}, "", ""
	}
	t.mu.RLock()
	defer t.mu.RUnlock()

	state, ok := t.keys[keyID]
	if !ok {
		return 0, []string{}, "", ""
	}

	ips := make([]string, 0, len(state.activeIPs))
	for ip := range state.activeIPs {
		ips = append(ips, ip)
	}

	var atStr string
	if !state.lastUsedAt.IsZero() {
		atStr = state.lastUsedAt.Format(time.RFC3339)
	}

	return state.activeCount, ips, state.lastUsedIP, atStr
}

// IsIPActive checks if a specific IP currently has active in-flight requests for the key.
func (t *LiveTracker) IsIPActive(keyID, clientIP string) bool {
	if t == nil || keyID == "" || clientIP == "" {
		return false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()

	state, ok := t.keys[keyID]
	if !ok {
		return false
	}
	return state.activeIPs[clientIP] > 0
}

