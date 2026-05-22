// internal/tools/read_skill_tool.go
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	ctxpkg "github.com/sunpuxi/go-tiny-claw/internal/context"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
)

// ReadSkillTool 实现了按名称读取专业技能(Skill)完整指令的工具
type ReadSkillTool struct {
	loader *ctxpkg.SkillLoader
}

func NewReadSkillTool(workDir string) *ReadSkillTool {
	return &ReadSkillTool{
		loader: ctxpkg.NewSkillLoader(workDir),
	}
}

func (t *ReadSkillTool) Name() string {
	return "read_skill"
}

func (t *ReadSkillTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "读取指定名称的专业技能(Skill)的完整指令。技能名称需从可用技能列表中获取，调用此工具后将返回包含执行指南的详细内容。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "技能名称，需与可用技能列表中显示的名称完全一致",
				},
			},
			"required": []string{"name"},
		},
	}
}

type readSkillArgs struct {
	Name string `json:"name"`
}

func (t *ReadSkillTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input readSkillArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	if input.Name == "" {
		return "", fmt.Errorf("技能名称不能为空")
	}

	skill, err := t.loader.ReadSkill(input.Name)
	if err != nil {
		return fmt.Sprintf("未找到技能 '%s'，请从可用技能列表中选择正确的名称", input.Name), nil
	}

	result := fmt.Sprintf("## 技能名称: %s\n", skill.Name)
	result += fmt.Sprintf("**触发条件**: %s\n\n", skill.Description)
	result += "**执行指南**:\n"
	result += skill.Body

	return result, nil
}
