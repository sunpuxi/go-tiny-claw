package engine

import (
	"context"
	"fmt"
	"github.com/sunpuxi/go-tiny-claw/internal/observability"
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
	compactor      ctxpkg.CompactorInterface
	recovery       *ctxpkg.RecoveryManager // 【新增】自愈管理器
	injector       *ReminderInjector
}

func NewAgentEngine(p provider.LLMProvider, compactor ctxpkg.CompactorInterface, r tools.Registry, enableThinking bool, planMode bool) *AgentEngine {
	return &AgentEngine{
		provider:       p,
		registry:       r,
		EnableThinking: enableThinking,
		PlanMode:       planMode,
		compactor:      compactor,
		recovery:       ctxpkg.NewRecoveryManager(), // 初始化 Recovery
		injector:       NewReminderInjector(),       // 初始化死循环注入处理器
	}
}

func (e *AgentEngine) Run(ctx context.Context, session *ctxpkg.Session, reporter Reporter) error {
	log.Printf("[Engine] 唤醒会话 [%s]，锁定工作区: %s (PlanMode: %v)\n", session.ID, session.WorkDir, e.PlanMode)

	// 【埋点1】开启Root Span，记录整个任务的生命周期
	ctx, rootSpan := observability.StartSpan(ctx, "root")
	rootSpan.AddAttribute("session_id", session.ID)
	rootSpan.AddAttribute("WorkDir", session.WorkDir)

	// 保证在引擎退出时能够将追踪信息保存到文件中
	// todo 后续可以做的优化：当程序退出时，将本次任务执行的概要记录到文件中，当重启 Agent 的时候，提供一个可以从历史会话恢复 memory
	defer func() {
		rootSpan.EndSpan()
		_ = observability.ExportTraceToFile(rootSpan, session.WorkDir, session.ID)
		log.Printf("📊 [Tracing] 本次任务的执行回放链路已保存至工作区的 .claw/traces 目录下\n")
	}()

	// 全局的Prompt加载
	composer := ctxpkg.NewPromptComposer(session.WorkDir, e.PlanMode)
	// 构建提示词，并读取所有的项目中的skill，仅提供skill名称
	systemMsg := composer.Build()

	// ReAct 循环的轮次
	turn := 0
	for {
		turn++
		// 【埋点2】 记录单次的 ReAct 循环
		turnCtx, turnSpan := observability.StartSpan(ctx, fmt.Sprintf("turn_%d", turn))
		defer turnSpan.EndSpan()

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

		// 记录发给模型的真实的上下文大小
		turnSpan.AddAttribute("context_message_count", len(compactedContext))

		// 深度思考模式
		currentTurnThinkingContent, compactedContext, err := e.think(turnCtx, compactedContext, reporter)
		if err != nil {
			return err
		}

		// 【埋点4】记录工具执行
		actCtx, actSpan := observability.StartSpan(turnCtx, "llm.act")
		// 深度思考模式之后，挂载工具列表，开始后续的任务执行
		actionResp, err := e.provider.Generate(actCtx, compactedContext, availableTools)
		actSpan.EndSpan()
		if err != nil {
			return fmt.Errorf("action 阶段失败: %w", err)
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
			actSpan.EndSpan()
			break
		}

		// 并发执行所有工具调用 此处的工具并发工具执行需要由模型去决策，可以将工具调用分为多batch，每个不同的batch中的工具调用可以并发执行，batch 与 batch 之间必须顺序执行。不需要人工或者代码介入
		observationMsgs, lastToolCall, lastToolResult := e.executeTools(turnCtx, actionResp.ToolCalls, reporter)

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

	// 【埋点3】记录thinking阶段
	thinkingCtx, thinkingSpan := observability.StartSpan(ctx, "llm.thinking")
	defer thinkingSpan.EndSpan()

	if reporter != nil {
		reporter.OnThinking(ctx)
	}

	// 调用LLM，不传入工具列表，强制模型先思考规划
	thinkResp, err := e.provider.Generate(thinkingCtx, compactedContext, nil)
	if err != nil {
		return "", nil, fmt.Errorf("thinking 阶段失败: %w", err)
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

// RunSub 是专为 Subagent 拉起的一次性受限循环。
// 它不依赖外部 Session，打完就跑。
// Reporter：为了让用户在终端看到子智能体的工作轨迹，我们将主线程的 Reporter 透传进来，并打上特殊标记。
func (e *AgentEngine) RunSub(ctx context.Context, llModel provider.LLMProvider, taskPrompt string, readOnlyRegistry tools.Registry, reporter any) (string, error) {

	// 【核心优化】：子智能体极其容易偷懒。我们必须在 System Prompt 中严厉警告它必须使用工具！
	contextHistory := []schema.Message{
		{
			Role: schema.RoleSystem,
			Content: `你是一个专门负责深度探索的探路者 (Explorer Subagent)。
你的任务是根据主架构师的指令，在当前工作区内仔细阅读代码、查阅日志，搜集足够的信息。

【核心纪律】
1. 你必须、且只能依靠内置工具（如 bash 的 find/grep，或 read_file）去寻找答案。绝对不允许凭空捏造或猜测！
2. 如果你没有找到确切的答案，你必须继续使用工具深入搜索。
3. 当且仅当你找到了确切的线索后，停止调用工具，直接输出一段纯文本作为你的终极汇报。主架构师会根据你的汇报来做下一步决策。`,
		},
		{
			Role:    schema.RoleUser,
			Content: taskPrompt,
		},
	}

	// 限制子智能体最多只能跑 10 个 Turn，防止它自己卡死
	const maxSubTurns = 10
	turnCount := 0

	for {
		turnCount++
		if turnCount > maxSubTurns {
			return "", fmt.Errorf("子智能体探索过于深入，超过 %d 轮被强制召回，请主 Agent 给它更明确的指令", maxSubTurns)
		}

		// 【驾驭底线】：子智能体仅能获取传入的只读工具注册表
		availableTools := readOnlyRegistry.GetAvailableTools()

		compactedContext := e.compactor.Compact(contextHistory)

		// 子任务要求急速响应，强制关闭主体的慢思考，直接预测行动
		actionResp, err := llModel.Generate(ctx, compactedContext, availableTools)
		if err != nil {
			return "", fmt.Errorf("子智能体推理失败: %w", err)
		}

		contextHistory = append(contextHistory, *actionResp)

		// 【核心退出条件】：子智能体一旦不调用工具了，说明它做好了总结汇报
		if len(actionResp.ToolCalls) == 0 {
			// 直接将它的这段汇报内容剥离出来返回给上层
			return actionResp.Content, nil
		}

		// 执行只读工具的并发循环
		var r Reporter
		if reporter != nil {
			r = reporter.(Reporter)
		}
		observationMsgs, _, _ := e.executeTools(ctx, actionResp.ToolCalls, r)

		contextHistory = append(contextHistory, observationMsgs...)
	}
}
