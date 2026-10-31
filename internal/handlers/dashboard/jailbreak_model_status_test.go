package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"9router/proxy/internal/db"
)

// The status table answers "which models are injected right now?", so the
// verdict has to come from the resolver the request path uses — keyword
// coverage and the deny lists included — rather than from the exact-binding
// list alone. Four ids, three verdicts, one sweep.
func TestJailbreakModelStatusResolvesKeywordAndExclude(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	r := setupTestRouter(repo)

	// A keyword covers a whole family across providers.
	if err := repo.SaveJailbreakPreset(&db.JailbreakDoc{
		ID:       "deepseek",
		Name:     "Deepseek",
		Prompt:   "DS RULE",
		Keywords: []db.JailbreakKeyword{{Text: "deepseek"}},
	}); err != nil {
		t.Fatalf("save preset: %v", err)
	}
	// An exact binding that is switched off: listed, not injecting.
	if err := repo.SetJailbreakModelPreset("openai/gpt-5", "deepseek", false); err != nil {
		t.Fatalf("bind model: %v", err)
	}
	// The global deny list outranks the keyword.
	if err := repo.SaveJailbreakExcludes([]db.JailbreakKeyword{{Text: "providerx"}}); err != nil {
		t.Fatalf("save excludes: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"ids": []string{
		"providera/deepseek-4-pro",
		"providerx/deepseek-4-pro",
		"openai/gpt-5",
		"anthropic/claude-sonnet-5",
	}})
	req := httptest.NewRequest(http.MethodPost, "/api/jailbreak/models/status", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	var got struct {
		Models []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"models"`
		Counts map[string]int `json:"counts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := map[string]string{
		"providera/deepseek-4-pro":  "injected",
		"providerx/deepseek-4-pro":  "excluded",
		"openai/gpt-5":              "off",
		"anthropic/claude-sonnet-5": "off",
	}
	if len(got.Models) != len(want) {
		t.Fatalf("models = %d rows, want %d", len(got.Models), len(want))
	}
	for _, m := range got.Models {
		if want[m.ID] != m.Status {
			t.Errorf("%s status = %q (reason %q), want %q", m.ID, m.Status, m.Reason, want[m.ID])
		}
	}
	if got.Counts["injected"] != 1 || got.Counts["excluded"] != 1 || got.Counts["off"] != 2 {
		t.Errorf("counts = %+v, want injected 1, excluded 1, off 2", got.Counts)
	}
}

// An absent body is the Refresh case: sweep everything. A route that insists on
// a JSON body would 400 on its own default.
func TestJailbreakModelStatusAcceptsEmptyBody(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	r := setupTestRouter(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/jailbreak/models/status", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		Models []json.RawMessage `json:"models"`
		Counts map[string]int    `json:"counts"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Models == nil {
		t.Error("models is missing or null on a catalog sweep")
	}
	if got.Counts == nil {
		t.Error("counts is missing or null on a catalog sweep")
	}
}
// The status table is also a summary of what would be applied, so a row that
// resolves to a preset must carry that preset's kind, inject mode and lifetime
// usage, read once for the whole sweep rather than per row.
func TestJailbreakModelStatusCarriesPresetMetadata(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	r := setupTestRouter(repo)

	if err := repo.SaveJailbreakPreset(&db.JailbreakDoc{
		ID:         "override",
		Name:       "Override",
		Prompt:     "OVERRIDE RULE",
		Kind:       "SOUL",
		InjectMode: db.JailbreakInjectReplace,
		Keywords:   []db.JailbreakKeyword{{Text: "deepseek"}},
	}); err != nil {
		t.Fatalf("save preset: %v", err)
	}
	if err := repo.RecordJailbreakUsage("override"); err != nil {
		t.Fatalf("record usage: %v", err)
	}
	if err := repo.RecordJailbreakUsage("override"); err != nil {
		t.Fatalf("record usage: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"ids": []string{"providera/deepseek-4-pro"}})
	req := httptest.NewRequest(http.MethodPost, "/api/jailbreak/models/status", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}

	var got struct {
		Models []struct {
			ID         string `json:"id"`
			Status     string `json:"status"`
			PresetID   string `json:"presetId"`
			PresetKind string `json:"presetKind"`
			InjectMode string `json:"injectMode"`
			Usage      int64  `json:"usage"`
		} `json:"models"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Models) != 1 {
		t.Fatalf("models = %d rows, want 1", len(got.Models))
	}
	m := got.Models[0]
	if m.Status != "injected" || m.PresetID != "override" {
		t.Fatalf("row = %+v, want the override preset injected", m)
	}
	if m.PresetKind != "SOUL" {
		t.Errorf("presetKind = %q, want SOUL", m.PresetKind)
	}
	if m.InjectMode != db.JailbreakInjectReplace {
		t.Errorf("injectMode = %q, want %q", m.InjectMode, db.JailbreakInjectReplace)
	}
	if m.Usage != 2 {
		t.Errorf("usage = %d, want 2", m.Usage)
	}
}
