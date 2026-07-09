// internal/mcp/mcp_test.go
// MCP 集成测试：启动本地 echo MCP Server，验证完整通信链路
package mcp

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestMCPIntegration 端到端测试 MCP 通信链路
// 需要 go 命令可用（用于启动 echo MCP Server）
func TestMCPIntegration(t *testing.T) {
	// 检查 go 命令是否可用
	if _, err := os.Stat("../../go.mod"); os.IsNotExist(err) {
		t.Skip("不在项目目录下运行，跳过集成测试")
	}

	serverPath := "./testserver/echo_server.go"

	// 1. 创建 MCP Client 并启动 echo server
	client, err := NewClient("echo_test", "go", []string{"run", serverPath}, nil)
	if err != nil {
		t.Fatalf("创建 MCP 客户端失败: %v", err)
	}
	defer client.Close()

	if !client.IsHealthy() {
		t.Fatal("客户端应该处于健康状态")
	}

	t.Logf("服务器信息: %s v%s", client.ServerInfo().Name, client.ServerInfo().Version)

	// 2. 发现工具
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tools, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("tools/list 失败: %v", err)
	}

	if len(tools) != 2 {
		t.Fatalf("期望 2 个工具，实际得到 %d 个", len(tools))
	}

	t.Logf("发现 %d 个工具:", len(tools))
	for _, tool := range tools {
		t.Logf("  - %s: %s", tool.Name, tool.Description)
	}

	// 3. 调用 echo 工具
	echoResult, err := client.CallTool(ctx, "echo", map[string]interface{}{
		"message": "hello world",
	})
	if err != nil {
		t.Fatalf("tools/call echo 失败: %v", err)
	}

	if echoResult.IsError {
		t.Fatal("echo 工具不应返回错误")
	}

	if len(echoResult.Content) == 0 {
		t.Fatal("echo 工具应返回内容")
	}

	t.Logf("echo 返回: %s", echoResult.Content[0].Text)

	// 4. 测试 MCPTool 适配器
	toolDef := tools[0] // echo
	mcpTool := NewMCPTool("echo_test", toolDef, client)

	if mcpTool.Name() != "mcp_echo_test_echo" {
		t.Fatalf("工具名称错误: 期望 mcp_echo_test_echo，实际 %s", mcpTool.Name())
	}

	def := mcpTool.Definition()
	if def.Name != "mcp_echo_test_echo" {
		t.Fatalf("Definition 名称错误: %s", def.Name)
	}

	// 5. 测试 Execute
	args, _ := json.Marshal(map[string]string{"message": "integration test"})
	output, err := mcpTool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("MCPTool.Execute 返回错误: %v", err)
	}

	t.Logf("MCPTool.Execute 输出: %s", output)
}
