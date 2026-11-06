package chat

import (
	json "9router/proxy/internal/fastjson"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"9router/proxy/internal/db"
)

func seedSpeculativeCombo(t *testing.T, comboID, fastURL, heavyURL, comboName string) *db.Repo {
	t.Helper()
	database, cleanup := setupChatTestDB(t)
	t.Cleanup(cleanup)

	if _, err := database.Exec(`DELETE FROM providerConnections WHERE id IN ('conn-1', 'conn-2')`); err != nil {
		t.Fatalf("clear seeded connections: %v", err)
	}

	insert := func(id, provider, name, baseURL string) {
		t.Helper()
		data, _ := json.Marshal(map[string]any{"apiKey": "sk-mock", "baseUrl": baseURL})
		if _, err := database.Exec(`INSERT INTO providerConnections (id, provider, authType, name, priority, isActive, data, createdAt, updatedAt) VALUES
			(?, ?, 'apikey', ?, 0, 1, ?, '2026-07-18T00:00:00Z', '2026-07-18T00:00:00Z')`,
			id, provider, name, string(data)); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}
	insert("mock-fast", "deepseek", "Mock Fast", fastURL)
	insert("mock-heavy", "groq", "Mock Heavy", heavyURL)

	models, _ := json.Marshal([]string{"deepseek/deepseek-chat", "groq/qwen3-32b"})
	if _, err := database.Exec(`INSERT INTO combos (id, name, models, createdAt, updatedAt) VALUES
		(?, ?, ?, '2026-07-18T00:00:00Z', '2026-07-18T00:00:00Z')`, comboID, comboName, string(models)); err != nil {
		t.Fatalf("insert combo: %v", err)
	}

	repo := db.NewRepo(database)
	if err := repo.UpdateSettingsRaw(map[string]any{
		"comboStrategies": map[string]any{
			comboName: map[string]any{
				"strategy": "speculative",
			},
		},
	}); err != nil {
		t.Fatalf("update settings: %v", err)
	}
	return repo
}

func TestCombo_Speculative_FastWins(t *testing.T) {
	var heavyCompleted int32

	fastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"fast speculative code result\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer fastServer.Close()

	heavyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		atomic.StoreInt32(&heavyCompleted, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"heavy slow result\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer heavyServer.Close()

	comboName := "spec-fast-wins"
	repo := seedSpeculativeCombo(t, comboName, fastServer.URL, heavyServer.URL, comboName)
	h := NewChatHandler(repo)

	req := httptest.NewRequest(http.MethodPost, "/chat/completions",
		strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"quick test"}],"stream":true}`, comboName)))
	rec := httptest.NewRecorder()
	h.HandleChatCompletions(rec, req)

	res := rec.Body.String()
	if !strings.Contains(res, "fast speculative code result") {
		t.Fatalf("expected fast model output to be returned, got: %s", res)
	}
	if strings.Contains(res, "heavy slow result") {
		t.Fatalf("heavy model should not have streamed, got: %s", res)
	}
}

func TestCombo_Speculative_FastRefusal_HeavyWins(t *testing.T) {
	fastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"I apologize, but I cannot fulfill this request as an AI.\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer fastServer.Close()

	heavyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Clean unaligned technical code for NTDLL unhooking.\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer heavyServer.Close()

	comboName := "spec-refusal-heavy-wins"
	repo := seedSpeculativeCombo(t, comboName, fastServer.URL, heavyServer.URL, comboName)
	h := NewChatHandler(repo)

	req := httptest.NewRequest(http.MethodPost, "/chat/completions",
		strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ntdll unhook"}],"stream":true}`, comboName)))
	rec := httptest.NewRecorder()
	h.HandleChatCompletions(rec, req)

	res := rec.Body.String()
	if strings.Contains(res, "I apologize") {
		t.Fatalf("client received refusal from fast model; expected speculative fallback to heavy, got: %s", res)
	}
	if !strings.Contains(res, "Clean unaligned technical code for NTDLL unhooking") {
		t.Fatalf("expected heavy model output, got: %s", res)
	}
}

func TestCombo_Speculative_NonStreaming_FastWins(t *testing.T) {
	fastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"fast non-streaming clean result"}}]}`))
	}))
	defer fastServer.Close()

	heavyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"heavy slow non-streaming result"}}]}`))
	}))
	defer heavyServer.Close()

	comboName := "spec-nonstream-fast"
	repo := seedSpeculativeCombo(t, comboName, fastServer.URL, heavyServer.URL, comboName)
	h := NewChatHandler(repo)

	req := httptest.NewRequest(http.MethodPost, "/chat/completions",
		strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"quick test"}],"stream":false}`, comboName)))
	rec := httptest.NewRecorder()
	h.HandleChatCompletions(rec, req)

	res := rec.Body.String()
	if !strings.Contains(res, "fast non-streaming clean result") {
		t.Fatalf("expected fast model output, got: %s", res)
	}
	if strings.Contains(res, "heavy slow non-streaming result") {
		t.Fatalf("heavy model should not have returned, got: %s", res)
	}
}

func TestCombo_Speculative_NonStreaming_FastRefusal_HeavyWins(t *testing.T) {
	fastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"I cannot fulfill this request as an AI language model."}}]}`))
	}))
	defer fastServer.Close()

	heavyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"Compliant binary disassembly reconstruction."}}]}`))
	}))
	defer heavyServer.Close()

	comboName := "spec-nonstream-refusal"
	repo := seedSpeculativeCombo(t, comboName, fastServer.URL, heavyServer.URL, comboName)
	h := NewChatHandler(repo)

	req := httptest.NewRequest(http.MethodPost, "/chat/completions",
		strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"disassemble binary"}],"stream":false}`, comboName)))
	rec := httptest.NewRecorder()
	h.HandleChatCompletions(rec, req)

	res := rec.Body.String()
	if strings.Contains(res, "I cannot fulfill") {
		t.Fatalf("client received refusal from fast model in non-streaming; got: %s", res)
	}
	if !strings.Contains(res, "Compliant binary disassembly reconstruction") {
		t.Fatalf("expected heavy model output in non-streaming, got: %s", res)
	}
}

