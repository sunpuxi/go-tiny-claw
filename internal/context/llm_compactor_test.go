package context

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sunpuxi/go-tiny-claw/internal/schema"
	"github.com/tmc/langchaingo/llms/openai"
)

// ======================== EstimateLength 测试 ========================

func TestEstimateLength_Empty(t *testing.T) {
	length := EstimateLength(nil)
	if length != 0 {
		t.Errorf("nil messages should have length 0, got %d", length)
	}

	length = EstimateLength([]schema.Message{})
	if length != 0 {
		t.Errorf("empty messages should have length 0, got %d", length)
	}
}

func TestEstimateLength_ContentOnly(t *testing.T) {
	msgs := []schema.Message{
		{Role: schema.RoleSystem, Content: "Hello"}, // 5 chars
		{Role: schema.RoleUser, Content: "World"},   // 5 chars
	}
	length := EstimateLength(msgs)
	if length != 10 {
		t.Errorf("expected length 10, got %d", length)
	}
}

func TestEstimateLength_WithToolCalls(t *testing.T) {
	msgs := []schema.Message{
		{
			Role:    schema.RoleAssistant,
			Content: "Let me search", // 13 chars
			ToolCalls: []schema.ToolCall{
				{Name: "search", Arguments: json.RawMessage(`{"query":"test"}`)},
			},
		},
	}
	// 13 (content) + 6 (name "search") + 18 (arguments `{"query":"test"}`) = 37
	length := EstimateLength(msgs)
	expected := 13 + len("search") + len(`{"query":"test"}`)
	if length != expected {
		t.Errorf("expected length %d, got %d", expected, length)
	}
}

func TestEstimateLength_MultipleToolCalls(t *testing.T) {
	msgs := []schema.Message{
		{
			Role:    schema.RoleAssistant,
			Content: "calling tools",
			ToolCalls: []schema.ToolCall{
				{Name: "read", Arguments: json.RawMessage(`{}`)},
				{Name: "write", Arguments: json.RawMessage(`{"path":"/tmp"}`)},
			},
		},
	}
	// Content: 13, read+{}: 4+2=6, write+{"path":"/tmp"}: 5+15=20, total: 13+6+20=39
	length := EstimateLength(msgs)
	expected := len("calling tools") + len("read") + len("{}") + len("write") + len(`{"path":"/tmp"}`)
	if length != expected {
		t.Errorf("expected length %d, got %d", expected, length)
	}
}

// ======================== Compact 测试 ========================

func TestLLMCompactor_BelowThreshold_ReturnsOriginal(t *testing.T) {
	l := &LLMCompactor{
		llm:       nil, // 不会被调用
		maxLength: 10000,
	}

	msgs := []schema.Message{
		{Role: schema.RoleSystem, Content: "You are a helpful assistant."},
		{Role: schema.RoleUser, Content: "Hello"},
		{Role: schema.RoleAssistant, Content: "Hi there!"},
	}

	result := l.Compact(msgs)

	if len(result) != len(msgs) {
		t.Fatalf("expected %d messages, got %d", len(msgs), len(result))
	}
	for i := range msgs {
		if result[i].Content != msgs[i].Content {
			t.Errorf("message %d: content changed unexpectedly (%q → %q)",
				i, msgs[i].Content, result[i].Content)
		}
		if result[i].Role != msgs[i].Role {
			t.Errorf("message %d: role changed unexpectedly (%q → %q)",
				i, msgs[i].Role, result[i].Role)
		}
	}
}

func TestLLMCompactor_EmptyMessages(t *testing.T) {
	l := &LLMCompactor{
		llm:       nil,
		maxLength: 100,
	}

	msgs := []schema.Message{}
	result := l.Compact(msgs)

	if len(result) != 0 {
		t.Errorf("expected 0 messages, got %d", len(result))
	}
}

func TestLLMCompactor_ExactlyAtThreshold_TriggersCompression(t *testing.T) {
	// 长度等于阈值时（非严格小于），会触发压缩路径
	content := "Hello World" // 11 chars
	l := &LLMCompactor{
		llm:       nil,
		maxLength: len(content), // threshold = 11, length = 11 → 11 < 11 is false
	}

	msgs := []schema.Message{
		{Role: schema.RoleUser, Content: content},
	}

	// 会进入 LLM 调用路径，llm 为 nil 导致 panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic: threshold equal to length still triggers compression path")
		}
	}()

	l.Compact(msgs)
}

