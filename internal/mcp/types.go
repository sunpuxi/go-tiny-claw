// internal/mcp/types.go
// MCP (Model Context Protocol) JSON-RPC 2.0 协议类型定义
// 仅实现 tools 相关的能力（v1 不包含 resources 和 prompts）
package mcp

import "encoding/json"

// ============================================================================
// JSON-RPC 2.0 基础信封
// ============================================================================

// jsonrpcRequest 通用 JSON-RPC 2.0 请求
type jsonrpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id,omitempty"` // 通知消息无 id
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonrpcResponse 通用 JSON-RPC 2.0 响应
type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonrpcError   `json:"error,omitempty"`
}

// jsonrpcError JSON-RPC 2.0 错误对象
type jsonrpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ============================================================================
// initialize 握手
// ============================================================================

// initializeParams 客户端发送的初始化参数
type initializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    clientCapabilities `json:"capabilities"`
	ClientInfo      clientInfo         `json:"clientInfo"`
}

type clientCapabilities struct {
	Tools *struct{} `json:"tools,omitempty"`
}

type clientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// initializeResult 服务器返回的初始化结果
type initializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    serverCapabilities `json:"capabilities"`
	ServerInfo      serverInfo         `json:"serverInfo"`
}

type serverCapabilities struct {
	Tools *struct{} `json:"tools,omitempty"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// ============================================================================
// tools/list — 工具发现
// ============================================================================

// listToolsResult tools/list 的响应
type listToolsResult struct {
	Tools []ToolDef `json:"tools"`
}

// ToolDef 单个 MCP 工具的定义（与 schema.ToolDefinition 对应但不完全相同）
type ToolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

// ============================================================================
// tools/call — 工具执行
// ============================================================================

// callToolParams tools/call 的请求参数
type callToolParams struct {
	Name      string      `json:"name"`
	Arguments interface{} `json:"arguments"`
}

// callToolResult tools/call 的响应结果
type callToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock MCP 内容块（text / image / resource）
type ContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	Data     string `json:"data,omitempty"`
}
