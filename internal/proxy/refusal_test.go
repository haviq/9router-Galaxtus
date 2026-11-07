package proxy

import (
	"testing"
)

func TestMatchesRefusalPhrase(t *testing.T) {
	tests := []struct {
		input       string
		wantMatched bool
	}{
		{"I cannot fulfill this request.", true},
		{"I am unable to fulfill your request.", true},
		{"I'm sorry, but I cannot assist with this.", true},
		{"Sorry! I apologize, but as an AI I must decline.", true},
		{"As an AI language model, I cannot provide exploit code.", true},
		{"This request violates our safety policy.", true},
		{"Sure, here is the explanation of buffer overflows.", false},
		{"Hello! How can I help you today?", false},
		{"I can help you analyze the binary disassembly.", false},
		{"", false},
	}

	for _, tt := range tests {
		_, matched := MatchesRefusalPhrase(tt.input)
		if matched != tt.wantMatched {
			t.Errorf("MatchesRefusalPhrase(%q) = %v, want %v", tt.input, matched, tt.wantMatched)
		}
	}
}

func TestIsPotentialRefusalPrefix(t *testing.T) {
	tests := []struct {
		input       string
		wantPrefix  bool
	}{
		{"I", true},
		{"I cannot", true},
		{"As an AI", true},
		{"Sure, here is", false},
		{"#include <stdio.h>", false},
		{"<think>", false},
	}

	for _, tt := range tests {
		got := IsPotentialRefusalPrefix(tt.input)
		if got != tt.wantPrefix {
			t.Errorf("IsPotentialRefusalPrefix(%q) = %v, want %v", tt.input, got, tt.wantPrefix)
		}
	}
}

func TestDetectRefusalInStream(t *testing.T) {
	// 1. finish_reason: content_filter
	cfData := []byte(`{"id":"chatcmpl-1","choices":[{"delta":{},"finish_reason":"content_filter"}]}`)
	if _, ok := DetectRefusalInStream(cfData); !ok {
		t.Errorf("expected refusal detected for content_filter")
	}

	// 2. delta.refusal
	refData := []byte(`{"id":"chatcmpl-2","choices":[{"delta":{"refusal":"I cannot fulfill this request."}}]}`)
	if _, ok := DetectRefusalInStream(refData); !ok {
		t.Errorf("expected refusal detected for delta.refusal")
	}

	// 3. content phrase refusal
	textData := []byte(`{"id":"chatcmpl-3","choices":[{"delta":{"content":"I apologize, but as an AI I cannot help."}}]}`)
	if _, ok := DetectRefusalInStream(textData); !ok {
		t.Errorf("expected refusal detected for text content")
	}

	// 4. normal response
	normalData := []byte(`{"id":"chatcmpl-4","choices":[{"delta":{"content":"Here is the proof of concept:"}}]}`)
	if _, ok := DetectRefusalInStream(normalData); ok {
		t.Errorf("expected normal response not to be flagged as refusal")
	}

	// 5. tool calls (never refusal)
	toolData := []byte(`{"id":"chatcmpl-5","choices":[{"delta":{"tool_calls":[{"function":{"name":"search"}}]}}]}`)
	if _, ok := DetectRefusalInStream(toolData); ok {
		t.Errorf("tool call must not be flagged as refusal")
	}
}

func TestDetectRefusalInPayload(t *testing.T) {
	body := []byte(`{
		"id": "chatcmpl-1",
		"choices": [{
			"message": {
				"role": "assistant",
				"content": "I am unable to fulfill this request because it violates safety policy."
			},
			"finish_reason": "stop"
		}]
	}`)
	if _, ok := DetectRefusalInPayload(body); !ok {
		t.Errorf("expected non-streaming payload to detect refusal")
	}

	goodBody := []byte(`{
		"id": "chatcmpl-2",
		"choices": [{
			"message": {
				"role": "assistant",
				"content": "To inspect the process memory, use GDB or WinDbg."
			},
			"finish_reason": "stop"
		}]
	}`)
	if _, ok := DetectRefusalInPayload(goodBody); ok {
		t.Errorf("good non-streaming payload should not be flagged as refusal")
	}
}