func TestLLMCompactor_SingleCharBelowThreshold(t *testing.T) {
	// 长度刚好超过阈值 1 个字符时，应该触发压缩
	content := "Hello World" // 11 chars
	l := &LLMCompactor{
		llm:       nil,
		maxLength: len(content) - 1, // threshold is 10, content is 11 → above
	}

	msgs := []schema.Message{
		{Role: schema.RoleUser, Content: content},
	}

	// llm is nil → would panic if it tried to call it
	// 我们用 defer recover 来验证确实触发了压缩路径
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic due to nil LLM (threshold exceeded path was entered)")
		}
	}()

	l.Compact(msgs)
}

func TestLLMCompactor_SystemMessagePreserved(t *testing.T) {
	// 系统消息应该被原样保留（如果总长度在阈值内）
	l := &LLMCompactor{
		llm:       nil,
		maxLength: 10000,
	}

	systemContent := "You are a precise and careful assistant."
	msgs := []schema.Message{
		{Role: schema.RoleSystem, Content: systemContent},
		{Role: schema.RoleUser, Content: "Do something."},
	}

	result := l.Compact(msgs)
	if result[0].Role != schema.RoleSystem {
		t.Errorf("system message role changed: %q", result[0].Role)
	}
	if result[0].Content != systemContent {
		t.Errorf("system message content changed")
	}
}

// ======================== 带 mock LLM 的集成测试 ========================

// newMockLLM 创建一个指向测试服务器的 openai.LLM，用于模拟 API 响应
func newMockLLM(t *testing.T, handler http.HandlerFunc) (*openai.LLM, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	llm, err := openai.New(
		openai.WithModel("test-model"),
		openai.WithToken("test-api-key"),
		openai.WithBaseURL(server.URL),
	)
	if err != nil {
		t.Fatalf("failed to create mock LLM: %v", err)
	}
	return llm, server
}

// makeChatCompletionHandler 返回一个返回指定 content 的 OpenAI 兼容 handler
func makeChatCompletionHandler(content string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"id":      "chatcmpl-test",
			"object":  "chat.completion",
			"created": 1677652288,
			"model":   "test-model",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": content,
					},
					"finish_reason": "stop",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func TestLLMCompactor_AboveThreshold_SuccessfulCompression(t *testing.T) {
	compressedMsgs := []schema.Message{
		{Role: schema.RoleSystem, Content: "compressed system prompt"},
		{Role: schema.RoleUser, Content: "compressed user message"},
	}
	compressedJSON, err := json.Marshal(compressedMsgs)
	if err != nil {
		t.Fatal(err)
	}

	llm, _ := newMockLLM(t, makeChatCompletionHandler(string(compressedJSON)))

	// 构造超过阈值的长消息
	longContent := ""
	for i := 0; i < 200; i++ {
		longContent += "this is a very long message that exceeds the threshold. "
	}

	l := &LLMCompactor{
		llm:       llm,
		maxLength: 100, // 阈值很低，确保触发压缩
	}

	msgs := []schema.Message{
		{Role: schema.RoleSystem, Content: "You are a helpful assistant."},
		{Role: schema.RoleUser, Content: longContent},
	}

	result := l.Compact(msgs)

	if len(result) != len(compressedMsgs) {
		t.Fatalf("expected %d compressed messages, got %d", len(compressedMsgs), len(result))
	}
	if result[0].Content != "compressed system prompt" {
		t.Errorf("unexpected content in result[0]: %q", result[0].Content)
	}
	if result[1].Content != "compressed user message" {
		t.Errorf("unexpected content in result[1]: %q", result[1].Content)
	}
}

func TestLLMCompactor_AboveThreshold_UnmarshalFailure_Fallback(t *testing.T) {
	// LLM 返回非法 JSON → json.Unmarshal 失败 → 应返回原始消息
	llm, _ := newMockLLM(t, makeChatCompletionHandler("this is not valid json {{"))

	longContent := ""
	for i := 0; i < 200; i++ {
		longContent += "padding padding padding. "
	}

	l := &LLMCompactor{
		llm:       llm,
		maxLength: 100,
	}

	original := []schema.Message{
		{Role: schema.RoleSystem, Content: "System prompt."},
		{Role: schema.RoleUser, Content: longContent},
	}

	result := l.Compact(original)

	// 反序列化失败时应返回原始消息
	if len(result) != len(original) {
		t.Fatalf("fallback: expected %d messages, got %d", len(original), len(result))
	}
	for i := range original {
		if result[i].Content != original[i].Content {
			t.Errorf("fallback: message %d content should be unchanged", i)
		}
	}
}

