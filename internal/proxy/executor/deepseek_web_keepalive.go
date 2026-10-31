package executor

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"9router/proxy/internal/db"
	"9router/proxy/internal/log"
	"9router/proxy/internal/models"
)

var (
	deepSeekKeepAliveMu      sync.Mutex
	deepSeekKeepAliveStarted bool
)

const (
	deepSeekKeepAliveInterval = 5 * time.Minute
	deepSeekKeepAliveInitial  = 15 * time.Second
)

// StartDeepSeekWebKeepAlive launches a background heartbeat loop for DeepSeek Web connections
// to keep tokens fresh, prevent inactivity timeouts, and warm up sessions.
func StartDeepSeekWebKeepAlive(ctx context.Context, repo *db.Repo) {
	deepSeekKeepAliveMu.Lock()
	if deepSeekKeepAliveStarted {
		deepSeekKeepAliveMu.Unlock()
		return
	}
	deepSeekKeepAliveStarted = true
	deepSeekKeepAliveMu.Unlock()

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(deepSeekKeepAliveInitial):
		}

		ticker := time.NewTicker(deepSeekKeepAliveInterval)
		defer ticker.Stop()

		deepSeekKeepAliveTick(ctx, repo)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				deepSeekKeepAliveTick(ctx, repo)
			}
		}
	}()
	log.Info("deepseek-web", "keep-alive background heartbeat scheduler started")
}

func deepSeekKeepAliveTick(ctx context.Context, repo *db.Repo) {
	if repo == nil {
		return
	}
	conns, err := repo.GetProviderConnections("deepseek-web", true)
	if err != nil {
		return
	}

	for _, conn := range conns {
		if ctx.Err() != nil {
			return
		}
		if conn == nil || conn.IsActive == 0 {
			continue
		}
		pingDeepSeekConnection(ctx, repo, conn)
	}
}

func pingDeepSeekConnection(ctx context.Context, repo *db.Repo, conn *models.ProviderConnection) {
	raw := map[string]any{}
	if conn.Data != "" {
		_ = json.Unmarshal([]byte(conn.Data), &raw)
	}
	userToken, _ := raw["apiKey"].(string)
	if userToken == "" {
		return
	}

	baseURL := deepSeekWebBase
	if b, ok := raw["baseUrl"].(string); ok && strings.TrimSpace(b) != "" {
		baseURL = b
	}

	stealth := deepSeekStealthFor(raw)
	turn := newDeepSeekTurnFromBaseURL(http.DefaultClient, baseURL, stealth).
		withBrowserFingerprint(stealth.cookie, stealth.deviceID)

	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	_, err := turn.acquireAccessToken(callCtx, userToken)
	if err != nil {
		log.Warn("deepseek-web", "keepalive probe failed", "conn", conn.ID, "error", err)
	} else {
		log.Info("deepseek-web", "keepalive heartbeat successful; session preserved", "conn", conn.ID)
		_ = repo.ClearConnectionRateLimit(conn.ID)
	}
}
