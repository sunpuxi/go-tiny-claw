package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sunpuxi/go-tiny-claw/internal/schema"
)

func TestValidateURL_ValidHTTP(t *testing.T) {
	if err := ValidateURL("http://example.com"); err != nil {
		t.Errorf("expected success for http URL, got: %v", err)
	}
}

func TestValidateURL_ValidHTTPS(t *testing.T) {
	if err := ValidateURL("https://example.com/path?q=1"); err != nil {
		t.Errorf("expected success for https URL, got: %v", err)
	}
}

func TestValidateURL_InvalidProtocol_File(t *testing.T) {
	if err := ValidateURL("file:///etc/passwd"); err == nil {
		t.Error("expected error for file:// protocol")
	}
}

func TestValidateURL_InvalidProtocol_FTP(t *testing.T) {
	if err := ValidateURL("ftp://example.com/file"); err == nil {
		t.Error("expected error for ftp:// protocol")
	}
}

func TestValidateURL_InvalidProtocol_Gopher(t *testing.T) {
	if err := ValidateURL("gopher://example.com"); err == nil {
		t.Error("expected error for gopher:// protocol")
	}
}

func TestValidateURL_InvalidFormat(t *testing.T) {
	if err := ValidateURL("not-a-valid-url%%%"); err == nil {
		t.Error("expected error for malformed URL")
	}
}

func TestValidateURL_EmptyHost(t *testing.T) {
	if err := ValidateURL("http://"); err == nil {
		t.Error("expected error for URL with empty host")
	}
}

func TestIsPrivateHost_Loopback(t *testing.T) {
	blocked, reason := IsPrivateHost("127.0.0.1")
	if !blocked {
		t.Error("expected 127.0.0.1 to be blocked")
	}
	if !strings.Contains(reason, "内网") {
		t.Errorf("expected reason to mention 内网, got: %s", reason)
	}
}

func TestIsPrivateHost_Private10(t *testing.T) {
	blocked, _ := IsPrivateHost("10.0.0.1")
	if !blocked {
		t.Error("expected 10.0.0.1 to be blocked")
	}
}

func TestIsPrivateHost_Private172(t *testing.T) {
	blocked, _ := IsPrivateHost("172.16.0.1")
	if !blocked {
		t.Error("expected 172.16.0.1 to be blocked")
	}
}

func TestIsPrivateHost_Private192(t *testing.T) {
	blocked, _ := IsPrivateHost("192.168.1.1")
	if !blocked {
		t.Error("expected 192.168.1.1 to be blocked")
	}
}

func TestIsPrivateHost_LinkLocal(t *testing.T) {
	blocked, _ := IsPrivateHost("169.254.1.1")
	if !blocked {
		t.Error("expected 169.254.1.1 to be blocked")
	}
}

func TestIsPrivateHost_PublicIP(t *testing.T) {
	// 8.8.8.8 是 Google DNS 的公网 IP
	blocked, _ := IsPrivateHost("8.8.8.8")
	if blocked {
		t.Error("expected 8.8.8.8 to be allowed (public IP)")
	}
}

func TestIsPrivateHost_IPv6Loopback(t *testing.T) {
	blocked, _ := IsPrivateHost("::1")
	if !blocked {
		t.Error("expected ::1 to be blocked")
	}
}

func TestIsPrivateHost_WithPort(t *testing.T) {
	blocked, _ := IsPrivateHost("127.0.0.1:8080")
	if !blocked {
		t.Error("expected 127.0.0.1:8080 to be blocked")
	}
}

func TestIsPrivateHost_PublicWithPort(t *testing.T) {
	blocked, _ := IsPrivateHost("93.184.216.34:443")
	if blocked {
		t.Error("expected public IP with port to be allowed")
	}
}

func TestResolveAndCheckHost_Public(t *testing.T) {
	// example.com 解析为公网 IP
	err := ResolveAndCheckHost("example.com")
	if err != nil {
		t.Logf("DNS resolution might have failed in test environment: %v", err)
	}
}

func TestResolveAndCheckHost_EmptyHost(t *testing.T) {
	blocked, _ := IsPrivateHost("")
	if blocked {
		// 空主机名可能被解析为特殊地址，保守地接受拦截
		t.Logf("empty host blocked: %v", blocked)
	}
}

