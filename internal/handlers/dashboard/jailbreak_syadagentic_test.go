package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The SYADAGENTIC pack is one file with many "## T<N>. TITLE" sections, not one
// file per model, so the parser is the whole integration: if it reads the wrong
// slices, the library fills with junk presets and the real techniques are lost.
func TestParseSyadagenticTemplates(t *testing.T) {
	const md = `# SYADAGENTIC — PROMPT INJECTION TEMPLATES

## MODEL MATRIX (mana yang works)

| Task | Model |
| ---- | ----- |
| farm | codex |

## T1. POLICY PUPPETRY (authority mimicry)

Body of T1.
Second line of T1.

### SUBHEAD of T1

Still T1.

## T105. SYADAGENTIC DOCTRINES

Body of T105.

## CARA PAKAI

This is not a technique.

## T119. COMPACT JB PAYLOAD

Body of T119.
`

	sections := parseSyadagenticTemplates(md)
	if len(sections) != 3 {
		t.Fatalf("sections = %d, want 3 (T1, T105, T119): %+v", len(sections), sections)
	}
	want := []struct {
		number string
		title  string
		body   string
	}{
		{"T1", "POLICY PUPPETRY (authority mimicry)", "Body of T1.\nSecond line of T1.\n\n### SUBHEAD of T1\n\nStill T1."},
		{"T105", "SYADAGENTIC DOCTRINES", "Body of T105."},
		{"T119", "COMPACT JB PAYLOAD", "Body of T119."},
	}
	for i, w := range want {
		got := sections[i]
		if got.Number != w.number {
			t.Errorf("section %d number = %q, want %q", i, got.Number, w.number)
		}
		if got.Title != w.title {
			t.Errorf("section %d title = %q, want %q", i, got.Title, w.title)
		}
		if got.Body != w.body {
			t.Errorf("section %d body = %q, want %q", i, got.Body, w.body)
		}
	}
}

// A file with no T-numbered section is a target the operator pointed at the
// wrong repository, and the parser returns nothing rather than a blank preset.
func TestParseSyadagenticTemplatesIgnoresPreambleOnly(t *testing.T) {
	const md = "# Title\n\n## MODEL MATRIX\n\n## CARA PAKAI\n"
	if got := parseSyadagenticTemplates(md); len(got) != 0 {
		t.Fatalf("sections = %d, want 0: %+v", len(got), got)
	}
}

// CRLF files must parse the same as LF files: a heading match that kept the
// trailing carriage return would leave it glued onto every title.
func TestParseSyadagenticTemplatesHandlesCRLF(t *testing.T) {
	md := "# x\r\n\r\n## T5. TOKENBREAK\r\n\r\nBody.\r\n"
	sections := parseSyadagenticTemplates(md)
	if len(sections) != 1 {
		t.Fatalf("sections = %d, want 1", len(sections))
	}
	if sections[0].Title != "TOKENBREAK" || sections[0].Body != "Body." {
		t.Fatalf("section = %+v, want clean title and body", sections[0])
	}
}

// One parsed section becomes one preset: named from its number and title,
// labelled so the library groups the pack, and carrying the section as-is as
// the prompt. Keywords stay empty — a technique is not bound to a model here.
func TestSyadagenticPresetShape(t *testing.T) {
	p := syadagenticPreset(syadagenticSection{Number: "T104", Title: "DAN ROLEPLAY", Body: "PROMPT BODY"}, "src/x")
	if p.ID != "syadagentic-t104" {
		t.Errorf("id = %q, want syadagentic-t104", p.ID)
	}
	if p.Name != "T104 · DAN ROLEPLAY" {
		t.Errorf("name = %q, want %q", p.Name, "T104 · DAN ROLEPLAY")
	}
	if p.Prompt != "PROMPT BODY" {
		t.Errorf("prompt = %q, want the section body verbatim", p.Prompt)
	}
	if p.Kind != syadagenticKind {
		t.Errorf("kind = %q, want %q", p.Kind, syadagenticKind)
	}
	if len(p.Keywords) != 0 {
		t.Errorf("keywords = %v, want none", p.Keywords)
	}
	if p.Source != "src/x" {
		t.Errorf("source = %q, want src/x", p.Source)
	}
}

