package tools

// WebFetchTool 查询 URL 中对应信息的工具
type WebFetchTool struct {
    ID int `json:"id"`
    Name string `json:"name"`
    Arguments json.RawMessage `json:"arguments"`
}
