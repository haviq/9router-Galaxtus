package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"

	"9router/proxy/internal/handlers/chat"

	"github.com/go-chi/chi/v5"

	"9router/proxy/internal/db"
	"9router/proxy/internal/handlerutil"
)

// Jailbreak management: agent prompt presets library, plus per-model preset binding.
// Injection is opt-in, default off.

func (h *DashboardHandler) HandleGetJailbreak(w http.ResponseWriter, r *http.Request) {
	doc, err := h.Repo.GetJailbreakDoc()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, doc)
}

// A jailbreak prompt is often an uploaded .md document rather than a typed
// line, so the ceiling is generous while still bounded.
const maxJailbreakBodyBytes = 512 * 1024

func (h *DashboardHandler) HandlePutJailbreak(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxJailbreakBodyBytes+1))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	var doc db.JailbreakDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	// Reject a bad keyword or exclude here so the operator sees 400 with the
	// reason, not a 500 from the storage layer.
	if err := validateJailbreakRules(&doc); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Repo.SaveJailbreakDoc(&doc); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "doc": doc})
}

func (h *DashboardHandler) HandleListJailbreakPresets(w http.ResponseWriter, r *http.Request) {
	presets, err := h.Repo.ListJailbreakPresets()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, presets)
}

func (h *DashboardHandler) HandlePutJailbreakPreset(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxJailbreakBodyBytes+1))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	var doc db.JailbreakDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if doc.ID == "" {
		base := doc.Name
		if base == "" {
			base = doc.Source
		}
		doc.ID = slugify(base)
	}
	if doc.Name == "" {
		if doc.Source != "" {
			doc.Name = doc.Source
		} else {
			doc.Name = doc.ID
		}
	}
	if err := validateJailbreakRules(&doc); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Repo.SaveJailbreakPreset(&doc); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "preset": doc})
}

func (h *DashboardHandler) HandleDeleteJailbreakPreset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing preset id")
		return
	}
	if err := h.Repo.DeleteJailbreakPreset(id); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

// validateJailbreakRules checks a preset's include and exclude rules before
// they are stored. Both lists share one grammar and one set of limits.
func validateJailbreakRules(doc *db.JailbreakDoc) error {
	keywords, err := db.NormalizeJailbreakKeywords(doc.Keywords)
	if err != nil {
		return err
	}
	excludes, err := db.NormalizeJailbreakKeywords(doc.Excludes)
	if err != nil {
		return err
	}
	mode, err := db.NormalizeJailbreakInjectMode(doc.InjectMode)
	if err != nil {
		return err
	}
	doc.Keywords, doc.Excludes, doc.InjectMode = keywords, excludes, mode
	return nil
}

// HandleGetJailbreakExcludes serves the global deny list, which overrides
// every preset and every exact binding.
func (h *DashboardHandler) HandleGetJailbreakExcludes(w http.ResponseWriter, r *http.Request) {
	excludes, err := h.Repo.GetJailbreakExcludes()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if excludes == nil {
		excludes = []db.JailbreakKeyword{}
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"excludes": excludes})
}

func (h *DashboardHandler) HandlePutJailbreakExcludes(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxJailbreakBodyBytes+1))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		Excludes []db.JailbreakKeyword `json:"excludes"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	excludes, err := db.NormalizeJailbreakKeywords(req.Excludes)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.Repo.SaveJailbreakExcludes(excludes); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "excludes": excludes})
}

func (h *DashboardHandler) HandleListJailbreakModels(w http.ResponseWriter, r *http.Request) {
	configs, err := h.Repo.ListJailbreakModelConfigs()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, configs)
}

func (h *DashboardHandler) HandleSetJailbreakModel(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4*1024))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	// The id travels in the body rather than the path on purpose: model ids
	// contain "/" and ":" ("tkh/mimo-v2.6-flash:free"), and chi hands back the
	// still-encoded segment for those, so a path parameter silently stored a key
	// that never matched the real id again — the toggle looked on, then came
	// back off after a reload.
	var req struct {
		ID       string `json:"id"`
		Enabled  *bool  `json:"enabled"`
		PresetID string `json:"presetId"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.ID == "" || len(req.ID) > 300 {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid model id")
		return
	}
	enabled := false
	if req.Enabled != nil {
		enabled = *req.Enabled
	} else if req.PresetID != "" {
		enabled = req.PresetID != "off" && req.PresetID != "none"
	}
	presetID := req.PresetID
	if enabled && (presetID == "" || presetID == "on") {
		presetID = "default"
	}
	if err := h.Repo.SetJailbreakModelPreset(req.ID, presetID, enabled); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"id":       req.ID,
		"enabled":  enabled,
		"presetId": presetID,
	})
}

