# go-tiny-claw

go-tiny-claw 是一个基于 **驾驭工程（Steering Engineering）** 理念的 Go 语言 AI Agent 引擎。它连接大语言模型（LLM）与本地开发环境，赋予 AI 自主阅读、创建、修改代码和执行命令的能力。

## 核心架构

```
cmd/claw/main.go  →  入口：初始化 Provider、工具注册表，启动引擎
internal/
├── engine/          引擎主循环 & Reporter 接口
│   ├── loop.go          Agent 主循环（Thinking → Action → Tool Execution）
│   ├── repoter.go       Reporter 接口定义
│   └── terminal_reporter.go  CLI 终端输出实现
├── provider/          LLM 提供方适配层
│   ├── interface.go     LLMProvider 接口定义
│   ├── openAI.go        OpenAI / 智谱 (Zhipu) 兼容协议适配
│   └── claude.go        Anthropic Claude / 智谱兼容协议适配
├── context/           会话管理、上下文压缩、技能加载、自愈系统
│   ├── session.go      Session 管理（历史记录、工作记忆截取）
│   ├── compactor.go    上下文压缩（Token 窗口管理）
│   ├── composer.go     动态 System Prompt 构建
│   ├── skill.go        技能(Skill)加载与解析
│   └── recovery.go     工具执行失败自愈（Recovery Hint 注入）
├── schema/           内部数据模型
│   └── message.go      Message、ToolCall、ToolDefinition、ToolResult
├── tools/             Agent 可用工具集
│   ├── registry.go      工具注册与分发
│   ├── read_file_tool.go   读取文件
│   ├── writre_file_tool.go  写入文件
│   ├── edit_file.go      文件局部替换（四级模糊匹配降级）
│   └── bash_tool.go      Bash/PowerShell 命令执行
└── feishu/           飞书机器人集成
    └── bot.go           Feishu Bot 事件处理 & Reporter 实现
```

## 特性

- **Thinking-Action 双阶段循环**：支持慢思考（Reasoning）与行动（Tool Calling）分离
- **多模型支持**：适配 OpenAI 协议（智谱 GLM 系列）和 Anthropic Claude 协议
- **工具调用**：提供 read_file、write_file、edit_file、bash 四种内置工具
- **上下文压缩**：Compactor 自动管理 Token 窗口，防止 OOM
- **自愈机制**：RecoveryManager 在工具执行失败时智能注入恢复建议
- **计划模式 (Plan Mode)**：支持长程任务的 PLAN.md/TODO.md 状态持久化与断点续传
- **技能系统**：从 `.claw/skills/` 动态加载 SKILL.md 技能模板
- **飞书集成**：FeishuBot 将 Agent 能力接入飞书即时通讯
- **会话隔离**：全局 SessionManager 支持多会话并发管理

## 快速开始

1. 在项目根目录创建 `.env` 或导出环境变量：
   ```bash
   export ZHIPU_API_KEY=<你的智谱 API Key>
   ```

2. 运行：
   ```bash
   cd cmd/claw && go run main.go
   ```

## 环境变量

| 变量 | 说明 |
|------|------|
| `ZHIPU_API_KEY` | 智谱 AI API 密钥（必需） |
| `FEISHU_APP_ID` | 飞书应用 ID |
| `FEISHU_APP_SECRET` | 飞书应用 Secret |
| `FEISHU_ENCRYPT_KEY` | 飞书加密 Key |
| `FEISHU_VERIFY_TOKEN` | 飞书验证 Token |

## 工作区目录

- `workspace/` — Agent 可操作的工作目录
- `workspace/.claw/skills/` — 技能模板存放目录（每子目录下放 SKILL.md）
- `workspace/AGENTS.md` — 项目专属规范，自动注入 System Prompt

## 技术栈

- **语言**：Go 1.25+
- **LLM SDK**：OpenAI Go SDK v3 / Anthropic Go SDK
- **飞书 SDK**：larksuite/oapi-sdk-go
