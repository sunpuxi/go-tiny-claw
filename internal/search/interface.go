// internal/search/interface.go
package search

import (
	"context"
	"time"
)

// SearchResult 单条搜索结果
type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// SearchResponse 搜索响应
type SearchResponse struct {
	Query      string         `json:"query"`
	Results    []SearchResult `json:"results"`
	EngineName string         `json:"engine_name"`
	CostMs     int64          `json:"cost_ms"`
}

// SearchOptions 搜索选项
type SearchOptions struct {
	MaxResults int    `json:"max_results"` // 最大返回数 (1-10)
	TimeRange  string `json:"time_range"`  // day/week/month/year/all
	Language   string `json:"language"`    // 语言偏好 (如 "zh", "en")
}

// SearchProvider 搜索后端接口
type SearchProvider interface {
	// Name 返回搜索引擎名称 (用于日志和结果标识)
	Name() string

	// Search 执行搜索，按相关性排序返回结果
	Search(ctx context.Context, query string, opts SearchOptions) (*SearchResponse, error)
}

// FetchResult 网页抓取结果
type FetchResult struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Content     string `json:"content"`      // 提取后的正文 (markdown/text)
	ContentType string `json:"content_type"` // 原始 Content-Type
	SizeBytes   int64  `json:"size_bytes"`   // 原始下载大小
	StatusCode  int    `json:"status_code"`  // HTTP 状态码
}

// FetchOptions 抓取选项
type FetchOptions struct {
	MaxLength  int    // 返回内容的最大字符数，默认 8000
	TimeoutSec int    // 超时秒数，默认 30
	MaxRedirect int   // 最大重定向次数，默认 3
}

// cachedItem 缓存条目
type cachedItem struct {
	Data      any
	ExpiresAt time.Time
}