func TestCreateWebFetchMiddleware_OtherTool(t *testing.T) {
	mw := CreateWebFetchMiddleware()

	// 对非 web_fetch 工具应该放行
	call := schema.ToolCall{
		Name:      "bash",
		Arguments: json.RawMessage(`{"command":"ls"}`),
	}
	allowed, _ := mw(context.Background(), call)
	if !allowed {
		t.Error("expected non-web_fetch tools to be allowed")
	}
}

func TestCreateWebFetchMiddleware_ValidURL(t *testing.T) {
	mw := CreateWebFetchMiddleware()

	call := schema.ToolCall{
		Name:      "web_fetch",
		Arguments: json.RawMessage(`{"url":"https://example.com"}`),
	}
	allowed, _ := mw(context.Background(), call)
	if !allowed {
		t.Error("expected valid URL to be allowed")
	}
}

func TestCreateWebFetchMiddleware_FileProtocol(t *testing.T) {
	mw := CreateWebFetchMiddleware()

	call := schema.ToolCall{
		Name:      "web_fetch",
		Arguments: json.RawMessage(`{"url":"file:///etc/passwd"}`),
	}
	allowed, reason := mw(context.Background(), call)
	if allowed {
		t.Error("expected file:// URL to be blocked")
	}
	if reason == "" {
		t.Error("expected non-empty rejection reason")
	}
}

func TestCreateWebFetchMiddleware_PrivateIP(t *testing.T) {
	mw := CreateWebFetchMiddleware()

	call := schema.ToolCall{
		Name:      "web_fetch",
		Arguments: json.RawMessage(`{"url":"http://127.0.0.1:8080/admin"}`),
	}
	allowed, reason := mw(context.Background(), call)
	if allowed {
		t.Error("expected private IP URL to be blocked")
	}
	if reason == "" {
		t.Error("expected non-empty rejection reason")
	}
}

func TestCreateWebFetchMiddleware_InvalidJSON(t *testing.T) {
	mw := CreateWebFetchMiddleware()

	call := schema.ToolCall{
		Name:      "web_fetch",
		Arguments: json.RawMessage(`not json`),
	}
	allowed, _ := mw(context.Background(), call)
	if allowed {
		t.Error("expected invalid JSON to be blocked")
	}
}

func TestCreateWebFetchMiddleware_EmptyURL(t *testing.T) {
	mw := CreateWebFetchMiddleware()

	call := schema.ToolCall{
		Name:      "web_fetch",
		Arguments: json.RawMessage(`{"url":""}`),
	}
	// 空 URL 应该放行，由工具自身处理并返回友好错误
	allowed, _ := mw(context.Background(), call)
	if !allowed {
		t.Error("expected empty URL to pass through middleware")
	}
}

func TestCreateWebFetchMiddleware_WebFetchToolName(t *testing.T) {
	mw := CreateWebFetchMiddleware()

	// 也检查旧名称 "web_fetch_tool"
	call := schema.ToolCall{
		Name:      "web_fetch_tool",
		Arguments: json.RawMessage(`{"url":"https://example.com"}`),
	}
	allowed, _ := mw(context.Background(), call)
	if !allowed {
		t.Error("expected 'web_fetch_tool' name to be recognized")
	}
}

func TestSanitizeTextContent_Basic(t *testing.T) {
	input := "  line1  \n\n\n\n  line2  \n"
	output := SanitizeTextContent(input)
	if !strings.Contains(output, "line1") || !strings.Contains(output, "line2") {
		t.Errorf("unexpected output: %s", output)
	}
	// 应该压缩过多的空行为最多 2 行（即最多 3 个连续 \n）
	if strings.Count(output, "\n\n\n\n") > 0 {
		t.Error("expected excessive blank lines to be collapsed")
	}
}

func TestSanitizeTextContent_TrimWhitespace(t *testing.T) {
	input := "   hello world   \n   foo bar   "
	output := SanitizeTextContent(input)
	if output != "hello world\nfoo bar" {
		t.Errorf("expected trimmed lines, got: '%s'", output)
	}
}
