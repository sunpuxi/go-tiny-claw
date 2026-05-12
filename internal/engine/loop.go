package engine

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/sunpuxi/go-tiny-claw/internal/provider"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
	"github.com/sunpuxi/go-tiny-claw/internal/tools"
)

type AgentEngine struct {
	provider       provider.LLMProvider
	registry       tools.Registry
	WorkDir        string
	EnableThinking bool
}

func NewAgentEngine(p provider.LLMProvider, r tools.Registry, workDir string, enableThinking bool) *AgentEngine {
	return &AgentEngine{
		provider:       p,
		registry:       r,
		WorkDir:        workDir,
		EnableThinking: enableThinking,
	}
}

func (e *AgentEngine) Run(ctx context.Context, userPrompt string, repoter Reporter) error {
	log.Printf("[Engine] 引擎启动，锁定工作区: %s\n", e.WorkDir)
	log.Printf("[Engine] 慢思考模式 (Thinking Phase): %v\n", e.EnableThinking)

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

	turnCount := 0

	for {
		turnCount++
		log.Printf("\n========== [Turn %d] 开始 ==========\n", turnCount)

		availableTools := e.registry.GetAvailableTools()

		// Phase 1: 慢思考阶段
		if e.EnableThinking {
			// 思考阶段的输出
			if repoter != nil {
				repoter.OnThinking(ctx)
			}

			thinkResp, err := e.provider.Generate(ctx, contextHistory, nil)
			if err != nil {
				return fmt.Errorf("Thinking 阶段生成失败: %w", err)
			}
			if thinkResp.Content != "" {
				fmt.Printf("🧠 [内部思考 Trace]: \n%s\n", thinkResp.Content)
				contextHistory = append(contextHistory, *thinkResp)
			}
		}

		// Phase 2: 行动阶段
		log.Println("[Engine][Phase 2] 恢复工具挂载，等待模型采取行动...")
		actionResp, err := e.provider.Generate(ctx, contextHistory, availableTools)
		if err != nil {
			return fmt.Errorf("Action 阶段生成失败: %w", err)
		}

		contextHistory = append(contextHistory, *actionResp)

		if actionResp.Content != "" && repoter != nil {
			fmt.Printf("🤖 [对外回复]: \n%s\n", actionResp.Content)
			repoter.OnMessage(ctx, actionResp.Content)
		}

		if len(actionResp.ToolCalls) == 0 {
			log.Println("[Engine] 模型未请求调用工具，任务宣告完成。")
			break
		}

		log.Printf("[Engine] 模型请求并发调用 %d 个工具...\n", len(actionResp.ToolCalls))

		// ================= 并发执行逻辑 =================

		// 预分配切片以保证顺序并避免并发写入锁
		observationMsgs := make([]schema.Message, len(actionResp.ToolCalls))
		var wg sync.WaitGroup

		for i, toolCall := range actionResp.ToolCalls {
			wg.Add(1)

			go func(idx int, call schema.ToolCall) {
				defer wg.Done()

				if repoter != nil {
					repoter.OnToolCall(ctx, toolCall.Name, string(toolCall.Arguments))
				}

				// 执行底层工具
				result := e.registry.Execute(ctx, call)

				// 将工具执行的结果返回客户端
				if repoter != nil {
					displayOut := result.Output
					if len(displayOut) > 200 {
						displayOut = displayOut[:200] + "...已截断"
					}
					repoter.OnToolResult(ctx, toolCall.Name, displayOut, result.IsError)
				}

				// 安全写入对应索引
				observationMsgs[idx] = schema.Message{
					Role:       schema.RoleUser,
					Content:    result.Output,
					ToolCallID: call.ID,
				}
			}(i, toolCall)
		}

		wg.Wait() // 阻塞聚合
		log.Println("[Engine] 所有并发工具执行完毕，开始聚合观察结果 (Observation)...")

		// 按序追加回 Context
		for _, obs := range observationMsgs {
			contextHistory = append(contextHistory, obs)
		}
	}

	return nil
}
