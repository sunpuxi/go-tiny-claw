package tools

import (
	"context"
	"encoding/json"
	"fmt"
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

// MiddlewareFunc 是工具执行前后进行拦截的中间件函数
type MiddlewareFunc func(ctx context.Context, call schema.ToolCall) (allowed bool, rejectReason string)

// Registry 定义了工具的注册与分发接口
type Registry interface {
	// Register 挂载一个新的工具到系统中
	Register(tool BaseTool)

	// Use 挂载一个中间件到系统中
	Use(middleware MiddlewareFunc)

	// GetAvailableTools 返回当前系统挂载的所有工具的 Schema，供 Main Loop 交给 Provider
	GetAvailableTools() []schema.ToolDefinition

	// Execute 实际路由并执行模型请求的工具调用
	Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult
}

// registryImpl 是 Registry 接口的默认实现
type registryImpl struct {
	// 使用 map 以工具的 Name 作为 Key 进行快速 O(1) 路由查找
	tools map[string]BaseTool
	mw    []MiddlewareFunc
}

func NewRegistry() Registry {
	return &registryImpl{
		tools: make(map[string]BaseTool),
		mw:    make([]MiddlewareFunc, 0),
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

func (r *registryImpl) GetAvailableTools() []schema.ToolDefinition {
	var defs []schema.ToolDefinition
	for _, tool := range r.tools {
		defs = append(defs, tool.Definition())
	}
	return defs
}

func (r *registryImpl) Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult {
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

	// 2. 执行工具逻辑：将原始的 JSON 字节流直接丢给具体工具
	output, err := tool.Execute(ctx, call.Arguments)

	// 3. 封装结果：将执行结果或底层物理错误封装后返回给 Main Loop
	if err != nil {
		errMsg := fmt.Sprintf("Error executing %s: %v", call.Name, err)
		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     errMsg,
			IsError:    true,
		}
	}

	return schema.ToolResult{
		ToolCallID: call.ID,
		Output:     output,
		IsError:    false,
	}
}
