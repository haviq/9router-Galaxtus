package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"9router/proxy/internal/db"
	"9router/proxy/internal/dbtest"
	"9router/proxy/internal/keikey"
)

func TestRequireApiKey_AllowedIPs(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "testdb-auth-ip-*.sqlite")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	database, err := db.OpenDatabase(tmpFile.Name())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	if err := dbtest.CreateTables(database); err != nil {
		t.Fatalf("create tables: %v", err)
	}

	repo := db.NewRepo(database)
	keyID := "test-key-ip"
	secret := "sk-test-ip-key-12345"

	if err := repo.CreateApiKey(keyID, secret, "IP Test Key", "m1"); err != nil {
		t.Fatalf("create api key: %v", err)
	}

	// Restrict to 192.168.1.0/24 and 10.0.0.1
	if err := repo.SetApiKeyAllowedIPs(keyID, "192.168.1.0/24, 10.0.0.1"); err != nil {
		t.Fatalf("set allowed ips: %v", err)
	}

	handler := RequireApiKey(repo)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		activeCount, activeIPs, _, _ := keikey.DefaultLiveTracker().GetStats(keyID)
		if activeCount < 1 {
			t.Errorf("expected activeCount >= 1 inside handler, got %d", activeCount)
		}
		if len(activeIPs) == 0 {
			t.Errorf("expected activeIPs inside handler")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))

	// 1. Request from forbidden IP 203.0.113.5
	reqForbidden := httptest.NewRequest("GET", "/test", nil)
	reqForbidden.Header.Set("Authorization", "Bearer "+secret)
	reqForbidden.RemoteAddr = "203.0.113.5:12345"
	recForbidden := httptest.NewRecorder()

	handler.ServeHTTP(recForbidden, reqForbidden)
	if recForbidden.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for unlisted IP, got %d", recForbidden.Code)
	}

	// 2. Request from allowed IP 192.168.1.50 (in CIDR 192.168.1.0/24)
	reqAllowed := httptest.NewRequest("GET", "/test", nil)
	reqAllowed.Header.Set("Authorization", "Bearer "+secret)
	reqAllowed.RemoteAddr = "192.168.1.50:54321"
	reqAllowed.Header.Set("User-Agent", "TestClient/1.0")
	recAllowed := httptest.NewRecorder()

	handler.ServeHTTP(recAllowed, reqAllowed)
	if recAllowed.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for allowed IP, got %d", recAllowed.Code)
	}

	// After request finished, in-flight activeCount should be 0
	activeCount, _, _, _ := keikey.DefaultLiveTracker().GetStats(keyID)
	if activeCount != 0 {
		t.Fatalf("expected activeCount 0 after completion, got %d", activeCount)
	}

	// Wait for async telemetry queue to flush to DB
	keikey.FlushTelemetry()

	// Check client recorded in DB
	clients, err := repo.GetApiKeyClients(keyID)
	if err != nil {
		t.Fatalf("get api key clients: %v", err)
	}
	if len(clients) == 0 {
		t.Fatalf("expected client recorded in api_key_clients")
	}
	if clients[0].IP != "192.168.1.50" {
		t.Fatalf("expected client IP 192.168.1.50, got %s", clients[0].IP)
	}
	if clients[0].UserAgent != "TestClient/1.0" {
		t.Fatalf("expected user agent TestClient/1.0, got %s", clients[0].UserAgent)
	}
}
