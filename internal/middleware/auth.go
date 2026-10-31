package middleware

import (
	"9router/proxy/internal/log"
	"context"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"9router/proxy/internal/auth"
	"9router/proxy/internal/db"
	"9router/proxy/internal/handlerutil"
	"9router/proxy/internal/keikey"
	"9router/proxy/internal/models"
)

// ContextKey is a custom type for context keys to avoid collisions.
type ContextKey string

// ApiKeyContextKey is the context key for the authenticated API key object.
const ApiKeyContextKey ContextKey = "apiKey"

// RequireApiKey creates a middleware handler that authenticates requests using client API keys.
// It checks the Authorization header (Bearer <key>) and the query parameter `key`.
// Valid keys are retrieved from the SQLite database; inactive or disabled keys are rejected with 401.
func RequireApiKey(repo *db.Repo) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKeyString := ExtractApiKey(r)
			if apiKeyString == "" {
				// Allow requests authenticated via dashboard session (or local dashboard when login not required)
				if !auth.RequireLogin(repo) || auth.SessionValid(r) {
					dashName := "Dashboard Session"
					dashKey := &models.APIKey{
						ID:       "dashboard-session",
						Name:     &dashName,
						IsActive: 1,
					}
					ctx := context.WithValue(r.Context(), ApiKeyContextKey, dashKey)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}

				handlerutil.WriteJSONError(w, http.StatusUnauthorized, "Authentication required. Provide an API key via Authorization: Bearer <key> header or ?key=<key> query parameter.")
				return
			}

			// Validate via SQLite repository and retrieve details
			lookup := keikey.LookupHash(apiKeyString)
			apiKeyObj, err := resolveApiKey(repo, lookup, apiKeyString)
			if err != nil {
				log.Error("auth", "DB lookup error", "error", err)
				handlerutil.WriteJSONError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
			if apiKeyObj == nil {
				handlerutil.WriteJSONError(w, http.StatusUnauthorized, "Invalid API key.")
				return
			}

			if !verifyApiKey(apiKeyString, apiKeyObj) {
				handlerutil.WriteJSONError(w, http.StatusUnauthorized, "Invalid API key.")
				return
			}

			// Only a verified key enters the cache, and only for 5s: a
			// revoked or deleted key cannot outlive that window.
			if lookup != "" {
				keikey.DefaultAuthCache().Set(lookup, apiKeyObj)
			}

			if apiKeyObj.IsActive != 1 {
				handlerutil.WriteJSONError(w, http.StatusUnauthorized, "Invalid or inactive API key.")
				return
			}

			// F-14: Check key expiry
			if apiKeyObj.ExpiresAt != nil && *apiKeyObj.ExpiresAt != "" {
				expires, err := time.Parse(time.RFC3339, *apiKeyObj.ExpiresAt)
				if err == nil && time.Now().After(expires) {
					handlerutil.WriteJSONError(w, http.StatusUnauthorized, "API key expired.")
					return
				}
			}

			// Caller IP detection & allowlist enforcement
			clientIP := auth.LoginClientIP(r)
			if apiKeyObj.AllowedIPs != nil && *apiKeyObj.AllowedIPs != "" {
				if !isIPAllowed(clientIP, *apiKeyObj.AllowedIPs) {
					handlerutil.WriteJSONError(w, http.StatusForbidden, "Access denied: IP address "+clientIP+" is not permitted for this API key.")
					return
				}
			}

			// Live tracking: register in-flight request and active caller IP
			cleanup := keikey.DefaultLiveTracker().TrackRequest(apiKeyObj.ID, clientIP)
			defer cleanup()

			// High-throughput async telemetry: queues usage & client telemetry to background worker,
			// eliminating hot-path SQLite write lock contention when swarmed by concurrent requests.
			if apiKeyObj.ID != "" {
				keikey.EnqueueTelemetry(repo, apiKeyObj.ID, clientIP, r.UserAgent())
			}

			// Inject API Key info into the request context for downstream handlers/logging
			ctx := context.WithValue(r.Context(), ApiKeyContextKey, apiKeyObj)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// resolveApiKey finds the row for an incoming key. Since issue #199 the
// plaintext column is the primary path again, matching upstream; the argon2id
// lookup remains only so a row created under F-6 keeps authenticating after the
// upgrade. There is no self-heal in either direction: rewriting a row would
// destroy the one property the dashboard now depends on, which is that the
// secret can be read back.
func resolveApiKey(repo *db.Repo, lookup, plaintext string) (*models.APIKey, error) {
	if cached, ok := keikey.DefaultAuthCache().Get(lookup); ok {
		return cached.(*models.APIKey), nil
	}
	apiKey, err := repo.GetApiKeyByKey(plaintext)
	if err != nil || apiKey == nil {
		if err != nil {
			return nil, err
		}
		return repo.FindApiKeyByLookup(lookup)
	}
	return apiKey, nil
}

// verifyApiKey checks the secret against the row. A row written before issue
// #199 carries an argon2id verifier and is confirmed with it; every other row
// was matched on the plaintext column itself, so there is nothing left to check.
func verifyApiKey(plaintext string, apiKey *models.APIKey) bool {
	if apiKey.KeyHash == nil || *apiKey.KeyHash == "" {
		return true
	}
	return keikey.Verify(plaintext, *apiKey.KeyHash)
}

// GetAuthenticatedApiKey retrieves the authenticated APIKey object from the request context.
func GetAuthenticatedApiKey(r *http.Request) *models.APIKey {
	val := r.Context().Value(ApiKeyContextKey)
	if val == nil {
		return nil
	}
	keyObj, ok := val.(*models.APIKey)
	if !ok {
		return nil
	}
	return keyObj
}

// AuthenticatedApiKeyFromContext retrieves the authenticated API key from a
// context rather than a request.
//
// The usage ledger needs it after the response is written, when the request
// object is gone (#365): a per-key token budget is measured from what the
// caller spent, so the row has to name the caller. The auth middleware already
// stores the key on the request context, so this is the same value the request
// path sees, read through the only handle the writer still holds.
func AuthenticatedApiKeyFromContext(ctx context.Context) *models.APIKey {
	if ctx == nil {
		return nil
	}
	val := ctx.Value(ApiKeyContextKey)
	if val == nil {
		return nil
	}
	keyObj, ok := val.(*models.APIKey)
	if !ok {
		return nil
	}
	return keyObj
}

// ExtractApiKey extracts the client API key from the request.
// For standard API routes, only header-based auth is accepted to avoid leaks.
// For EventSource / SSE stream endpoints (where browsers cannot set custom headers),
// query parameters `key` or `apiKey` are accepted as a fallback.
func ExtractApiKey(r *http.Request) string {
	// 1. Try Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
			return strings.TrimSpace(parts[1])
		}
	}

	// 2. Try custom X-API-Key header as fallback
	if xApiKey := r.Header.Get("X-API-Key"); xApiKey != "" {
		return xApiKey
	}

	// 3. For EventSource / SSE endpoints, accept query parameter key or apiKey
	if strings.HasSuffix(r.URL.Path, "/stream") {
		if qKey := r.URL.Query().Get("key"); qKey != "" {
			return qKey
		}
		if qKey := r.URL.Query().Get("apiKey"); qKey != "" {
			return qKey
		}
	}

	return ""
}

// isIPAllowed checks whether clientIP matches any entry in a comma-separated
// allowedIPs list. Entries can be single IPs or CIDR subnets (e.g. 192.168.1.0/24).
func isIPAllowed(clientIP, allowedIPs string) bool {
	allowedIPs = strings.TrimSpace(allowedIPs)
	if allowedIPs == "" {
		return true
	}
	clientIP = strings.TrimSpace(clientIP)
	clientAddr, err := netip.ParseAddr(clientIP)
	hasValidAddr := err == nil
	if hasValidAddr {
		clientAddr = clientAddr.Unmap()
	}

	entries := strings.Split(allowedIPs, ",")
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if entry == clientIP {
			return true
		}
		if prefix, err := netip.ParsePrefix(entry); err == nil && hasValidAddr {
			if prefix.Contains(clientAddr) {
				return true
			}
		}
		if addr, err := netip.ParseAddr(entry); err == nil && hasValidAddr {
			if addr.Unmap() == clientAddr {
				return true
			}
		}
	}
	return false
}

