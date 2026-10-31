package keikey

import (
	"sync"
	"testing"
)

func TestLiveTracker_TrackAndCleanup(t *testing.T) {
	tracker := NewLiveTracker()
	keyID := "key-123"
	ip1 := "192.168.1.10"
	ip2 := "10.0.0.5"

	cleanup1 := tracker.TrackRequest(keyID, ip1)
	cleanup2 := tracker.TrackRequest(keyID, ip1)
	cleanup3 := tracker.TrackRequest(keyID, ip2)

	activeReqs, activeIPs, lastIP, _ := tracker.GetStats(keyID)
	if activeReqs != 3 {
		t.Fatalf("expected 3 active requests, got %d", activeReqs)
	}
	if len(activeIPs) != 2 {
		t.Fatalf("expected 2 active IPs, got %d", len(activeIPs))
	}
	if lastIP != ip2 {
		t.Fatalf("expected last IP %s, got %s", ip2, lastIP)
	}
	if !tracker.IsIPActive(keyID, ip1) {
		t.Fatalf("expected IP1 to be active")
	}

	// Clean up one request from IP1
	cleanup1()
	activeReqs, activeIPs, _, _ = tracker.GetStats(keyID)
	if activeReqs != 2 {
		t.Fatalf("expected 2 active requests, got %d", activeReqs)
	}
	if !tracker.IsIPActive(keyID, ip1) {
		t.Fatalf("IP1 should still be active because cleanup2 is still running")
	}

	// Clean up second request from IP1
	cleanup2()
	if tracker.IsIPActive(keyID, ip1) {
		t.Fatalf("IP1 should no longer be active")
	}

	// Clean up IP2
	cleanup3()
	activeReqs, activeIPs, _, _ = tracker.GetStats(keyID)
	if activeReqs != 0 {
		t.Fatalf("expected 0 active requests, got %d", activeReqs)
	}
	if len(activeIPs) != 0 {
		t.Fatalf("expected 0 active IPs, got %d", len(activeIPs))
	}
}

func TestLiveTracker_ConcurrentAccess(t *testing.T) {
	tracker := NewLiveTracker()
	keyID := "key-concur"
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ip := "192.168.1.1"
			cleanup := tracker.TrackRequest(keyID, ip)
			cleanup()
		}(i)
	}
	wg.Wait()

	activeReqs, _, _, _ := tracker.GetStats(keyID)
	if activeReqs != 0 {
		t.Fatalf("expected 0 active requests after all completed, got %d", activeReqs)
	}
}

