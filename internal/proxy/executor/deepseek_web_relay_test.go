package executor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	json "encoding/json/v2"

	"9router/proxy/internal/providers"
	"9router/proxy/internal/proxy"
)

// deepSeekRelayFake simulates an edge relay (e.g. Vercel, Cloudflare, Deno).
// It rejects any request without x-relay-target with HTTP 400.
// On success, it forwards according to x-relay-path.
type deepSeekRelayFake struct {
	*httptest.Server
	mu         sync.Mutex
	gotTargets []string
	gotPaths   []string
	gotHeaders []http.Header
}

func newDeepSeekRelayFake(t *testing.T, upstream http.Handler) *deepSeekRelayFake {
	t.Helper()
	fake := &deepSeekRelayFake{}
	fake.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		target := r.Header.Get("x-relay-target")
		path := r.Header.Get("x-relay-path")
		fake.gotTargets = append(fake.gotTargets, target)
		fake.gotPaths = append(fake.gotPaths, path)
		fake.gotHeaders = append(fake.gotHeaders, r.Header.Clone())
		fake.mu.Unlock()

		if target == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"Missing x-relay-target header"}`))
			return
		}

		// Rewrite request URL path to x-relay-path to simulate upstream forwarding
		r.URL.Path = path
		upstream.ServeHTTP(w, r)
	}))
	t.Cleanup(fake.Close)
	return fake
}

func TestDeepSeekWeb_EdgeRelayForwarding(t *testing.T) {
	// Fake upstream that serves valid deepseek-web endpoints
	upstream := newDeepSeekFakeUpstream(t,
		`data: {"p":"response/fragments","v":[{"type":"ANSWER","content":"Hello from DeepSeek via relay"}]}`+"\n\n",
		`data: {"p":"response/status","v":"FINISHED"}`+"\n\n",
	)

	// Fake edge relay fronting the upstream
	relay := newDeepSeekRelayFake(t, upstream)

	// Base target configuration as registered in 9router
	providerCfg, ok := providers.KnownProviders["deepseek-web"]
	if !ok {
		t.Fatal("deepseek-web not in KnownProviders")
	}

	// Chat connections logic builds edge relay headers
	relayHeaders := proxy.BuildEdgeRelayHeaders(providerCfg.BaseURL, map[string]string{
		"x-custom-test": "operator-val",
	})
	if relayHeaders["x-relay-target"] == "" {
		t.Fatal("x-relay-target header missing")
	}

	cfg := &providers.ProviderConfig{
		BaseURL:       relay.URL,
		StaticHeaders: relayHeaders,
	}

	body, _ := json.Marshal(map[string]any{
		"model": "deepseek-chat",
		"messages": []map[string]any{
			{"role": "user", "content": "ping"},
		},
		"stream": false,
	})

	rec := httptest.NewRecorder()
	req := &Request{
		Ctx:      context.Background(),
		Client:   http.DefaultClient,
		Config:   cfg,
		Provider: "deepseek-web",
		APIKey:   "fake-user-token",
		Body:     body,
		IsStream: false,
		ConnData: map[string]any{
			"deepSeekCookie": "user_session=abc123xyz",
		},
	}

	err := ForwardDeepSeekWeb(rec, req)
	if err != nil {
		t.Fatalf("ForwardDeepSeekWeb failed: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status code %d: %s", rec.Code, rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), "Hello from DeepSeek via relay") {
		t.Fatalf("response missing expected text: %s", rec.Body.String())
	}

	// Verify that ALL requests sent to the relay carried x-relay-target and valid x-relay-path
	relay.mu.Lock()
	defer relay.mu.Unlock()

	if len(relay.gotTargets) == 0 {
		t.Fatal("relay received zero calls")
	}

	expectedTarget := "https://chat.deepseek.com"
	expectedPaths := []string{
		"/api/v0/users/current",
		"/api/v0/chat_session/create",
		"/api/v0/chat/create_pow_challenge",
		"/api/v0/chat/completion",
	}

	for i, target := range relay.gotTargets {
		if target != expectedTarget {
			t.Errorf("call %d: x-relay-target = %q, want %q", i, target, expectedTarget)
		}
	}

	for _, wantPath := range expectedPaths {
		found := false
		for _, gotPath := range relay.gotPaths {
			if gotPath == wantPath {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected path %q was not called on relay; got paths: %v", wantPath, relay.gotPaths)
		}
	}

	// Verify custom headers and cookies were passed to relay
	for i, headers := range relay.gotHeaders {
		if headers.Get("x-custom-test") != "operator-val" {
			t.Errorf("call %d: missing custom header x-custom-test", i)
		}
	}
}
