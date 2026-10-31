package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"9router/proxy/internal/providers"
)

func TestForwardGemini_EdgeRelayHeaders(t *testing.T) {
	var gotTarget, gotPath, gotAuth, gotUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("x-relay-target")
		gotPath = r.Header.Get("x-relay-path")
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")

		if gotTarget == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "Missing x-relay-target header"}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"candidates": []}`))
	}))
	defer server.Close()

	cfg := &providers.ProviderConfig{
		BaseURL: server.URL,
		StaticHeaders: map[string]string{
			"x-relay-target": "https://daily-cloudcode-pa.googleapis.com",
			"x-relay-path":   "/",
		},
	}

	body := `{"messages":[{"role":"user","content":"hello"}]}`

	// Test Antigravity streaming
	resp, err := ForwardGemini(
		context.Background(),
		server.Client(),
		cfg,
		"test-api-key",
		body,
		true, // isStream
		"test-project-123",
		"gemini-3.8-flash-tiered",
	)
	if err != nil {
		t.Fatalf("ForwardGemini failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	if gotTarget != "https://daily-cloudcode-pa.googleapis.com" {
		t.Errorf("expected x-relay-target 'https://daily-cloudcode-pa.googleapis.com', got %q", gotTarget)
	}
	expectedPath := "/v1internal:streamGenerateContent?alt=sse"
	if gotPath != expectedPath {
		t.Errorf("expected x-relay-path %q, got %q", expectedPath, gotPath)
	}
	if gotAuth != "Bearer test-api-key" {
		t.Errorf("expected Authorization 'Bearer test-api-key', got %q", gotAuth)
	}
	if gotUA != providers.AntigravityCLIUserAgent {
		t.Errorf("expected User-Agent %q, got %q", providers.AntigravityCLIUserAgent, gotUA)
	}
}

func TestForwardGemini_NonStreamingAndGeminiBeta(t *testing.T) {
	var gotTarget, gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTarget = r.Header.Get("x-relay-target")
		gotPath = r.Header.Get("x-relay-path")

		if gotTarget == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"candidates": []}`))
	}))
	defer server.Close()

	cfg := &providers.ProviderConfig{
		BaseURL: server.URL,
		StaticHeaders: map[string]string{
			"x-relay-target": "https://generativelanguage.googleapis.com",
			"x-relay-path":   "/",
		},
	}

	body := `{"messages":[{"role":"user","content":"hello"}]}`

	// Test standard Gemini non-streaming
	resp, err := ForwardGemini(
		context.Background(),
		server.Client(),
		cfg,
		"test-api-key",
		body,
		false, // isStream = false
		"",    // projectID empty -> standard gemini
		"gemini-2.5-flash",
	)
	if err != nil {
		t.Fatalf("ForwardGemini non-stream failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	if gotTarget != "https://generativelanguage.googleapis.com" {
		t.Errorf("expected x-relay-target 'https://generativelanguage.googleapis.com', got %q", gotTarget)
	}
	expectedPath := "/v1beta/models/gemini-2.5-flash:generateContent"
	if gotPath != expectedPath {
		t.Errorf("expected x-relay-path %q, got %q", expectedPath, gotPath)
	}
}

func TestForwardGemini_StaticHeadersAndOverrides(t *testing.T) {
	var gotCustomHeader, gotOverride string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCustomHeader = r.Header.Get("X-Custom-Foo")
		gotOverride = r.Header.Get("X-Client-Trace")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"candidates": []}`))
	}))
	defer server.Close()

	cfg := &providers.ProviderConfig{
		BaseURL: server.URL,
		StaticHeaders: map[string]string{
			"X-Custom-Foo": "bar",
		},
		HeaderOverrides: map[string]string{
			"X-Client-Trace": "trace-12345",
		},
	}

	body := `{"messages":[{"role":"user","content":"hello"}]}`

	resp, err := ForwardGemini(
		context.Background(),
		server.Client(),
		cfg,
		"test-api-key",
		body,
		false,
		"test-project-123",
		"gemini-3.8-flash-tiered",
	)
	if err != nil {
		t.Fatalf("ForwardGemini failed: %v", err)
	}
	_ = resp.Body.Close()

	if gotCustomHeader != "bar" {
		t.Errorf("expected X-Custom-Foo 'bar', got %q", gotCustomHeader)
	}
	if gotOverride != "trace-12345" {
		t.Errorf("expected X-Client-Trace 'trace-12345', got %q", gotOverride)
	}
}