func slugify(s string) string {
	s = strings.TrimSuffix(s, ".md")
	s = strings.TrimSuffix(s, ".markdown")
	s = strings.TrimSuffix(s, ".txt")
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
			b.WriteRune('-')
		}
	}
	res := strings.Trim(b.String(), "-")
	if res == "" {
		return fmt.Sprintf("preset-%d", time.Now().UnixNano())
	}
	return res
}

// HandleExplainJailbreak answers "which preset wins for this model id, and
// why?" — POST /api/jailbreak/resolve with {"id":"providerx/deepseek-4-pro"}.
// It is the read-only twin of the injection path: the same resolver runs, and
// the winning rule is reported so an operator can verify a keyword or an
// exclude without sending a real request.
func (h *DashboardHandler) HandleExplainJailbreak(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4*1024))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	id := strings.TrimSpace(req.ID)
	if id == "" || len(id) > 300 {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid model id")
		return
	}
	match, err := h.Repo.ExplainJailbreakDoc(id)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, match)
}

// HandleDuplicateJailbreakPreset clones a preset under a new id, keeping its
// prompt, kind, safety switch and both rule lists. POST
// /api/jailbreak/presets/{id}/duplicate with an optional {"id":"new-id"}; when
// no id is given the clone is named "<id>-copy", then "-copy-2" and so on until
// the name is free.
func (h *DashboardHandler) HandleDuplicateJailbreakPreset(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing preset id")
		return
	}
	src, err := h.Repo.GetJailbreakPreset(id)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if src == nil {
		handlerutil.WriteJSONError(w, http.StatusNotFound, "preset not found")
		return
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 4*1024))
	var req struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &req)

	newID := strings.TrimSpace(req.ID)
	if newID == "" {
		base := src.ID
		if base == "" {
			base = slugify(src.Name)
		}
		newID = base + "-copy"
		for n := 2; ; n++ {
			existing, err := h.Repo.GetJailbreakPreset(newID)
			if err != nil {
				handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			// An absent preset comes back as an empty doc, not nil, so the
			// "is this id free?" test is an empty prompt rather than a nil
			// pointer — checking for nil looped forever.
			if existing == nil || existing.Prompt == "" {
				break
			}
			newID = fmt.Sprintf("%s-copy-%d", base, n)
		}
	}
	if newID == src.ID {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "duplicate id must differ from the source")
		return
	}
	clone := &db.JailbreakDoc{
		ID:           newID,
		Name:         src.Name + " (copy)",
		Prompt:       src.Prompt,
		Kind:         src.Kind,
		GeminiSafety: src.GeminiSafety,
		InjectMode:   src.InjectMode,
		Source:       src.Source,
		Keywords:     src.Keywords,
		Excludes:     src.Excludes,
	}
	if err := h.Repo.SaveJailbreakPreset(clone); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "preset": clone})
}

// HandleJailbreakUsage serves the per-preset injection counters, so the preset
// list can show which presets actually earn their keep. GET
// /api/jailbreak/usage → {"usage":{"<preset id>":<count>}}.
func (h *DashboardHandler) HandleJailbreakUsage(w http.ResponseWriter, r *http.Request) {
	usage, err := h.Repo.JailbreakUsage()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"usage": usage})
}

