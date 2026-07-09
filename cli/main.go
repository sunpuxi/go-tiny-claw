// cmd/claw/main.go
package main

import (
	"context"
	"log"
	"os"
	"time"

	ctxpkg "github.com/sunpuxi/go-tiny-claw/internal/context"
	"github.com/sunpuxi/go-tiny-claw/internal/engine"
	"github.com/sunpuxi/go-tiny-claw/internal/mcp"
	"github.com/sunpuxi/go-tiny-claw/internal/provider"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
	"github.com/sunpuxi/go-tiny-claw/internal/tools"
)

func main() {
	if os.Getenv("ZHIPU_API_KEY") == "" {
		log.Fatal("请先导出 ZHIPU_API_KEY 环境变量")
	}

	workDir, _ := os.Getwd()
	workDir += "/workspace"
	llmProvider := provider.NewZhipuOpenAIProvider("glm-4.5-air")

	// 工具注册列表
	registry := tools.NewRegistry()
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))

	// ===== MCP 工具集成 =====
	mcpCfg, err := mcp.LoadMCPConfig("config/mcp_servers.yaml")
	if err != nil {
		log.Printf("[MCP] 配置加载失败: %v，跳过 MCP 工具集成", err)
	} else if mcpCfg != nil && len(mcpCfg.Servers) > 0 {
		mcpManager := mcp.NewManager(mcpCfg)
		startCtx, startCancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := mcpManager.Start(startCtx); err != nil {
			log.Printf("[MCP] MCP 服务器启动失败: %v", err)
		} else {
			log.Printf("[MCP] 成功启动，共发现 %d 个 MCP 工具", mcpManager.ToolCount())
			mcpManager.RegisterAll(registry)
		}
		startCancel()
		defer mcpManager.Shutdown()
	}

	// 上下文压缩策略
	llmCompactor := ctxpkg.NewLLMCompactor("deepseek-chat", 4000)

	eng := engine.NewAgentEngine(llmProvider, llmCompactor, registry, false, false)
	reporter := engine.NewTerminalReporter()
	sess := ctxpkg.GlobalSessionMgr.GetOrCreate("test_trace_001", workDir)

	// 触发一个跨工具类型的并发任务
	prompt := `
    在当前的工作目录下，新建一个测试项目，在这个项目中比对快速排序和冒泡排序在给一个长度为10000的切片进行排序时的性能比较
    `
	sess.Append(schema.Message{Role: schema.RoleUser, Content: prompt})

	log.Println("\n>>> 🚀 启动带 Tracing 链路追踪的测试...")
	err = eng.Run(context.Background(), sess, reporter)
	if err != nil {
		log.Fatalf("引擎崩溃: %v", err)
	}
}
