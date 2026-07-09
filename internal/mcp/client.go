// internal/mcp/client.go
// MCP 客户端：封装与单个 MCP Server 的完整交互生命周期
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// Client 管理与一个 MCP Server 的完整连接
type Client struct {
	transport  *StdioTransport
	serverInfo serverInfo
	name       string // 服务器配置名称
}

// NewClient 创建 MCP 客户端并完成初始化握手
// 整个过程在 30s 超时内完成，超时则返回错误
func NewClient(name, command string, args []string, env map[string]string) (*Client, error) {
	transport, err := NewStdioTransport(name, command, args, env)
	if err != nil {
		return nil, fmt.Errorf("创建传输层失败: %w", err)
	}

	// 标记为健康状态
	transport.healthy.Store(true)

	c := &Client{
		transport: transport,
		name:      name,
	}

	// 初始化握手（30s 超时）
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := c.initialize(ctx); err != nil {
		transport.Close()
		return nil, fmt.Errorf("初始化握手失败: %w", err)
	}

	return c, nil
}

// initialize 执行 MCP 初始化握手：initialize → notifications/initialized
func (c *Client) initialize(ctx context.Context) error {
	// 1. 发送 initialize 请求
	params := initializeParams{
		ProtocolVersion: "2025-03-26",
		Capabilities: clientCapabilities{
			Tools: &struct{}{},
		},
		ClientInfo: clientInfo{
			Name:    "go-tiny-claw",
			Version: "1.0.0",
		},
	}

	resp, err := c.transport.SendRequest(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("initialize 请求失败: %w", err)
	}

	var result initializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("解析 initialize 响应失败: %w", err)
	}

	c.serverInfo = result.ServerInfo
	log.Printf("[MCP-%s] 握手成功: 服务器=%s v%s, 协议=%s",
		c.name, result.ServerInfo.Name, result.ServerInfo.Version, result.ProtocolVersion)

	// 2. 发送 initialized 通知
	if err := c.transport.SendNotification("notifications/initialized", nil); err != nil {
		return fmt.Errorf("发送 initialized 通知失败: %w", err)
	}

	return nil
}

// ListTools 获取服务器提供的所有工具定义
func (c *Client) ListTools(ctx context.Context) ([]ToolDef, error) {
	resp, err := c.transport.SendRequest(ctx, "tools/list", nil)
	if err != nil {
		return nil, fmt.Errorf("tools/list 请求失败: %w", err)
	}

	var result listToolsResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("解析 tools/list 响应失败: %w", err)
	}

	log.Printf("[MCP-%s] 发现 %d 个工具", c.name, len(result.Tools))
	return result.Tools, nil
}

// CallTool 调用服务器上的一个工具
// name: 工具的原始名称
// arguments: JSON 参数（已解析为 map 或保持原始 JSON）
func (c *Client) CallTool(ctx context.Context, name string, arguments interface{}) (*callToolResult, error) {
	params := callToolParams{
		Name:      name,
		Arguments: arguments,
	}

	resp, err := c.transport.SendRequest(ctx, "tools/call", params)
	if err != nil {
		return nil, fmt.Errorf("tools/call 请求失败: %w", err)
	}

	var result callToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("解析 tools/call 响应失败: %w", err)
	}

	return &result, nil
}

// IsHealthy 返回服务器连接是否健康
func (c *Client) IsHealthy() bool {
	return c.transport.IsHealthy()
}

// ServerName 返回配置中的服务器名称
func (c *Client) ServerName() string {
	return c.name
}

// ServerInfo 返回 initialize 握手获得的服务器信息
func (c *Client) ServerInfo() serverInfo {
	return c.serverInfo
}

// Close 关闭与 MCP Server 的连接
func (c *Client) Close() error {
	return c.transport.Close()
}