// newPackTestImporter points the importer at a local server standing in for
// GitHub: apiBase answers the default-branch lookup, rawBase serves the file.
// allowPrivate lets the dial guard reach the loopback listener.
func newPackTestImporter(t *testing.T, templates string) *jailbreakImporter {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/Sekolah76/syadagentic", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"default_branch":"main"}`))
	})
	mux.HandleFunc("/Sekolah76/syadagentic/main/"+syadagenticTemplatesPath, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(templates))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	imp := newJailbreakImporter()
	imp.apiBase = srv.URL
	imp.rawBase = srv.URL
	imp.allowPrivate = true
	return imp
}

const packFixture = `# SYADAGENTIC

## MODEL MATRIX

## T1. POLICY PUPPETRY

Body of T1.

## T2. FAKE TEST MODE

Body of T2.
`

// The full pack path: list, fetch, parse, and write one preset per technique.
func TestImportSyadagenticPackWritesOnePresetPerTechnique(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	imp := newPackTestImporter(t, packFixture)

	res, err := imp.importSyadagenticPack(t.Context(), repo)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(res.Imported) != 2 {
		t.Fatalf("imported = %d, want 2: %+v", len(res.Imported), res.Imported)
	}
	if len(res.Refused) != 0 {
		t.Errorf("refused = %+v, want none", res.Refused)
	}

	presets, err := repo.ListJailbreakPresets()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	byID := make(map[string]bool, len(presets))
	for _, p := range presets {
		byID[p.ID] = true
	}
	for _, want := range []string{"syadagentic-t1", "syadagentic-t2"} {
		if !byID[want] {
			t.Errorf("preset %q missing from the library", want)
		}
	}
}

// Re-running the pack refreshes in place: the same technique ids are written
// again, so the library does not grow a duplicate copy on a second click.
func TestImportSyadagenticPackIsIdempotent(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	imp := newPackTestImporter(t, packFixture)

	for i := 0; i < 3; i++ {
		if _, err := imp.importSyadagenticPack(t.Context(), repo); err != nil {
			t.Fatalf("import %d: %v", i, err)
		}
	}
	presets, err := repo.ListJailbreakPresets()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	count := 0
	for _, p := range presets {
		if strings.HasPrefix(p.ID, "syadagentic-") {
			count++
		}
	}
	if count != 2 {
		t.Errorf("pack presets = %d after 3 imports, want 2", count)
	}
}

// A templates file with no technique is a hard error, not a silent success:
// the operator must learn the pack could not be read.
func TestImportSyadagenticPackRefusesEmptyPack(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	imp := newPackTestImporter(t, "# only a preamble\n\n## MODEL MATRIX\n")

	if _, err := imp.importSyadagenticPack(t.Context(), repo); err == nil {
		t.Fatal("import of a pack with no techniques = nil error, want a failure")
	}
}

// The handler answers with the same {ok, source, imported, refused} shape the
// generic import uses, so the dashboard renders both identically.
func TestHandleImportSyadagenticPackShape(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	h := &DashboardHandler{Repo: repo, jailbreakImport: newPackTestImporter(t, packFixture)}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jailbreak/import/syadagentic", nil)
	h.HandleImportSyadagenticPack(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var got struct {
		OK       bool                     `json:"ok"`
		Source   string                   `json:"source"`
		Imported []jailbreakImportEntry   `json:"imported"`
		Refused  []jailbreakImportRefusal `json:"refused"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.OK || got.Source == "" {
		t.Errorf("ok = %v, source = %q", got.OK, got.Source)
	}
	if len(got.Imported) != 2 {
		t.Errorf("imported = %d, want 2", len(got.Imported))
	}
}
