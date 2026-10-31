package chat

import (
	"9router/proxy/internal/db"
	json "encoding/json/v2"
	"testing"
)

func TestApplyModelParametersToPayload(t *testing.T) {
	temp := 1.0
	topP := 1.0
	topK := 0
	repPen := 1.0
	freqPen := 0.0
	presPen := 0.0
	maxTokens := 8192
	ctxLen := 131072
	stream := "on"

	params := &db.ModelParameters{
		Temperature:       &temp,
		TopP:              &topP,
		TopK:              &topK,
		RepetitionPenalty: &repPen,
		FrequencyPenalty:  &freqPen,
		PresencePenalty:   &presPen,
		MaxTokens:         &maxTokens,
		ContextLength:     &ctxLen,
		Stream:            &stream,
	}

	initialJSON := []byte(`{"model":"test-model","messages":[{"role":"user","content":"hi"}],"temperature":0.5,"stream":false}`)

	updated, streamOverridden, newStreamVal, err := ApplyModelParametersToPayload(initialJSON, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !streamOverridden || !newStreamVal {
		t.Fatalf("expected stream to be overridden to true, got overridden=%v val=%v", streamOverridden, newStreamVal)
	}

	var parsed map[string]any
	if err := json.Unmarshal(updated, &parsed); err != nil {
		t.Fatalf("failed to unmarshal updated JSON: %v", err)
	}

	if parsed["temperature"] != 1.0 {
		t.Errorf("expected temperature 1.0, got %v", parsed["temperature"])
	}
	if parsed["top_p"] != 1.0 {
		t.Errorf("expected top_p 1.0, got %v", parsed["top_p"])
	}
	if parsed["top_k"] != float64(0) {
		t.Errorf("expected top_k 0, got %v", parsed["top_k"])
	}
	if parsed["repetition_penalty"] != 1.0 {
		t.Errorf("expected repetition_penalty 1.0, got %v", parsed["repetition_penalty"])
	}
	if parsed["frequency_penalty"] != 0.0 {
		t.Errorf("expected frequency_penalty 0, got %v", parsed["frequency_penalty"])
	}
	if parsed["presence_penalty"] != 0.0 {
		t.Errorf("expected presence_penalty 0, got %v", parsed["presence_penalty"])
	}
	if parsed["max_tokens"] != float64(8192) {
		t.Errorf("expected max_tokens 8192, got %v", parsed["max_tokens"])
	}
	if parsed["context_length"] != float64(131072) {
		t.Errorf("expected context_length 131072, got %v", parsed["context_length"])
	}
	if parsed["stream"] != true {
		t.Errorf("expected stream true, got %v", parsed["stream"])
	}
}

func TestLookupModelParameters(t *testing.T) {
	temp := 0.7
	settings := &db.SettingsData{
		ModelParameters: map[string]db.ModelParameters{
			"FREE": {
				Temperature: &temp,
			},
			"anthropic/claude-3-5-sonnet": {
				Temperature: &temp,
			},
		},
	}

	// Exact match
	if p := settings.LookupModelParameters("FREE"); p == nil || *p.Temperature != 0.7 {
		t.Errorf("expected exact match for FREE")
	}

	// Case-insensitive match
	if p := settings.LookupModelParameters("free"); p == nil || *p.Temperature != 0.7 {
		t.Errorf("expected case-insensitive match for free")
	}

	// Suffix match
	if p := settings.LookupModelParameters("claude-3-5-sonnet"); p == nil || *p.Temperature != 0.7 {
		t.Errorf("expected suffix match for claude-3-5-sonnet")
	}

	// Not found
	if p := settings.LookupModelParameters("nonexistent"); p != nil {
		t.Errorf("expected nil for nonexistent")
	}
}

