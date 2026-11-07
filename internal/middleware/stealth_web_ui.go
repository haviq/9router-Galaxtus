package middleware

import (
	"9router/proxy/internal/auth"
	"9router/proxy/internal/db"
	"net"
	"net/http"
	"strings"
)

// StealthWebUIGate protects web frontend routes when StealthWebUI is enabled.
// When active, unauthorized visitors receive a clean HTTP 404 Not Found response,
// making the web dashboard completely hidden from the base URL.
//
// Authorized callers admitted past the gate:
// 1. Requests with a valid dashboard session cookie (auth.SessionValid)
// 2. Requests with the local CLI token header
// 3. Requests presenting a matching bypass key in query parameter (?key=..., ?admin_key=..., ?portal_key=...)
//    or header (X-Portal-Key, X-Admin-Key)
// 4. Requests with an active 9r_stealth_bypass cookie minted by a previous valid bypass key
// 5. Direct local developer on loopback (localhost, 127.0.0.1, [::1])
func StealthWebUIGate(repo *db.Repo, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		settings, err := repo.GetSettings()
		if err != nil || settings == nil || !settings.StealthWebUI {
			next.ServeHTTP(w, r)
			return
		}

		// Admitted: already logged into dashboard session
		if auth.SessionValid(r) {
			next.ServeHTTP(w, r)
			return
		}

		// Admitted: local CLI token
		if auth.ValidCLIToken(r.Header.Get(auth.CLITokenHeader)) {
			next.ServeHTTP(w, r)
			return
		}

		// Admitted: direct local developer on loopback with no external proxy headers
		remoteIP, _, _ := net.SplitHostPort(r.RemoteAddr)
		if remoteIP == "" {
			remoteIP = r.RemoteAddr
		}
		ip := net.ParseIP(remoteIP)
		isLoopbackIP := ip != nil && ip.IsLoopback()
		hostIsLocal := strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.0.0.1") || strings.HasPrefix(r.Host, "[::1]")
		isDirectLocal := (isLoopbackIP || hostIsLocal) &&
			r.Header.Get("X-Forwarded-For") == "" &&
			r.Header.Get("CF-Connecting-IP") == ""
		if isDirectLocal {
			next.ServeHTTP(w, r)
			return
		}

		bypassKey := strings.TrimSpace(settings.StealthBypassKey)

		// Check bypass cookie
		if cookie, err := r.Cookie("9r_stealth_bypass"); err == nil && cookie != nil && cookie.Value != "" {
			if bypassKey == "" || cookie.Value == bypassKey {
				next.ServeHTTP(w, r)
				return
			}
		}

		// Check query parameter or header for bypass key
		providedKey := r.URL.Query().Get("key")
		if providedKey == "" {
			providedKey = r.URL.Query().Get("admin_key")
		}
		if providedKey == "" {
			providedKey = r.URL.Query().Get("portal_key")
		}
		if providedKey == "" {
			providedKey = r.Header.Get("X-Portal-Key")
		}
		if providedKey == "" {
			providedKey = r.Header.Get("X-Admin-Key")
		}

		if bypassKey != "" && providedKey == bypassKey {
			http.SetCookie(w, &http.Cookie{
				Name:     "9r_stealth_bypass",
				Value:    bypassKey,
				Path:     "/",
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   86400 * 30, // 30 days
			})
			next.ServeHTTP(w, r)
			return
		}

		// If StealthBypassKey is empty, any valid active API key can unlock
		if bypassKey == "" && providedKey != "" {
			if obj, err := repo.GetApiKeyByKey(providedKey); err == nil && obj != nil && obj.IsActive == 1 {
				http.SetCookie(w, &http.Cookie{
					Name:     "9r_stealth_bypass",
					Value:    providedKey,
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
					MaxAge:   86400 * 30,
				})
				next.ServeHTTP(w, r)
				return
			}
		}

		// Not authorized: return 404 Not Found to make base URL appear as non-existent web service
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("404 page not found\n"))
	})
}

