package schema

import "encoding/json"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message 消息的协议格式约定
type Message struct {
	Role       Role       `json:"role"`         // 角色定义
	Content    string     `json:"content"`      // 存放纯文本内容
	ToolCalls  []ToolCall `json:"tool_call"`    // 需要调用的工具列表
	ToolCallID string     `json:"tool_call_id"` // 工具调用之后，调用的工具的ID，作为上下文消息传递，保证消息的连续性
}

// ToolDefinition 工具的定义
type ToolDefinition struct {
	ID          int64       `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

// ToolCall 工具调用的请求
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolResult 工具调用的返回结果
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Output     string `json:"output"`
	IsError    bool   `json:"is_error"`
}
