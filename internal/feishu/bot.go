// internal/feishu/bot.go
package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/ws"
	"github.com/sunpuxi/go-tiny-claw/internal/schema"
	"log"
	"os"
	"strings"

	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	ctxpkg "github.com/sunpuxi/go-tiny-claw/internal/context"
	"github.com/sunpuxi/go-tiny-claw/internal/engine"

	lark "github.com/larksuite/oapi-sdk-go/v3"
)

// FeishuBot 封装了飞书机器人的配置与核心业务流
type FeishuBot struct {
	client    *lark.Client
	appID     string
	appSecret string
	engine    *engine.AgentEngine // 持有核心引擎引用
	sess      *ctxpkg.Session
	r         *FeishuReporter
}

func NewFeishuBot(eng *engine.AgentEngine, sess *ctxpkg.Session) *FeishuBot {
	appID := os.Getenv("FEISHU_APP_ID")
	appSecret := os.Getenv("FEISHU_APP_SECRET")

	if appID == "" || appSecret == "" {
		log.Fatal("请设置 FEISHU_APP_ID 和 FEISHU_APP_SECRET")
	}

	// 实例化飞书官方客户端
	client := lark.NewClient(appID, appSecret)

	return &FeishuBot{
		client:    client,
		appID:     appID,
		appSecret: appSecret,
		engine:    eng,
		sess:      sess,
	}
}

// StartWebSocket 启动 WebSocket 长连接方式接收飞书事件（推荐方式）
// 优势：无需公网 IP、无需配置回调 URL、自动重连、部署简单
func (b *FeishuBot) StartWebSocket(ctx context.Context) error {
	log.Println("🔌 正在启动 WebSocket 长连接模式...")

	// 创建事件处理器（长连接模式下 verifyToken 和 encryptKey 可以为空）
	eventHandler := b.GetEventDispatcher()

	// 创建 WebSocket 客户端
	wsClient := ws.NewClient(
		b.appID,
		b.appSecret,
		ws.WithEventHandler(eventHandler),
		ws.WithLogLevel(larkcore.LogLevelInfo),
		ws.WithAutoReconnect(true), // 自动重连
	)

	log.Println("✅ WebSocket 客户端已创建，正在连接飞书服务器...")

	// 启动长连接（阻塞式调用，会一直运行直到连接断开或 context 取消）
	return wsClient.Start(ctx)
}

// GetEventDispatcher 用于注册到 HTTP 服务器，处理来自飞书的 POST 事件
func (b *FeishuBot) GetEventDispatcher() *dispatcher.EventDispatcher {
	encryptKey := os.Getenv("FEISHU_ENCRYPT_KEY")
	verifyToken := os.Getenv("FEISHU_VERIFY_TOKEN")

	// 使用官方 SDK 构建调度器，监听 "接收消息" 事件
	handler := dispatcher.NewEventDispatcher(verifyToken, encryptKey).
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			// 由于飞书消息体是 JSON，我们需要粗略地提取其中的文本内容。
			// 这里简单处理：去掉开头结尾的特殊转义字符和引用的机器人名字。
			contentStr := *event.Event.Message.Content
			contentStr = strings.TrimPrefix(contentStr, `{"text":"`)
			contentStr = strings.TrimSuffix(contentStr, `"}`)

			chatId := *event.Event.Message.ChatId
			log.Printf("[Feishu] 收到会话 %s 消息: %s\n", chatId, contentStr)

			// 【新增】：拦截人工审批的特殊口令
			if strings.HasPrefix(contentStr, "approve ") {
				taskID := strings.TrimPrefix(contentStr, "approve ")
				taskID = strings.TrimSpace(taskID)
				// 唤醒挂起的引擎协程！
				GlobalApprovalMgr.ResolveApproval(taskID, true, "人类管理员已批准操作")
				log.Printf("[Feishu] 会话 %s: ✅ 已为您批准任务 %s", chatId, taskID)
				return nil
			}

			if strings.HasPrefix(contentStr, "reject ") {
				taskID := strings.TrimPrefix(contentStr, "reject ")
				taskID = strings.TrimSpace(taskID) // 唤醒挂起的引擎协程，并反馈拒绝理由！
				GlobalApprovalMgr.ResolveApproval(taskID, false, "人类管理员认为该操作存在极高风险，已无情拒绝")
				log.Printf("[Feishu] 会话 %s: 🚫 已拒绝任务 %s", chatId, taskID)
				return nil
			}

			// 【驾驭并发】：收到消息后，绝不能阻塞 HTTP 回调。
			// 我们要为每个请求开启一个独立的 Goroutine 跑 Agent 任务！
			go b.handleAgentRun(chatId, contentStr)

			return nil
		}).
		OnP2MessageReadV1(func(ctx context.Context, event *larkim.P2MessageReadV1) error {
			// 消息已读事件，静默忽略（避免日志干扰）
			return nil
		})

	return handler
}

// handleAgentRun 是连接飞书与底层引擎的桥梁
func (b *FeishuBot) handleAgentRun(chatId string, prompt string) {
	// 为当前聊天窗口实例化一个专属的 Reporter
	reporter := &FeishuReporter{
		client: b.client,
		chatId: chatId,
	}

	// 添加prompt
	b.sess.Append(schema.Message{
		Content: prompt,
		Role:    schema.RoleUser,
	})

	// 启动引擎！
	err := b.engine.Run(context.Background(), b.sess, reporter)
	if err != nil {
		reporter.sendMsg(fmt.Sprintf("❌ Agent 运行崩溃: %v", err))
	}
}

type FeishuReporter struct {
	client *lark.Client
	chatId string
}

func (b *FeishuBot) Reporter() *FeishuReporter { return b.r }

// sendMsg 封装了调用飞书 OpenAPI 发送卡片/文本的操作
func (r *FeishuReporter) sendMsg(text string) {
	// 构建文本消息内容
	textContent := map[string]string{
		"text": text,
	}
	contentBytes, _ := json.Marshal(textContent)
	contentStr := string(contentBytes)

	msgReq := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(r.chatId).
			MsgType(larkim.MsgTypeText).
			Content(contentStr).
			Build()).
		Build()

	_, _ = r.client.Im.Message.Create(context.Background(), msgReq)
}

func (r *FeishuReporter) OnThinking(ctx context.Context) {
	// 仅发一个轻量级提示，避免飞书刷屏
	r.sendMsg("🤔 模型正在慢思考 (Thinking)...")
}

func (r *FeishuReporter) OnToolCall(ctx context.Context, toolName string, args string) {
	r.sendMsg(fmt.Sprintf("🛠️ **正在执行工具**：`%s`\n参数：`%s`", toolName, args))
}

func (r *FeishuReporter) OnToolResult(ctx context.Context, toolName string, result string, isError bool) {
	if isError {
		r.sendMsg(fmt.Sprintf("⚠️ **执行报错** (%s)：\n%s", toolName, result))
	} else {
		// 成功时仅汇报成功，不刷全量日志
		r.sendMsg(fmt.Sprintf("✅ **执行成功** (%s)", toolName))
	}
}

func (r *FeishuReporter) OnMessage(ctx context.Context, content string) {
	// 将模型最终的纯文本回答发给用户
	r.sendMsg(content)
}

// 编译时类型检查：确保 FeishuReporter 实现了 Reporter 接口
var _ engine.Reporter = (*FeishuReporter)(nil)
