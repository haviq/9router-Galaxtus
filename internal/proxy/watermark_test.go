package proxy

import (
	json "encoding/json/v2"
	"strings"
	"testing"
)

func TestApplyWatermarkToJSON_OpenAISuffix(t *testing.T) {
	input := []byte(`{"id":"123","choices":[{"index":0,"message":{"role":"assistant","content":"Hello world"},"finish_reason":"stop"}]}`)
	wm := "\n\n⚡ Served by Galactus AI"
	out := ApplyWatermarkToJSON(input, wm, "suffix")

	var data map[string]any
	if err := json.Unmarshal(out, &data); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	choices := data["choices"].([]any)
	choice := choices[0].(map[string]any)
	msg := choice["message"].(map[string]any)
	content := msg["content"].(string)

	expected := "Hello world\n\n⚡ Served by Galactus AI"
	if content != expected {
		t.Fatalf("expected content %q, got %q", expected, content)
	}
}

func TestApplyWatermarkToJSON_OpenAIPrefix(t *testing.T) {
	input := []byte(`{"id":"123","choices":[{"index":0,"message":{"role":"assistant","content":"Hello world"},"finish_reason":"stop"}]}`)
	wm := "[GALACTUS] "
	out := ApplyWatermarkToJSON(input, wm, "prefix")

	var data map[string]any
	if err := json.Unmarshal(out, &data); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	choices := data["choices"].([]any)
	choice := choices[0].(map[string]any)
	msg := choice["message"].(map[string]any)
	content := msg["content"].(string)

	expected := "[GALACTUS] Hello world"
	if content != expected {
		t.Fatalf("expected content %q, got %q", expected, content)
	}
}

func TestApplyWatermarkToJSON_ClaudeFormat(t *testing.T) {
	input := []byte(`{"id":"msg_123","type":"message","role":"assistant","content":[{"type":"text","text":"Claude response"}]}`)
	wm := "\n\n⚡ Galactus"
	out := ApplyWatermarkToJSON(input, wm, "suffix")

	var data map[string]any
	if err := json.Unmarshal(out, &data); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	contentArr := data["content"].([]any)
	block := contentArr[0].(map[string]any)
	text := block["text"].(string)

	expected := "Claude response\n\n⚡ Galactus"
	if text != expected {
		t.Fatalf("expected text %q, got %q", expected, text)
	}
}

func TestBuildWatermarkSSEChunk(t *testing.T) {
	wm := "\n\n⚡ Served by Galactus AI"
	chunk := BuildWatermarkSSEChunk(wm)

	chunkStr := string(chunk)
	if !strings.HasPrefix(chunkStr, "data: ") || !strings.HasSuffix(chunkStr, "\n\n") {
		t.Fatalf("chunk does not follow SSE framing: %q", chunkStr)
	}

	payload := strings.TrimPrefix(chunkStr, "data: ")
	payload = strings.TrimSuffix(payload, "\n\n")

	var data map[string]any
	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		t.Fatalf("invalid SSE payload JSON: %v", err)
	}

	choices := data["choices"].([]any)
	choice := choices[0].(map[string]any)
	delta := choice["delta"].(map[string]any)
	if delta["content"] != wm {
		t.Fatalf("expected delta content %q, got %q", wm, delta["content"])
	}
}

