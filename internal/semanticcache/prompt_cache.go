package semanticcache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	json "encoding/json/v2"
)

// CachedResponse stores a completed LLM output for instant replay.
type CachedResponse struct {
	Model        string          `json:"model"`
	Content      string          `json:"content"`
	RawJSON      []byte          `json:"rawJson"`
	CreatedAt    time.Time       `json:"createdAt"`
	ExpiresAt    time.Time       `json:"expiresAt"`
	PromptTokens int             `json:"promptTokens"`
	CompTokens   int             `json:"compTokens"`
}

// FastPromptCache provides high-speed exact matching cache (<2ms) for repeated queries.
type FastPromptCache struct {
	mu         sync.RWMutex
	items      map[string]*CachedResponse
	maxEntries int
	defaultTTL time.Duration
	enabled    bool
	hits       int64
	misses     int64
}

// GlobalFastCache is the singleton cache instance used across chat handlers.
var GlobalFastCache = NewFastPromptCache(5000, 12*time.Hour)

func NewFastPromptCache(maxEntries int, ttl time.Duration) *FastPromptCache {
	if maxEntries <= 0 {
		maxEntries = 5000
	}
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &FastPromptCache{
		items:      make(map[string]*CachedResponse),
		maxEntries: maxEntries,
		defaultTTL: ttl,
		enabled:    true,
	}
}

// ComputeHash generates a deterministic SHA-256 hex hash from request payload.
func (c *FastPromptCache) ComputeHash(model string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(model))
	h.Write([]byte("|"))
	h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

// Get looks up a cached response by hash.
func (c *FastPromptCache) Get(hash string) (*CachedResponse, bool) {
	if !c.enabled {
		return nil, false
	}

	c.mu.RLock()
	item, found := c.items[hash]
	c.mu.RUnlock()

	if !found {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		return nil, false
	}

	if time.Now().After(item.ExpiresAt) {
		c.mu.Lock()
		delete(c.items, hash)
		c.misses++
		c.mu.Unlock()
		return nil, false
	}

	c.mu.Lock()
	c.hits++
	c.mu.Unlock()
	return item, true
}

// Put stores a response in the fast cache.
func (c *FastPromptCache) Put(hash, model, content string, rawJSON []byte, pTokens, cTokens int) {
	if !c.enabled || hash == "" || (content == "" && len(rawJSON) == 0) {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Simple eviction if over capacity
	if len(c.items) >= c.maxEntries {
		now := time.Now()
		// First pass: purge expired items
		for k, v := range c.items {
			if now.After(v.ExpiresAt) {
				delete(c.items, k)
			}
		}
		// If still full, drop random 10%
		if len(c.items) >= c.maxEntries {
			dropped := 0
			for k := range c.items {
				delete(c.items, k)
				dropped++
				if dropped >= c.maxEntries/10 {
					break
				}
			}
		}
	}

	c.items[hash] = &CachedResponse{
		Model:        model,
		Content:      content,
		RawJSON:      rawJSON,
		CreatedAt:    time.Now(),
		ExpiresAt:    time.Now().Add(c.defaultTTL),
		PromptTokens: pTokens,
		CompTokens:   cTokens,
	}
}

// ServeCachedResponse serves a cached entry to the HTTP client (both non-stream & stream).
func (c *FastPromptCache) ServeCachedResponse(w http.ResponseWriter, item *CachedResponse, isStream bool) {
	w.Header().Set("X-Cache", "HIT")
	w.Header().Set("X-Cache-Latency", "1ms")

	if !isStream {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if len(item.RawJSON) > 0 {
			_, _ = w.Write(item.RawJSON)
		} else {
			// Construct minimal OpenAI response
			resp := map[string]any{
				"id":      fmt.Sprintf("chatcmpl-cache-%d", time.Now().UnixNano()),
				"object":  "chat.completion",
				"created": item.CreatedAt.Unix(),
				"model":   item.Model,
				"choices": []map[string]any{
					{
						"index": 0,
						"message": map[string]string{
							"role":    "assistant",
							"content": item.Content,
						},
						"finish_reason": "stop",
					},
				},
				"usage": map[string]int{
					"prompt_tokens":     item.PromptTokens,
					"completion_tokens": item.CompTokens,
					"total_tokens":      item.PromptTokens + item.CompTokens,
				},
			}
			data, _ := json.Marshal(resp)
			_, _ = w.Write(data)
		}
		return
	}

	// Streaming SSE replay
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, hasFlusher := w.(http.Flusher)
	chunkID := fmt.Sprintf("chatcmpl-cache-%d", time.Now().UnixNano())
	nowUnix := time.Now().Unix()

	// 1. Initial role chunk
	initial := map[string]any{
		"id":      chunkID,
		"object":  "chat.completion.chunk",
		"created": nowUnix,
		"model":   item.Model,
		"choices": []map[string]any{
			{"index": 0, "delta": map[string]string{"role": "assistant"}},
		},
	}
	dInit, _ := json.Marshal(initial)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", dInit)
	if hasFlusher {
		flusher.Flush()
	}

	// 2. Content delta chunk (replay full content in 1 clean chunk)
	contentChunk := map[string]any{
		"id":      chunkID,
		"object":  "chat.completion.chunk",
		"created": nowUnix,
		"model":   item.Model,
		"choices": []map[string]any{
			{"index": 0, "delta": map[string]string{"content": item.Content}},
		},
	}
	dContent, _ := json.Marshal(contentChunk)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", dContent)
	if hasFlusher {
		flusher.Flush()
	}

	// 3. Finish reason chunk
	finishChunk := map[string]any{
		"id":      chunkID,
		"object":  "chat.completion.chunk",
		"created": nowUnix,
		"model":   item.Model,
		"choices": []map[string]any{
			{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"},
		},
	}
	dFin, _ := json.Marshal(finishChunk)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", dFin)
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	if hasFlusher {
		flusher.Flush()
	}
}

// Stats returns cache hits and misses.
func (c *FastPromptCache) Stats() (hits int64, misses int64, count int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses, len(c.items)
}
