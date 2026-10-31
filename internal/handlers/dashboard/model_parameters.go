package dashboard

import (
	"9router/proxy/internal/db"
	"9router/proxy/internal/handlerutil"
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// HandleListModelParameters returns all configured model/combo parameter overrides.
// GET /api/model-parameters
func (h *DashboardHandler) HandleListModelParameters(w http.ResponseWriter, r *http.Request) {
	if h.Repo == nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "database repository unavailable")
		return
	}
	params, err := h.Repo.GetModelParameters()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, params)
}

// HandleGetModelParameters returns parameter overrides for a specific target.
// GET /api/model-parameters/* or /api/model-parameters/{target}
func (h *DashboardHandler) HandleGetModelParameters(w http.ResponseWriter, r *http.Request) {
	if h.Repo == nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "database repository unavailable")
		return
	}
	target := extractTargetParam(r)
	if target == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "target parameter required")
		return
	}

	p, err := h.Repo.GetModelParametersForTarget(target)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if p == nil {
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{})
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, p)
}

// HandleSetModelParameters stores or updates parameter overrides for a target.
// PUT /api/model-parameters/* or /api/model-parameters/{target}
func (h *DashboardHandler) HandleSetModelParameters(w http.ResponseWriter, r *http.Request) {
	if h.Repo == nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "database repository unavailable")
		return
	}
	target := extractTargetParam(r)
	if target == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "target parameter required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	defer r.Body.Close()

	var params db.ModelParameters
	if err := json.Unmarshal(body, &params); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := h.Repo.SetModelParameters(target, params); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "target": target})
}

// HandleDeleteModelParameters removes parameter overrides for a target.
// DELETE /api/model-parameters/* or /api/model-parameters/{target}
func (h *DashboardHandler) HandleDeleteModelParameters(w http.ResponseWriter, r *http.Request) {
	if h.Repo == nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "database repository unavailable")
		return
	}
	target := extractTargetParam(r)
	if target == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "target parameter required")
		return
	}

	if err := h.Repo.DeleteModelParameters(target); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"status": "ok", "deleted": target})
}

func extractTargetParam(r *http.Request) string {
	target := getURLParam(r, "target")
	if target == "" {
		target = getURLParam(r, "*")
	}
	if unescaped, err := url.PathUnescape(target); err == nil && unescaped != "" {
		target = unescaped
	}
	return strings.Trim(strings.TrimSpace(target), "/")
}

