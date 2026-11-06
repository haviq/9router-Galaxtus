package executor

import (
	json "encoding/json/v2"
	"strings"
	"sync"
	"time"
)

type deepSeekAccountSlot struct {
	token            string
	quarantinedUntil time.Time
	inFlight         int
	lastUsed         time.Time
}

// DeepSeekTokenPool manages multiple deepseek-web credentials with auto-quarantine on 429.
type DeepSeekTokenPool struct {
	mu    sync.Mutex
	slots map[string]*deepSeekAccountSlot
}

var globalDeepSeekPool = &DeepSeekTokenPool{
	slots: make(map[string]*deepSeekAccountSlot),
}

// ParseDeepSeekTokens splits single or multi-token strings.
// Supports comma, newline, semicolon, or JSON array formats.
func ParseDeepSeekTokens(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	// Try JSON array first
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		var arr []string
		if err := json.Unmarshal([]byte(raw), &arr); err == nil && len(arr) > 0 {
			var res []string
			seen := make(map[string]bool)
			for _, t := range arr {
				norm := deepSeekUserToken(t)
				if norm != "" && !seen[norm] {
					seen[norm] = true
					res = append(res, norm)
				}
			}
			if len(res) > 0 {
				return res
			}
		}
	}

	// Split by newline, comma, semicolon
	separators := func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	}
	parts := strings.FieldsFunc(raw, separators)
	var tokens []string
	seen := make(map[string]bool)
	for _, p := range parts {
		norm := deepSeekUserToken(p)
		if norm != "" && !seen[norm] {
			seen[norm] = true
			tokens = append(tokens, norm)
		}
	}
	return tokens
}

// PickToken selects the best token from the candidate list:
// It prioritizes non-quarantined tokens with the lowest in-flight count.
func (p *DeepSeekTokenPool) PickToken(candidates []string) string {
	if len(candidates) == 0 {
		return ""
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	var bestToken string
	minInFlight := 999999

	// First pass: find non-quarantined token
	for _, tok := range candidates {
		slot, exists := p.slots[tok]
		if !exists {
			slot = &deepSeekAccountSlot{token: tok}
			p.slots[tok] = slot
		}

		if slot.quarantinedUntil.After(now) {
			continue // skip quarantined
		}

		if slot.inFlight < minInFlight {
			minInFlight = slot.inFlight
			bestToken = tok
		}
	}

	// If all are quarantined, pick the one expiring earliest
	if bestToken == "" {
		earliest := now.Add(24 * time.Hour)
		for _, tok := range candidates {
			slot := p.slots[tok]
			if slot != nil && slot.quarantinedUntil.Before(earliest) {
				earliest = slot.quarantinedUntil
				bestToken = tok
			}
		}
	}

	if bestToken == "" {
		bestToken = candidates[0]
	}

	if slot := p.slots[bestToken]; slot != nil {
		slot.inFlight++
		slot.lastUsed = now
	}
	return bestToken
}

// ReleaseToken decrements in-flight count.
func (p *DeepSeekTokenPool) ReleaseToken(tok string) {
	if tok == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if slot := p.slots[tok]; slot != nil && slot.inFlight > 0 {
		slot.inFlight--
	}
}

// QuarantineToken marks a token as rate-limited (HTTP 429) for a given duration.
func (p *DeepSeekTokenPool) QuarantineToken(tok string, dur time.Duration) {
	if tok == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	slot, exists := p.slots[tok]
	if !exists {
		slot = &deepSeekAccountSlot{token: tok}
		p.slots[tok] = slot
	}
	if dur <= 0 {
		dur = 60 * time.Second
	}
	slot.quarantinedUntil = time.Now().Add(dur)
}

