package proxy

import (
	"bytes"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"

	"9router/proxy/internal/handlerutil"
)

// RefusalPhrases are canonical AI model safety refusal patterns (checked case-insensitively).
var RefusalPhrases = []string{
	"i cannot fulfill",
	"i am unable to fulfill",
	"i can't fulfill",
	"i cannot assist with",
	"i am unable to assist with",
	"i can't assist with",
	"i cannot assist you with",
	"i am unable to assist you with",
	"i cannot comply",
	"i am unable to comply",
	"i apologize, but i cannot",
	"i apologize, but as an ai",
	"as an ai language model",
	"as an ai, i cannot",
	"as an ai assistant, i cannot",
	"i'm sorry, but i cannot",
	"i am sorry, but i cannot",
	"i'm sorry, but i'm unable",
	"i am sorry, but i am unable",
	"i must decline",
	"against our safety guidelines",
	"against my safety guidelines",
	"against safety guidelines",
	"against our safety policy",
	"violates our safety policy",
	"violates safety policy",
	"violates our content policy",
	"violates open-source safety guidelines",
	"my safety guidelines strictly prohibit",
	"i cannot generate exploits",
	"i cannot help you write exploits",
	"i cannot assist with hacking",
	"i cannot provide instructions for bypassing",
	"i cannot provide code for malicious",
	"i cannot assist with creating malware",
	"i cannot help with malware",
}

// conversationalGreetings are conversational openers stripped before testing refusal patterns.
var conversationalGreetings = []string{
	"i'm sorry, but ", "i am sorry, but ",
	"i'm sorry, ", "i am sorry, ",
	"sorry, but ", "sorry, ", "sorry. ", "sorry! ",
	"hello! ", "hello, ", "hi! ", "hi, ",
	"this request ", "your request ",
}

// CleanRefusalPreamble strips leading whitespace and common conversational openers.
func CleanRefusalPreamble(text string) string {
	s := strings.TrimSpace(text)
	lower := strings.ToLower(s)
	changed := true
	for changed {
		changed = false
		for _, g := range conversationalGreetings {
			if strings.HasPrefix(lower, g) {
				s = s[len(g):]
				lower = strings.ToLower(strings.TrimSpace(s))
				changed = true
			}
		}
	}
	return strings.TrimSpace(lower)
}

// MatchesRefusalPhrase checks if text begins with or contains a known AI model refusal phrase.
func MatchesRefusalPhrase(text string) (string, bool) {
	cleaned := CleanRefusalPreamble(text)
	if len(cleaned) == 0 {
		return "", false
	}
	for _, phrase := range RefusalPhrases {
		if strings.HasPrefix(cleaned, phrase) {
			return phrase, true
		}
		// Policy phrases often occur after introductory sentence
		if strings.Contains(phrase, "violates") || strings.Contains(phrase, "guidelines") {
			if strings.Contains(cleaned, phrase) {
				return phrase, true
			}
		}
	}
	return "", false
}

// IsPotentialRefusalPrefix checks if text is a non-empty prefix of a known refusal phrase.
// Used in streaming peeks to decide if peeking should buffer another chunk before committing headers.
func IsPotentialRefusalPrefix(text string) bool {
	cleaned := CleanRefusalPreamble(text)
	if len(cleaned) == 0 {
		return false
	}
	for _, phrase := range RefusalPhrases {
		if strings.HasPrefix(phrase, cleaned) {
			return true
		}
	}
	return false
}

type refusalStreamPayload struct {
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Delta        struct {
			Content   jsontext.Value `json:"content"`
			Refusal   jsontext.Value `json:"refusal"`
			ToolCalls jsontext.Value `json:"tool_calls"`
		} `json:"delta"`
		Message struct {
			Content   jsontext.Value `json:"content"`
			Refusal   jsontext.Value `json:"refusal"`
			ToolCalls jsontext.Value `json:"tool_calls"`
		} `json:"message"`
		Text string `json:"text"`
	} `json:"choices"`
	StopReason string `json:"stop_reason"`
	Type       string `json:"type"`
	Delta      *struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
}

