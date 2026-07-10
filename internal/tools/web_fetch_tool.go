package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
)

// WebFetchTool 抓取指定 URL 的网页内容，提取并返回可读文本
// 职责：深入阅读 web_search 返回的搜索结果页面
type WebFetchTool struct{}

// NewWebFetchTool 创建网页抓取工具
func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{}
}

func (w *WebFetchTool) Name() string {
	return "web_fetch"
}

func (w *WebFetchTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name: w.Name(),
		Description: "抓取指定 URL 的网页内容，提取正文并以纯文本格式返回。" +
			"用于深入阅读 web_search 返回的搜索结果页面。" +
			"注意：此工具只用于获取具体页面内容，不适合作为搜索工具使用。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"url": map[string]interface{}{
					"type":        "string",
					"description": "要抓取的网页 URL。必须包含协议 (http/https)。",
				},
				"extract_mode": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"text", "markdown"},
					"description": "提取模式：text 纯文本，markdown 保留基础结构。默认 text",
					"default":     "text",
				},
				"max_length": map[string]interface{}{
					"type":        "integer",
					"description": "返回内容的最大字符数，默认 8000，最大 32000",
					"default":     8000,
				},
			},
			"required": []string{"url"},
		},
	}
}

type webFetchArgs struct {
	URL         string `json:"url"`
	ExtractMode string `json:"extract_mode"`
	MaxLength   int    `json:"max_length"`
}

func (w *WebFetchTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	// 1. 反序列化参数
	var input webFetchArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("web_fetch 参数解析失败: %w", err)
	}

	// 2. 参数校验（自愈模式：错误以字符串返回）
	if input.URL == "" {
		return "错误：URL 不能为空，请提供有效的网页地址。", nil
	}

	// 3. 安全校验：协议白名单
	if err := ValidateURL(input.URL); err != nil {
		return fmt.Sprintf("URL 安全校验失败: %v", err), nil
	}

	// 4. 安全校验：SSRF 防护
	parsedURL, _ := url.Parse(input.URL)
	if blocked, reason := IsPrivateHost(parsedURL.Host); blocked {
		return fmt.Sprintf("安全拦截: %s", reason), nil
	}

	// 5. 应用默认值
	if input.ExtractMode != "markdown" {
		input.ExtractMode = "text"
	}
	if input.MaxLength <= 0 {
		input.MaxLength = 8000
	}
	if input.MaxLength > 32000 {
		input.MaxLength = 32000
	}

	// 6. 创建安全的 HTTP 客户端并发起请求
	client := createSecureHTTPClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return fmt.Sprintf("创建 HTTP 请求失败: %v", err), nil
	}
	req.Header.Set("User-Agent", "go-tiny-claw/1.0 (Agent Web Fetcher)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Sprintf("网页抓取失败: %v", err), nil
	}
	defer resp.Body.Close()

	// 7. 读取响应体（限制 5MB）
	limitedReader := io.LimitReader(resp.Body, 5*1024*1024)
	bodyBytes, err := io.ReadAll(limitedReader)
	if err != nil {
		return fmt.Sprintf("读取网页内容失败: %v", err), nil
	}
	bodySize := len(bodyBytes)

	// 8. 非 200 状态码
	if resp.StatusCode != http.StatusOK {
		return fmt.Sprintf("网页抓取失败: HTTP %d %s", resp.StatusCode, resp.Status), nil
	}

	// 9. 根据 Content-Type 提取内容
	contentType := resp.Header.Get("Content-Type")
	var title, content string

	if strings.Contains(contentType, "text/html") || strings.Contains(contentType, "application/xhtml") {
		title, content = extractHTMLContent(string(bodyBytes), input.ExtractMode)
	} else {
		// 纯文本或其他类型：直接返回原文
		content = string(bodyBytes)
		title = parsedURL.Host
	}

	// 10. 内容清洗
	content = SanitizeTextContent(content)

	// 11. 截断
	truncated := false
	if len([]rune(content)) > input.MaxLength {
		runes := []rune(content)
		content = string(runes[:input.MaxLength])
		truncated = true
	}

	// 12. 格式化输出
	return formatFetchResult(input.URL, title, content, bodySize, input.ExtractMode, truncated, resp.StatusCode), nil
}

