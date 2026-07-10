package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sunpuxi/go-tiny-claw/internal/schema"
	"github.com/sunpuxi/go-tiny-claw/internal/search"
)

// WebSearchTool 联网搜索工具，遵循 Search + Fetch 分离模式
// 职责：搜索互联网，返回候选 URL 列表及摘要
// 深入阅读需 LLM 配合 web_fetch_tool 完成
type WebSearchTool struct {
	provider search.SearchProvider
}

// NewWebSearchTool 创建搜索工具，注入可插拔的搜索后端
func NewWebSearchTool(provider search.SearchProvider) *WebSearchTool {
	return &WebSearchTool{provider: provider}
}

func (w *WebSearchTool) Name() string {
	return "web_search"
}

func (w *WebSearchTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name: w.Name(),
		Description: "搜索互联网获取最新信息。返回包含标题、URL 和摘要的搜索结果列表。" +
			"用于查找实时信息、最新文档、新闻事件等。" +
			"注意：此工具只返回搜索结果摘要，如需深入阅读某个页面，请使用 web_fetch 工具。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "搜索关键词。保持简短精准，3-8 个词为佳。对于复杂问题，建议分多次搜索而非一次宽泛搜索。",
				},
				"max_results": map[string]interface{}{
					"type":        "integer",
					"description": "最大返回结果数，默认 5，最大 10",
					"default":     5,
				},
			},
			"required": []string{"query"},
		},
	}
}

type webSearchArgs struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

func (w *WebSearchTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	// 1. 反序列化参数
	var input webSearchArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("web_search 参数解析失败: %w", err)
	}

	// 2. 参数校验与默认值（自愈模式：错误以字符串返回让 LLM 自纠正）
	if input.Query == "" {
		return "错误：搜索关键词不能为空，请提供一个有效的 query 参数。", nil
	}
	if input.MaxResults <= 0 {
		input.MaxResults = 5
	}
	if input.MaxResults > 10 {
		input.MaxResults = 10
	}

	// 3. 调用搜索后端
	resp, err := w.provider.Search(ctx, input.Query, search.SearchOptions{
		MaxResults: input.MaxResults,
	})
	if err != nil {
		return fmt.Sprintf("搜索执行失败: %v\n请稍后重试或尝试使用更简洁的查询词。", err), nil
	}

	// 4. 格式化输出
	return formatSearchResults(resp), nil
}

// formatSearchResults 将搜索响应格式化为 LLM 易读的文本
func formatSearchResults(resp *search.SearchResponse) string {
	if len(resp.Results) == 0 {
		return fmt.Sprintf("搜索 \"%s\" 未找到相关结果。\n建议：尝试使用不同的关键词或更简洁的查询。(引擎: %s, 耗时: %dms)",
			resp.Query, resp.EngineName, resp.CostMs)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("搜索结果 (共 %d 条，引擎: %s，耗时 %dms):\n\n",
		len(resp.Results), resp.EngineName, resp.CostMs))

	for i, r := range resp.Results {
		sb.WriteString(fmt.Sprintf("%d. [%s](%s)\n", i+1, r.Title, r.URL))
		if r.Snippet != "" {
			// 截断过长的摘要
			snippet := r.Snippet
			if len(snippet) > 300 {
				snippet = snippet[:300] + "..."
			}
			sb.WriteString(fmt.Sprintf("   摘要: %s\n", snippet))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
