package engine

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/sunpuxi/go-tiny-claw/internal/schema"
)

// ReminderInjector 负责在运行时监控上下文，并在模型陷入执念时动态注入强力打断信息
type ReminderInjector struct {
	// 用于记录连续失败的工具调用指纹 (ToolName + Arguments 的 Hash)
	consecutiveFailures map[string]int
}

func NewReminderInjector() *ReminderInjector {
	return &ReminderInjector{
		consecutiveFailures: make(map[string]int),
	}
}

// generateFingerprint 生成工具调用的唯一指纹，用于判断大模型是否在重复相同的动作
func generateFingerprint(toolName string, args []byte) string {
	hasher := md5.New()
	hasher.Write([]byte(toolName))
	hasher.Write(args)
	return hex.EncodeToString(hasher.Sum(nil))
}

// CheckAndInject 分析本轮的执行结果，决定是否要在 Context 尾部追加 Reminder
// 返回的 schema.Message 将作为最新的用户输入，强制大模型优先阅读。
func (r *ReminderInjector) CheckAndInject(lastToolCall schema.ToolCall, lastResult schema.ToolResult) *schema.Message {
	fingerprint := generateFingerprint(lastToolCall.Name, lastToolCall.Arguments)

	// 如果工具执行成功，说明 Agent 在这条路径上走通了，清空所有失败计数器
	if !lastResult.IsError {
		r.consecutiveFailures = make(map[string]int)
		return nil
	}

	// 如果执行失败，累加该特征的失败次数
	r.consecutiveFailures[fingerprint]++
	failCount := r.consecutiveFailures[fingerprint]

	log.Printf("[Reminder] 监控到工具 %s 执行失败，该参数特征连续失败次数: %d\n", lastToolCall.Name, failCount)

	// 【驾驭底线】：触发死循环打断机制！
	if failCount >= 3 {
		log.Println("[Reminder] ⚠️ 触发死循环干预！注入强力修正指令。")

		// 构造一条极其严厉的行动指南
		nudgeMsg := fmt.Sprintf(`[SYSTEM REMINDER 警告] 
你似乎陷入了死循环。你刚刚连续 %d 次使用相同的参数调用了 '%s' 工具，并且都失败了。
请立即停止这种无效的重试！你的注意力被当前的报错过度吸引了。
你需要：
1. 停止猜测参数。跳出当前的局部思维。
2. 彻底改变你的策略。
3. 如果你确实无法通过系统工具解决当前问题，请直接结束任务并向用户说明你需要什么人工帮助，而不是继续盲目消耗 API 资源尝试。`, failCount, lastToolCall.Name)

		return &schema.Message{
			Role:    schema.RoleUser, // 【核心】必须是 RoleUser，以保证在下一次 API 请求时拥有最高的近因效应权重
			Content: nudgeMsg,
		}
	}

	return nil
}
