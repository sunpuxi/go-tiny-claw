package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/sunpuxi/go-tiny-claw/internal/schema"
)

// ssrfBlockedCIDRs 定义需要拦截的私有/内网 IP 段
var ssrfBlockedCIDRs = []string{
	"127.0.0.0/8",    // Loopback
	"10.0.0.0/8",     // Private A
	"172.16.0.0/12",  // Private B
	"192.168.0.0/16", // Private C
	"169.254.0.0/16", // Link-local
	"0.0.0.0/8",      // Current network (non-routable)
	"100.64.0.0/10",  // Carrier-grade NAT
	"224.0.0.0/4",    // Multicast
	"240.0.0.0/4",    // Reserved
	"::1/128",        // IPv6 loopback
	"fc00::/7",       // IPv6 unique local
	"fe80::/10",      // IPv6 link-local
}

// blockedNetworks 在 init 中解析的拦截网段列表
var blockedNetworks []*net.IPNet

func init() {
	blockedNetworks = make([]*net.IPNet, 0, len(ssrfBlockedCIDRs))
	for _, cidr := range ssrfBlockedCIDRs {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		blockedNetworks = append(blockedNetworks, block)
	}
}

// ValidateURL 对 URL 做协议白名单检查，仅允许 http/https
func ValidateURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("URL 格式无效: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("不支持的协议 '%s'，仅允许 http/https", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("URL 缺少主机名")
	}
	return nil
}

// IsPrivateHost 检查主机名是否解析到内网 IP
// 返回 (是否被拦截, 拦截原因)
// 可通过设置 ALLOW_PRIVATE_IP=true 环境变量跳过内网 IP 检查（用于开发/测试环境）
func IsPrivateHost(host string) (bool, string) {
	// 检查环境变量开关
	if os.Getenv("ALLOW_PRIVATE_IP") == "true" {
		return false, ""
	}

	// 剥离端口号
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}

	// 先检查是否为 IP 字面量
	if ip := net.ParseIP(hostname); ip != nil {
		for _, blocked := range blockedNetworks {
			if blocked.Contains(ip) {
				return true, fmt.Sprintf("禁止访问内网地址 '%s'", hostname)
			}
		}
		return false, ""
	}

	// DNS 解析
	ips, err := net.LookupIP(hostname)
	if err != nil {
		// DNS 解析失败时，从安全角度考虑，拒绝访问
		return true, fmt.Sprintf("无法解析域名 '%s': %v", hostname, err)
	}

	for _, ip := range ips {
		for _, blocked := range blockedNetworks {
			if blocked.Contains(ip) {
				return true, fmt.Sprintf("禁止访问内网地址 '%s' (解析 IP: %s)", hostname, ip.String())
			}
		}
	}

	return false, ""
}

// ResolveAndCheckHost 解析主机名并检查是否为内网地址
func ResolveAndCheckHost(host string) error {
	blocked, reason := IsPrivateHost(host)
	if blocked {
		return fmt.Errorf("SSRF 防护拦截: %s", reason)
	}
	return nil
}

// CreateWebFetchMiddleware 返回一个 MiddlewareFunc，对 web_fetch 调用做 URL 安全检查
// 这是设计文档 7.1 节中第 3 层防护（中间件层）
func CreateWebFetchMiddleware() MiddlewareFunc {
	return func(ctx context.Context, call schema.ToolCall) (allowed bool, rejectReason string) {
		// 只检查 web_fetch 工具
		if call.Name != "web_fetch" && call.Name != "web_fetch_tool" {
			return true, ""
		}

		// 解析参数中的 URL
		var args struct {
			URL string `json:"url"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return false, fmt.Sprintf("web_fetch 参数解析失败: %v", err)
		}

		if args.URL == "" {
			return true, "" // 空 URL 由工具自身处理并返回友好错误
		}

		// 协议白名单检查
		if err := ValidateURL(args.URL); err != nil {
			return false, err.Error()
		}

		// SSRF 检查
		parsed, _ := url.Parse(args.URL)
		if blocked, reason := IsPrivateHost(parsed.Host); blocked {
			return false, reason
		}

		return true, ""
	}
}

// SanitizeTextContent 对抓取到的文本内容做基础清洗
// 移除过多的空行，压缩空白字符
func SanitizeTextContent(raw string) string {
	lines := strings.Split(raw, "\n")
	var cleanLines []string
	emptyCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			emptyCount++
			if emptyCount <= 2 { // 最多保留连续 2 个空行
				cleanLines = append(cleanLines, "")
			}
		} else {
			emptyCount = 0
			cleanLines = append(cleanLines, trimmed)
		}
	}

	return strings.TrimSpace(strings.Join(cleanLines, "\n"))
}
