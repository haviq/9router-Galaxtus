package chat

import "9router/proxy/internal/proxy"

// Re-export watermark utilities from internal/proxy
type WatermarkConfig = proxy.WatermarkConfig

var (
	WithWatermark                = proxy.WithWatermark
	WatermarkFromContext         = proxy.WatermarkFromContext
	ApplyWatermarkToJSON         = proxy.ApplyWatermarkToJSON
	BuildWatermarkSSEChunk       = proxy.BuildWatermarkSSEChunk
	BuildClaudeWatermarkSSEChunk = proxy.BuildClaudeWatermarkSSEChunk
)