// createSecureHTTPClient 创建带安全配置的 HTTP 客户端
// 设计文档 8.2 节：超时 30s，最大重定向 3 次
func createSecureHTTPClient() *http.Client {
	redirectCount := 0
	return &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			redirectCount++
			if redirectCount > 3 {
				return fmt.Errorf("超过最大重定向次数 (3 次)")
			}
			// 每次重定向前检查目标是否为内网地址
			if blocked, _ := IsPrivateHost(req.URL.Host); blocked {
				return fmt.Errorf("重定向目标被 SSRF 防护拦截: %s", req.URL.Host)
			}
			return nil
		},
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		},
	}
}

// extractHTMLContent 从 HTML 中提取标题和正文
func extractHTMLContent(htmlContent string, mode string) (title string, content string) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return "", htmlContent // 解析失败，返回原文
	}

	// 提取标题
	title = strings.TrimSpace(doc.Find("title").First().Text())

	// 移除干扰元素
	doc.Find("script, style, noscript, iframe, svg, nav, footer, header, " +
		".sidebar, .nav, .footer, .header, .menu, .advertisement, .ads").Each(
		func(i int, s *goquery.Selection) {
			s.Remove()
		})

	// 提取主体
	body := doc.Find("body")
	if body.Length() == 0 {
		body = doc.Selection // fallback 到整个文档
	}

	if mode == "markdown" {
		content = extractBasicMarkdown(body)
	} else {
		content = extractPlainText(body)
	}

	return title, content
}

// extractPlainText 从 goquery Selection 中提取纯文本
func extractPlainText(sel *goquery.Selection) string {
	text := sel.Text()
	return text
}

// extractBasicMarkdown 从 goquery Selection 中提取基础 Markdown 格式
// 保留标题、链接、列表、引用块等结构
func extractBasicMarkdown(sel *goquery.Selection) string {
	var sb strings.Builder

	sel.Find("h1, h2, h3, h4, h5, h6, p, a, ul, ol, li, blockquote, pre, code, strong, em, br").Each(
		func(i int, s *goquery.Selection) {
			tag := goquery.NodeName(s)
			text := strings.TrimSpace(s.Text())
			if text == "" && tag != "br" {
				return
			}

			switch tag {
			case "h1":
				sb.WriteString("\n\n# " + text + "\n")
			case "h2":
				sb.WriteString("\n\n## " + text + "\n")
			case "h3":
				sb.WriteString("\n\n### " + text + "\n")
			case "h4":
				sb.WriteString("\n\n#### " + text + "\n")
			case "h5":
				sb.WriteString("\n\n##### " + text + "\n")
			case "h6":
				sb.WriteString("\n\n###### " + text + "\n")
			case "p":
				sb.WriteString("\n\n" + text)
			case "a":
				if href, exists := s.Attr("href"); exists {
					sb.WriteString(fmt.Sprintf("[%s](%s)", text, href))
				} else {
					sb.WriteString(text)
				}
			case "strong", "b":
				sb.WriteString(" **" + text + "** ")
			case "em", "i":
				sb.WriteString(" *" + text + "* ")
			case "blockquote":
				for _, line := range strings.Split(text, "\n") {
					sb.WriteString("\n> " + line)
				}
			case "pre", "code":
				sb.WriteString("\n```\n" + text + "\n```\n")
			case "br":
				sb.WriteString("\n")
			}
		})

	result := sb.String()
	if result == "" {
		// fallback: 无结构提取，返回纯文本
		return extractPlainText(sel)
	}
	return result
}

// formatFetchResult 格式化 web_fetch 的输出
func formatFetchResult(urlStr, title, content string, sizeBytes int, mode string, truncated bool, statusCode int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("[web_fetch] %s\n", urlStr))
	if title != "" {
		sb.WriteString(fmt.Sprintf("标题: %s\n", title))
	}
	sb.WriteString(fmt.Sprintf("HTTP %d | 大小: %s | 模式: %s\n",
		statusCode, formatBytes(sizeBytes), mode))
	sb.WriteString(strings.Repeat("-", 40) + "\n")
	sb.WriteString(content)

	if truncated {
		sb.WriteString("\n\n[...内容已被系统截断...]")
	}

	return sb.String()
}
