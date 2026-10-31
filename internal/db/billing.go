package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ModelPricing defines the token tariff for a specific model or pattern.
type ModelPricing struct {
	ID            string  `json:"id"`
	ModelPattern  string  `json:"modelPattern"`
	PriceInput1k  float64 `json:"priceInput1k"`  // Price per 1,000 input tokens
	PriceOutput1k float64 `json:"priceOutput1k"` // Price per 1,000 output tokens
	Currency      string  `json:"currency"`      // "IDR" or "USD"
	CreatedAt     string  `json:"createdAt"`
	UpdatedAt     string  `json:"updatedAt"`
}

// ListModelPricing returns all configured model pricing rules.
func (r *Repo) ListModelPricing() ([]*ModelPricing, error) {
	rows, err := r.db.Query(`
		SELECT id, model_pattern, price_input_1k, price_output_1k, currency, created_at, updated_at
		FROM model_pricing
		ORDER BY model_pattern ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list model pricing: %w", err)
	}
	defer rows.Close()

	var result []*ModelPricing
	for rows.Next() {
		p := &ModelPricing{}
		if err := rows.Scan(&p.ID, &p.ModelPattern, &p.PriceInput1k, &p.PriceOutput1k, &p.Currency, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan model pricing: %w", err)
		}
		result = append(result, p)
	}
	return result, nil
}

// GetModelPricing looks up the pricing rule for a specific model (exact or wildcard pattern).
func (r *Repo) GetModelPricing(model string) (*ModelPricing, error) {
	// 1. Try exact match
	var p ModelPricing
	err := r.db.QueryRow(`
		SELECT id, model_pattern, price_input_1k, price_output_1k, currency, created_at, updated_at
		FROM model_pricing
		WHERE model_pattern = ?
		LIMIT 1
	`, model).Scan(&p.ID, &p.ModelPattern, &p.PriceInput1k, &p.PriceOutput1k, &p.Currency, &p.CreatedAt, &p.UpdatedAt)
	if err == nil {
		return &p, nil
	}

	// 2. Try prefix/pattern matching
	rows, err := r.db.Query(`
		SELECT id, model_pattern, price_input_1k, price_output_1k, currency, created_at, updated_at
		FROM model_pricing
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lowerModel := strings.ToLower(model)
	var defaultPricing *ModelPricing

	for rows.Next() {
		var item ModelPricing
		if err := rows.Scan(&item.ID, &item.ModelPattern, &item.PriceInput1k, &item.PriceOutput1k, &item.Currency, &item.CreatedAt, &item.UpdatedAt); err != nil {
			continue
		}
		pat := strings.ToLower(item.ModelPattern)
		if pat == "*" || pat == "default" {
			defaultPricing = &item
			continue
		}
		// Prefix or wildcard check
		if strings.HasSuffix(pat, "*") {
			prefix := strings.TrimSuffix(pat, "*")
			if strings.HasPrefix(lowerModel, prefix) {
				return &item, nil
			}
		}
		if strings.Contains(lowerModel, pat) {
			return &item, nil
		}
	}

	if defaultPricing != nil {
		return defaultPricing, nil
	}

	return nil, sql.ErrNoRows
}

// SetModelPricing creates or updates a model pricing entry.
func (r *Repo) SetModelPricing(pricing *ModelPricing) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if pricing.ID == "" {
		pricing.ID = uuid.NewString()
		pricing.CreatedAt = now
	}
	pricing.UpdatedAt = now
	if pricing.Currency == "" {
		pricing.Currency = "IDR"
	}

	_, err := r.db.Exec(`
		INSERT INTO model_pricing (id, model_pattern, price_input_1k, price_output_1k, currency, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(model_pattern) DO UPDATE SET
			price_input_1k = excluded.price_input_1k,
			price_output_1k = excluded.price_output_1k,
			currency = excluded.currency,
			updated_at = excluded.updated_at
	`, pricing.ID, pricing.ModelPattern, pricing.PriceInput1k, pricing.PriceOutput1k, pricing.Currency, pricing.CreatedAt, pricing.UpdatedAt)

	return err
}

// DeleteModelPricing removes a pricing rule.
func (r *Repo) DeleteModelPricing(id string) error {
	_, err := r.db.Exec("DELETE FROM model_pricing WHERE id = ?", id)
	return err
}

