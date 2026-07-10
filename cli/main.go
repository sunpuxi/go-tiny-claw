// cmd/claw/main.go
package main

import (
	"context"
	"log"
	"os"

	ctxpkg "github.com/sunpuxi/go-tiny-claw/internal/context"
	"github.com/sunpuxi/go-tiny-claw/internal/engine"
	"github.com/sunpuxi/go-tiny-claw/internal/provider"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
	"github.com/sunpuxi/go-tiny-claw/internal/search"
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

	// 注册联网搜索工具
	searchProvider := search.NewDuckDuckGoProvider()
	registry.Register(tools.NewWebSearchTool(searchProvider))
	registry.Register(tools.NewWebFetchTool())

	// 上下文压缩策略
	llmCompactor := ctxpkg.NewLLMCompactor("deepseek-chat", 4000)

	eng := engine.NewAgentEngine(llmProvider, llmCompactor, registry, false, false)
	reporter := engine.NewTerminalReporter()
	sess := ctxpkg.GlobalSessionMgr.GetOrCreate("test_trace_001", workDir)

	// 触发一个联网搜索任务
	prompt := `请使用 web_search 工具搜索 "golang 1.25 release notes"，告诉我 Go 1.25 有哪些重要的新特性，并使用 web_fetch 工具深入阅读最相关的一两个页面获取详细内容。`

	sess.Append(schema.Message{Role: schema.RoleUser, Content: prompt})

	log.Println("\n>>> 启动联网搜索测试...")
	err := eng.Run(context.Background(), sess, reporter)
	if err != nil {
		log.Fatalf("引擎崩溃: %v", err)
	}
}
