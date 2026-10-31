package db

import (
	"database/sql"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"strings"
)

// mapKey is a helper type to avoid linter complaining about general map keys
type mapKey = string

// parseJSONString helper unquotes/unmarshals a JSON string if it is JSON-encoded.
// Otherwise, it returns the string as-is.
func parseJSONString(raw string) string {
	var val string
	// Check if it looks like a JSON string representation (enclosed in quotes)
	if strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
		if err := json.Unmarshal([]byte(raw), &val); err == nil {
			return val
		}
	}
	return raw
}

// ModelRemaskRule specifies an inbound model alias mapping with anti-detection cloaking and context rules.
type ModelRemaskRule struct {
	Alias         string `json:"alias"`
	Target        string `json:"target"`
	Enabled       bool   `json:"enabled"`
	CloakIdentity bool   `json:"cloakIdentity,omitempty"`
	StripThinking bool   `json:"stripThinking,omitempty"`
	ContextWindow int    `json:"contextWindow,omitempty"`
	MaxOutput     int    `json:"maxOutput,omitempty"`
	CustomPrompt  string `json:"customPrompt,omitempty"`
	Watermark     string `json:"watermark,omitempty"`
	WatermarkMode string `json:"watermarkMode,omitempty"` // "append", "prepend", "invisible"
}

func parseRemaskRule(key, raw string) *ModelRemaskRule {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "{") {
		var rule ModelRemaskRule
		if err := json.Unmarshal([]byte(trimmed), &rule); err == nil && rule.Target != "" {
			if rule.Alias == "" {
				rule.Alias = key
			}
			return &rule
		}
	}
	target := parseJSONString(raw)
	if target == "" {
		return nil
	}
	return &ModelRemaskRule{
		Alias:   key,
		Target:  target,
		Enabled: true,
	}
}

// GetModelRemaskRule retrieves the ModelRemaskRule for a given alias.
func (r *Repo) GetModelRemaskRule(alias string) (*ModelRemaskRule, error) {
	var rawVal string
	err := r.db.QueryRow(
		"SELECT value FROM kv WHERE scope = 'modelAliases' AND key = ? LIMIT 1",
		alias,
	).Scan(&rawVal)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get model remask rule %s: %w", alias, err)
	}
	return parseRemaskRule(alias, rawVal), nil
}