// DeductApiKeyBalance deducts an amount from the API key balance atomically.
// Returns the new remaining balance. If balance is negative (< 0), it is unlimited.
func (r *Repo) DeductApiKeyBalance(apiKeyID string, amount float64) (float64, error) {
	if amount <= 0 {
		return 0, nil
	}

	var currentBal sql.NullFloat64
	err := r.db.QueryRow("SELECT balance FROM apiKeys WHERE id = ? OR key = ?", apiKeyID, apiKeyID).Scan(&currentBal)
	if err != nil {
		return 0, fmt.Errorf("read key balance: %w", err)
	}

	// -1 or NULL means unlimited
	if !currentBal.Valid || currentBal.Float64 < 0 {
		return -1, nil
	}

	newBal := currentBal.Float64 - amount
	if newBal < 0 {
		newBal = 0
	}

	_, err = r.db.Exec("UPDATE apiKeys SET balance = ? WHERE id = ? OR key = ?", newBal, apiKeyID, apiKeyID)
	if err != nil {
		return currentBal.Float64, fmt.Errorf("update key balance: %w", err)
	}

	return newBal, nil
}

// SetApiKeyBalance sets the balance of an API key (e.g. top-up).
func (r *Repo) SetApiKeyBalance(apiKeyID string, balance float64) error {
	_, err := r.db.Exec("UPDATE apiKeys SET balance = ? WHERE id = ? OR key = ?", balance, apiKeyID, apiKeyID)
	return err
}

// ClientRecentRequest represents a single request entry for customer telemetry.
type ClientRecentRequest struct {
	ID               int64   `json:"id"`
	Timestamp        string  `json:"timestamp"`
	Model            string  `json:"model"`
	Endpoint         string  `json:"endpoint"`
	PromptTokens     int     `json:"promptTokens"`
	CompletionTokens int     `json:"completionTokens"`
	TotalTokens      int     `json:"totalTokens"`
	Cost             float64 `json:"cost"`
	Status           string  `json:"status"`
}

// ClientTokenStats aggregates historical usage for a given client key.
type ClientTokenStats struct {
	TotalRequests    int64   `json:"totalRequests"`
	PromptTokens     int64   `json:"promptTokens"`
	CompletionTokens int64   `json:"completionTokens"`
	TotalTokens      int64   `json:"totalTokens"`
	TotalCost        float64 `json:"totalCost"`
}

// GetClientRecentUsage returns recent inference calls executed with this key.
func (r *Repo) GetClientRecentUsage(clientKey string, limit int) ([]ClientRecentRequest, error) {
	if limit <= 0 || limit > 50 {
		limit = 15
	}
	masked := clientKey
	if len(clientKey) > 12 {
		masked = clientKey[:8] + "***" + clientKey[len(clientKey)-4:]
	}

	rows, err := r.db.Query(`
		SELECT id, timestamp, COALESCE(model, ''), COALESCE(endpoint, ''), 
		       COALESCE(promptTokens, 0), COALESCE(completionTokens, 0), 
		       COALESCE(cost, 0.0), COALESCE(status, 'ok')
		FROM usageHistory
		WHERE clientKey = ? OR clientKey = ? OR apiKey = ? OR apiKey = ?
		ORDER BY id DESC
		LIMIT ?
	`, clientKey, masked, clientKey, masked, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]ClientRecentRequest, 0, limit)
	for rows.Next() {
		var req ClientRecentRequest
		if err := rows.Scan(&req.ID, &req.Timestamp, &req.Model, &req.Endpoint, &req.PromptTokens, &req.CompletionTokens, &req.Cost, &req.Status); err != nil {
			continue
		}
		req.TotalTokens = req.PromptTokens + req.CompletionTokens
		list = append(list, req)
	}
	return list, nil
}

// GetClientTokenStats returns aggregate token count and spent cost for a client key.
func (r *Repo) GetClientTokenStats(clientKey string) ClientTokenStats {
	var s ClientTokenStats
	masked := clientKey
	if len(clientKey) > 12 {
		masked = clientKey[:8] + "***" + clientKey[len(clientKey)-4:]
	}

	_ = r.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(promptTokens), 0), COALESCE(SUM(completionTokens), 0), COALESCE(SUM(cost), 0.0)
		FROM usageHistory
		WHERE clientKey = ? OR clientKey = ? OR apiKey = ? OR apiKey = ?
	`, clientKey, masked, clientKey, masked).Scan(&s.TotalRequests, &s.PromptTokens, &s.CompletionTokens, &s.TotalCost)
	s.TotalTokens = s.PromptTokens + s.CompletionTokens
	return s
}

