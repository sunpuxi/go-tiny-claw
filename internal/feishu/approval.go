package feishu

import (
	"fmt"
	"log"
	"regexp"
	"sync"
)

// ApprovalResult 审批结果包
type ApprovalResult struct {
	Allowed bool
	Reason  string
}

// ApprovalManager 统一管理当前正在等待人类审批的任务
type ApprovalManager struct {
	mu sync.RWMutex
	// Key 是用于审批的唯一 TaskID，Value 是接收审批结果的 Channel
	pendingTasks map[string]chan ApprovalResult
}

// 全局单例，方便在 Registry Middleware 和 Feishu Webhook 之间共享状态
var GlobalApprovalMgr = &ApprovalManager{
	pendingTasks: make(map[string]chan ApprovalResult),
}

// WaitForApproval 发送飞书通知，并阻塞当前协程等待回调结果
func (m *ApprovalManager) WaitForApproval(taskID string, toolName string, args string, reporter *FeishuReporter) (bool, string) {
	// 1. 创建用于阻塞当前引擎协程的 channel (容量为 1 防止死锁)
	ch := make(chan ApprovalResult, 1)

	m.mu.Lock()
	m.pendingTasks[taskID] = ch
	m.mu.Unlock()

	// 2. 通过 Reporter 向飞书发送请求信息
	// (在实际的高级应用中，这里可以构建一张带有交互 Button 的精致飞书卡片)
	noticeMsg := fmt.Sprintf(`⚠️ **高危操作审批请求**
Agent 试图执行以下动作:
- 工具: %s
- 参数: %s

任务 ID: **%s**

👉 请在此消息下方回复 "approve %s" 或 "reject %s" 来决定是否放行。`, toolName, args, taskID, taskID, taskID)

	// 注意：因为 Middleware 的签名里没有带 Reporter，我们在 main.go 里初始化时必须把 reporter 传进来
	if reporter != nil {
		reporter.sendMsg(noticeMsg)
	} else {
		// 回退到终端打印 (兼容本地 CLI 模式)
		fmt.Printf("\n\033[31m[需要审批 TaskID: %s]\033[0m %s\n", taskID, noticeMsg)
	}

	log.Printf("[Approval] 已发送审批请求 (TaskID: %s)，协程挂起等待...\n", taskID)

	// 3. 【驾驭核心】：死死阻塞，等待飞书 Webhook 唤醒！
	result := <-ch

	// 4. 获取到结果后，清理内存资源
	m.mu.Lock()
	delete(m.pendingTasks, taskID)
	m.mu.Unlock()

	return result.Allowed, result.Reason
}

// ResolveApproval 由飞书 Webhook 回调触发，向 channel 发送信号解开阻塞
func (m *ApprovalManager) ResolveApproval(taskID string, allowed bool, reason string) {
	m.mu.RLock()
	ch, exists := m.pendingTasks[taskID]
	m.mu.RUnlock()

	if exists {
		log.Printf("[Approval] 收到来自飞书的审批结果 (TaskID: %s, Allowed: %v)\n", taskID, allowed)
		ch <- ApprovalResult{Allowed: allowed, Reason: reason}
	} else {
		log.Printf("[Approval] 找不到对应的 TaskID: %s，可能已超时或处理完毕\n", taskID)
	}
}

// IsDangerousCommand 简单的正则检查黑名单，判断该工具调用是否需要审批
func IsDangerousCommand(toolName string, args string) bool {
	// 对于纯读取的工具，默认 YOLO 模式，全部放行
	if toolName != "bash" && toolName != "write_file" && toolName != "edit_file" {
		return false
	}

	// 针对 bash 的高危模式匹配
	if toolName == "bash" {
		dangerousPatterns := []string{
			`rm\s+-r`, // 级联删除
			`sudo\s+`, // 提权
			`drop\s+`, // 数据库删除
			`>.*\.go`, // 恶意覆盖源代码
		}
		for _, p := range dangerousPatterns {
			matched, _ := regexp.MatchString(p, args)
			if matched {
				return true
			}
		}
	}
	return false
}
