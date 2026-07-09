package tools

// WebSearchTool 网络搜索工具（搜索文本信息，如果搜索到url，还需要webFetch工具）
type WebSearchTool struct {
    ID int `json:"id"`
    Name string `json:"name"`
    Arguments json.RawMessage `json:"arguments"`
}