// HandlePreviewJailbreak shows the exact outbound body one request would carry:
// the preset's prompt prepended as the first system message, the original
// messages intact after it. POST /api/jailbreak/preview with
// {"id":"providerx/deepseek-4-pro","messages":[…]}.
//
// It is a pure function of the resolver plus the supplied messages — nothing is
// forwarded, no upstream is contacted, and the request body is bounded like
// every other jailbreak write.
func (h *DashboardHandler) HandlePreviewJailbreak(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxJailbreakBodyBytes))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		ID       string           `json:"id"`
		Messages []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	id := strings.TrimSpace(req.ID)
	if id == "" || len(id) > 300 {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid model id")
		return
	}
	match, err := h.Repo.ExplainJailbreakDoc(id)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// The preview carries the caller's own messages, so an empty list is a
	// caller mistake rather than a reason to invent content.
	out := map[string]any{
		"injected": match.Injected,
		"reason":   match.Reason,
		"messages": req.Messages,
	}
	if match.Injected && match.PresetID != "" {
		doc, err := h.Repo.GetJailbreakPreset(match.PresetID)
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if doc != nil && doc.Prompt != "" {
			out["presetId"] = doc.ID
			out["presetName"] = doc.Name
			out["prompt"] = doc.Prompt
			out["injectMode"] = previewInjectMode(doc.InjectMode)
			out["messages"] = previewMessages(req.Messages, doc.Prompt, doc.InjectMode)
		}
	}
	handlerutil.WriteJSON(w, http.StatusOK, out)
}

// previewInjectMode maps an unset mode to prepend so the preview reports the
// mode that will actually be applied.
func previewInjectMode(mode string) string {
	if mode == "" {
		return db.JailbreakInjectPrepend
	}
	return mode
}

// previewMessages renders the same messages list the forward path would carry,
// so the operator sees prepend/append/replace on their own text. It mirrors the
// chat side's applySystemPrompt for the OpenAI messages shape the preview uses.
func previewMessages(messages []map[string]any, prompt string, mode string) []map[string]any {
	switch previewInjectMode(mode) {
	case db.JailbreakInjectAppend:
		return append(append([]map[string]any{}, messages...), systemMessage(prompt)[0])
	case db.JailbreakInjectReplace:
		out := make([]map[string]any, 0, len(messages))
		for _, m := range messages {
			if role, _ := m["role"].(string); role == "system" {
				continue
			}
			out = append(out, m)
		}
		return append(append([]map[string]any{}, systemMessage(prompt)...), out...)
	default:
		return append(append([]map[string]any{}, systemMessage(prompt)...), messages...)
	}
}

// systemMessage builds the one-element OpenAI system message the preview adds.
func systemMessage(prompt string) []map[string]any {
	return []map[string]any{{"role": "system", "content": prompt}}
}

