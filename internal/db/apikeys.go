package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"9router/proxy/internal/models"
)

// errNoSuchApiKey signals an update that matched no row.
var errNoSuchApiKey = errors.New("no such api key")

// ValidateApiKey checks if the given API key exists and is active.
// Legacy plaintext path; hashed rows are resolved through
// FindApiKeyByLookup plus an argon2id verification.
func (r *Repo) ValidateApiKey(key string) (bool, error) {
	var active int
	err := r.db.QueryRow("SELECT isActive FROM apiKeys WHERE key = ? LIMIT 1", key).Scan(&active)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return active == 1, nil
}

const apiKeyColumns = `SELECT id, key, name, machineId, isActive, createdAt,
	rateLimitRPM, rateLimitTPM, rateLimitConcurrency,
	tokenQuota, queueTimeoutMs,
	keyHash, lookupHash, keyDisplay,
	expiresAt, lastUsedAt, usedCount, metadata, planId,
	createdBy, lastUsedIP, allowedIPs, balance`

// apiKeyScan returns the Scan destinations for apiKeyColumns plus an applier
// that must run after Scan has filled the nullable columns.
func apiKeyScan(k *models.APIKey) (dest []any, apply func()) {
	var nameVal, machineVal sql.NullString
	var rateRPM, rateTPM, rateConc sql.NullInt64
	var tokenQuota, queueTimeout sql.NullInt64
	var keyHash, lookupHash, keyDisplay sql.NullString
	var expiresAt, lastUsedAt, metadata sql.NullString
	var usedCount sql.NullInt64
	var planID sql.NullString
	var createdBy, lastUsedIP, allowedIPs sql.NullString
	var balance sql.NullFloat64
	dest = []any{&k.ID, &k.Key, &nameVal, &machineVal, &k.IsActive, &k.CreatedAt,
		&rateRPM, &rateTPM, &rateConc, &tokenQuota, &queueTimeout, &keyHash, &lookupHash, &keyDisplay,
		&expiresAt, &lastUsedAt, &usedCount, &metadata, &planID,
		&createdBy, &lastUsedIP, &allowedIPs, &balance}
	apply = func() {
		applyApiKeyNulls(k, nameVal, machineVal, rateRPM, rateTPM, rateConc, tokenQuota, queueTimeout, usedCount,
			keyHash, lookupHash, keyDisplay, expiresAt, lastUsedAt, metadata, planID,
			createdBy, lastUsedIP, allowedIPs, balance)
	}
	return dest, apply
}

// applyApiKeyNulls copies the scanned nullable columns onto the model.
func applyApiKeyNulls(k *models.APIKey, nameVal, machineVal sql.NullString,
	rateRPM, rateTPM, rateConc, tokenQuota, queueTimeout, usedCount sql.NullInt64,
	keyHash, lookupHash, keyDisplay sql.NullString,
	expiresAt, lastUsedAt, metadata, planID sql.NullString,
	createdBy, lastUsedIP, allowedIPs sql.NullString,
	balance sql.NullFloat64) {
	if nameVal.Valid {
		k.Name = &nameVal.String
	}
	if machineVal.Valid {
		k.MachineID = &machineVal.String
	}
	if rateRPM.Valid {
		v := int(rateRPM.Int64)
		k.RateLimitRPM = &v
	}
	if rateTPM.Valid {
		v := int(rateTPM.Int64)
		k.RateLimitTPM = &v
	}
	if rateConc.Valid {
		v := int(rateConc.Int64)
		k.RateLimitConcurrency = &v
	}
	if tokenQuota.Valid {
		v := int(tokenQuota.Int64)
		k.TokenQuota = &v
	}
	if queueTimeout.Valid {
		v := int(queueTimeout.Int64)
		k.QueueTimeoutMs = &v
	}
	if keyHash.Valid {
		k.KeyHash = &keyHash.String
	}
	if lookupHash.Valid {
		k.LookupHash = &lookupHash.String
	}
	if keyDisplay.Valid {
		k.KeyDisplay = &keyDisplay.String
	}
	if expiresAt.Valid {
		k.ExpiresAt = &expiresAt.String
	}
	if lastUsedAt.Valid {
		k.LastUsedAt = &lastUsedAt.String
	}
	if usedCount.Valid {
		v := int(usedCount.Int64)
		k.UsedCount = &v
	}
	if metadata.Valid {
		k.Metadata = &metadata.String
	}
	if planID.Valid {
		k.PlanID = &planID.String
	}
	if createdBy.Valid {
		k.CreatedBy = &createdBy.String
	}
	if lastUsedIP.Valid {
		k.LastUsedIP = &lastUsedIP.String
	}
	if allowedIPs.Valid {
		k.AllowedIPs = &allowedIPs.String
	}
	if balance.Valid {
		v := balance.Float64
		k.Balance = &v
	}
}