// DetectRefusalInStream inspects one SSE data chunk payload for refusal indicators.
func DetectRefusalInStream(data []byte) (string, bool) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("[DONE]")) || trimmed[0] != '{' {
		return "", false
	}

	var p refusalStreamPayload
	if err := json.Unmarshal(trimmed, &p, handlerutil.UpstreamBody); err != nil {
		return "", false
	}

	// Claude Messages stop_reason
	if p.StopReason == "refusal" || p.StopReason == "safety" {
		return "stop_reason: " + p.StopReason, true
	}
	if p.Delta != nil && (p.Delta.StopReason == "refusal" || p.Delta.StopReason == "safety") {
		return "delta.stop_reason: " + p.Delta.StopReason, true
	}

	if len(p.Choices) > 0 {
		c0 := p.Choices[0]
		if c0.FinishReason == "content_filter" || c0.FinishReason == "refusal" {
			return "finish_reason: " + c0.FinishReason, true
		}
		if ref := extractRefusalText(c0.Delta.Refusal); ref != "" {
			return "refusal: " + ref, true
		}
		if ref := extractRefusalText(c0.Message.Refusal); ref != "" {
			return "refusal: " + ref, true
		}
		// If tool calls are present, this is not a refusal
		if carriesValue(c0.Delta.ToolCalls) || carriesValue(c0.Message.ToolCalls) {
			return "", false
		}
		content := extractContentText(c0.Delta.Content)
		if content == "" {
			content = extractContentText(c0.Message.Content)
		}
		if content == "" {
			content = c0.Text
		}
		if phrase, matched := MatchesRefusalPhrase(content); matched {
			return "model text refusal: " + phrase, true
		}
	}

	return "", false
}

// ExtractTextChunk extracts text content from an SSE data payload if present.
func ExtractTextChunk(data []byte) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("[DONE]")) || trimmed[0] != '{' {
		return ""
	}
	var p refusalStreamPayload
	if err := json.Unmarshal(trimmed, &p, handlerutil.UpstreamBody); err != nil {
		return ""
	}
	if len(p.Choices) > 0 {
		c0 := p.Choices[0]
		if carriesValue(c0.Delta.ToolCalls) || carriesValue(c0.Message.ToolCalls) {
			return ""
		}
		if t := extractContentText(c0.Delta.Content); t != "" {
			return t
		}
		if t := extractContentText(c0.Message.Content); t != "" {
			return t
		}
		return c0.Text
	}
	return ""
}

// DetectRefusalInPayload inspects a complete non-streaming response body for refusal indicators.
func DetectRefusalInPayload(data []byte) (string, bool) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return "", false
	}
	var p refusalStreamPayload
	if err := json.Unmarshal(trimmed, &p, handlerutil.UpstreamBody); err != nil {
		return "", false
	}

	if p.StopReason == "refusal" || p.StopReason == "safety" {
		return "stop_reason: " + p.StopReason, true
	}
	if len(p.Choices) > 0 {
		c0 := p.Choices[0]
		if c0.FinishReason == "content_filter" || c0.FinishReason == "refusal" {
			return "finish_reason: " + c0.FinishReason, true
		}
		if ref := extractRefusalText(c0.Message.Refusal); ref != "" {
			return "refusal: " + ref, true
		}
		if carriesValue(c0.Message.ToolCalls) {
			return "", false
		}
		content := extractContentText(c0.Message.Content)
		if content == "" {
			content = c0.Text
		}
		if phrase, matched := MatchesRefusalPhrase(content); matched {
			return "model text refusal: " + phrase, true
		}
	}
	return "", false
}

func extractRefusalText(raw jsontext.Value) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s, handlerutil.UpstreamBody); err == nil {
			return strings.TrimSpace(s)
		}
	}
	return strings.TrimSpace(string(trimmed))
}

func extractContentText(raw jsontext.Value) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s, handlerutil.UpstreamBody); err == nil {
			return s
		}
	} else if trimmed[0] == '[' {
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(trimmed, &blocks, handlerutil.UpstreamBody); err == nil {
			var sb strings.Builder
			for _, b := range blocks {
				if b.Text != "" {
					sb.WriteString(b.Text)
					sb.WriteString(" ")
				}
			}
			return strings.TrimSpace(sb.String())
		}
	}
	return ""
}
