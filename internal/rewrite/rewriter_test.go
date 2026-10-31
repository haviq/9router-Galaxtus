package rewrite

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	json "9router/proxy/internal/fastjson"
)

func TestApplyChatRewrite(t *testing.T) {
	inputJSON := `{
		"model": "deepseek-chat",
		"messages": [
			{"role": "system", "content": "old system 1"},
			{"role": "user", "content": "hello from user"},
			{"role": "system", "content": "old system 2"},
			{"role": "assistant", "content": "previous response"},
			{"role": "user", "content": "next prompt"}
		],
		"temperature": 0.2,
		"top_p": 0.5
	}`

	cfg := Config{
		Enabled:         true,
		SystemPrompt:    "CUSTOM_SYSTEM_TEXT_TEST",
		AssistantPrompt: "CUSTOM_ASSISTANT_TEXT_TEST",
		Temperature:     0.9,
		TopP:            0.95,
	}

	rewritten, err := ApplyChatRewrite([]byte(inputJSON), cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(rewritten, &parsed); err != nil {
		t.Fatalf("failed to unmarshal rewritten json: %v", err)
	}

	// 1. Check temperature and top_p
	temp, ok := parsed["temperature"].(float64)
	if !ok || temp != 0.9 {
		t.Errorf("expected temperature 0.9, got %v", parsed["temperature"])
	}

	topP, ok := parsed["top_p"].(float64)
	if !ok || topP != 0.95 {
		t.Errorf("expected top_p 0.95, got %v", parsed["top_p"])
	}

	// 2. Check messages
	msgs, ok := parsed["messages"].([]any)
	if !ok {
		t.Fatalf("expected messages slice, got %T", parsed["messages"])
	}

	// Original had 5 messages (2 system, 2 user, 1 assistant).
	// Stripping 2 system leaves 3 messages.
	// Adding 2 messages at index 0 and 1 results in 3 + 2 = 5 messages.
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d: %+v", len(msgs), msgs)
	}

	// Index 0: system
	m0, ok := msgs[0].(map[string]any)
	if !ok || m0["role"] != "system" || m0["content"] != "CUSTOM_SYSTEM_TEXT_TEST" {
		t.Errorf("message[0] mismatch: got %+v", msgs[0])
	}

	// Index 1: assistant
	m1, ok := msgs[1].(map[string]any)
	if !ok || m1["role"] != "assistant" || m1["content"] != "CUSTOM_ASSISTANT_TEXT_TEST" {
		t.Errorf("message[1] mismatch: got %+v", msgs[1])
	}

	// Index 2: original user
	m2, ok := msgs[2].(map[string]any)
	if !ok || m2["role"] != "user" || m2["content"] != "hello from user" {
		t.Errorf("message[2] mismatch: got %+v", msgs[2])
	}

	// Index 3: original assistant
	m3, ok := msgs[3].(map[string]any)
	if !ok || m3["role"] != "assistant" || m3["content"] != "previous response" {
		t.Errorf("message[3] mismatch: got %+v", msgs[3])
	}

	// Index 4: original user
	m4, ok := msgs[4].(map[string]any)
	if !ok || m4["role"] != "user" || m4["content"] != "next prompt" {
		t.Errorf("message[4] mismatch: got %+v", msgs[4])
	}

	// Verify no old system messages exist
	for i, m := range msgs {
		msgMap := m.(map[string]any)
		if i > 0 && msgMap["role"] == "system" {
			t.Errorf("unexpected system message at index %d: %+v", i, msgMap)
		}
	}
}

func TestHTTPMiddleware_PreservesHeaders(t *testing.T) {
	t.Setenv("CHAT_REWRITE_ENABLED", "true")
	inputJSON := `{"model":"gpt-4o","messages":[{"role":"user","content":"test"}]}`

	var capturedBody []byte
	var capturedHeader http.Header

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeader = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		capturedBody = b
		w.WriteHeader(http.StatusOK)
	})

	mw := HTTPMiddleware(nil)(handler)

	req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(inputJSON)))
	req.Header.Set("Authorization", "Bearer sk-secret-token")
	req.Header.Set("X-Custom-Client", "galactus-client-v1")
	req.Header.Set("User-Agent", "CustomAgent/1.0")
	req.Header.Set("anthropic-beta", "tools-2024-04-04")

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	// Check response status
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", rec.Code)
	}

	// Check headers are completely preserved
	if capturedHeader.Get("Authorization") != "Bearer sk-secret-token" {
		t.Errorf("Authorization header corrupted: %s", capturedHeader.Get("Authorization"))
	}
	if capturedHeader.Get("X-Custom-Client") != "galactus-client-v1" {
		t.Errorf("X-Custom-Client header corrupted: %s", capturedHeader.Get("X-Custom-Client"))
	}
	if capturedHeader.Get("User-Agent") != "CustomAgent/1.0" {
		t.Errorf("User-Agent header corrupted: %s", capturedHeader.Get("User-Agent"))
	}
	if capturedHeader.Get("anthropic-beta") != "tools-2024-04-04" {
		t.Errorf("anthropic-beta header corrupted: %s", capturedHeader.Get("anthropic-beta"))
	}

	// Check that captured body was rewritten
	var parsed map[string]any
	if err := json.Unmarshal(capturedBody, &parsed); err != nil {
		t.Fatalf("failed to unmarshal captured body: %v", err)
	}

	if parsed["temperature"] != 0.9 {
		t.Errorf("expected temperature 0.9, got %v", parsed["temperature"])
	}
	if parsed["top_p"] != 0.95 {
		t.Errorf("expected top_p 0.95, got %v", parsed["top_p"])
	}

	msgs := parsed["messages"].([]any)
	if len(msgs) != 3 { // 2 prepended (sys, asst) + 1 original user
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].(map[string]any)["role"] != "system" {
		t.Errorf("expected msg 0 system, got %v", msgs[0])
	}
	if msgs[1].(map[string]any)["role"] != "assistant" {
		t.Errorf("expected msg 1 assistant, got %v", msgs[1])
	}
	if msgs[2].(map[string]any)["role"] != "user" {
		t.Errorf("expected msg 2 user, got %v", msgs[2])
	}
}

func TestHTTPMiddleware_IgnoresOtherEndpoints(t *testing.T) {
	inputJSON := `{"model":"gpt-4o","input":"text"}`

	var capturedBody []byte
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = b
		w.WriteHeader(http.StatusOK)
	})

	mw := HTTPMiddleware(nil)(handler)

	// Call non-chat endpoint
	req := httptest.NewRequest("POST", "/v1/embeddings", bytes.NewReader([]byte(inputJSON)))
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if string(capturedBody) != inputJSON {
		t.Errorf("non-chat endpoint should not be modified: got %s, want %s", string(capturedBody), inputJSON)
	}
}