// GetApiKeyByKey retrieves detailed APIKey information by plaintext key.
// Kept for legacy rows whose `key` column still holds the secret; hashed
// rows have a blank `key` and are found via FindApiKeyByLookup.
func (r *Repo) GetApiKeyByKey(key string) (*models.APIKey, error) {
	return r.queryApiKey("FROM apiKeys WHERE key = ? LIMIT 1", key)
}

// FindApiKeyByLookup retrieves an APIKey by its SHA-256 lookup hash (F-6).
// This is the primary lookup path for hashed rows; it never compares
// plaintext. It returns (nil, nil) when no row matches.
func (r *Repo) FindApiKeyByLookup(lookupHash string) (*models.APIKey, error) {
	if lookupHash == "" {
		return nil, nil
	}
	return r.queryApiKey("FROM apiKeys WHERE lookupHash = ? LIMIT 1", lookupHash)
}

// queryApiKey runs a single-row apiKeys read and maps the scanned columns.
func (r *Repo) queryApiKey(whereClause string, arg string) (*models.APIKey, error) {
	var apiKey models.APIKey
	dest, apply := apiKeyScan(&apiKey)
	err := r.db.QueryRow(apiKeyColumns+" "+whereClause, arg).Scan(dest...)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get api key: %w", err)
	}
	apply()
	return &apiKey, nil
}

// hashedKeyPrefix marks a `key` column as a hashed row standing in for the
// original secret. `apiKeys.key` is TEXT UNIQUE NOT NULL, so every row cannot
// simply be blanked to the same value — the second hashed key would collide
// with the first. The sentinel is derived from that row's lookup hash, which
// makes it unique per row, and carries a prefix no client ever sends, so a
// legacy `WHERE key = ?` lookup can never authenticate against it.
const hashedKeyPrefix = "\x00argon2id:"

func hashedKeySentinel(lookupHash string) string {
	return hashedKeyPrefix + lookupHash
}

// IsHashedKeyRow reports whether a row is a pre-#199 hashed row: its `key`
// column holds the sentinel rather than a secret, so the plaintext is
// unrecoverable. Such a row still authenticates through its argon2id verifier
// (see middleware.resolveApiKey) but cannot be revealed in the dashboard.
func IsHashedKeyRow(k *models.APIKey) bool {
	return k != nil && strings.HasPrefix(k.Key, hashedKeyPrefix)
}

