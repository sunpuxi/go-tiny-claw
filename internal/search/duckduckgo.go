package search

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// DuckDuckGoProvider 基于 DuckDuckGo HTML 搜索页面的搜索后端
// 免费、无需 API Key，通过解析 html.duckduckgo.com 的搜索结果页面获取结果
type DuckDuckGoProvider struct {
	httpClient *http.Client
	baseURL    string
}

// NewDuckDuckGoProvider 创建 DuckDuckGo 搜索后端
func NewDuckDuckGoProvider() *DuckDuckGoProvider {
	return &DuckDuckGoProvider{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		baseURL: "https://html.duckduckgo.com/html/",
	}
}

func (p *DuckDuckGoProvider) Name() string {
	return "duckduckgo"
}

func (p *DuckDuckGoProvider) Search(ctx context.Context, query string, opts SearchOptions) (*SearchResponse, error) {
	start := time.Now()

	// 构造请求
	reqURL := p.baseURL + "?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("构造搜索请求失败: %w", err)
	}

	// 设置必要的请求头，模拟浏览器行为
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "+
		"(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("搜索请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("搜索引擎返回异常状态码: %d", resp.StatusCode)
	}

	// 使用 goquery 解析 HTML
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析搜索结果页面失败: %w", err)
	}

	maxResults := opts.MaxResults
	if maxResults <= 0 || maxResults > 10 {
		maxResults = 5
	}

	results := make([]SearchResult, 0, maxResults)

	// DuckDuckGo HTML 搜索结果的选择器
	// .result 是每个搜索结果的容器
	doc.Find(".result").Each(func(i int, s *goquery.Selection) {
		if len(results) >= maxResults {
			return
		}

		titleEl := s.Find(".result__title")
		snippetEl := s.Find(".result__snippet")
		urlEl := s.Find(".result__url")

		title := strings.TrimSpace(titleEl.Text())
		snippet := strings.TrimSpace(snippetEl.Text())
		resultURL := strings.TrimSpace(urlEl.Text())

		// 尝试从链接中提取完整 URL
		if link, exists := titleEl.Find("a").Attr("href"); exists {
			// DuckDuckGo 返回的链接格式为 //duckduckgo.com/l/?uddg=<encoded_url>
			// 尝试提取真实 URL
			if cleanURL := extractDDGURL(link); cleanURL != "" {
				resultURL = cleanURL
			}
		}

		// 跳过没有标题或 URL 的结果
		if title == "" || resultURL == "" {
			return
		}

		results = append(results, SearchResult{
			Title:   title,
			URL:     resultURL,
			Snippet: snippet,
		})
	})

	// 兼容旧的 CSS 类名（DuckDuckGo 可能会更改页面结构）
	if len(results) == 0 {
		doc.Find(".web-result, .web-result-standard").Each(func(i int, s *goquery.Selection) {
			if len(results) >= maxResults {
				return
			}

			titleEl := s.Find(".result__a, a.result-link")
			snippetEl := s.Find(".result__snippet, .snippet")
			urlEl := s.Find(".result__url, .link-text")

			title := strings.TrimSpace(titleEl.Text())
			snippet := strings.TrimSpace(snippetEl.Text())
			resultURL := strings.TrimSpace(urlEl.Text())

			if link, exists := titleEl.Attr("href"); exists {
				if cleanURL := extractDDGURL(link); cleanURL != "" {
					resultURL = cleanURL
				}
			}

			if title == "" || resultURL == "" {
				return
			}

			results = append(results, SearchResult{
				Title:   title,
				URL:     resultURL,
				Snippet: snippet,
			})
		})
	}

	return &SearchResponse{
		Query:      query,
		Results:    results,
		EngineName: p.Name(),
		CostMs:     time.Since(start).Milliseconds(),
	}, nil
}

// extractDDGURL 从 DuckDuckGo 的重定向链接中提取真实 URL
// DuckDuckGo 搜索结果中的链接格式为：//duckduckgo.com/l/?uddg=<encoded_url>&rut=...
func extractDDGURL(raw string) string {
	// 处理形如 //duckduckgo.com/l/?uddg=xxx 的链接
	if strings.Contains(raw, "uddg=") {
		// 解析查询参数
		if u, err := url.Parse(raw); err == nil {
			// 可能有 host 也可能没有（以 // 开头）
			q := u.Query()
			if uddg := q.Get("uddg"); uddg != "" {
				decoded, err := url.QueryUnescape(uddg)
				if err != nil {
					return uddg
				}
				return decoded
			}
		}
	}

	// 如果已经是完整的 http/https URL，直接返回
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}

	return raw
}
