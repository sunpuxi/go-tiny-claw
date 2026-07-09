package tools

import (
	"context"
	"encoding/json"

	"github.com/sunpuxi/go-tiny-claw/internal/schema"
)

// WebSearchTool 搜索信息的工具
type WebSearchTool struct {
    workDir string
}

func NewWebSearchTool(workDir string) *WebSearchTool {
    return &WebSearchTool{workDir:workDir}
}

func (w *WebSearchTool) Name() string {
    return "web_search_tool"
}

func (w *WebSearchTool) Definition() schema.ToolDefinition {
    return schema.ToolDefinition{}
}

func (w *WebSearchTool) Execute(ctx context.Context,arg json.RawMessage) (string,error) {
    return "",nil
}