// HashApiKeyRow fills the F-6 hash columns for a row and replaces the
// plaintext `key` column with the hashed-row sentinel. A sentinel `key` is how
// a hashed row is recognised; legacy rows keep their plaintext and
// authenticate through GetApiKeyByKey until this runs. After it runs the secret
// exists only as an argon2id verifier and a masked display value.
func (r *Repo) HashApiKeyRow(id, keyHash, lookupHash, keyDisplay string) error {
	res, err := r.db.Exec(
		`UPDATE apiKeys SET key = ?, keyHash = ?, lookupHash = ?, keyDisplay = ? WHERE id = ?`,
		hashedKeySentinel(lookupHash), keyHash, lookupHash, keyDisplay, id,
	)
	if err != nil {
		return fmt.Errorf("hash api key row %s: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("hash api key row %s: %w", id, errNoSuchApiKey)
	}
	return nil
}

// GetApiKeyByID retrieves one key by its row id rather than by its secret.
// The dashboard policy editor uses it so a partial update can read the current
// values without ever touching the credential columns.
func (r *Repo) GetApiKeyByID(id string) (*models.APIKey, error) {
	return r.queryApiKey("FROM apiKeys WHERE id = ? LIMIT 1", id)
}

// RotateApiKeySecret replaces a row's secret with a new plaintext value and
// refreshes its masked display.
//
// The hash columns are cleared rather than left stale: a row that still carried
// a verifier from before issue #199 would keep being resolved through the
// argon2id path, so its new secret would never be found and the key would
// appear dead to every client holding it.
func (r *Repo) RotateApiKeySecret(id, plaintext, keyDisplay string) error {
	res, err := r.db.Exec(
		`UPDATE apiKeys SET key = ?, keyDisplay = ?, keyHash = '', lookupHash = '' WHERE id = ?`,
		plaintext, keyDisplay, id,
	)
	if err != nil {
		return fmt.Errorf("rotate api key secret %s: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("rotate api key secret %s: %w", id, errNoSuchApiKey)
	}
	return nil
}

// SetApiKeyDisplay records the masked form of a row's secret without touching
// the secret itself.
func (r *Repo) SetApiKeyDisplay(id, keyDisplay string) error {
	res, err := r.db.Exec(`UPDATE apiKeys SET keyDisplay = ? WHERE id = ?`, keyDisplay, id)
	if err != nil {
		return fmt.Errorf("set api key display %s: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("set api key display %s: %w", id, errNoSuchApiKey)
	}
	return nil
}

// GetFirstActiveApiKey returns the first active client API key, or nil if none exist.
func (r *Repo) GetFirstActiveApiKey() (*models.APIKey, error) {
	keys, err := r.GetApiKeys()
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		if k.IsActive == 1 {
			return k, nil
		}
	}
	return nil, nil
}

// SetApiKeyCreatedBy sets the creator/owner identity of an API key.
func (r *Repo) SetApiKeyCreatedBy(id, createdBy string) error {
	res, err := r.db.Exec(`UPDATE apiKeys SET createdBy = ? WHERE id = ?`, createdBy, id)
	if err != nil {
		return fmt.Errorf("set api key createdBy %s: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("set api key createdBy %s: %w", id, errNoSuchApiKey)
	}
	return nil
}

// SetApiKeyAllowedIPs updates the allowed IP / CIDR whitelist for an API key.
func (r *Repo) SetApiKeyAllowedIPs(id, allowedIPs string) error {
	res, err := r.db.Exec(`UPDATE apiKeys SET allowedIPs = ? WHERE id = ?`, allowedIPs, id)
	if err != nil {
		return fmt.Errorf("set api key allowedIPs %s: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("set api key allowedIPs %s: %w", id, errNoSuchApiKey)
	}
	return nil
}

// RecordApiKeyUsageWithClient records the call, updates lastUsedAt and lastUsedIP on apiKeys,
// and upserts caller client telemetry into api_key_clients.
func (r *Repo) RecordApiKeyUsageWithClient(keyID, ip, userAgent string) error {
	if keyID == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if ip == "" {
		ip = "127.0.0.1"
	}

	// Update apiKeys row
	_, err := r.db.Exec(
		`UPDATE apiKeys SET lastUsedAt = ?, lastUsedIP = ?, usedCount = usedCount + 1 WHERE id = ?`,
		now, ip, keyID,
	)
	if err != nil {
		return fmt.Errorf("update api key last used %s: %w", keyID, err)
	}

	// Upsert api_key_clients record
	_, err = r.db.Exec(`
		INSERT INTO api_key_clients (api_key_id, ip, user_agent, request_count, first_seen_at, last_seen_at)
		VALUES (?, ?, ?, 1, ?, ?)
		ON CONFLICT(api_key_id, ip) DO UPDATE SET
			request_count = request_count + 1,
			last_seen_at = excluded.last_seen_at,
			user_agent = CASE WHEN excluded.user_agent != '' THEN excluded.user_agent ELSE api_key_clients.user_agent END
	`, keyID, ip, userAgent, now, now)
	if err != nil {
		return fmt.Errorf("upsert api key client %s / %s: %w", keyID, ip, err)
	}
	return nil
}

// GetApiKeyClients returns all client IPs that have authenticated using this key.
func (r *Repo) GetApiKeyClients(keyID string) ([]models.APIKeyClient, error) {
	rows, err := r.db.Query(
		`SELECT api_key_id, ip, user_agent, request_count, first_seen_at, last_seen_at
		 FROM api_key_clients
		 WHERE api_key_id = ?
		 ORDER BY last_seen_at DESC`,
		keyID,
	)
	if err != nil {
		return nil, fmt.Errorf("get api key clients %s: %w", keyID, err)
	}
	defer rows.Close()

	var clients []models.APIKeyClient
	for rows.Next() {
		var c models.APIKeyClient
		var ua sql.NullString
		if err := rows.Scan(&c.APIKeyID, &c.IP, &ua, &c.RequestCount, &c.FirstSeenAt, &c.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan api key client: %w", err)
		}
		if ua.Valid {
			c.UserAgent = ua.String
		}
		clients = append(clients, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api key clients: %w", err)
	}
	return clients, nil
}

