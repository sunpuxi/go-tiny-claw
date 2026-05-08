package provider

import (
	"context"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
)

type LLMProvider interface {

	// Generate 提供LLM的基础能力
	Generate(ctx context.Context, message []schema.Message, tool []schema.ToolDefinition) (*schema.Message, error)
}
