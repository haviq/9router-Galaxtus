package keikey

import (
	"sync"
)

// TelemetryRecorder records usage and detected client telemetry.
type TelemetryRecorder interface {
	RecordApiKeyUsageWithClient(keyID, ip, userAgent string) error
}

type telemetryEvent struct {
	recorder  TelemetryRecorder
	keyID     string
	ip        string
	userAgent string
}

const (
	// DefaultTelemetryQueueCapacity bounds pending async telemetry entries.
	// 50,000 entries can easily buffer massive concurrent spikes.
	DefaultTelemetryQueueCapacity = 50000
)

var (
	telemetryQueueOnce sync.Once
	telemetryChan      chan telemetryEvent
	telemetryWG        sync.WaitGroup
)

func initTelemetryQueue() {
	telemetryChan = make(chan telemetryEvent, DefaultTelemetryQueueCapacity)
	go telemetryWorker()
}

// telemetryWorker executes sequentially in the background. Because SQLite
// has a single-writer lock anyway, having a single dedicated worker serialize writes
// completely eliminates lock contention (SQLITE_BUSY) on the hot request path.
func telemetryWorker() {
	for ev := range telemetryChan {
		if ev.recorder != nil && ev.keyID != "" {
			_ = ev.recorder.RecordApiKeyUsageWithClient(ev.keyID, ev.ip, ev.userAgent)
		}
		telemetryWG.Done()
	}
}

// EnqueueTelemetry queues an async telemetry recording. It never blocks the caller:
// if the buffer is full under extreme load, it silently drops the event rather
// than degrading proxy latency or dropping user requests.
func EnqueueTelemetry(recorder TelemetryRecorder, keyID, ip, userAgent string) bool {
	telemetryQueueOnce.Do(initTelemetryQueue)
	if recorder == nil || keyID == "" {
		return false
	}
	ev := telemetryEvent{
		recorder:  recorder,
		keyID:     keyID,
		ip:        ip,
		userAgent: userAgent,
	}

	telemetryWG.Add(1)
	select {
	case telemetryChan <- ev:
		return true
	default:
		telemetryWG.Done()
		// Queue saturated under massive traffic spike: preserve proxy throughput
		return false
	}
}

// FlushTelemetry waits for all pending telemetry events to be written.
// Useful during graceful shutdown or unit tests.
func FlushTelemetry() {
	telemetryWG.Wait()
}
