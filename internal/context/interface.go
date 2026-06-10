package context

import "github.com/sunpuxi/go-tiny-claw/internal/schema"

type CompactorInterface interface {

	// Compact 压缩上下文信息
	Compact(msgs []schema.Message) []schema.Message
}
