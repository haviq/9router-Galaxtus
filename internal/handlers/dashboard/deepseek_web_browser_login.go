package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"9router/proxy/internal/browserlogin"
	"9router/proxy/internal/handlerutil"
	"9router/proxy/internal/log"
	"9router/proxy/internal/models"
	"9router/proxy/internal/proxy/executor"
)

// The browser sign-in for DeepSeek Web.
//
// The credential this provider needs is a browser session, and it comes in
// two parts that only a real browser can produce together:
//
//   - the userToken, which is the API's Bearer credential. The site mints it
//     during a sign-in and keeps it in localStorage; nothing else issues
//     one, so there is no "refresh token" to exchange.
//   - the site's own cookies, which the WAF reads. The WAF's token is
//     renewed by the browser running the WAF's JavaScript challenge, and
//     that challenge is what makes the cookie worth having.
//
// A browser sign-in produces both in one step, and the profile it uses is
// deleted on the way out, so nothing is left running that could rotate the
// session afterwards — the same reasoning as the Gemini Web flow, which is
// where the shape of this comes from.

// deepSeekBrowserLoginTimeout bounds the whole flow. A sign-in that takes
// longer than this is a sign-in that is not going to happen.
const deepSeekBrowserLoginTimeout = 4 * time.Minute

// deepSeekBrowserLoginRun is the flow that opens the browser. It is a
// variable so a test can stand in for it, the way the other outbound calls
// in this package are.
var deepSeekBrowserLoginRun = browserlogin.RunDeepSeek

// HandleDeepSeekWebBrowserLogin opens a throwaway browser, waits for a
// DeepSeek sign-in, and answers with the credential it captured.
//
// The answer is the credential the caller pastes into the connection form.
// It is never stored here: the caller decides which connection it belongs
// to.
func (h *DashboardHandler) HandleDeepSeekWebBrowserLogin(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), deepSeekBrowserLoginTimeout)
	defer cancel()

	log.Info("deepseek-web", "opening a browser for the sign-in")
	result, err := deepSeekBrowserLoginRun(ctx)
	if err != nil {
		status := http.StatusBadRequest
		switch {
		case errors.Is(err, browserlogin.ErrNoBrowser):
			status = http.StatusNotImplemented
		case errors.Is(err, context.DeadlineExceeded):
			status = http.StatusGatewayTimeout
		}
		log.Warn("deepseek-web", "the browser sign-in did not finish", "error", err)
		handlerutil.WriteJSONError(w, status, err.Error())
		return
	}

	log.Info("deepseek-web", "the browser sign-in finished")
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"userToken": result.UserToken,
		"cookie":    result.CookieHeader,
	})
}


// HandleDeepSeekWebQuickSync receives credentials directly from browser scripts
// (e.g. console snippet on chat.deepseek.com) and applies them directly to the connection.
// It supports CORS headers so fetch() from chat.deepseek.com succeeds cleanly.
func (h *DashboardHandler) HandleDeepSeekWebQuickSync(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	var payload struct {
		ConnectionID string `json:"connectionId"`
		UserToken    string `json:"userToken"`
		Cookie       string `json:"cookie"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	token := browserlogin.DeepSeekUserTokenFromLocalStorage(payload.UserToken)
	if token == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "userToken is required and cannot be empty")
		return
	}

	// Find connection for deepseek-web
	conns, err := h.Repo.GetProviderConnections("deepseek-web", false)
	if err != nil || len(conns) == 0 {
		handlerutil.WriteJSONError(w, http.StatusNotFound, "no deepseek-web connection found in 9router")
		return
	}

	var targetConn *models.ProviderConnection
	if payload.ConnectionID != "" {
		for _, c := range conns {
			if c.ID == payload.ConnectionID {
				targetConn = c
				break
			}
		}
	}
	if targetConn == nil {
		targetConn = conns[0]
	}

	result := &browserlogin.DeepSeekResult{
		UserToken:    token,
		CookieHeader: payload.Cookie,
	}

	if err := executor.StoreDeepSeekCredential(r.Context(), h.Repo, targetConn, result); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Info("deepseek-web", "quick-sync session stored successfully", "conn", targetConn.ID)
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"connectionId": targetConn.ID,
		"message":      "DeepSeek session synced successfully!",
	})
}
