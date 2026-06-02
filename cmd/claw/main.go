// cmd/claw/main.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/larksuite/oapi-sdk-go/v3/core/httpserverext"
	"github.com/sunpuxi/go-tiny-claw/config"
	"github.com/sunpuxi/go-tiny-claw/internal/feishu"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	ctxpkg "github.com/sunpuxi/go-tiny-claw/internal/context"
	"github.com/sunpuxi/go-tiny-claw/internal/engine"
	"github.com/sunpuxi/go-tiny-claw/internal/provider"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
	"github.com/sunpuxi/go-tiny-claw/internal/tools"
)

// cmd/claw/main.go

func main() {
	if os.Getenv("ZHIPU_API_KEY") == "" {
		log.Fatal("请先导出 ZHIPU_API_KEY 环境变量")
	}

	workDir, _ := os.Getwd()
	workDir += "/workspace"

	llmProvider := provider.NewZhipuOpenAIProvider("glm-4.5-air")
	reporter := engine.NewTerminalReporter()

	// 初始化危险命令配置，启动时加载 config.yaml，每 5s 热加载
	dc := config.InitDangerConfig("config/config.yaml")
	dc.StartWatching(5 * time.Second)
	defer dc.Stop()

	// 【防御沙箱】为子智能体准备受限的只读注册表
	readOnlyRegistry := tools.NewRegistry()
	readOnlyRegistry.Register(tools.NewReadFileTool(workDir))
	readOnlyRegistry.Register(tools.NewBashTool(workDir)) // 允许简单的 grep 等搜索操作

	// 主 Agent 的工具箱
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))

	// 引擎实现
	eng := engine.NewAgentEngine(llmProvider, registry, false, false)

	// 绑定 subAgent工具
	registry.Register(tools.NewSubagentTool(eng, readOnlyRegistry, reporter))

	// 假设一个bot绑定一个session
	sessionID := "test_command_intercept_001"
	sess := ctxpkg.GlobalSessionMgr.GetOrCreate(sessionID, workDir)
	sess.Append(schema.Message{Role: schema.RoleUser, Content: ""})

	bot := feishu.NewFeishuBot(eng, sess)
	handler := httpserverext.NewEventHandlerFunc(bot.GetEventDispatcher())

	// 【核心注入】注册安全拦截 Middleware
	registry.Use(func(ctx context.Context, call schema.ToolCall) (bool, string) {
		argsStr := string(call.Arguments)

		// 检查是否命中高危特征库
		if feishu.IsDangerousCommand(call.Name, argsStr) {
			taskID := call.ID // 使用大模型生成的唯一 ToolCallID 作为 TaskID

			// 挂起当前协程，发送消息给飞书，死死等待人类的审批！
			allowed, reason := feishu.GlobalApprovalMgr.WaitForApproval(taskID, call.Name, argsStr, bot.Reporter())

			if !allowed {
				return false, reason // 拒绝，将理由传回给大模型
			}
			return true, "" // 同意，放行底层工具
		}

		// 没命中黑名单，直接 YOLO 放行
		return true, ""
	})

	// 3. 注册路由并启动 HTTP 服务
	http.HandleFunc("/webhook/event", func(w http.ResponseWriter, r *http.Request) {
		// 飞书验证回调地址时会 POST 一个 JSON：{"challenge":"xxx"}
		// 需要原样返回 challenge 才能通过校验
		body, err := io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			log.Printf("[Feishu] 读取请求体失败: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err == nil {
			if ch, ok := req["challenge"]; ok {
				log.Println("[Feishu] 响应 Challenge 校验")
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(map[string]interface{}{
					"challenge": ch,
				}); err != nil {
					log.Printf("[Feishu] 编码 Challenge 响应失败: %v", err)
				}
				return
			}
		}

		// 正常事件回调，恢复 body 后交由 SDK handler 处理
		r.Body = io.NopCloser(bytes.NewReader(body))

		// 通过上面的 handler 中的feushuRepoter进行消息的回传
		handler(w, r)
	})

	port := ":48080"
	log.Printf("🚀 go-tiny-claw 飞书服务端已启动，正在监听 %s 端口\n", port)

	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
