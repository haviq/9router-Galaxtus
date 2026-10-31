package dashboard

import (
	"encoding/json"
	"net/http"

	"9router/proxy/internal/db"
	"9router/proxy/internal/handlerutil"
	"github.com/go-chi/chi/v5"
)

// HandleListModelPricing returns all model pricing rules.
func (h *DashboardHandler) HandleListModelPricing(w http.ResponseWriter, r *http.Request) {
	list, err := h.Repo.ListModelPricing()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*db.ModelPricing{}
	}
	handlerutil.WriteJSON(w, http.StatusOK, list)
}

// HandleSetModelPricing creates or updates a model pricing rule.
func (h *DashboardHandler) HandleSetModelPricing(w http.ResponseWriter, r *http.Request) {
	var p db.ModelPricing
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if p.ModelPattern == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "modelPattern is required")
		return
	}

	if err := h.Repo.SetModelPricing(&p); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, p)
}

// HandleDeleteModelPricing removes a model pricing rule.
func (h *DashboardHandler) HandleDeleteModelPricing(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "id is required")
		return
	}
	if err := h.Repo.DeleteModelPricing(id); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// TopupRequest defines payload for adding balance to an API key.
type TopupRequest struct {
	ApiKeyID string  `json:"apiKeyId"`
	Balance  float64 `json:"balance"`
}

// HandleTopupApiKey sets or updates the saldo of an API key.
func (h *DashboardHandler) HandleTopupApiKey(w http.ResponseWriter, r *http.Request) {
	var req TopupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if req.ApiKeyID == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "apiKeyId is required")
		return
	}

	if err := h.Repo.SetApiKeyBalance(req.ApiKeyID, req.Balance); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"apiKeyId": req.ApiKeyID,
		"balance":  req.Balance,
		"status":   "ok",
	})
}
