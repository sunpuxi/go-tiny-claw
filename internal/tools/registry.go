package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/sunpuxi/go-tiny-claw/internal/observability"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
	"log"
)

type ToolRegistry interface {

	// GetAvailableTools 获取可用的工具列表
	GetAvailableTools() []schema.ToolDefinition

	// Execute 执行工具调用
	Execute(ctx context.Context, tool schema.ToolCall) schema.ToolResult
}

// BaseTool 是所有具体工具必须实现的通用接口
type BaseTool interface {
	// Name 返回工具的全局唯一名称 (大模型通过这个名字调用它)
	Name() string

	// Definition 返回用于提交给大模型的工具元信息和参数 JSON Schema
	Definition() schema.ToolDefinition

	// Execute 接收大模型吐出的 JSON 参数，执行具体业务逻辑
	// 注意：参数是 json.RawMessage，反序列化由各个具体工具内部自行处理
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

// MiddlewareFunc 是工具执行前后进行拦截的中间件函数（前置门禁）
type MiddlewareFunc func(ctx context.Context, call schema.ToolCall) (allowed bool, rejectReason string)

// AroundFunc 是环绕工具执行的中间件函数，采用洋葱圈模型
// next 是下一层中间件（或最终的工具执行逻辑），返回工具的原始输出和错误
type AroundFunc func(ctx context.Context, call schema.ToolCall, next func() (string, error)) (string, error)

// Registry 定义了工具的注册与分发接口
type Registry interface {
	// Register 挂载一个新的工具到系统中
	Register(tool BaseTool)

	// Use 挂载一个前置门禁中间件到系统中
	Use(middleware MiddlewareFunc)

	// UseAround 挂载一个环绕中间件到系统中（洋葱圈模型，先注册的最外层）
	UseAround(around AroundFunc)

	// GetAvailableTools 返回当前系统挂载的所有工具的 Schema，供 Main Loop 交给 Provider
	GetAvailableTools() []schema.ToolDefinition

	// Execute 实际路由并执行模型请求的工具调用
	Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult
}

// registryImpl 是 Registry 接口的默认实现
type registryImpl struct {
	// 使用 map 以工具的 Name 作为 Key 进行快速 O(1) 路由查找
	tools  map[string]BaseTool
	mw     []MiddlewareFunc
	around []AroundFunc
}

func NewRegistry() Registry {
	return &registryImpl{
		tools:  make(map[string]BaseTool),
		mw:     make([]MiddlewareFunc, 0),
		around: make([]AroundFunc, 0),
	}
}

func (r *registryImpl) Register(tool BaseTool) {
	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		log.Printf("[Warning] 工具 '%s' 已经被注册，将被覆盖。\n", name)
	}
	r.tools[name] = tool
	log.Printf("[Registry] 成功挂载工具: %s\n", name)
}

func (r *registryImpl) Use(middleware MiddlewareFunc) {
	r.mw = append(r.mw, middleware)
}

func (r *registryImpl) UseAround(around AroundFunc) {
	r.around = append(r.around, around)
}

func (r *registryImpl) GetAvailableTools() []schema.ToolDefinition {
	var defs []schema.ToolDefinition
	for _, tool := range r.tools {
		defs = append(defs, tool.Definition())
	}
	return defs
}

func (r *registryImpl) Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult {
	// 【埋点5】追踪工具的执行
	ctx, toolSpan := observability.StartSpan(ctx, "tools.Execute")
	toolSpan.AddAttribute("tool.name", call.Name)
	toolSpan.AddAttribute("arguments", string(call.Arguments))
	defer toolSpan.EndSpan()

	// 1. 路由查找：如果在注册表中找不到该工具，这是模型产生了幻觉，直接向模型抛出错误
	tool, exists := r.tools[call.Name]
	if !exists {
		errMsg := fmt.Sprintf("Error: 系统中不存在名为 '%s' 的工具。", call.Name)
		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     errMsg,
			IsError:    true, // 标记为错误，模型看到后会尝试纠正
		}
	}

	// 工具执行之前的审核(返回拒绝原因，强制LLM阅读并纠错)
	for _, mw := range r.mw {
		allowed, reason := mw(ctx, call)
		if !allowed {
			log.Printf("[Registry] ⚠️ 工具 %s 被 Middleware 拦截: %s\n", call.Name, reason)
			return schema.ToolResult{
				ToolCallID: call.ID,
				Output:     fmt.Sprintf("工具执行请求被拦截，原因: %s", reason),
				IsError:    true,
			}
		}
	}

	// 2. 构建执行链：around 中间件洋葱圈 + 底层工具执行
	// 最内层是工具的实际执行逻辑
	core := func() (string, error) {
		return tool.Execute(ctx, call.Arguments)
	}

	// 倒序包裹，确保先注册的 around 中间件在最外层
	for i := len(r.around) - 1; i >= 0; i-- {
		a := r.around[i]
		prev := core
		core = func() (string, error) {
			return a(ctx, call, prev)
		}
	}
	output, err := core()

	// 3. 封装结果：将执行结果或底层物理错误封装后返回给 Main Loop
	if err != nil {
		errMsg := fmt.Sprintf("Error executing %s: %v", call.Name, err)
		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     errMsg,
			IsError:    true,
		}
	}

	// 将执行工具的结果输出添加进埋点信息中
	toolSpan.AddAttribute("output_preview", truncate(output, 100))

	return schema.ToolResult{
		ToolCallID: call.ID,
		Output:     output,
		IsError:    false,
	}
}

// 截取部分的执行结果即可，防止追踪文件的大小膨胀
func truncate(s string, maxLength int) string {
	if len(s) > maxLength {
		s = s[:maxLength] + "..."
	}
	return s
}
