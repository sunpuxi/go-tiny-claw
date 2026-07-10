package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/sunpuxi/go-tiny-claw/internal/search"
)

// mockSearchProvider 用于测试的搜索后端 mock
type mockSearchProvider struct {
	name    string
	results []search.SearchResult
	err     error
}

func (m *mockSearchProvider) Name() string {
	if m.name == "" {
		return "mock"
	}
	return m.name
}

func (m *mockSearchProvider) Search(ctx context.Context, query string, opts search.SearchOptions) (*search.SearchResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &search.SearchResponse{
		Query:      query,
		Results:    m.results,
		EngineName: m.Name(),
		CostMs:     42,
	}, nil
}

func TestWebSearchTool_Name(t *testing.T) {
	tool := NewWebSearchTool(&mockSearchProvider{})
	if tool.Name() != "web_search" {
		t.Errorf("expected 'web_search', got '%s'", tool.Name())
	}
}

func TestWebSearchTool_Definition(t *testing.T) {
	tool := NewWebSearchTool(&mockSearchProvider{})
	def := tool.Definition()

	if def.Name != "web_search" {
		t.Errorf("expected name 'web_search', got '%s'", def.Name)
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
	hasQuery := false
	for _, r := range required {
		if r == "query" {
			hasQuery = true
			break
		}
	}
	if !hasQuery {
		t.Error("expected 'query' to be required")
	}
}

func TestWebSearchTool_Execute_Success(t *testing.T) {
	provider := &mockSearchProvider{
		results: []search.SearchResult{
			{Title: "Result One", URL: "https://example.com/1", Snippet: "First snippet."},
			{Title: "Result Two", URL: "https://example.com/2", Snippet: "Second snippet."},
		},
	}
	tool := NewWebSearchTool(provider)

	args, _ := json.Marshal(map[string]interface{}{
		"query":       "test query",
		"max_results": 3,
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "Result One") {
		t.Error("expected output to contain 'Result One'")
	}
	if !strings.Contains(output, "https://example.com/1") {
		t.Error("expected output to contain URL")
	}
	if !strings.Contains(output, "mock") {
		t.Error("expected output to contain engine name")
	}
	if !strings.Contains(output, "42ms") {
		t.Error("expected output to contain cost")
	}
}

func TestWebSearchTool_Execute_NoResults(t *testing.T) {
	provider := &mockSearchProvider{
		results: []search.SearchResult{},
	}
	tool := NewWebSearchTool(provider)

	args, _ := json.Marshal(map[string]interface{}{
		"query": "no results query",
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "未找到相关结果") {
		t.Errorf("expected '未找到相关结果' in output, got: %s", output)
	}
}

func TestWebSearchTool_Execute_EmptyQuery(t *testing.T) {
	provider := &mockSearchProvider{}
	tool := NewWebSearchTool(provider)

	args, _ := json.Marshal(map[string]interface{}{
		"query": "",
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !strings.Contains(output, "错误") && !strings.Contains(output, "不能为空") {
		t.Errorf("expected error message for empty query, got: %s", output)
	}
}

func TestWebSearchTool_Execute_ProviderError(t *testing.T) {
	provider := &mockSearchProvider{
		err: fmt.Errorf("search backend timeout"),
	}
	tool := NewWebSearchTool(provider)

	args, _ := json.Marshal(map[string]interface{}{
		"query": "test",
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("expected self-healing mode (nil error), got: %v", err)
	}
	if !strings.Contains(output, "搜索执行失败") {
		t.Errorf("expected '搜索执行失败' in output, got: %s", output)
	}
}

func TestWebSearchTool_Execute_InvalidJSON(t *testing.T) {
	provider := &mockSearchProvider{}
	tool := NewWebSearchTool(provider)

	_, err := tool.Execute(context.Background(), []byte("not valid json"))
	if err == nil {
		t.Fatal("expected Go error for invalid JSON")
	}
}

func TestWebSearchTool_Execute_DefaultMaxResults(t *testing.T) {
	provider := &mockSearchProvider{
		results: []search.SearchResult{
			{Title: "T1", URL: "https://a.com", Snippet: "S1"},
		},
	}
	tool := NewWebSearchTool(provider)

	// 不传 max_results
	args, _ := json.Marshal(map[string]interface{}{
		"query": "test",
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "T1") {
		t.Error("expected result in output")
	}
}

func TestWebSearchTool_Execute_MaxResultsClamped(t *testing.T) {
	provider := &mockSearchProvider{
		results: []search.SearchResult{
			{Title: "T1", URL: "https://a.com", Snippet: "S1"},
		},
	}
	tool := NewWebSearchTool(provider)

	// max_results 超过 10
	args, _ := json.Marshal(map[string]interface{}{
		"query":       "test",
		"max_results": 100,
	})

	output, err := tool.Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(output, "T1") {
		t.Error("expected result despite invalid max_results")
	}
}

func TestFormatSearchResults_Empty(t *testing.T) {
	resp := &search.SearchResponse{
		Query:      "nothing",
		Results:    []search.SearchResult{},
		EngineName: "test",
		CostMs:     10,
	}
	output := formatSearchResults(resp)
	if !strings.Contains(output, "未找到相关结果") {
		t.Errorf("expected '未找到相关结果', got: %s", output)
	}
}

func TestFormatSearchResults_LongSnippet(t *testing.T) {
	longSnippet := strings.Repeat("A", 500)
	resp := &search.SearchResponse{
		Query: "test",
		Results: []search.SearchResult{
			{Title: "T", URL: "https://x.com", Snippet: longSnippet},
		},
		EngineName: "test",
		CostMs:     10,
	}
	output := formatSearchResults(resp)
	if !strings.Contains(output, "...") && len(longSnippet) > 300 {
		t.Error("expected long snippet to be truncated with '...'")
	}
}
