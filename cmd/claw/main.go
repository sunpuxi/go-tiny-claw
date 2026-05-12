// cmd/claw/main.go
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/larksuite/oapi-sdk-go/v3/core/httpserverext"
	"github.com/sunpuxi/go-tiny-claw/internal/engine"
	"github.com/sunpuxi/go-tiny-claw/internal/feishu"
	"github.com/sunpuxi/go-tiny-claw/internal/provider"
	"github.com/sunpuxi/go-tiny-claw/internal/tools"
)

func main() {
	// 1. 初始化引擎依赖
	workDir, _ := os.Getwd()

	// 默认使用智谱 GLM-4
	if os.Getenv("ZHIPU_API_KEY") == "" {
		log.Fatal("请先导出 ZHIPU_API_KEY 环境变量")
	}
	llmProvider := provider.NewZhipuOpenAIProvider("glm-4.5-air")

	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))

	// 开启慢思考
	eng := engine.NewAgentEngine(llmProvider, registry, workDir, true)

	// 2. 初始化飞书 Bot 调度器
	bot := feishu.NewFeishuBot(eng)
	handler := httpserverext.NewEventHandlerFunc(bot.GetEventDispatcher())

	// 3. 注册路由并启动 HTTP 服务
	http.HandleFunc("/webhook/event", func(w http.ResponseWriter, r *http.Request) {
		// 飞书验证回调地址时会 POST 一个 JSON：{"challenge":"xxx"}
		// 需要原样返回 challenge 才能通过校验
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()

		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err == nil {
			if ch, ok := req["challenge"]; ok {
				log.Println("[Feishu] 响应 Challenge 校验")
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"challenge": ch,
				})
				return
			}
		}

		// 正常事件回调，恢复 body 后交由 SDK handler 处理
		r.Body = io.NopCloser(bytes.NewReader(body))
		handler(w, r)
	})

	port := ":48080"
	log.Printf("🚀 go-tiny-claw 飞书服务端已启动，正在监听 %s 端口\n", port)

	err := http.ListenAndServe(port, nil)
	if err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}
