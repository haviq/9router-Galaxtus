package chat

import (
	"9router/proxy/internal/db"
	json "encoding/json/v2"
	"strings"
)

// ApplyModelParametersToPayload applies non-nil ModelParameters overrides to an outbound JSON payload.
// Returns the updated JSON payload bytes, whether stream was overridden and its new boolean value, and any error.
func ApplyModelParametersToPayload(body []byte, params *db.ModelParameters) ([]byte, bool, bool, error) {
	if params == nil || len(body) == 0 {
		return body, false, false, nil
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return body, false, false, err
	}
	if raw == nil {
		raw = make(map[string]any)
	}

	streamOverridden := false
	newStreamVal := false

	if params.Temperature != nil {
		raw["temperature"] = *params.Temperature
	}
	if params.TopP != nil {
		raw["top_p"] = *params.TopP
	}
	if params.TopK != nil {
		raw["top_k"] = *params.TopK
	}
	if params.RepetitionPenalty != nil {
		raw["repetition_penalty"] = *params.RepetitionPenalty
	}
	if params.FrequencyPenalty != nil {
		raw["frequency_penalty"] = *params.FrequencyPenalty
	}
	if params.PresencePenalty != nil {
		raw["presence_penalty"] = *params.PresencePenalty
	}
	if params.MaxTokens != nil {
		raw["max_tokens"] = *params.MaxTokens
		if _, hasMct := raw["max_completion_tokens"]; hasMct {
			raw["max_completion_tokens"] = *params.MaxTokens
		}
	}
	if params.ContextLength != nil {
		raw["context_length"] = *params.ContextLength
		if opt, ok := raw["options"].(map[string]any); ok {
			opt["num_ctx"] = *params.ContextLength
		}
	}
	if params.Stream != nil {
		s := strings.ToLower(strings.TrimSpace(*params.Stream))
		if s == "on" || s == "true" {
			raw["stream"] = true
			streamOverridden = true
			newStreamVal = true
		} else if s == "off" || s == "false" {
			raw["stream"] = false
			streamOverridden = true
			newStreamVal = false
		}
	}

	updated, err := json.Marshal(raw)
	if err != nil {
		return body, false, false, err
	}
	return updated, streamOverridden, newStreamVal, nil
}

