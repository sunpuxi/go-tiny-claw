package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// allowPrivateIPForTest 临时设置 ALLOW_PRIVATE_IP，返回恢复函数
// httptest 服务器默认绑定 127.0.0.1，测试需要绕过 SSRF 检查
func allowPrivateIPForTest() func() {
	os.Setenv("ALLOW_PRIVATE_IP", "true")
	return func() {
		os.Unsetenv("ALLOW_PRIVATE_IP")
	}
}

func TestWebFetchTool_Name(t *testing.T) {
	tool := NewWebFetchTool()
	if tool.Name() != "web_fetch" {
		t.Errorf("expected 'web_fetch', got '%s'", tool.Name())
	}
}

func TestWebFetchTool_Definition(t *testing.T) {
	tool := NewWebFetchTool()
	def := tool.Definition()

	if def.Name != "web_fetch" {
		t.Errorf("expected name 'web_fetch', got '%s'", def.Name)
	}
	if def.Description == "" {
		t.Error("expected non-empty description")
	}

	schema, ok := def.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatal("expected InputSchema to be map[string]interface{}")
	}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatal("expected required to be []string")
	}
	hasURL := false
	for _, r := range required {
		if r == "url" {
			hasURL = true
			break
		}
	}
	if !hasURL {
		t.Error("expected 'url' to be required")
	}
}

func TestWebFetchTool_Execute_Success(t *testing.T) {
	defer allowPrivateIPForTest()()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html>
<html><head><title>Test Page</title></head>
<body>
  <h1>Hello World</h1>
  <p>This is a test paragraph with some content.</p>
  <p>Another paragraph here.</p>
</body></html>`))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	args, _ := json.Marshal(map[string]interface{}{
		"url":          server.URL + "/page",
		"extract_mode": "text",
		"max_length":   8000,
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "[web_fetch]") {
		t.Errorf("expected output to contain '[web_fetch]', got: %s", output)
	}
	if !strings.Contains(output, "Test Page") {
		t.Errorf("expected output to contain page title, got: %s", output)
	}
	if !strings.Contains(output, "Hello World") {
		t.Errorf("expected output to contain 'Hello World', got: %s", output)
	}
}

func TestWebFetchTool_Execute_SSRFBlocked(t *testing.T) {
	// 注意：不设置 ALLOW_PRIVATE_IP，验证 SSRF 拦截生效
	tool := NewWebFetchTool()

	args, _ := json.Marshal(map[string]interface{}{
		"url": "http://127.0.0.1/admin",
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !strings.Contains(output, "安全拦截") && !strings.Contains(output, "内网") {
		t.Errorf("expected SSRF block message, got: %s", output)
	}
}

func TestWebFetchTool_Execute_InvalidProtocol(t *testing.T) {
	tool := NewWebFetchTool()

	args, _ := json.Marshal(map[string]interface{}{
		"url": "file:///etc/passwd",
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !strings.Contains(output, "URL 安全校验失败") && !strings.Contains(output, "不支持的协议") {
		t.Errorf("expected protocol block message, got: %s", output)
	}
}

func TestWebFetchTool_Execute_HTTPError(t *testing.T) {
	defer allowPrivateIPForTest()()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	args, _ := json.Marshal(map[string]interface{}{
		"url": server.URL + "/nonexistent",
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !strings.Contains(output, "HTTP 404") {
		t.Errorf("expected HTTP 404 in output, got: %s", output)
	}
}

func TestWebFetchTool_Execute_EmptyURL(t *testing.T) {
	tool := NewWebFetchTool()
	args, _ := json.Marshal(map[string]interface{}{
		"url": "",
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !strings.Contains(output, "错误") && !strings.Contains(output, "不能为空") {
		t.Errorf("expected empty URL error, got: %s", output)
	}
}

func TestWebFetchTool_Execute_InvalidJSON(t *testing.T) {
	tool := NewWebFetchTool()
	_, err := tool.Execute(context.Background(), []byte("not json"))
	if err == nil {
		t.Fatal("expected Go error for invalid JSON")
	}
}

func TestWebFetchTool_Execute_Truncation(t *testing.T) {
	defer allowPrivateIPForTest()()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(strings.Repeat("A", 5000)))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	args, _ := json.Marshal(map[string]interface{}{
		"url":        server.URL + "/big",
		"max_length": 100,
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 检查截断提示
	if !strings.Contains(output, "已被系统截断") {
		t.Errorf("expected truncation notice, got: %s", output)
	}

	// 切割掉 header，检查正文长度不超过 max_length
	sep := strings.Repeat("-", 40)
	idx := strings.Index(output, sep)
	if idx < 0 {
		t.Fatalf("expected content separator, got: %s", output)
	}
	content := output[idx+len(sep):]
	if len([]rune(content)) > 200 {
		t.Errorf("expected content to be truncated to ~100 chars, got %d chars", len([]rune(content)))
	}
}

func TestWebFetchTool_Execute_RedirectFollow(t *testing.T) {
	defer allowPrivateIPForTest()()

	// 目标服务器
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><head><title>Redirected Page</title></head><body><p>You made it!</p></body></html>`))
	}))
	defer targetServer.Close()

	// 重定向服务器
	redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, targetServer.URL+"/final", http.StatusMovedPermanently)
	}))
	defer redirectServer.Close()

	tool := NewWebFetchTool()
	args, _ := json.Marshal(map[string]interface{}{
		"url": redirectServer.URL + "/start",
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "Redirected Page") {
		t.Errorf("expected redirected page content, got: %s", output)
	}
}

