package engine

import (
	"context"
	"fmt"
	"log"

	"github.com/sunpuxi/go-tiny-claw/internal/provider"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
	"github.com/sunpuxi/go-tiny-claw/internal/tools"
)

type AgentEngine struct {
	Provider       provider.LLMProvider
	ToolRegistry   tools.ToolRegistry
	WorkDir        string
	EnableThinking bool
}

func NewAgentEngine(
	provider provider.LLMProvider,
	toolRegistry tools.ToolRegistry,
	workDir string,
	enableThinking bool) *AgentEngine {
	return &AgentEngine{
		Provider:       provider,
		ToolRegistry:   toolRegistry,
		WorkDir:        workDir,
		EnableThinking: enableThinking,
	}
}

func (a *AgentEngine) Run(ctx context.Context, userPrompt string) error {
	log.Printf("[Engine] 引擎启动，锁定工作区: %s\n", a.WorkDir)
	log.Printf("[Engine] enableThinking is %+v\n", a.EnableThinking)

	// 首先是角色的定义
	contextHistory := []schema.Message{
		{
			Role:    schema.RoleSystem,
			Content: "You are go-tiny-claw, an expert coding assistant. You have full access to tools in the workspace.",
		},
		{
			Role:    schema.RoleUser,
			Content: userPrompt,
		},
	}

	// 开始 ReAct
	turnCount := 0
	for {
		// 轮次加一
		turnCount++
		log.Printf("========== [Turn %d] 开始 ==========\n", turnCount)

		// 获取当前的可用的工具列表
		availableTools := a.ToolRegistry.GetAvailableTools()

		// 如果当前打开了思考模式，那么第一次调用先不必传递工具列表，防止大模型盲目调用参数
		if a.EnableThinking {
			log.Printf("[Engine] start thinking\n")
			msg, err := a.Provider.Generate(ctx, contextHistory, nil)
			if err != nil {
				log.Printf("error is %+v\n", err)
				return err
			}

			// 打印思考结果
			if msg.Content != "" {
				log.Printf("[Engine] enable thinking result content is %s\n", msg.Content)
			}

			// 追加返回信息至历史上下文中
			contextHistory = append(contextHistory, *msg)
		}

		// 调用模型进行处理
		respMessage, err := a.Provider.Generate(ctx, contextHistory, availableTools)
		if err != nil {
			return fmt.Errorf("模型生成失败: %w", err)
		}

		// 将返回的结果追加到历史上下文之中，注意这里无论是否需要调用工具，都要追加之上下文中，
		//因为模型后续可能不会调用工具，但是上一次调用工具返回的结果，是下一次推理所必须的信息
		contextHistory = append(contextHistory, *respMessage)

		// 打印出模型返回的文本，通常是思考内容或者文本信息
		if respMessage.Content != "" {
			log.Printf("🤖 模型: %s\n", respMessage.Content)
		}

		// 退出条件判断
		if len(respMessage.ToolCalls) == 0 {
			log.Println("[Engine] 任务完成，退出循环。")
			break
		}

		for _, toolCall := range respMessage.ToolCalls {
			log.Printf(" -> 🛠️ 执行工具: %s, 参数: %s\n", toolCall.Name, string(toolCall.Arguments))
			// 通过 Registry 路由并执行底层工具
			result := a.ToolRegistry.Execute(ctx, *toolCall)
			if result.IsError {
				log.Printf(" -> ❌ 工具执行报错: %s\n", result.Output)
			} else {
				log.Printf(" -> ✅ 工具执行成功 (返回 %d 字节)\n", len(result.Output))
			}
			// 将工具执行的观察结果 (Observation) 封装为 User Message 追加到上下文中 // 注意：ToolCallID 必须携带！这是维系大模型推理链条的关键
			observationMsg := &schema.Message{
				Role:       schema.RoleUser,
				Content:    result.Output,
				ToolCallID: toolCall.ID,
			}
			contextHistory = append(contextHistory, *observationMsg)
		}
	}

	return nil
}
