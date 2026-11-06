package chat

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCombo_RefusalAutoFailover(t *testing.T) {
	// Upstream 1: Returns a refusal
	refusing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"I apologize, but I cannot fulfill this request as an AI.\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer refusing.Close()

	// Upstream 2: Returns the compliant technical answer
	compliant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Here is the full technical memory layout analysis.\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer compliant.Close()

	comboName := "refusal-failover-test"
	repo := seedComboWithTwoUpstreams(t, comboName, refusing.URL, compliant.URL, comboName)
	h := NewChatHandler(repo)

	req := httptest.NewRequest(http.MethodPost, "/chat/completions",
		strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"analyze memory layout"}],"stream":true}`, comboName)))
	rec := httptest.NewRecorder()
	h.HandleChatCompletions(rec, req)

	res := rec.Body.String()
	if strings.Contains(res, "I apologize") {
		t.Fatalf("client received refusal from first model; expected auto-failover, got: %s", res)
	}
	if !strings.Contains(res, "Here is the full technical memory layout analysis") {
		t.Fatalf("client did not receive compliant response from fallback model; got: %s", res)
	}
}