// HandleReorderJailbreakPresets stores the operator's arrangement. PUT
// /api/jailbreak/presets/order with {"order":["deepseek","soul","agents"]} —
// the array is the full list in the order the dashboard shows it, so a preset
// missing from the array keeps its stored position rather than being reset.
func (h *DashboardHandler) HandleReorderJailbreakPresets(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 16*1024))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		Order []string `json:"order"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Order) == 0 {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing order")
		return
	}
	if len(req.Order) > 200 {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "too many presets")
		return
	}
	if err := h.Repo.ReorderJailbreakPresets(req.Order); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// HandleJailbreakModelCatalog serves every model id the gateway can route, for
// the dashboard's bulk-arm picker. GET /api/jailbreak/models/catalog.
//
// It lives in the dashboard group rather than reusing GET /api/models, which
// sits in the API-key group: a dashboard session is not an engine key, so the
// engine route answers 401 to the SPA. The catalog is the same list the engine
// route builds, minus the per-key access filter — an operator holding a
// dashboard session already sees every connection.
func (h *DashboardHandler) HandleJailbreakModelCatalog(w http.ResponseWriter, r *http.Request) {
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"models": h.jailbreakModelIDs(r.Context())})
}

// jailbreakModelIDs lists every id the gateway can route, sorted, for the
// dashboard's bulk-arm picker and its status table. Both ask the same question
// of the same handler, so the build lives here rather than twice.
func (h *DashboardHandler) jailbreakModelIDs(ctx context.Context) []string {
	chatH := chat.NewChatHandler(h.Repo, nil)
	result := chatH.BuildModelsListResult(ctx, chat.ListModeCatalog)
	ids := make([]string, 0, len(result.Models))
	for _, m := range result.Models {
		if m.ID == "" {
			continue
		}
		ids = append(ids, m.ID)
	}
	sort.Strings(ids)
	return ids
}

// maxJailbreakStatusIDs bounds one sweep. A full catalog is ~1500 ids and the
// resolver reads an in-memory snapshot, so the cap is only there to keep a
// hand-written body from becoming an unbounded allocation.
const maxJailbreakStatusIDs = 5000

// jailbreakStatus is one model's resolved state: the id the client would send,
// plus the verdict the request path's own resolver gives it.
type jailbreakStatus struct {
	ID string `json:"id"`
	// Status buckets the verdict for the summary chips: injected, excluded, off.
	Status string `json:"status"`
	// PresetKind and InjectMode are the winning preset's instruction-file kind
	// and how its prompt reaches the body, so the table can show what would be
	// applied without opening the preset.
	PresetKind string `json:"presetKind,omitempty"`
	InjectMode string `json:"injectMode,omitempty"`
	// Usage is the winning preset's lifetime injection count, so the table can
	// tell a preset that earns its keep from one that never fires.
	Usage int64 `json:"usage,omitempty"`
	db.JailbreakMatch
}

// enrichStatus fills the winning preset's display fields onto one row. It is a
// no-op for a row that resolved to nothing, or one whose preset has since gone.
func enrichStatus(row *jailbreakStatus, presets map[string]*db.JailbreakDoc, usage map[string]int64) {
	if !row.Injected || row.PresetID == "" {
		return
	}
	row.Usage = usage[row.PresetID]
	if doc, ok := presets[row.PresetID]; ok && doc != nil {
		row.PresetKind = doc.Kind
		row.InjectMode = previewInjectMode(doc.InjectMode)
	}
}

func jailbreakStatusBucket(m *db.JailbreakMatch) string {
	switch {
	case m.Injected:
		return "injected"
	case m.Reason == "global-exclude" || m.Reason == "preset-exclude":
		return "excluded"
	default:
		return "off"
	}
}

// HandleJailbreakModelStatus answers "which models are injected right now?"
// for the dashboard's status table. POST /api/jailbreak/models/status with an
// optional {"ids":[…]} — an absent or empty list sweeps the whole catalog.
//
// Each id goes through ExplainJailbreakDoc, the same resolver a real request
// uses, so the answer counts keyword coverage and combo legs rather than just
// exact bindings, and reports which rule won per model.
func (h *DashboardHandler) HandleJailbreakModelStatus(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 256*1024))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		IDs []string `json:"ids"`
	}
	// No body at all is the common case (sweep everything), so an empty body is
	// not a parse error the way it is for the other POST routes.
	if strings.TrimSpace(string(body)) != "" {
		if err := json.Unmarshal(body, &req); err != nil {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	ids := req.IDs
	if len(ids) == 0 {
		ids = h.jailbreakModelIDs(r.Context())
	}
	if len(ids) > maxJailbreakStatusIDs {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "too many model ids")
		return
	}
	models := make([]jailbreakStatus, 0, len(ids))
	counts := map[string]int{"injected": 0, "excluded": 0, "off": 0}
	// Preset metadata and usage counters are read once for the whole sweep:
	// they are display-only enrichment, so a read failure degrades the row
	// rather than failing the table.
	presets, _ := h.Repo.ListJailbreakPresets()
	presetByID := make(map[string]*db.JailbreakDoc, len(presets))
	for _, p := range presets {
		if p != nil {
			presetByID[p.ID] = p
		}
	}
	usage, _ := h.Repo.JailbreakUsage()
	for _, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" || len(id) > 300 {
			continue
		}
		match, err := h.Repo.ExplainJailbreakDoc(id)
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		status := jailbreakStatus{ID: id, Status: jailbreakStatusBucket(match), JailbreakMatch: *match}
		enrichStatus(&status, presetByID, usage)
		models = append(models, status)
		counts[status.Status]++
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"models": models, "counts": counts})
}

// HandleDeleteJailbreakModel removes one binding outright.
// DELETE /api/jailbreak/models with {"id":"providerx/deepseek-4-pro"}.
//
// The id travels in the body rather than the path for the same reason
// HandleSetJailbreakModel does it: model ids contain "/" and ":", and a chi
// path parameter hands those back still-encoded.
func (h *DashboardHandler) HandleDeleteJailbreakModel(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4*1024))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.ID) == "" || len(req.ID) > 300 {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid model id")
		return
	}
	if err := h.Repo.DeleteJailbreakModel(req.ID); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "id": req.ID})
}
