package dashboard

import (
	"encoding/json"
	"net/http"

	"9router/proxy/internal/handlerutil"
	"9router/proxy/internal/middleware"
)

// HandleClientTopup processes an automated client self-service top-up.
func (h *DashboardHandler) HandleClientTopup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	var req struct {
		ApiKeyID string  `json:"apiKeyId"`
		Balance  float64 `json:"balance"`
		Amount   float64 `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	apiKeyStr := middleware.ExtractApiKey(r)
	keyIdentifier := req.ApiKeyID
	if keyIdentifier == "" {
		keyIdentifier = apiKeyStr
	}

	if keyIdentifier == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "apiKeyId or Bearer auth is required")
		return
	}

	newBalance := req.Balance
	if newBalance <= 0 && req.Amount > 0 {
		keyObj, err := h.Repo.GetApiKeyByKey(keyIdentifier)
		if err == nil && keyObj != nil && keyObj.Balance != nil && *keyObj.Balance > 0 {
			newBalance = *keyObj.Balance + req.Amount
		} else {
			newBalance = req.Amount
		}
	}

	if err := h.Repo.SetApiKeyBalance(keyIdentifier, newBalance); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"balance": newBalance,
	})
}

// HandleGetClientPortalInfo returns client self-service metadata authenticated by the client's Bearer API key.
func (h *DashboardHandler) HandleGetClientPortalInfo(w http.ResponseWriter, r *http.Request) {
	apiKeyStr := middleware.ExtractApiKey(r)
	if apiKeyStr == "" {
		handlerutil.WriteJSONError(w, http.StatusUnauthorized, "API key required. Provide Bearer <key> in Authorization header.")
		return
	}

	keyObj, err := h.Repo.GetApiKeyByKey(apiKeyStr)
	if err != nil || keyObj == nil {
		handlerutil.WriteJSONError(w, http.StatusUnauthorized, "Invalid API key.")
		return
	}

	// 1. Balance
	balance := -1.0 // unlimited
	if keyObj.Balance != nil {
		balance = *keyObj.Balance
	}

	// 2. Allowed models
	allowedModels, _ := h.Repo.GetAllowedModels(keyObj.ID)
	if len(allowedModels) == 0 {
		allowedModels = []string{"*"}
	}

	// 3. Rate limits
	rpm := 0
	if keyObj.RateLimitRPM != nil {
		rpm = *keyObj.RateLimitRPM
	}
	tpm := 0
	if keyObj.RateLimitTPM != nil {
		tpm = *keyObj.RateLimitTPM
	}
	conc := 0
	if keyObj.RateLimitConcurrency != nil {
		conc = *keyObj.RateLimitConcurrency
	}

	// 4. Token quota & usage
	tokenQuota := 0
	if keyObj.TokenQuota != nil {
		tokenQuota = *keyObj.TokenQuota
	}
	usedTokens := 0
	if keyObj.UsedCount != nil {
		usedTokens = *keyObj.UsedCount
	}

	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "localhost:20130"
	}
	baseURL := scheme + "://" + host + "/v1"

	keyMasked := apiKeyStr
	if len(apiKeyStr) > 10 {
		keyMasked = apiKeyStr[:6] + "..." + apiKeyStr[len(apiKeyStr)-4:]
	}

	name := "Client Key"
	if keyObj.Name != nil && *keyObj.Name != "" {
		name = *keyObj.Name
	}

	firstModel := allowedModels[0]
	if firstModel == "*" {
		firstModel = "gpt-4o"
	}

	recentReqs, _ := h.Repo.GetClientRecentUsage(apiKeyStr, 15)
	clientStats := h.Repo.GetClientTokenStats(apiKeyStr)
	pricingList, _ := h.Repo.ListModelPricing()

	resp := map[string]any{
		"key":            apiKeyStr,
		"keyMasked":      keyMasked,
		"name":           name,
		"balance":        balance,
		"currency":       "IDR",
		"tokenQuota":     tokenQuota,
		"usedTokens":     usedTokens,
		"rateLimitRpm":   rpm,
		"rateLimitTpm":   tpm,
		"concurrency":    conc,
		"allowedModels":  allowedModels,
		"expiresAt":      keyObj.ExpiresAt,
		"isActive":       keyObj.IsActive == 1,
		"recentRequests": recentReqs,
		"stats":          clientStats,
		"pricingTable":   pricingList,
		"systemStatus": map[string]any{
			"status":      "operational",
			"version":     "9router-v1.2",
			"loadBalance": "least-busy-concurrency",
			"promptCache": "fast-exact-caching-active",
			"antiLeak":    "active",
		},
		"configs": map[string]any{
			"baseUrl": baseURL,
			"cursor": map[string]string{
				"openaiBaseUrl": baseURL,
				"apiKey":        apiKeyStr,
			},
			"cline": map[string]string{
				"provider": "OpenAI Compatible",
				"baseUrl":  baseURL,
				"apiKey":   apiKeyStr,
			},
			"windsurf": map[string]string{
				"modelProvider": "openai-compatible",
				"endpoint":      baseURL,
				"apiKey":        apiKeyStr,
			},
			"pythonSdk": `from openai import OpenAI

client = OpenAI(
    base_url="` + baseURL + `",
    api_key="` + apiKeyStr + `"
)

response = client.chat.completions.create(
    model="` + firstModel + `",
    messages=[{"role": "user", "content": "Hello!"}]
)
print(response.choices[0].message.content)`,
		},
	}

	handlerutil.WriteJSON(w, http.StatusOK, resp)
}
