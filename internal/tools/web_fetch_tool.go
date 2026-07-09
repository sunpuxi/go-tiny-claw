package tools

import(
    ""
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

func (w *WebFetchTool) Execute()
