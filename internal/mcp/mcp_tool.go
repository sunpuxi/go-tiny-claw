// internal/mcp/mcp_tool.go
// MCPTool：将单个 MCP 工具包装为 tools.BaseTool，使其可直接注册到现有 Registry
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sunpuxi/go-tiny-claw/internal/schema"
)

// MCPTool 将 MCP Server 的一个工具适配为 tools.BaseTool
// 实现 Name() / Definition() / Execute() 三个方法
type MCPTool struct {
	client       *Client
	registryName string   // 注册到 Registry 的唯一名称，格式: "mcp_<server>_<tool>"
	def          ToolDef  // MCP 工具原始定义
	serverName   string   // 所属 MCP 服务器的配置名称
}

// NewMCPTool 创建一个 MCP 工具适配器
// serverName: MCP 服务器的配置名称
// def: 从 tools/list 获取的工具定义
// client: 与该服务器通信的 MCP 客户端
func NewMCPTool(serverName string, def ToolDef, client *Client) *MCPTool {
	// 命名规则: mcp_<server_name>_<tool_name>，避免与内置工具名冲突
	registryName := fmt.Sprintf("mcp_%s_%s", serverName, def.Name)

	return &MCPTool{
		client:       client,
		registryName: registryName,
		def:          def,
		serverName:   serverName,
	}
}

// Name 返回工具在 Registry 中的唯一名称
func (t *MCPTool) Name() string {
	return t.registryName
}

// Definition 返回工具定义，通过 Registry 传递给 LLM
func (t *MCPTool) Definition() schema.ToolDefinition {
	// 在描述中标注来源，让 LLM 知晓这是来自 MCP 的外部工具
	desc := fmt.Sprintf("[MCP/%s] %s", t.serverName, t.def.Description)

	return schema.ToolDefinition{
		Name:        t.registryName,
		Description: desc,
		InputSchema: t.def.InputSchema, // 直接透传 MCP 服务的 JSON Schema
	}
}

// Execute 执行 MCP 工具调用
// 遵循 BashTool 的自愈模式：物理错误以字符串形式返回（而非 Go error），
// 让 LLM 可以读取错误信息并自我纠正
func (t *MCPTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	// 1. 健康检查
	if !t.client.IsHealthy() {
		return fmt.Sprintf("MCP 服务器 '%s' 已断开连接，工具 '%s' 当前不可用。",
			t.serverName, t.def.Name), nil
	}

	// 2. 解析参数为 map，传给 MCP Server
	var argsMap map[string]interface{}
	if err := json.Unmarshal(args, &argsMap); err != nil {
		// 参数解析失败是调用方（LLM）的错误，返回描述信息
		return fmt.Sprintf("MCP 工具 '%s' 参数解析失败: %v\n原始参数: %s",
			t.def.Name, err, string(args)), nil
	}

	// 3. 调用远端 MCP 工具
	result, err := t.client.CallTool(ctx, t.def.Name, argsMap)
	if err != nil {
		// JSON-RPC 层错误（超时、断开等），返回描述让 LLM 自愈
		return fmt.Sprintf("MCP 工具 '%s' 调用失败: %v", t.def.Name, err), nil
	}

	// 4. 提取 content[].text 拼接输出
	var output strings.Builder
	hasImage := false
	for _, block := range result.Content {
		switch block.Type {
		case "text":
			output.WriteString(block.Text)
		case "image":
			hasImage = true
		}
	}

	resultStr := output.String()

	// 如果有图片类型的内容，追加提示
	if hasImage {
		if resultStr != "" {
			resultStr += "\n"
		}
		resultStr += "[注: 该工具返回了图片数据，无法在纯文本模式下显示]"
	}

	// 5. 如果 MCP Server 标记了 isError，附加标记
	if result.IsError {
		return fmt.Sprintf("[MCP 工具 '%s' 返回错误]\n%s", t.def.Name, resultStr), nil
	}

	return resultStr, nil
}
