package helper

import (
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

// claudeModelRejectsSampling 判断模型是否已移除采样参数。
// Opus 4.7/4.8 起 temperature/top_p/top_k 一律返回 400
// ("`temperature` is deprecated for this model"),
// 且 thinking.type="enabled"(budget_tokens) 也已移除,仅支持 adaptive。
func claudeModelRejectsSampling(model string) bool {
	return strings.HasPrefix(model, "claude-opus-4-7") ||
		strings.HasPrefix(model, "claude-opus-4-8") ||
		strings.HasPrefix(model, "claude-fable")
}

// NormalizeClaudeSamplingForModel 按上游模型的实际限制清理请求参数:
// 对已移除采样参数的模型剥掉 temperature/top_p/top_k,
// 并把 enabled(budget_tokens) thinking 转为 adaptive,避免 Anthropic 400。
// 对其他模型不做任何修改。
func NormalizeClaudeSamplingForModel(req *dto.ClaudeRequest) {
	if req == nil || !claudeModelRejectsSampling(req.Model) {
		return
	}
	req.Temperature = nil
	req.TopP = nil
	req.TopK = nil
	if req.Thinking != nil && req.Thinking.Type == "enabled" {
		display := req.Thinking.Display
		if display == "" {
			// enabled thinking 在旧模型上默认可见,转 adaptive 后保持可见摘要
			display = "summarized"
		}
		req.Thinking = &dto.Thinking{Type: "adaptive", Display: display}
	}
}
