package context

import (
	"fmt"
	"log"

	"github.com/sunpuxi/go-tiny-claw/internal/schema"
)

// Compactor 负责监控和压缩上下文内存，防止大模型发生 OOM
type Compactor struct {
	MaxChars       int // 触发压缩的最大字符数阈值 (水位线，可参考使用的大模型的token窗口大小)
	RetainLastMsgs int // Working Memory 保护区：最近的 N 条消息
}

func NewCompactor(maxChars int, retainLastMsgs int) *Compactor {
	return &Compactor{
		MaxChars:       maxChars,
		RetainLastMsgs: retainLastMsgs,
	}
}

// Compact 接收准备发送给大模型的消息数组。
// 如果总长度超标，对远期历史区进行全量掩码 (Masking)，对短期保护区进行超长局部截断 (Truncation)。
func (c *Compactor) Compact(msgs []schema.Message) []schema.Message {
	currentLength := EstimateLength(msgs)

	// 如果没有超过水位线，直接返回原数组 (大多数情况下的正常路径)
	if currentLength < c.MaxChars {
		return msgs
	}

	log.Printf("[Compactor] ⚠️ 内存告警：当前上下文长度 (%d 字符) 超过阈值 (%d)，触发压缩清理...\n", currentLength, c.MaxChars)

	var compacted []schema.Message
	msgCount := len(msgs)

	// 计算受保护的 Working Memory 起始索引
	protectStartIndex := msgCount - c.RetainLastMsgs
	if protectStartIndex < 0 {
		protectStartIndex = 0
	}

	for i, msg := range msgs {
		// 1. 系统提示词 (System Prompt) 绝对不能动，直接保留
		if msg.Role == schema.RoleSystem {
			compacted = append(compacted, msg)
			continue
		}

		// 我们必须拷贝一份新消息，因为在并发环境中直接修改原引用可能导致底层数据结构被污染
		newMsg := msg

		isInWorkingMemory := i >= protectStartIndex

		// 【核心驾驭逻辑】: 双重降级防线
		if msg.Role == schema.RoleUser && msg.ToolCallID != "" {
			// 对于工具的返回结果 (Observation/ToolResult)
			if !isInWorkingMemory {
				// 【第一道防线：远期历史】如果是早期对话，执行无情替换 (Full Masking)
				if len(msg.Content) > 200 {
					newMsg.Content = fmt.Sprintf("...[为了节省内存，早期的工具输出已被系统强制清理。原始长度: %d 字节]...", len(msg.Content))
				}
			} else {
				// 【第二道防线：短期记忆】即使处于近期保护区，只要单条内容过大，也必须截断防 OOM (Head-Tail Truncation)
				// 我们保留前 500 字符和后 500 字符（掐头去尾法，大模型通常只需要看开头报错和结尾总结）
				const maxKeep = 1000
				if len(msg.Content) > maxKeep {
					head := msg.Content[:500]
					tail := msg.Content[len(msg.Content)-500:]
					newMsg.Content = fmt.Sprintf("%s\n\n...[内容过长，中间 %d 字节已被系统截断]...\n\n%s", head, len(msg.Content)-maxKeep, tail)
				}
			}
		} else if msg.Role == schema.RoleAssistant && msg.Content != "" {
			// 对于大模型的冗长推理废话 (Thinking Trace)
			if !isInWorkingMemory && len(msg.Content) > 200 {
				newMsg.Content = "...[早期的推理思考过程已折叠]..."
			}
		}

		// 注意：我们绝不会去动 msg.ToolCalls，因为这是模型行动的证据，是维系逻辑链的关键！
		compacted = append(compacted, newMsg)
	}

	newLength := EstimateLength(compacted)
	log.Printf("[Compactor] ✅ 压缩完成。上下文长度从 %d 降至 %d 字符。\n", currentLength, newLength)

	return compacted
}

// EstimateLength 粗略计算当前上下文的总字符长度
func EstimateLength(msgs []schema.Message) int {
	length := 0
	for _, msg := range msgs {
		length += len(msg.Content)
		for _, tc := range msg.ToolCalls {
			length += len(tc.Name) + len(tc.Arguments)
		}
	}
	return length
}
