package tools

import (
	"context"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
)

type ToolRegistry interface {

	// GetAvailableTools 获取可用的工具列表
	GetAvailableTools(ctx context.Context) []schema.ToolDefinition

	// ExecuteToolCall 执行工具调用
	ExecuteToolCall(ctx context.Context, tool schema.ToolCall) schema.ToolResult
}
