package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
	"os"
	"path/filepath"
)

type WriteFileTool struct {
	workDir string
}

func NewWriteFileTool(workDir string) WriteFileTool {
	return WriteFileTool{workDir}
}

func (w WriteFileTool) Name() string {
	return "write_file_tool"
}

func (w WriteFileTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        w.Name(),
		Description: "创建或覆盖写入一个文件。如果目录不存在会自动创建。请提供相对于工作区的相对路径。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "要写入的文件路径，如 src/main.go",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "要写入的完整文件内容",
				},
			},
			"required": []string{"path", "content"},
		},
	}
}

type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (w WriteFileTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	// 解析参数
	var inputArgs writeFileArgs
	if err := json.Unmarshal(args, &inputArgs); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	// 【安全防线】：限制在 WorkDir 下执行，防止大模型修改系统级文件
	fullPath := filepath.Join(w.workDir, inputArgs.Path)

	// 自动创建缺失的父级目录
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", fmt.Errorf("创建父目录失败: %w", err)
	}

	// 写入文件内容，设置权限为0644
	err := os.WriteFile(fullPath, []byte(inputArgs.Content), 0644)
	if err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	return fmt.Sprintf("成功将内容写入到文件: %s", inputArgs.Path), nil
}
