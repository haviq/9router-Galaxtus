package proxy

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"time"
)

type watermarkKey struct{}

// WatermarkConfig holds model response watermarking settings.
type WatermarkConfig struct {
	Enabled  bool
	Text     string
	Position string // "suffix" (default) or "prefix"
}

// WithWatermark attaches watermark configuration to the request context.
func WithWatermark(ctx context.Context, wm WatermarkConfig) context.Context {
	return context.WithValue(ctx, watermarkKey{}, wm)
}

// WatermarkFromContext retrieves the watermark configuration from the request context.
func WatermarkFromContext(ctx context.Context) (WatermarkConfig, bool) {
	if ctx == nil {
		return WatermarkConfig{}, false
	}
	wm, ok := ctx.Value(watermarkKey{}).(WatermarkConfig)
	return wm, ok
}

// ApplyWatermarkToJSON injects watermark text into a non-streaming chat completion JSON response.
// Supports standard OpenAI chat completions (choices[0].message.content),
// legacy completions (choices[0].text), and Anthropic messages (content[0].text).
func ApplyWatermarkToJSON(body []byte, watermarkText string, position string) []byte {
	if len(body) == 0 || watermarkText == "" {
		return body
	}
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return body
	}

	// 1. OpenAI chat.completion format
	if choices, ok := data["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if msg, ok := choice["message"].(map[string]any); ok {
				if content, ok := msg["content"].(string); ok && content != "" {
					if position == "prefix" {
						msg["content"] = watermarkText + content
					} else {
						msg["content"] = content + watermarkText
					}
					if out, err := json.Marshal(data); err == nil {
						return out
					}
				}
			} else if text, ok := choice["text"].(string); ok && text != "" {
				if position == "prefix" {
					choice["text"] = watermarkText + text
				} else {
					choice["text"] = text + watermarkText
				}
				if out, err := json.Marshal(data); err == nil {
					return out
				}
			}
		}
		return body
	}

	// 2. Anthropic Messages format
	if contentArr, ok := data["content"].([]any); ok && len(contentArr) > 0 {
		for _, item := range contentArr {
			if m, ok := item.(map[string]any); ok && m["type"] == "text" {
				if text, ok := m["text"].(string); ok {
					if position == "prefix" {
						m["text"] = watermarkText + text
					} else {
						m["text"] = text + watermarkText
					}
					if out, err := json.Marshal(data); err == nil {
						return out
					}
				}
			}
		}
	}

	return body
}

// BuildWatermarkSSEChunk builds an OpenAI-compatible SSE chunk carrying the watermark text delta.
func BuildWatermarkSSEChunk(text string) []byte {
	payload := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-wm-%d", time.Now().UnixMilli()),
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{
					"content": text,
				},
			},
		},
	}
	b, _ := json.Marshal(payload)
	frame := append([]byte("data: "), b...)
	frame = append(frame, '\n', '\n')
	return frame
}

// BuildClaudeWatermarkSSEChunk builds an Anthropic Claude SSE event carrying the watermark text delta.
func BuildClaudeWatermarkSSEChunk(text string) []byte {
	payload := map[string]any{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]any{
			"type": "text_delta",
			"text": text,
		},
	}
	b, _ := json.Marshal(payload)
	frame := append([]byte("event: content_block_delta\ndata: "), b...)
	frame = append(frame, '\n', '\n')
	return frame
}

