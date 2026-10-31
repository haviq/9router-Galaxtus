package rewrite

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"9router/proxy/internal/db"
	json "9router/proxy/internal/fastjson"
	"9router/proxy/internal/log"
)

type contextKey struct{}

var rewrittenKey = contextKey{}

// Config contains settings for the chat completions rewrite flow.
type Config struct {
	Enabled         bool
	SystemPrompt    string
	AssistantPrompt string
	Temperature     float64
	TopP            float64
}

// DefaultConfig provides fallback values for the rewrite flow.
func DefaultConfig() Config {
	return Config{
		Enabled:         false,
		SystemPrompt:    "You are a helpful AI assistant.",
		AssistantPrompt: "Understood. I am ready to assist you.",
		Temperature:     0.9,
		TopP:            0.95,
	}
}

// LoadConfig resolves active settings from SQLite repo, text files, and environment variables.
func LoadConfig(repo *db.Repo) Config {
	cfg := DefaultConfig()

	// 1. Text files (rewrite_system.txt and rewrite_assistant.txt) as file-based fallback
	if data, err := os.ReadFile("rewrite_system.txt"); err == nil && len(bytes.TrimSpace(data)) > 0 {
		cfg.SystemPrompt = string(bytes.TrimSpace(data))
	}
	if data, err := os.ReadFile("rewrite_assistant.txt"); err == nil && len(bytes.TrimSpace(data)) > 0 {
		cfg.AssistantPrompt = string(bytes.TrimSpace(data))
	}

	// 2. Database Settings (operator settings take precedence over text files)
	if repo != nil {
		if s, err := repo.GetSettings(); err == nil && s != nil {
			cfg.Enabled = s.ChatRewriteEnabled
			if s.ChatRewriteSystem != "" {
				cfg.SystemPrompt = s.ChatRewriteSystem
			}
			if s.ChatRewriteAssistant != "" {
				cfg.AssistantPrompt = s.ChatRewriteAssistant
			}
			if s.ChatRewriteTemperature > 0 {
				cfg.Temperature = s.ChatRewriteTemperature
			}
			if s.ChatRewriteTopP > 0 {
				cfg.TopP = s.ChatRewriteTopP
			}
		}
	}

	// 3. Environment variables (highest priority)
	if envVal := os.Getenv("CHAT_REWRITE_ENABLED"); envVal != "" {
		cfg.Enabled = envVal == "true" || envVal == "1"
	}
	if envVal := os.Getenv("CHAT_REWRITE_SYSTEM"); envVal != "" {
		cfg.SystemPrompt = envVal
	}
	if envVal := os.Getenv("CHAT_REWRITE_ASSISTANT"); envVal != "" {
		cfg.AssistantPrompt = envVal
	}
	if envVal := os.Getenv("CHAT_REWRITE_TEMPERATURE"); envVal != "" {
		if f, err := strconv.ParseFloat(envVal, 64); err == nil {
			cfg.Temperature = f
		}
	}
	if envVal := os.Getenv("CHAT_REWRITE_TOP_P"); envVal != "" {
		if f, err := strconv.ParseFloat(envVal, 64); err == nil {
			cfg.TopP = f
		}
	}

	return cfg
}

// ShouldRewrite reports whether the request path triggers the rewrite flow.
// Matches when the path or URI contains "/v1/chat/completions" or "/chat/completions".
func ShouldRewrite(path, uri string) bool {
	p := strings.ToLower(path)
	u := strings.ToLower(uri)
	return strings.Contains(p, "/v1/chat/completions") ||
		strings.Contains(p, "/chat/completions") ||
		strings.Contains(u, "/v1/chat/completions") ||
		strings.Contains(u, "/chat/completions")
}

// IsRewritten checks if the request was already rewritten in this turn.
func IsRewritten(ctx context.Context) bool {
	v, ok := ctx.Value(rewrittenKey).(bool)
	return ok && v
}

// WithRewritten marks a context as having already undergone rewrite.
func WithRewritten(ctx context.Context) context.Context {
	return context.WithValue(ctx, rewrittenKey, true)
}

// ApplyChatRewrite performs the JSON body transformations:
// 1. Parses JSON body
// 2. Removes all element messages where role == "system"
// 3. Inserts two new messages at index 0 and 1:
//    - index 0: {"role": "system", "content": cfg.SystemPrompt}
//    - index 1: {"role": "assistant", "content": cfg.AssistantPrompt}
// 4. Sets temperature = cfg.Temperature (0.9)
// 5. Sets top_p = cfg.TopP (0.95)
func ApplyChatRewrite(body []byte, cfg Config) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	// Filter messages: remove all role == "system"
	var filtered []any
	if rawMsgs, ok := payload["messages"].([]any); ok {
		filtered = make([]any, 0, len(rawMsgs))
		for _, msg := range rawMsgs {
			if m, isMap := msg.(map[string]any); isMap {
				if role, hasRole := m["role"].(string); hasRole && strings.EqualFold(role, "system") {
					continue // Drop existing system messages
				}
			}
			filtered = append(filtered, msg)
		}
	} else {
		filtered = make([]any, 0, 2)
	}

	// Insert two new messages at index 0 and 1
	newMessages := make([]any, 0, len(filtered)+2)
	newMessages = append(newMessages, map[string]any{
		"role":    "system",
		"content": cfg.SystemPrompt,
	})
	newMessages = append(newMessages, map[string]any{
		"role":    "assistant",
		"content": cfg.AssistantPrompt,
	})
	newMessages = append(newMessages, filtered...)

	payload["messages"] = newMessages
	payload["temperature"] = cfg.Temperature
	payload["top_p"] = cfg.TopP

	return json.Marshal(payload)
}

// HTTPMiddleware returns a middleware that executes the rewrite flow.
// Note: HTTP request headers (r.Header) remain completely UNTOUCHED.
func HTTPMiddleware(repo *db.Repo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || !ShouldRewrite(r.URL.Path, r.RequestURI) {
				next.ServeHTTP(w, r)
				return
			}

			cfg := LoadConfig(repo)
			if !cfg.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			_ = r.Body.Close()

			rewrittenBytes, err := ApplyChatRewrite(bodyBytes, cfg)
			if err != nil {
				// On invalid JSON, restore original body and proceed
				r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				next.ServeHTTP(w, r)
				return
			}

			// Replace r.Body with rewritten payload; r.Header is left 100% UNMODIFIED.
			r.Body = io.NopCloser(bytes.NewReader(rewrittenBytes))
			r.ContentLength = int64(len(rewrittenBytes))

			ctx := WithRewritten(r.Context())
			r = r.WithContext(ctx)

			log.Info("chat_rewrite", "applied HTTP rewrite",
				"path", r.URL.Path,
				"temperature", cfg.Temperature,
				"top_p", cfg.TopP,
				"bytes", len(rewrittenBytes))

			next.ServeHTTP(w, r)
		})
	}
}

