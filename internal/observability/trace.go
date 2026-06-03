package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// traceKey context 中存放 span 的专属的key
type traceKey struct{}

// Span 链路追踪中的一个时间跨度和操作节点
type Span struct {
	Name       string                 `json:"name"`
	StartTime  time.Time              `json:"start_time"`
	EndTime    time.Time              `json:"end_time"`
	DurationMs int64                  `json:"duration_ms"`
	Attributes map[string]interface{} `json:"attributes"`
	Children   []*Span                `json:"children"`
	mu         sync.Mutex
}

// StartSpan 开启一个新的追踪跨度，并将其级联到 context 中
func StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	// 创建当前层级的 Span
	span := &Span{
		Name:       name,
		StartTime:  time.Now(),
		Attributes: make(map[string]interface{}),
	}

	// 从 context 中获取父span（尝试，不一定有，因为自身就是最顶层的span）
	if parent, ok := ctx.Value(traceKey{}).(*Span); ok {
		parent.mu.Lock()
		defer parent.mu.Unlock()
		parent.Children = append(parent.Children, span)
	}

	// 将当前创建的最新的 Span 作为最新的父Span，塞入衍生的Context中返回
	newCtx := context.WithValue(ctx, traceKey{}, span)
	return newCtx, span
}

// EndSpan 结束一个追踪跨度，计算耗时
func (s *Span) EndSpan() {
	s.EndTime = time.Now()
	s.DurationMs = s.EndTime.Sub(s.StartTime).Milliseconds()
}

// AddAttribute 将当前的 Span 记录为关键的元数据
func (s *Span) AddAttribute(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Attributes[key] = value
}

// ExportTraceToFile 当整个 Span 结束的时候，将其序列化保存为本地JSON文件
func ExportTraceToFile(rootSpan *Span, workDir string, sessionID string) error {
	// 工作目录下新建trace文件输出的目录
	traceDir := filepath.Join(workDir, "./claw", "traces")
	err := os.MkdirAll(traceDir, 0755)
	if err != nil {
		return fmt.Errorf("创建追踪文件目录失败: %w", err)
	}

	// 文件信息
	fileName := filepath.Join(traceDir, fmt.Sprintf("trace_%s_%d.json", sessionID, time.Now().Unix()))

	// 美化输出JSON
	data, err := json.MarshalIndent(rootSpan, "", " ")
	if err != nil {
		return fmt.Errorf("[ExportTraceToFile] 美化JSON失败，err：%w", err)
	}

	// 创建文件
	return os.WriteFile(fileName, data, 0644)
}
