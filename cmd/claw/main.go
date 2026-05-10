// cmd/claw/main.go
package main

import (
	"context"
	"log"
	"os"

	"github.com/sunpuxi/go-tiny-claw/internal/engine"
	"github.com/sunpuxi/go-tiny-claw/internal/provider"
	"github.com/sunpuxi/go-tiny-claw/internal/tools"
)

func main() {
	if os.Getenv("ZHIPU_API_KEY") == "" {
		log.Fatal("请先导出 ZHIPU_API_KEY 环境变量")
	}

	workDir, _ := os.Getwd()

	llmProvider := provider.NewZhipuOpenAIProvider("glm-4.5-air")

	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(workDir))
	// 挂载其他的极简工具
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))

	// 实例化引擎，开启 EnableThinking = true (开启慢思考，促使模型一次性统筹规划)
	eng := engine.NewAgentEngine(llmProvider, registry, workDir, true)

	// 下发一个需要收集多源信息的任务
	prompt := `
    我当前目录下有 a.txt, b.txt, c.txt 三个文件。
    为了节省时间，请你同时一次性读取这三个文件，并将它们的内容综合起来，告诉我它们分别记录了什么领域的信息。
    `

	err := eng.Run(context.Background(), prompt)
	if err != nil {
		log.Fatalf("引擎运行崩溃: %v", err)
	}
}