func TestLLMCompactor_AboveThreshold_LLMReturnsEmptyArray(t *testing.T) {
	// LLM 返回空数组 "[]"——有效的 JSON 但 0 条消息
	llm, _ := newMockLLM(t, makeChatCompletionHandler("[]"))

	longContent := ""
	for i := 0; i < 200; i++ {
		longContent += "padding. "
	}

	l := &LLMCompactor{
		llm:       llm,
		maxLength: 100,
	}

	msgs := []schema.Message{
		{Role: schema.RoleUser, Content: longContent},
	}

	result := l.Compact(msgs)

	// 空数组是合法的 JSON 反序列化结果
	if len(result) != 0 {
		t.Errorf("expected 0 messages from empty array, got %d", len(result))
	}
}

func TestLLMCompactor_AboveThreshold_LLMReturnsExtraWhitespaceJSON(t *testing.T) {
	// LLM 返回带前后空格的有效 JSON（模拟 LLM 可能添加多余空白的情况）
	llm, _ := newMockLLM(t, makeChatCompletionHandler(`
		[{"role":"user","content":"trimmed content"}]
	`))

	longContent := ""
	for i := 0; i < 200; i++ {
		longContent += "x"
	}

	l := &LLMCompactor{
		llm:       llm,
		maxLength: 100,
	}

	msgs := []schema.Message{
		{Role: schema.RoleUser, Content: longContent},
	}

	result := l.Compact(msgs)

	// JSON 标准库的 Unmarshal 能处理前后空白
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Content != "trimmed content" {
		t.Errorf("unexpected content: %q", result[0].Content)
	}
}

func TestLLMCompactor_SystemPromptOnly_BelowThreshold(t *testing.T) {
	// 只有 system prompt 且不超阈值时，不应触发压缩
	l := &LLMCompactor{
		llm:       nil,
		maxLength: 10000,
	}

	msgs := []schema.Message{
		{Role: schema.RoleSystem, Content: "You are a helpful coding assistant."},
	}

	result := l.Compact(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if result[0].Content != "You are a helpful coding assistant." {
		t.Errorf("system prompt was modified unexpectedly")
	}
}

func TestLLMCompactor_MessagesWithToolCalls_BelowThreshold(t *testing.T) {
	// 包含 tool calls 的消息，在阈值内应原样返回
	l := &LLMCompactor{
		llm:       nil,
		maxLength: 10000,
	}

	msgs := []schema.Message{
		{Role: schema.RoleSystem, Content: "System"},
		{Role: schema.RoleUser, Content: "Search for something"},
		{
			Role:    schema.RoleAssistant,
			Content: "I'll search for that",
			ToolCalls: []schema.ToolCall{
				{ID: "call_1", Name: "search", Arguments: json.RawMessage(`{"query":"test"}`)},
			},
		},
		{
			Role:       schema.RoleUser,
			Content:    "Search results: found nothing useful...",
			ToolCallID: "call_1",
		},
	}

	result := l.Compact(msgs)

	if len(result) != len(msgs) {
		t.Fatalf("expected %d messages, got %d", len(msgs), len(result))
	}
	if len(result[2].ToolCalls) != 1 {
		t.Errorf("tool calls should be preserved")
	}
	if result[2].ToolCalls[0].Name != "search" {
		t.Errorf("tool call name changed: %q", result[2].ToolCalls[0].Name)
	}
	if result[3].ToolCallID != "call_1" {
		t.Errorf("tool call ID changed: %q", result[3].ToolCallID)
	}
}

// ======================== 边界条件测试 ========================

func TestLLMCompactor_MaxLengthZero(t *testing.T) {
	// maxLength=0 时，任何非空消息都会触发压缩路径
	llm, _ := newMockLLM(t, makeChatCompletionHandler("[]"))

	l := &LLMCompactor{
		llm:       llm,
		maxLength: 0,
	}

	msgs := []schema.Message{
		{Role: schema.RoleUser, Content: "x"},
	}

	result := l.Compact(msgs)
	// 因为所有消息都超阈值，会进入 LLM 压缩路径
	if len(result) != 0 {
		t.Errorf("expected 0 messages (compressed to empty), got %d", len(result))
	}
}

func TestLLMCompactor_NilMessages_BelowThreshold(t *testing.T) {
	l := &LLMCompactor{
		llm:       nil,
		maxLength: 100,
	}

	result := l.Compact(nil)
	if result != nil {
		t.Errorf("expected nil result for nil input, got %v", result)
	}
}
