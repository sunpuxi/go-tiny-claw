// cmd/claw/main.go
package main

import (
	"context"
	"log"
	"os"

	"github.com/sunpuxi/go-tiny-claw/internal/engine"
	"github.com/sunpuxi/go-tiny-claw/internal/provider"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
)

// 伪造的工具注册表 (用于测试 Provider 的工具提取能力)
type mockRegistry struct{}

func (m *mockRegistry) GetAvailableTools(ctx context.Context) []schema.ToolDefinition {
	return []schema.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "获取指定城市的当前天气情况。",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"city": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []string{"city"},
			},
		},
	}
}

func (m *mockRegistry) ExecuteToolCall(ctx context.Context, call schema.ToolCall) schema.ToolResult {
	log.Printf("  -> [Mock 工具执行] 获取 %s 的天气中...\n", call.Name)
	return schema.ToolResult{
		ToolCallID: call.ID,
		Output:     "API 返回：今天是晴天，气温 25 度。",
		IsError:    false,
	}
}

func main() {
	// 确保已设置 ZHIPU_API_KEY
	if os.Getenv("ZHIPU_API_KEY") == "" {
		log.Fatal("请先导出 ZHIPU_API_KEY 环境变量")
	}

	workDir, _ := os.Getwd()

	// 1. 初始化真实的 Provider大脑 (指向智谱 GLM-4.5)
	// 这里你可以任意切换 NewZhipuClaudeProvider 或 NewZhipuOpenAIProvider，效果完全一致！
	llmProvider := provider.NewZhipuOpenAIProvider("glm-4.5-air")

	// 2. 注入伪造的工具注册表
	registry := &mockRegistry{}

	// 3. 实例化并运行引擎，开启 EnableThinking = true (开启慢思考阶段！)
	eng := engine.NewAgentEngine(llmProvider, registry, workDir, true)

	// 设定测试任务
	prompt := "我想去北京跑步，帮我查查天气适合吗？"

	err := eng.Run(context.Background(), prompt)
	if err != nil {
		log.Fatalf("引擎运行崩溃: %v", err)
	}
}
