package antiflaky

import (
	"bytes"
	"encoding/json"
	"strings"
)

// FlakyReason indicates why a response was flagged as flaky.
type FlakyReason string

const (
	ReasonEmptyBody        FlakyReason = "empty_body"
	ReasonDegenerateLoop   FlakyReason = "degenerate_repetition_loop"
	ReasonIdenticalPhrases FlakyReason = "identical_phrases_loop"
)

// hasConsecutiveSubstringRepetition checks if any substring of length 2..40 is repeated
// consecutively at least minRepeat times.
func hasConsecutiveSubstringRepetition(s string, minRepeat int) bool {
	n := len(s)
	if n < 16 {
		return false
	}
	// Check pattern lengths from 2 to 40
	maxPatLen := 40
	if maxPatLen > n/minRepeat {
		maxPatLen = n / minRepeat
	}

	for patLen := 2; patLen <= maxPatLen; patLen++ {
		for i := 0; i <= n-(patLen*minRepeat); {
			pat := s[i : i+patLen]
			repeats := 1
			idx := i + patLen
			for idx+patLen <= n && s[idx:idx+patLen] == pat {
				repeats++
				idx += patLen
				if repeats >= minRepeat {
					return true
				}
			}
			if repeats > 1 {
				i = idx
			} else {
				i++
			}
		}
	}
	return false
}

// IsFlakyResponse inspects a completed response body and determines if it is empty
// or stuck in a degenerate repetition loop (e.g. "OK.OK.OK..." or empty output).
func IsFlakyResponse(body []byte) (bool, FlakyReason) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return true, ReasonEmptyBody
	}

	// Try parsing standard OpenAI chat completion JSON structure
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content any `json:"content"`
			} `json:"message"`
			Delta struct {
				Content any `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}

	contentStr := ""
	if err := json.Unmarshal(trimmed, &chatResp); err == nil && len(chatResp.Choices) > 0 {
		for _, ch := range chatResp.Choices {
			if str, ok := ch.Message.Content.(string); ok && str != "" {
				contentStr += str
			} else if str, ok := ch.Delta.Content.(string); ok && str != "" {
				contentStr += str
			}
		}
		if len(strings.TrimSpace(contentStr)) == 0 {
			return true, ReasonEmptyBody
		}
	} else {
		// If raw text or non-standard JSON
		contentStr = string(trimmed)
	}

	clean := strings.TrimSpace(contentStr)
	if len(clean) == 0 {
		return true, ReasonEmptyBody
	}

	// Check 1: repetitive substring loop (e.g. "OK." repeated 8+ times)
	if hasConsecutiveSubstringRepetition(clean, 8) {
		return true, ReasonDegenerateLoop
	}

	// Check 2: line-level repetition (same line repeated 6+ times consecutively)
	lines := strings.Split(clean, "\n")
	consecutiveSame := 1
	var lastLine string
	for _, l := range lines {
		lTrim := strings.TrimSpace(l)
		if len(lTrim) < 3 {
			continue
		}
		if lTrim == lastLine {
			consecutiveSame++
			if consecutiveSame >= 6 {
				return true, ReasonIdenticalPhrases
			}
		} else {
			lastLine = lTrim
			consecutiveSame = 1
		}
	}

	return false, ""
}
