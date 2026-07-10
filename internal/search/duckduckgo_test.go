package search

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestDuckDuckGoProvider 创建指向测试服务器的 DuckDuckGo 后端
func newTestDuckDuckGoProvider(server *httptest.Server) *DuckDuckGoProvider {
	return &DuckDuckGoProvider{
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		baseURL: server.URL + "/html/",
	}
}

func TestDuckDuckGoSearch_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证查询参数
		if q := r.URL.Query().Get("q"); q != "test query" && q != "test+query" {
			t.Logf("unexpected query: %s", q)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html>
<html><body>
<div class="result">
  <h2 class="result__title"><a href="https://example.com/page1">Test Result One</a></h2>
  <div class="result__url">example.com/page1</div>
  <div class="result__snippet">This is the first test snippet.</div>
</div>
<div class="result">
  <h2 class="result__title"><a href="https://example.com/page2">Test Result Two</a></h2>
  <div class="result__url">example.com/page2</div>
  <div class="result__snippet">This is the second test snippet.</div>
</div>
</body></html>`))
	}))
	defer server.Close()

	provider := newTestDuckDuckGoProvider(server)
	resp, err := provider.Search(context.Background(), "test query", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.EngineName != "duckduckgo" {
		t.Errorf("expected engine 'duckduckgo', got '%s'", resp.EngineName)
	}
	if resp.Query != "test query" {
		t.Errorf("expected query 'test query', got '%s'", resp.Query)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}

	r1 := resp.Results[0]
	if r1.Title != "Test Result One" {
		t.Errorf("expected title 'Test Result One', got '%s'", r1.Title)
	}
	if r1.URL != "https://example.com/page1" {
		t.Errorf("expected URL 'https://example.com/page1', got '%s'", r1.URL)
	}
	if r1.Snippet != "This is the first test snippet." {
		t.Errorf("expected snippet 'This is the first test snippet.', got '%s'", r1.Snippet)
	}
}

func TestDuckDuckGoSearch_NoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html>
<html><body>
<div class="no-results">No results found.</div>
</body></html>`))
	}))
	defer server.Close()

	provider := newTestDuckDuckGoProvider(server)
	resp, err := provider.Search(context.Background(), "noresults", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(resp.Results))
	}
}

func TestDuckDuckGoSearch_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	provider := newTestDuckDuckGoProvider(server)
	_, err := provider.Search(context.Background(), "error query", SearchOptions{MaxResults: 5})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestDuckDuckGoSearch_MaxResultsLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		// 返回 5 个结果
		html := `<!DOCTYPE html><html><body>`
		for i := 0; i < 5; i++ {
			html += `<div class="result">
  <h2 class="result__title"><a href="https://example.com/p` + string(rune('0'+i)) + `">Result ` + string(rune('0'+i)) + `</a></h2>
  <div class="result__snippet">Snippet ` + string(rune('0'+i)) + `</div>
</div>`
		}
		html += `</body></html>`
		w.Write([]byte(html))
	}))
	defer server.Close()

	provider := newTestDuckDuckGoProvider(server)
	// 限制为 2 条
	resp, err := provider.Search(context.Background(), "test", SearchOptions{MaxResults: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Results) != 2 {
		t.Errorf("expected 2 results (max limited), got %d", len(resp.Results))
	}
}

func TestDuckDuckGoSearch_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 故意延迟
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := &DuckDuckGoProvider{
		httpClient: &http.Client{
			Timeout: 50 * time.Millisecond,
		},
		baseURL: server.URL + "/html/",
	}

	_, err := provider.Search(context.Background(), "timeout", SearchOptions{MaxResults: 5})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestDuckDuckGoSearch_ChineseQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if query == "" {
			t.Error("expected non-empty query parameter")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html><html><body>
<div class="result">
  <h2 class="result__title"><a href="https://example.com/zh">中文结果</a></h2>
  <div class="result__snippet">这是一个中文搜索结果的摘要。</div>
</div>
</body></html>`))
	}))
	defer server.Close()

	provider := newTestDuckDuckGoProvider(server)
	resp, err := provider.Search(context.Background(), "中文测试", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Title != "中文结果" {
		t.Errorf("expected title '中文结果', got '%s'", resp.Results[0].Title)
	}
	if resp.Results[0].Snippet != "这是一个中文搜索结果的摘要。" {
		t.Errorf("expected Chinese snippet, got '%s'", resp.Results[0].Snippet)
	}
}

func TestDuckDuckGoSearch_CostMs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html><html><body></body></html>`))
	}))
	defer server.Close()

	provider := newTestDuckDuckGoProvider(server)
	resp, err := provider.Search(context.Background(), "test", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.CostMs < 0 {
		t.Errorf("expected non-negative CostMs, got %d", resp.CostMs)
	}
}

func TestDuckDuckGoSearch_DefaultMaxResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<!DOCTYPE html><html><body></body></html>`))
	}))
	defer server.Close()

	// MaxResults <= 0 时使用默认值 5（在 Search 方法内处理）
	provider := newTestDuckDuckGoProvider(server)
	resp, err := provider.Search(context.Background(), "test", SearchOptions{MaxResults: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 即使后端实现用默认值，也不应报错
	if resp.Query != "test" {
		t.Errorf("expected query 'test', got '%s'", resp.Query)
	}
}

func TestExtractDDGURL_WithUddg(t *testing.T) {
	raw := "//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fpage"
	result := extractDDGURL(raw)
	if result != "https://example.com/page" {
		t.Errorf("expected 'https://example.com/page', got '%s'", result)
	}
}

func TestExtractDDGURL_DirectHTTP(t *testing.T) {
	raw := "https://example.com/direct"
	result := extractDDGURL(raw)
	if result != "https://example.com/direct" {
		t.Errorf("expected 'https://example.com/direct', got '%s'", result)
	}
}

func TestExtractDDGURL_NoMatch(t *testing.T) {
	raw := "/some/relative/path"
	result := extractDDGURL(raw)
	if result != "/some/relative/path" {
		t.Errorf("expected unchanged, got '%s'", result)
	}
}
