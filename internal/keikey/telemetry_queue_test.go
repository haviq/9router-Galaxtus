package keikey

import (
	"sync"
	"testing"
	"time"
)

type mockTelemetryRecorder struct {
	mu    sync.Mutex
	calls []string
}

func (m *mockTelemetryRecorder) RecordApiKeyUsageWithClient(keyID, ip, userAgent string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, keyID+":"+ip+":"+userAgent)
	return nil
}

func TestEnqueueTelemetry(t *testing.T) {
	mock := &mockTelemetryRecorder{}
	ok := EnqueueTelemetry(mock, "key-123", "127.0.0.1", "curl/8.0")
	if !ok {
		t.Fatal("expected EnqueueTelemetry to return true")
	}

	// Wait briefly for worker to consume
	deadline := time.Now().Add(2 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		mock.mu.Lock()
		if len(mock.calls) > 0 && mock.calls[0] == "key-123:127.0.0.1:curl/8.0" {
			found = true
			mock.mu.Unlock()
			break
		}
		mock.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}

	if !found {
		t.Fatal("worker did not process enqueued telemetry event")
	}
}

func TestEnqueueTelemetryNilInputs(t *testing.T) {
	if EnqueueTelemetry(nil, "key-123", "127.0.0.1", "ua") {
		t.Fatal("expected false for nil recorder")
	}
	mock := &mockTelemetryRecorder{}
	if EnqueueTelemetry(mock, "", "127.0.0.1", "ua") {
		t.Fatal("expected false for empty keyID")
	}
}

