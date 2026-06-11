package helper

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func TestNormalizeClaudeSamplingForModel(t *testing.T) {
	t.Run("opus-4-8 strips sampling params", func(t *testing.T) {
		req := &dto.ClaudeRequest{
			Model:       "claude-opus-4-8",
			Temperature: common.GetPointer[float64](0.7),
			TopP:        common.GetPointer[float64](0.9),
			TopK:        common.GetPointer[int](40),
		}
		NormalizeClaudeSamplingForModel(req)
		if req.Temperature != nil || req.TopP != nil || req.TopK != nil {
			t.Fatalf("sampling params not stripped: %+v", req)
		}
	})

	t.Run("opus-4-8 converts enabled thinking to adaptive", func(t *testing.T) {
		req := &dto.ClaudeRequest{
			Model: "claude-opus-4-8",
			Thinking: &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer[int](2048),
			},
		}
		NormalizeClaudeSamplingForModel(req)
		if req.Thinking == nil || req.Thinking.Type != "adaptive" {
			t.Fatalf("thinking not converted to adaptive: %+v", req.Thinking)
		}
		if req.Thinking.BudgetTokens != nil {
			t.Fatalf("budget_tokens should be dropped")
		}
		if req.Thinking.Display != "summarized" {
			t.Fatalf("display should default to summarized, got %q", req.Thinking.Display)
		}
	})

	t.Run("adaptive thinking untouched", func(t *testing.T) {
		req := &dto.ClaudeRequest{
			Model:    "claude-opus-4-7",
			Thinking: &dto.Thinking{Type: "adaptive", Display: "omitted"},
		}
		NormalizeClaudeSamplingForModel(req)
		if req.Thinking.Type != "adaptive" || req.Thinking.Display != "omitted" {
			t.Fatalf("adaptive thinking should pass through: %+v", req.Thinking)
		}
	})

	t.Run("older models untouched", func(t *testing.T) {
		req := &dto.ClaudeRequest{
			Model:       "claude-opus-4-6",
			Temperature: common.GetPointer[float64](0.7),
			Thinking: &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer[int](2048),
			},
		}
		NormalizeClaudeSamplingForModel(req)
		if req.Temperature == nil || req.Thinking.Type != "enabled" {
			t.Fatalf("opus-4-6 request should not be modified: %+v", req)
		}
	})

	t.Run("nil request is a no-op", func(t *testing.T) {
		NormalizeClaudeSamplingForModel(nil)
	})
}
