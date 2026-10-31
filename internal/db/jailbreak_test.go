package db

import "testing"

func newSettingsRepo(t *testing.T) *Repo {
	t.Helper()
	database, cleanup := setupTestDB(t)
	t.Cleanup(cleanup)
	return NewRepo(database)
}

func TestJailbreakDocRoundTrip(t *testing.T) {
	repo := newSettingsRepo(t)

	// No doc saved yet: every model is silently left alone.
	doc, err := repo.GetJailbreakDoc()
	if err != nil {
		t.Fatalf("get missing: %v", err)
	}
	if doc.Prompt != "" || doc.GeminiSafety {
		t.Fatalf("expected zero doc, got %+v", doc)
	}

	in := &JailbreakDoc{Prompt: "agents.md text", GeminiSafety: true}
	if err := repo.SaveJailbreakDoc(in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := repo.GetJailbreakDoc()
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.Prompt != "agents.md text" || !out.GeminiSafety {
		t.Fatalf("round trip mismatch: %+v", out)
	}
}

func TestJailbreakModelToggleOptIn(t *testing.T) {
	repo := newSettingsRepo(t)

	// The key is the id /v1/models lists — prefixes and suffixes are part of
	// the id, exactly as the client sends them.
	const model = "tkh/mimo-v2.6-flash:free"

	// Missing entry must read as off: a model never gets injected just because
	// it exists.
	on, err := repo.JailbreakModelEnabled(model)
	if err != nil {
		t.Fatalf("enabled(missing): %v", err)
	}
	if on {
		t.Fatal("default must be off")
	}

	if err := repo.SetJailbreakModel(model, true); err != nil {
		t.Fatalf("set on: %v", err)
	}
	on, err = repo.JailbreakModelEnabled(model)
	if err != nil || !on {
		t.Fatalf("after enable: on=%v err=%v", on, err)
	}

	// A different model id stays off — no prefix/wildcard bleed.
	on, _ = repo.JailbreakModelEnabled("tkh/mimo-v2.6-flash")
	if on {
		t.Fatal("nearby model id must stay off")
	}

	if err := repo.SetJailbreakModel(model, false); err != nil {
		t.Fatalf("set off: %v", err)
	}
	on, err = repo.JailbreakModelEnabled(model)
	if err != nil || on {
		t.Fatalf("after disable: on=%v err=%v", on, err)
	}

	// List only reflects models currently opted in.
	if err := repo.SetJailbreakModel("gemini-flash", true); err != nil {
		t.Fatalf("set gemini-flash: %v", err)
	}
	list, err := repo.ListJailbreakModels()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || !list["gemini-flash"] {
		t.Fatalf("list = %v, want only gemini-flash", list)
	}
}

func TestJailbreakPresetsAndModelBinding(t *testing.T) {
	repo := newSettingsRepo(t)

	// Save two presets
	p1 := &JailbreakDoc{ID: "coder", Name: "Coding Agent", Prompt: "You are a senior coder.", GeminiSafety: true, Source: "coder.md"}
	p2 := &JailbreakDoc{ID: "writer", Name: "Creative Writer", Prompt: "You are a creative writer.", GeminiSafety: false, Source: "writer.md"}

	if err := repo.SaveJailbreakPreset(p1); err != nil {
		t.Fatalf("save p1: %v", err)
	}
	if err := repo.SaveJailbreakPreset(p2); err != nil {
		t.Fatalf("save p2: %v", err)
	}

	presets, err := repo.ListJailbreakPresets()
	if err != nil {
		t.Fatalf("list presets: %v", err)
	}
	if len(presets) < 2 {
		t.Fatalf("expected at least 2 presets, got %d", len(presets))
	}

	got1, err := repo.GetJailbreakPreset("coder")
	if err != nil || got1.Prompt != p1.Prompt || !got1.GeminiSafety {
		t.Fatalf("get coder preset failed: %+v, err: %v", got1, err)
	}

	// Model bindings
	const modelA = "openai/gpt-4o"
	const modelB = "anthropic/claude-3-5-sonnet"
	const modelC = "google/gemini-1.5-pro"

	if err := repo.SetJailbreakModelPreset(modelA, "coder", true); err != nil {
		t.Fatalf("set modelA: %v", err)
	}
	if err := repo.SetJailbreakModelPreset(modelB, "writer", true); err != nil {
		t.Fatalf("set modelB: %v", err)
	}
	// modelC disabled
	if err := repo.SetJailbreakModelPreset(modelC, "off", false); err != nil {
		t.Fatalf("set modelC: %v", err)
	}

	docA, err := repo.GetJailbreakDocForModel(modelA)
	if err != nil || docA == nil || docA.Prompt != p1.Prompt {
		t.Fatalf("docA mismatch: %+v, err: %v", docA, err)
	}

	docB, err := repo.GetJailbreakDocForModel(modelB)
	if err != nil || docB == nil || docB.Prompt != p2.Prompt {
		t.Fatalf("docB mismatch: %+v, err: %v", docB, err)
	}

	docC, err := repo.GetJailbreakDocForModel(modelC)
	if err != nil || docC != nil {
		t.Fatalf("docC should be nil, got: %+v, err: %v", docC, err)
	}

	// List configs
	cfgs, err := repo.ListJailbreakModelConfigs()
	if err != nil {
		t.Fatalf("list configs: %v", err)
	}
	if cfgs[modelA].PresetID != "coder" || !cfgs[modelA].Enabled {
		t.Fatalf("expected modelA coder config, got %+v", cfgs[modelA])
	}
	if cfgs[modelB].PresetID != "writer" || !cfgs[modelB].Enabled {
		t.Fatalf("expected modelB writer config, got %+v", cfgs[modelB])
	}
	if cfgs[modelC].Enabled {
		t.Fatalf("expected modelC disabled, got %+v", cfgs[modelC])
	}

	// Delete preset
	if err := repo.DeleteJailbreakPreset("writer"); err != nil {
		t.Fatalf("delete writer: %v", err)
	}
	// After deleting writer, modelB gets nil doc and is automatically unbound to off
	docBDeleted, err := repo.GetJailbreakDocForModel(modelB)
	if err != nil || docBDeleted != nil {
		t.Fatalf("expected nil after preset deletion, got %+v, err: %v", docBDeleted, err)
	}
	modelBEnabled, err := repo.JailbreakModelEnabled(modelB)
	if err != nil {
		t.Fatalf("modelB enabled check: %v", err)
	}
	if modelBEnabled {
		t.Fatalf("expected modelB to be automatically unbound/disabled after preset deletion")
	}

	cfgsAfter, err := repo.ListJailbreakModelConfigs()
	if err != nil {
		t.Fatalf("list configs after delete: %v", err)
	}
	if cfgsAfter[modelB].Enabled {
		t.Fatalf("expected modelB to not be enabled in configs after preset deletion")
	}
	if !cfgsAfter[modelA].Enabled || cfgsAfter[modelA].PresetID != "coder" {
		t.Fatalf("modelA should remain armed to coder, got: %+v", cfgsAfter[modelA])
	}
}

// TestJailbreakModelOffStaysListed pins that turning a binding off is a stored
// state, not a delete: the row stays in the list with its toggle off, and only
// DeleteJailbreakModel removes it. The dashboard's delete button is the only
// path that drops a row.
func TestJailbreakModelOffStaysListed(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := NewRepo(database)

	if err := repo.SetJailbreakModelPreset("providerx/deepseek-4-pro", "deepseek", true); err != nil {
		t.Fatalf("arm: %v", err)
	}
	// Turn it off — the row must survive.
	if err := repo.SetJailbreakModelPreset("providerx/deepseek-4-pro", "deepseek", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	cfgs, err := repo.ListJailbreakModelConfigs()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	cfg, ok := cfgs["providerx/deepseek-4-pro"]
	if !ok {
		t.Fatalf("binding vanished after being turned off; list = %+v", cfgs)
	}
	if cfg.Enabled {
		t.Errorf("enabled = true, want false")
	}
	if cfg.PresetID != "deepseek" {
		t.Errorf("presetId = %q, want deepseek (the row keeps what it was bound to)", cfg.PresetID)
	}

	// Explicit delete is the only path that drops the row.
	if err := repo.DeleteJailbreakModel("providerx/deepseek-4-pro"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	cfgs, err = repo.ListJailbreakModelConfigs()
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if _, ok := cfgs["providerx/deepseek-4-pro"]; ok {
		t.Errorf("binding still listed after delete")
	}
}

// NormalizeJailbreakInjectMode is the one gate a preset's inject mode passes
// through, so it must accept the empty default, canonicalize the three named
// modes, and refuse anything else at save time rather than storing it.
func TestNormalizeJailbreakInjectMode(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "empty defaults to prepend", in: "", want: JailbreakInjectPrepend},
		{name: "prepend passes through", in: JailbreakInjectPrepend, want: JailbreakInjectPrepend},
		{name: "append passes through", in: JailbreakInjectAppend, want: JailbreakInjectAppend},
		{name: "replace passes through", in: JailbreakInjectReplace, want: JailbreakInjectReplace},
		{name: "unknown mode is refused", in: "sidebar", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeJailbreakInjectMode(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NormalizeJailbreakInjectMode(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("NormalizeJailbreakInjectMode(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// A preset round-trips its inject mode through store, and an unset one is
// stored as prepend so the request path never has to guess.
func TestJailbreakInjectModeRoundTrip(t *testing.T) {
	repo := newSettingsRepo(t)

	if err := repo.SaveJailbreakPreset(&JailbreakDoc{ID: "override", Prompt: "P", InjectMode: JailbreakInjectReplace}); err != nil {
		t.Fatalf("save replace preset: %v", err)
	}
	got, err := repo.GetJailbreakPreset("override")
	if err != nil {
		t.Fatalf("get replace preset: %v", err)
	}
	if got.InjectMode != JailbreakInjectReplace {
		t.Errorf("inject mode = %q, want %q", got.InjectMode, JailbreakInjectReplace)
	}

	if err := repo.SaveJailbreakPreset(&JailbreakDoc{ID: "plain", Prompt: "P"}); err != nil {
		t.Fatalf("save plain preset: %v", err)
	}
	plain, err := repo.GetJailbreakPreset("plain")
	if err != nil {
		t.Fatalf("get plain preset: %v", err)
	}
	if plain.InjectMode != JailbreakInjectPrepend {
		t.Errorf("default inject mode = %q, want %q", plain.InjectMode, JailbreakInjectPrepend)
	}
}
