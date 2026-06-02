package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sunpuxi/go-tiny-claw/internal/constants"
	"github.com/sunpuxi/go-tiny-claw/internal/provider"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
	"log"
)

// AgentRunner 是一个打破循环依赖的抽象接口。
// 因为 SubagentTool 存在于 tools 包，而完整的 AgentEngine 存在于 engine 包。
// 为了让 Tool 能拉起 Engine，我们定义一个接口供外部注入。
type AgentRunner interface {
	// RunSub 启动一个匿名的、一次性的子智能体任务，并返回其最终梳理出的纯文本总结
	RunSub(ctx context.Context, provider provider.LLMProvider, taskPrompt string, readOnlyRegistry Registry, reporter interface{}) (string, error)
}

type SubagentTool struct {
	runner AgentRunner

	// 为子智能体准备的专属、受限的“只读”注册表
	readOnlyRegistry Registry
	reporter         interface{} // 暂时用 interface 规避包循环依赖，底层通过断言使用
}

// NewSubagentTool 构造函数
func NewSubagentTool(runner AgentRunner, readOnlyRegistry Registry, reporter interface{}) *SubagentTool {
	return &SubagentTool{
		runner:           runner,
		readOnlyRegistry: readOnlyRegistry,
		reporter:         reporter,
	}
}

func (t *SubagentTool) Name() string {
	return "spawn_subagent"
}

// Definition 向主 Agent 暴露这个工具的强大能力
func (t *SubagentTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "派出一个专门用于深度探索（Exploration）的子智能体。当你需要阅读大量代码、跨文件查找逻辑时请调用此工具。它在探索完毕后，会给你返回一份极度精炼的摘要报告。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task_prompt": map[string]interface{}{
					"type":        "string",
					"description": "给子智能体下达的明确指令。",
				},
				"llm_model_name": map[string]interface{}{
					"type":        "string",
					"description": fmt.Sprintf("启动subAgent时根据任务的复杂程度选择合适的模型,目前支持的模型信息:%s", constants.GetModelDescription()),
				},
			},
			"required": []string{"task_prompt"},
		},
	}
}

type subagentArgs struct {
	TaskPrompt   string `json:"task_prompt"`
	LLMModelName string `json:"llm_model_name"`
}

func (t *SubagentTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input subagentArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("解析参数失败: %w", err)
	}

	log.Printf("[Subagent] 🚀 主 Agent 发起委派！正在拉起探路者: [%s]...\n", input.TaskPrompt)

	// 根据模型选择的模型信息，创建provider
	llmProvider := provider.NewOpenAiProviderByModelName(input.LLMModelName)

	// 我们把针对该任务的专项指令传给子智能体，并仅提供 readOnlyRegistry。
	summary, err := t.runner.RunSub(ctx, llmProvider, input.TaskPrompt, t.readOnlyRegistry, t.reporter)

	if err != nil {
		return fmt.Errorf("子智能体执行失败: %v", err).Error(), nil
	}

	log.Printf("[Subagent] ✅ 子智能体任务结束。报告返回给主干...")

	// 就像一次普通的 API 调用一样，返回给了始终保持清醒的主 Agent。
	return fmt.Sprintf("【子智能体探索报告】:\n%s", summary), nil
}
