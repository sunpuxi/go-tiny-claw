package engine

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	ctxpkg "github.com/sunpuxi/go-tiny-claw/internal/context"
	"github.com/sunpuxi/go-tiny-claw/internal/provider"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
	"github.com/sunpuxi/go-tiny-claw/internal/tools"
)

type AgentEngine struct {
	provider       provider.LLMProvider
	registry       tools.Registry
	EnableThinking bool
	PlanMode       bool
	compactor      *ctxpkg.Compactor
	recovery       *ctxpkg.RecoveryManager // 【新增】自愈管理器
	injector       *ReminderInjector
}

func NewAgentEngine(p provider.LLMProvider, r tools.Registry, enableThinking bool, planMode bool) *AgentEngine {
	return &AgentEngine{
		provider:       p,
		registry:       r,
		EnableThinking: enableThinking,
		PlanMode:       planMode,
		compactor:      ctxpkg.NewCompactor(20000, 6),
		recovery:       ctxpkg.NewRecoveryManager(), // 初始化 Recovery
		injector:       NewReminderInjector(),       // 初始化死循环注入处理器
	}
}

func (e *AgentEngine) Run(ctx context.Context, session *ctxpkg.Session, reporter Reporter) error {
	log.Printf("[Engine] 唤醒会话 [%s]，锁定工作区: %s (PlanMode: %v)\n", session.ID, session.WorkDir, e.PlanMode)

	// 全局的Prompt加载
	composer := ctxpkg.NewPromptComposer(session.WorkDir, e.PlanMode)
	// 构建提示词，并读取所有的项目中的skill，仅提供skill名称
	systemMsg := composer.Build()

	for {
		// 加载所有的工具列表
		availableTools := e.registry.GetAvailableTools()
		// 获取短期的工作记忆
		workingMemory := session.GetWorkingMemory(20)
		// 拼接进历史的会话记录中
		var contextHistory []schema.Message
		contextHistory = append(contextHistory, systemMsg)
		contextHistory = append(contextHistory, workingMemory...)
		// 压缩信息（函数内部自行决断是否压缩，此处只需要显示的调用一下即可）
		compactedContext := e.compactor.Compact(contextHistory)

		// 深度思考模式
		currentTurnThinkingContent, compactedContext, err := e.think(ctx, compactedContext, reporter)
		if err != nil {
			return err
		}

		// 深度思考模式之后，挂载工具列表，开始后续的任务执行
		actionResp, err := e.provider.Generate(ctx, compactedContext, availableTools)
		if err != nil {
			return fmt.Errorf("Action 阶段失败: %w", err)
		}

		// (合并为合法的单条 Assistant 消息)
		finalAssistantMsg := schema.Message{
			Role:      schema.RoleAssistant,
			Content:   strings.TrimSpace(currentTurnThinkingContent + "\n" + actionResp.Content),
			ToolCalls: actionResp.ToolCalls,
		}
		session.Append(finalAssistantMsg)

		if actionResp.Content != "" && reporter != nil {
			reporter.OnMessage(ctx, actionResp.Content)
		}

		// 如果没有工具调用，则ReAct循环结束，认为当前的会话已经结束
		if len(actionResp.ToolCalls) == 0 {
			break
		}

		// 并发执行所有工具调用
		observationMsgs, lastToolCall, lastToolResult := e.executeTools(ctx, actionResp.ToolCalls, reporter)

		// 死循环检测与消息注入
		resultMessage := e.injector.CheckAndInject(lastToolCall, lastToolResult)
		if resultMessage != nil {
			observationMsgs = append(observationMsgs, *resultMessage)
		}

		// 追加上下文
		session.Append(observationMsgs...)
	}

	return nil
}

// think 执行深度思考阶段：不挂载工具列表，强制LLM先进行规划
func (e *AgentEngine) think(ctx context.Context, compactedContext []schema.Message, reporter Reporter) (string, []schema.Message, error) {
	if !e.EnableThinking {
		return "", compactedContext, nil
	}

	if reporter != nil {
		reporter.OnThinking(ctx)
	}

	// 调用LLM，不传入工具列表，强制模型先思考规划
	thinkResp, err := e.provider.Generate(ctx, compactedContext, nil)
	if err != nil {
		return "", nil, fmt.Errorf("Thinking 阶段失败: %w", err)
	}

	// 当前思考的结果信息，拼接进历史会话中
	currentTurnThinkingContent := ""
	if thinkResp.Content != "" {
		currentTurnThinkingContent = thinkResp.Content
		compactedContext = append(compactedContext, *thinkResp)
	}

	return currentTurnThinkingContent, compactedContext, nil
}

// executeTools 并发执行模型返回的所有工具调用，返回每个工具的执行结果消息
func (e *AgentEngine) executeTools(ctx context.Context, toolCalls []schema.ToolCall, reporter Reporter) ([]schema.Message, schema.ToolCall, schema.ToolResult) {
	observationMsgs := make([]schema.Message, len(toolCalls))
	var lastToolCall schema.ToolCall
	var lastToolResult schema.ToolResult

	var wg sync.WaitGroup

	for i, toolCall := range toolCalls {
		wg.Add(1)

		go func(idx int, call schema.ToolCall) {
			defer wg.Done()

			if reporter != nil {
				reporter.OnToolCall(ctx, call.Name, string(call.Arguments))
			}

			// 底层物理执行工具
			result := e.registry.Execute(ctx, call)

			// 核心拦截与注入：如果工具执行报错，注入恢复建议
			finalOutput := result.Output
			if result.IsError {
				finalOutput = e.recovery.AnalyzeAndInject(call.Name, result.Output)
				log.Printf("  -> [Go-%d] ❌ 注入救援指南: %s\n", idx, finalOutput)
			} else {
				log.Printf("  -> [Go-%d] ✅ 工具执行成功 (返回 %d 字节)\n", idx, len(result.Output))
			}

			if reporter != nil {
				displayOutput := finalOutput
				if len(displayOutput) > 200 {
					displayOutput = displayOutput[:200] + "... (已截断)"
				}
				reporter.OnToolResult(ctx, call.Name, displayOutput, result.IsError)
			}

			// 将最终结果写入上下文历史
			observationMsgs[idx] = schema.Message{
				Role:       schema.RoleUser,
				Content:    finalOutput,
				ToolCallID: call.ID,
			}

			// 最后一次的工具执行的结果
			if i == 0 {
				lastToolCall = toolCall
				lastToolResult = result
			}
		}(i, toolCall)
	}

	wg.Wait()
	return observationMsgs, lastToolCall, lastToolResult
}
