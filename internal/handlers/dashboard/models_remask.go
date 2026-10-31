package dashboard

import (
	json "encoding/json/v2"
	"io"
	"net/http"
	"strings"

	"9router/proxy/internal/db"
	"9router/proxy/internal/handlerutil"
)

// HandleGetModelRemaskRules handles GET /api/models/remask.
func (h *DashboardHandler) HandleGetModelRemaskRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.Repo.GetModelRemaskRules()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rules == nil {
		rules = []db.ModelRemaskRule{}
	}
	handlerutil.WriteJSON(w, http.StatusOK, rules)
}

// HandleSetModelRemaskRule handles POST /api/models/remask.
func (h *DashboardHandler) HandleSetModelRemaskRule(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	defer r.Body.Close()

	var rule db.ModelRemaskRule
	if err := json.Unmarshal(body, &rule); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid json body: "+err.Error())
		return
	}

	rule.Alias = strings.TrimSpace(rule.Alias)
	rule.Target = strings.TrimSpace(rule.Target)
	if rule.Alias == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "alias is required")
		return
	}
	if rule.Target == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "target is required")
		return
	}

	if err := h.Repo.SetModelRemaskRule(rule); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"rule":    rule,
	})
}

// HandleDeleteModelRemaskRule handles DELETE /api/models/remask.
func (h *DashboardHandler) HandleDeleteModelRemaskRule(w http.ResponseWriter, r *http.Request) {
	alias := strings.TrimSpace(r.URL.Query().Get("alias"))
	if alias == "" {
		var req struct {
			Alias string `json:"alias"`
		}
		body, _ := io.ReadAll(r.Body)
		if len(body) > 0 {
			_ = json.Unmarshal(body, &req)
			alias = strings.TrimSpace(req.Alias)
		}
	}
	if alias == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "alias is required")
		return
	}

	if err := h.Repo.DeleteModelRemaskRule(alias); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"success": true,
	})
}

