package tokenpool

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultCooldown is the quarantine cooldown for rate-limited (429) tokens.
const DefaultCooldown = 60 * time.Second

// AuthFailureCooldown is the quarantine cooldown for revoked or forbidden (401/403) tokens.
const AuthFailureCooldown = 10 * time.Minute

// TokenPool manages a collection of credentials for a single connection.
type TokenPool struct {
	mu         sync.RWMutex
	connID     string
	rawKeys    string
	tokens     []string
	cursor     uint64
	quarantine map[string]time.Time
}

// GlobalManager manages token pools across connection IDs.
type GlobalManager struct {
	mu    sync.RWMutex
	pools map[string]*TokenPool
}

var globalMgr = &GlobalManager{
	pools: make(map[string]*TokenPool),
}

// GetGlobalManager returns the singleton GlobalManager instance.
func GetGlobalManager() *GlobalManager {
	return globalMgr
}

// ParseTokens extracts individual tokens from comma, semicolon, or newline-delimited strings.
// Comments starting with # or // and surrounding quotes are automatically stripped.
func ParseTokens(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	lines := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})

	var tokens []string
	seen := make(map[string]bool)
	for _, line := range lines {
		t := strings.TrimSpace(line)
		t = strings.Trim(t, `"'`)
		if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "//") {
			continue
		}
		if !seen[t] {
			seen[t] = true
			tokens = append(tokens, t)
		}
	}
	return tokens
}

// FirstToken returns the first usable token from a raw string, or the raw string if empty.
func FirstToken(raw string) string {
	tokens := ParseTokens(raw)
	if len(tokens) > 0 {
		return tokens[0]
	}
	return strings.TrimSpace(raw)
}

// HasMultipleTokens reports whether raw contains more than one token.
func HasMultipleTokens(raw string) bool {
	return len(ParseTokens(raw)) > 1
}

// MaskToken returns a safe, redacted version of a token for logging (e.g. sk-12...34).
func MaskToken(token string) string {
	t := strings.TrimSpace(token)
	if len(t) <= 8 {
		return "***"
	}
	return t[:min(4, len(t))] + "..." + t[len(t)-min(4, len(t)):]
}

// GetPool returns or initializes a TokenPool for the specified connection ID and raw keys.
func (m *GlobalManager) GetPool(connID string, rawKeys string) *TokenPool {
	m.mu.Lock()
	defer m.mu.Unlock()

	poolKey := connID
	if poolKey == "" {
		h := sha256.Sum256([]byte(rawKeys))
		poolKey = hex.EncodeToString(h[:8])
	}

	pool, exists := m.pools[poolKey]
	tokens := ParseTokens(rawKeys)
	if !exists {
		pool = &TokenPool{
			connID:     connID,
			rawKeys:    rawKeys,
			tokens:     tokens,
			quarantine: make(map[string]time.Time),
		}
		m.pools[poolKey] = pool
		return pool
	}

	// Update token list if credentials changed
	if pool.rawKeys != rawKeys {
		pool.mu.Lock()
		pool.rawKeys = rawKeys
		pool.tokens = tokens
		pool.mu.Unlock()
	}

	return pool
}

// Total returns the total number of tokens in the pool.
func (p *TokenPool) Total() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.tokens)
}

// HasMultiple reports whether the pool contains more than one token.
func (p *TokenPool) HasMultiple() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.tokens) > 1
}

// GetNext returns the next un-quarantined token round-robin, along with pool counts.
// If all tokens are currently in quarantine, it falls open to the earliest-expiring token.
func (p *TokenPool) GetNext() (token string, total int, available int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	total = len(p.tokens)
	if total == 0 {
		return "", 0, 0
	}
	if total == 1 {
		return p.tokens[0], 1, 1
	}

	now := time.Now()
	var active []string
	var earliestExpiry time.Time
	var earliestToken string

	for _, t := range p.tokens {
		expiresAt, quarantined := p.quarantine[t]
		if quarantined {
			if now.After(expiresAt) {
				delete(p.quarantine, t)
				active = append(active, t)
			} else {
				if earliestExpiry.IsZero() || expiresAt.Before(earliestExpiry) {
					earliestExpiry = expiresAt
					earliestToken = t
				}
			}
		} else {
			active = append(active, t)
		}
	}

	available = len(active)
	if available > 0 {
		idx := atomic.AddUint64(&p.cursor, 1) - 1
		return active[idx%uint64(available)], total, available
	}

	// If all keys are in cooldown, fall open with earliest expiry
	if earliestToken != "" {
		return earliestToken, total, 0
	}
	return p.tokens[0], total, 0
}

// Quarantine marks a failed token into cooldown for the given duration.
func (p *TokenPool) Quarantine(token string, duration time.Duration) {
	if token == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if duration <= 0 {
		duration = DefaultCooldown
	}
	p.quarantine[token] = time.Now().Add(duration)
}

// ReportSuccess removes a token from quarantine upon successful completion.
func (p *TokenPool) ReportSuccess(token string) {
	if token == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.quarantine, token)
}

// ActiveCount returns the number of active, non-quarantined tokens.
func (p *TokenPool) ActiveCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	count := 0
	for _, t := range p.tokens {
		expiresAt, quarantined := p.quarantine[t]
		if quarantined {
			if now.After(expiresAt) {
				delete(p.quarantine, t)
				count++
			}
		} else {
			count++
		}
	}
	return count
}
