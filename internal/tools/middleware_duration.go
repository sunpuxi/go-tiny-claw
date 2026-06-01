package tools

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/sunpuxi/go-tiny-claw/internal/schema"
)

// DurationLogMiddleware 返回一个 AroundFunc，用于记录每次工具执行的耗时和结果摘要
func DurationLogMiddleware() AroundFunc {
	return func(ctx context.Context, call schema.ToolCall, next func() (string, error)) (string, error) {
		start := time.Now()

		output, err := next()

		elapsed := time.Since(start)
		if err != nil {
			log.Printf("[Registry] 工具 '%s' 执行完成 | 耗时: %v | 失败 | 错误: %v\n", call.Name, elapsed, err)
		} else {
			log.Printf("[Registry] 工具 '%s' 执行完成 | 耗时: %v | 成功 | 输出: %s\n", call.Name, elapsed, formatBytes(len(output)))
		}

		return output, err
	}
}

// formatBytes 将字节数格式化为人类可读的形式
func formatBytes(n int) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB (%d bytes)", float64(n)/(1024*1024), n)
	case n >= 1024:
		return fmt.Sprintf("%.1f KB (%d bytes)", float64(n)/1024, n)
	default:
		return fmt.Sprintf("%d bytes", n)
	}
}
