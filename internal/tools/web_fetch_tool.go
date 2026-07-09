package tools

import(
    "context"
    "github.com/sunpuxi/go-tiny-claw/internal/schema"
)

// WebFetchTool 查询 URL 中对应信息的工具
type WebFetchTool struct {
    workDir string
}

func NewWebFetchTool(workDir string) *WebFetchTool {
    return &WebFetchTool{workDir:workDir}
}

func (w *WebFetchTool) Name() string {
    return "web_fetch_tool"
}

func (w *WebFetchTool) Definition() schema.ToolDefinition {
    return schema.ToolDefinition{}
}

func (w *WebFetchTool) Execute(ctx context,arg json.RawMessage) (string,error) {
    return "",nil
}