func TestWebFetchTool_Execute_PlainText(t *testing.T) {
	defer allowPrivateIPForTest()()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("This is plain text content."))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	args, _ := json.Marshal(map[string]interface{}{
		"url": server.URL + "/text",
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "This is plain text content") {
		t.Errorf("expected plain text content in output, got: %s", output)
	}
}

func TestWebFetchTool_Execute_HTMLScriptStripped(t *testing.T) {
	defer allowPrivateIPForTest()()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html>
<html><head><title>Script Test</title></head>
<body>
  <p>Visible text</p>
  <script>alert('should be removed');</script>
  <style>.hidden { display: none; }</style>
  <p>More visible text</p>
</body></html>`))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	args, _ := json.Marshal(map[string]interface{}{
		"url": server.URL + "/script",
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(output, "should be removed") {
		t.Error("expected script content to be stripped")
	}
	if strings.Contains(output, ".hidden") {
		t.Error("expected style content to be stripped")
	}
	if !strings.Contains(output, "Visible text") {
		t.Error("expected visible text to remain")
	}
	if !strings.Contains(output, "More visible text") {
		t.Error("expected 'More visible text' to remain")
	}
}

func TestWebFetchTool_Execute_MarkdownMode(t *testing.T) {
	defer allowPrivateIPForTest()()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html>
<html><head><title>MD Test</title></head>
<body>
  <h1>Heading One</h1>
  <p>A paragraph with <a href="https://example.com">a link</a>.</p>
  <strong>Bold text</strong>
</body></html>`))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	args, _ := json.Marshal(map[string]interface{}{
		"url":          server.URL + "/md",
		"extract_mode": "markdown",
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "MD Test") {
		t.Errorf("expected title in output, got: %s", output)
	}
	// markdown 模式应该有 # 标题
	if !strings.Contains(output, "# ") && !strings.Contains(output, "Heading One") {
		t.Errorf("expected markdown heading, got: %s", output)
	}
}

func TestWebFetchTool_Execute_DefaultMaxLength(t *testing.T) {
	defer allowPrivateIPForTest()()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("short content"))
	}))
	defer server.Close()

	tool := NewWebFetchTool()
	args, _ := json.Marshal(map[string]interface{}{
		"url": server.URL + "/short",
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "short content") {
		t.Errorf("expected content in output, got: %s", output)
	}
}

func TestWebFetchTool_Execute_FTPProtocol(t *testing.T) {
	tool := NewWebFetchTool()
	args, _ := json.Marshal(map[string]interface{}{
		"url": "ftp://example.com/file",
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !strings.Contains(output, "URL 安全校验失败") && !strings.Contains(output, "协议") {
		t.Errorf("expected protocol error for FTP, got: %s", output)
	}
}

func TestWebFetchTool_Execute_PrivateIP192(t *testing.T) {
	tool := NewWebFetchTool()
	args, _ := json.Marshal(map[string]interface{}{
		"url": "http://192.168.1.1/config",
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !strings.Contains(output, "安全拦截") && !strings.Contains(output, "内网") {
		t.Errorf("expected SSRF block for 192.168.x.x, got: %s", output)
	}
}
