package chat

import (
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9router/proxy/internal/auth"
	"9router/proxy/internal/db"
)

func TestMaintenanceMode_BlocksUnauthorizedInference(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)

	customMsg := "Galactus AI is performing scheduled maintenance. Back in 10 minutes."
	err := repo.UpdateSettingsRaw(map[string]any{
		"maintenanceMode":    true,
		"maintenanceMessage": customMsg,
	})
	if err != nil {
		t.Fatalf("failed to update settings: %v", err)
	}

	h := NewChatHandler(repo, nil)

	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.HandleChatCompletions(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 Service Unavailable, got %d", rec.Code)
	}

	retryAfter := rec.Header().Get("Retry-After")
	if retryAfter != "300" {
		t.Errorf("expected Retry-After: 300, got %q", retryAfter)
	}

	var errResp map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	errObj := errResp["error"]
	if errObj["message"] != customMsg {
		t.Errorf("expected message %q, got %q", customMsg, errObj["message"])
	}
	if errObj["code"] != "service_maintenance" {
		t.Errorf("expected code service_maintenance, got %v", errObj["code"])
	}
}

func TestMaintenanceMode_AdmitsCLIToken(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)

	err := repo.UpdateSettingsRaw(map[string]any{
		"maintenanceMode": true,
	})
	if err != nil {
		t.Fatalf("failed to update settings: %v", err)
	}

	h := NewChatHandler(repo, nil)

	body := `{"model":"","messages":[]}` // will fail on missing model (400), not 503
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set(auth.CLITokenHeader, auth.CLIToken())
	rec := httptest.NewRecorder()

	h.HandleChatCompletions(rec, req)

	if rec.Code == http.StatusServiceUnavailable {
		t.Fatalf("CLI token should have bypassed maintenance mode, got 503")
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request (missing model), got %d", rec.Code)
	}
}
