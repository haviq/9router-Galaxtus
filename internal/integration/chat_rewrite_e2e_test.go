//go:build integration

package integration

import (
	"net/http"
	"os"
	"strings"
	"testing"

	json "encoding/json/v2"
)

// TestChatCompletions_HTTPRewrite_Flow tests the HTTP rewrite pipeline:
// Trigger: request path containing /v1/chat/completions
// Action:
// 1. parse JSON body
// 2. remove all element messages with role == "system"
// 3. insert two new messages at index 0 and 1:
//    - index 0: role system
//    - index 1: role assistant
// 4. set temperature to 0.9 and top_p to 0.95
// 5. preserve request headers unmodified
func TestChatCompletions_HTTPRewrite_Flow(t *testing.T) {
	env, upstream := newProviderEnv(t)

	// Configure rewrite in database settings
	err := env.Repo.UpdateSettingsRaw(map[string]any{
		"chatRewriteEnabled":     true,
		"chatRewriteSystem":      "SYSTEM_REWRITE_PROMPT_E2E",
		"chatRewriteAssistant":   "ASSISTANT_REWRITE_PROMPT_E2E",
		"chatRewriteTemperature": 0.9,
		"chatRewriteTopP":        0.95,
	})
	if err != nil {
		t.Fatalf("failed to update settings: %v", err)
	}

	payload := `{
		"model": "deepseek/deepseek-chat",
		"messages": [
			{"role": "system", "content": "original system prompt 1"},
			{"role": "user", "content": "user question"},
			{"role": "system", "content": "original system prompt 2"}
		],
		"temperature": 0.3,
		"top_p": 0.6
	}`

	req, err := http.NewRequest(http.MethodPost, env.BaseURL+"/v1/chat/completions", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+env.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Custom-Tracking-Header", "tracker-9router-xyz")

	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/chat/completions failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	// Inspect what the upstream received
	sent := upstream.Last(t)

	var upstreamPayload map[string]any
	if err := json.Unmarshal([]byte(sent.Body), &upstreamPayload); err != nil {
		t.Fatalf("upstream body is not valid JSON: %v, raw: %s", err, sent.Body)
	}

	// Verify temperature == 0.9
	temp, ok := upstreamPayload["temperature"].(float64)
	if !ok || temp != 0.9 {
		t.Errorf("upstream temperature = %v, want 0.9", upstreamPayload["temperature"])
	}

	// Verify top_p == 0.95
	topP, ok := upstreamPayload["top_p"].(float64)
	if !ok || topP != 0.95 {
		t.Errorf("upstream top_p = %v, want 0.95", upstreamPayload["top_p"])
	}

	// Verify messages
	rawMsgs, ok := upstreamPayload["messages"].([]any)
	if !ok {
		t.Fatalf("messages is not a slice: %T", upstreamPayload["messages"])
	}

	// 2 original system messages removed, 1 original user message kept + 2 prepended = 3 messages
	if len(rawMsgs) != 3 {
		t.Fatalf("expected 3 messages at upstream, got %d: %+v", len(rawMsgs), rawMsgs)
	}

	// Msg 0: system
	m0, ok := rawMsgs[0].(map[string]any)
	if !ok || m0["role"] != "system" || m0["content"] != "SYSTEM_REWRITE_PROMPT_E2E" {
		t.Errorf("expected msg[0] system prompt, got %+v", rawMsgs[0])
	}

	// Msg 1: assistant
	m1, ok := rawMsgs[1].(map[string]any)
	if !ok || m1["role"] != "assistant" || m1["content"] != "ASSISTANT_REWRITE_PROMPT_E2E" {
		t.Errorf("expected msg[1] assistant prompt, got %+v", rawMsgs[1])
	}

	// Msg 2: user
	m2, ok := rawMsgs[2].(map[string]any)
	if !ok || m2["role"] != "user" || m2["content"] != "user question" {
		t.Errorf("expected msg[2] user question, got %+v", rawMsgs[2])
	}
}

// TestChatCompletions_HTTPRewrite_FileBasedPrompt tests reading pasted prompt text from rewrite_system.txt and rewrite_assistant.txt
func TestChatCompletions_HTTPRewrite_FileBasedPrompt(t *testing.T) {
	_ = os.WriteFile("rewrite_system.txt", []byte("FILE_PASTED_SYSTEM_PROMPT"), 0644)
	_ = os.WriteFile("rewrite_assistant.txt", []byte("FILE_PASTED_ASSISTANT_PROMPT"), 0644)
	defer func() {
		_ = os.WriteFile("rewrite_system.txt", []byte("You are a helpful and knowledgeable AI assistant."), 0644)
		_ = os.WriteFile("rewrite_assistant.txt", []byte("Understood. I will follow your instructions precisely."), 0644)
	}()

	env, upstream := newProviderEnv(t)

	payload := `{
		"model": "deepseek/deepseek-chat",
		"messages": [
			{"role": "system", "content": "remove this old prompt"},
			{"role": "user", "content": "what is 2+2?"}
		]
	}`

	req, err := http.NewRequest(http.MethodPost, env.BaseURL+"/v1/chat/completions", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+env.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := env.client.Do(req)
	if err != nil {
		t.Fatalf("POST /v1/chat/completions failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	sent := upstream.Last(t)
	var upstreamPayload map[string]any
	if err := json.Unmarshal([]byte(sent.Body), &upstreamPayload); err != nil {
		t.Fatalf("upstream body is not valid JSON: %v", err)
	}

	rawMsgs := upstreamPayload["messages"].([]any)
	m0 := rawMsgs[0].(map[string]any)
	m1 := rawMsgs[1].(map[string]any)

	if m0["content"] != "FILE_PASTED_SYSTEM_PROMPT" {
		t.Errorf("msg[0] = %q, want FILE_PASTED_SYSTEM_PROMPT", m0["content"])
	}
	if m1["content"] != "FILE_PASTED_ASSISTANT_PROMPT" {
		t.Errorf("msg[1] = %q, want FILE_PASTED_ASSISTANT_PROMPT", m1["content"])
	}
}
