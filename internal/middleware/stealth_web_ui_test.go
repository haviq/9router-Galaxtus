package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"9router/proxy/internal/auth"
	"9router/proxy/internal/db"
)

func TestStealthWebUIGate_DisabledByDefault(t *testing.T) {
	sqliteDB, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(sqliteDB)

	handler := StealthWebUIGate(repo, okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK when stealth disabled, got %d", rec.Code)
	}
}

func TestStealthWebUIGate_Enabled_BlocksUnauthorized(t *testing.T) {
	sqliteDB, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(sqliteDB)

	err := repo.UpdateSettingsRaw(map[string]any{
		"stealthWebUI":     true,
		"stealthBypassKey": "galactus_secret",
	})
	if err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	handler := StealthWebUIGate(repo, okHandler())

	// Unauthorized request
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found when stealth enabled and unauthorized, got %d", rec.Code)
	}
}

func TestStealthWebUIGate_Enabled_AdmitsBypassKeyAndSetsCookie(t *testing.T) {
	sqliteDB, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(sqliteDB)

	err := repo.UpdateSettingsRaw(map[string]any{
		"stealthWebUI":     true,
		"stealthBypassKey": "galactus_secret",
	})
	if err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	handler := StealthWebUIGate(repo, okHandler())

	// Request with ?key=galactus_secret
	req := httptest.NewRequest(http.MethodGet, "/?key=galactus_secret", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with valid bypass key, got %d", rec.Code)
	}

	cookies := rec.Result().Cookies()
	var foundCookie bool
	for _, c := range cookies {
		if c.Name == "9r_stealth_bypass" && c.Value == "galactus_secret" {
			foundCookie = true
			break
		}
	}
	if !foundCookie {
		t.Fatalf("expected 9r_stealth_bypass cookie to be set")
	}

	// Subsequent request carrying the cookie
	req2 := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req2.AddCookie(&http.Cookie{Name: "9r_stealth_bypass", Value: "galactus_secret"})
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with bypass cookie, got %d", rec2.Code)
	}
}

func TestStealthWebUIGate_Enabled_AdmitsCLIToken(t *testing.T) {
	sqliteDB, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(sqliteDB)

	err := repo.UpdateSettingsRaw(map[string]any{
		"stealthWebUI":     true,
		"stealthBypassKey": "galactus_secret",
	})
	if err != nil {
		t.Fatalf("failed to save settings: %v", err)
	}

	handler := StealthWebUIGate(repo, okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(auth.CLITokenHeader, auth.CLIToken())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK with CLI token, got %d", rec.Code)
	}
}