// GetModelRemaskRules returns all configured model remask rules.
func (r *Repo) GetModelRemaskRules() ([]ModelRemaskRule, error) {
	rows, err := r.db.Query("SELECT key, value FROM kv WHERE scope = 'modelAliases'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []ModelRemaskRule
	for rows.Next() {
		var key, rawVal string
		if err := rows.Scan(&key, &rawVal); err != nil {
			return nil, err
		}
		if rule := parseRemaskRule(key, rawVal); rule != nil {
			list = append(list, *rule)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// SetModelRemaskRule persists a model remask rule.
func (r *Repo) SetModelRemaskRule(rule ModelRemaskRule) error {
	if rule.Alias == "" {
		return fmt.Errorf("alias is required")
	}
	if rule.Target == "" {
		return fmt.Errorf("target is required")
	}
	b, err := json.Marshal(rule)
	if err != nil {
		return fmt.Errorf("marshal rule: %w", err)
	}
	_, err = r.db.Exec(`
		INSERT INTO kv (scope, key, value)
		VALUES ('modelAliases', ?, ?)
		ON CONFLICT(scope, key) DO UPDATE SET
			value = excluded.value
	`, rule.Alias, string(b))
	return err
}

// DeleteModelRemaskRule removes a remask rule by alias.
func (r *Repo) DeleteModelRemaskRule(alias string) error {
	_, err := r.db.Exec("DELETE FROM kv WHERE scope = 'modelAliases' AND key = ?", alias)
	return err
}

// GetModelAlias retrieves the target model string for a given alias.
func (r *Repo) GetModelAlias(alias string) (string, error) {
	rule, err := r.GetModelRemaskRule(alias)
	if err != nil {
		return "", err
	}
	if rule == nil || !rule.Enabled {
		return "", nil
	}
	return rule.Target, nil
}

// GetModelAliases returns all model aliases as a key-value map.
func (r *Repo) GetModelAliases() (map[mapKey]string, error) {
	rules, err := r.GetModelRemaskRules()
	if err != nil {
		return nil, err
	}
	aliases := make(map[mapKey]string, len(rules))
	for _, rule := range rules {
		if rule.Enabled {
			aliases[rule.Alias] = rule.Target
		}
	}
	return aliases, nil
}

// CustomModel represents a user-defined model stored in kv scope customModels.
//
// ContextWindow and MaxOutput are the operator's own declaration of what the
// endpoint can actually take. Zero means "not declared": a provider node's
// model id is usually an open-source name that matches no capability pattern,
// so without these fields /v1/models had nothing better than a substring guess
// or the 128k floor.
type CustomModel struct {
	ProviderAlias string          `json:"providerAlias"`
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	Name          string          `json:"name"`
	Caps          map[string]bool `json:"caps"`
	ContextWindow int             `json:"contextWindow,omitempty"`
	MaxOutput     int             `json:"maxOutput,omitempty"`
}

// UnmarshalJSON reads a customModels row without letting one unusable `caps`
// value discard the row's limits.
//
// `caps` is a map of flags, but rows written by other builds and by hand carry
// non-boolean metadata in it — a bridge-added row with
// "thinkingFormat":"effort" is the real-world case. Decoding straight into
// map[string]bool fails on that value, and because encoding/json/v2 stops at
// the failing member, every field *after* caps (contextWindow, maxOutput) is
// left zero. The row then reads as "no limits declared", the substring table
// takes over, and /v1/models publishes 128000/4096 for an endpoint that
// declared 1M/64k.
//
// Non-boolean capability values are dropped, which is the policy upstream's
// sanitizeCaps already applies on write: only boolean flags are capabilities.
// The row itself — ids, name, declared limits — always survives.
func (cm *CustomModel) UnmarshalJSON(data []byte) error {
	type row struct {
		ProviderAlias string         `json:"providerAlias"`
		ID            string         `json:"id"`
		Type          string         `json:"type"`
		Name          string         `json:"name"`
		Caps          jsontext.Value `json:"caps"`
		ContextWindow int            `json:"contextWindow,omitempty"`
		MaxOutput     int            `json:"maxOutput,omitempty"`
	}
	var r row
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	cm.ProviderAlias = r.ProviderAlias
	cm.ID = r.ID
	cm.Type = r.Type
	cm.Name = r.Name
	cm.ContextWindow = r.ContextWindow
	cm.MaxOutput = r.MaxOutput
	cm.Caps = nil
	if len(r.Caps) == 0 || string(r.Caps) == "null" {
		return nil
	}
	var raw map[string]jsontext.Value
	if err := json.Unmarshal(r.Caps, &raw); err != nil {
		// A caps block that is not an object at all is metadata we cannot use,
		// not a reason to drop the row.
		return nil
	}
	caps := make(map[string]bool, len(raw))
	for key, value := range raw {
		var flag bool
		if err := json.Unmarshal(value, &flag); err == nil {
			caps[key] = flag
		}
	}
	if len(caps) > 0 {
		cm.Caps = caps
	}
	return nil
}

// GetCustomModels returns all custom models from kv scope customModels.
func (r *Repo) GetCustomModels() ([]*CustomModel, error) {
	rows, err := r.db.Query("SELECT key, value FROM kv WHERE scope = 'customModels'")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*CustomModel
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		var cm CustomModel
		_ = json.Unmarshal([]byte(raw), &cm)

		// Fallback parse from key if fields are missing in JSON value:
		// key format in Next.js: <providerAlias>|<modelId>|<kind> or <providerAlias>/<modelId>/<kind>
		if cm.ProviderAlias == "" || cm.ID == "" {
			parts := strings.Split(key, "|")
			if len(parts) < 2 {
				parts = strings.Split(key, "/")
			}
			if len(parts) >= 2 {
				if cm.ProviderAlias == "" {
					cm.ProviderAlias = parts[0]
				}
				if cm.ID == "" {
					cm.ID = parts[1]
				}
				if cm.Type == "" && len(parts) >= 3 {
					cm.Type = parts[2]
				}
			}
		}

		if cm.ID == "" || cm.ProviderAlias == "" {
			continue
		}
		out = append(out, &cm)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}