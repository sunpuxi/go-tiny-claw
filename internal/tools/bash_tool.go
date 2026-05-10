package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"time"

	"github.com/sunpuxi/go-tiny-claw/internal/schema"
)

type BashTool struct {
	workDir string // 工作区约束
}

func NewBashTool(workDir string) *BashTool {
	return &BashTool{workDir: workDir}
}

func (t *BashTool) Name() string {
	return "bash"
}

func (t *BashTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "在当前工作区执行任意的 bash 命令。支持链式命令(如 &&)。返回标准输出(stdout)和标准错误(stderr)。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "要执行的 bash 命令，例如: ls -la 或 go test ./...",
				},
			},
			"required": []string{"command"},
		},
	}
}

type bashArgs struct {
	Command string `json:"command"`
}

func (t *BashTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input bashArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	// 【驾驭底线 1】：Time Budgeting (时间预算与超时控制)
	// 给予 bash 命令一个最大执行时间，防止大模型卡死进程 (比如运行了 top 或持续监听的 Web 服务)
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		// Windows 环境：使用 powershell 执行
		// 加上 -NoProfile 可以避免加载冗长的用户脚本，防止命令干扰
		cmd = exec.CommandContext(timeoutCtx, "powershell", "-NoProfile", "-NonInteractive", "-Command", input.Command)
	} else {
		// macOS/Linux 环境：保持使用 bash
		cmd = exec.CommandContext(timeoutCtx, "bash", "-c", input.Command)
	}

	// 【驾驭底线 2】：绑定执行的工作区目录
	// 确保命令默认在用户指定的 WorkDir 下执行，而不是引擎启动时的绝对路径。
	cmd.Dir = t.workDir

	// 执行并捕获 CombinedOutput (合并 stdout 和 stderr)
	out, err := cmd.CombinedOutput()
	outputStr := string(out)

	// 如果命令执行超时，返回警告信息让模型知晓
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return outputStr + "\n[警告: 命令执行超时(30s)，已被系统强制终止。如果是启动常驻服务，请尝试将其转入后台。]", nil
	}

	// 【驾驭底线 3】：错误原样回传 (Self-Correction 自愈机制)
	// 当 bash 报错时（err != nil），我们绝对不能返回 Go 的 error 阻断程序！
	// 我们必须把 err 和 outputStr 拼接成字符串返回，利用大模型的自纠错能力自己分析报错！
	if err != nil {
		return fmt.Sprintf("执行报错: %v\n输出:\n%s", err, outputStr), nil
	}

	// 如果没有终端输出（比如仅仅执行了 mkdir），给模型一个明确的执行成功的反馈
	if outputStr == "" {
		return "命令执行成功，无终端输出。", nil
	}

	// 【驾驭底线 4】：长度截断保护 (防 OOM)
	const maxLen = 8000
	if len(outputStr) > maxLen {
		return fmt.Sprintf("%s\n\n...[终端输出过长，已截断至前 %d 字节]...", outputStr[:maxLen], maxLen), nil
	}

	return outputStr, nil
}
