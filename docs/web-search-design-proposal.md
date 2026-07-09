# go-tiny-claw 联网搜索功能设计方案

> **状态**: 草案 v1.0 | **日期**: 2026-07-08 | **作者**: Daniel

---

## 目录

1. [需求背景](#1-需求背景)
2. [业界主流方案调研](#2-业界主流方案调研)
3. [核心设计决策](#3-核心设计决策)
4. [架构设计](#4-架构设计)
5. [工具拆分设计](#5-工具拆分设计)
6. [搜索后端选型](#6-搜索后端选型)
7. [安全与风控](#7-安全与风控)
8. [成本与性能控制](#8-成本与性能控制)
9. [与现有架构的集成](#9-与现有架构的集成)
10. [分阶段实施建议](#10-分阶段实施建议)
11. [待讨论的开放问题](#11-待讨论的开放问题)

---

## 1. 需求背景

go-tiny-claw 是一个 ReAct 范式的 Agent 引擎，目前具备以下能力：

- **工具系统**: 基于 `BaseTool` 接口的可插拔工具注册（bash、read_file、write_file、edit_file、subagent）
- **LLM 接入**: 通过 `LLMProvider` 接口对接多种大模型（当前: 智谱 GLM-4.5）
- **安全中间件**: 前置门禁 Middleware + 环绕 AroundFunc（洋葱圈模型）
- **ReAct 引擎**: Think → Act → Observe 循环，支持工具并发执行
- **SubAgent**: 子智能体沙箱，只读注册表限制

当前缺少**联网搜索能力**，导致：

- Agent 无法获取实时信息（新闻、股价、天气等）
- 无法查阅最新文档和资料
- 知识截止日期之后的信息完全不可及
- SubAgent 探索范围仅限于本地工作区

---

## 2. 业界主流方案调研

### 2.1 五大平台的对比总结

| 平台 | 方案类型 | 工具拆分 | 搜索后端 | 特色 |
|------|---------|----------|---------|------|
| **Claude Code** | 内置工具 | `WebSearch` + `WebFetch` 分离 | Anthropic 服务端 | 模型驱动，无硬编码规则；MCP 生态扩展 |
| **OpenAI ChatGPT** | API 原生 | `web_search` 合并在 Responses API | OpenAI 自有 | 自动搜索或手动触发两种模式；内置 citation |
| **Coze** | 插件系统 | Bing Search + LinkReader 两插件 | Bing (免费) | 可视化 Workflow 编排；MCP Bridge 可导出 |
| **Dify** | 插件市场 | search + fetch 分离 | 多种第三方 | 开放插件架构；支持自定义 API 工具 |
| **LangChain** | 三方集成 | DuckDuckGo/SerpAPI/Tavily 任选 | 多后端 | Agent 框架内作为 Tool 注册；fallback 链模式 |

### 2.2 核心共识：Search + Fetch 分离

> **"Search 负责发现，Fetch 负责提取。"**

这是几乎所有主流 Agent 平台的共同选择（Claude Code、OpenClaw、OpenRouter、Gemini CLI、Crush、WeChat AI 等）。分离的理由：

- **职责清晰**: 搜索返回 URL+摘要，抓取返回正文，语义分明
- **安全边界**: 在 Fetch 层统一做 SSRF 防护、协议检查、大小限制
- **独立优化**: 可以独立替换搜索后端（DuckDuckGo → Tavily → Bing），不影响抓取逻辑
- **可测试性**: 两个工具可分别单测，Mock 更简单
- **缓存策略**: 搜索结果的缓存和网页内容的缓存可以有不同的 TTL

### 2.3 五阶段检索管线

业界标准的信息检索流程：

```
用户问题
    ↓
(1) SEARCH / 发现  —— 从 Web/索引中查找候选页面
    ↓
(2) SELECT / 选择  —— 挑选最相关的 1-N 个 URL
    ↓
(3) FETCH  / 抓取  —— 拉取页面/文档原始内容
    ↓
(4) EXTRACT/ 提纯  —— 正文提取、去噪、去重、分块、截断
    ↓
(5) SYNTHESIZE/合成 —— 带引用的总结性回答
```

> 注: 阶段 (2) 和 (5) 由 LLM 自主决策完成，(1)(3)(4) 由工具实现。

### 2.4 搜索策略：漏斗模式

来自 LobeHub web-search-guidelines 和 Crush agent 的最佳实践：

1. **先用已有知识**（0 次搜索）—— 训练数据中的知识优先
2. **针对性搜索**（1-3 次）—— 只为具体缺口搜索，不要宽泛探索
3. **深度抓取** —— 用 `web_fetch` 读取搜索命中的页面
4. **交叉验证** —— 多源对比，不同措辞重搜

关键原则：**多次精准搜索优于单次宽泛搜索**。

---

## 3. 核心设计决策

以下是针对 go-tiny-claw 的具体设计建议：

### 决策 1：采用 Split 模式

**建议**: 拆分为 `web_search` 和 `web_fetch` 两个独立工具。

这是整个方案的基石。理由已在 2.2 中详述。

### 决策 2：模型驱动，不做硬编码路由

**建议**: 不做意图分类、不做查询改写、不做硬编码的搜索策略。

遵循 Claude Code 的 "Less scaffolding, more model" 哲学：

- LLM 自行决定**何时**搜索、**搜索什么**、**如何综合结果**
- 不做 Intent Router → 增加延迟和错误率
- 不做 Query Rewriter → LLM 本身擅长此道
- 不做强制搜索触发规则 → 模型自会判断

工程层只提供优质的工具和安全的沙箱，策略交给模型。

### 决策 3：搜索后端可插拔

**建议**: 定义 `SearchProvider` 接口，支持多种后端，通过配置切换。

```
SearchProvider Interface
├── DuckDuckGo (免费，无需 API Key，适合开发/演示)
├── Tavily      (付费，专为 Agent 设计，质量最高)
├── SerpAPI     (付费，Google 搜索结果)
├── Brave Search (免费额度，隐私友好)
├── Bing API    (微软生态)
└── SearXNG     (自托管，完全隐私)
```

### 决策 4：优先集成到现有工具系统

**建议**: 新工具实现 `BaseTool` 接口，注册到现有 `Registry`，复用 Middleware 链路。

不需要新增任何架构概念，完全融入现有的工具注册-分发-执行体系。

---

## 4. 架构设计

### 4.1 整体架构图

```
┌─────────────────────────────────────────────────────────────────┐
│                        go-tiny-claw Agent Engine                 │
│  ┌───────────┐   ┌──────────┐   ┌──────────┐   ┌─────────────┐ │
│  │   Think   │ → │   Act    │ → │ Observe  │ → │  (loop)     │ │
│  └───────────┘   └──────────┘   └──────────┘   └─────────────┘ │
│                       │                           ↑              │
│                       ▼                           │              │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                    Tool Registry (工具注册表)                  │ │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌───────────────┐  │ │
│  │  │  bash    │ │read_file │ │write_file│ │ sub_agent     │  │ │
│  │  └──────────┘ └──────────┘ └──────────┘ └───────────────┘  │ │
│  │  ┌──────────────────┐ ┌───────────────────┐                 │ │
│  │  │  web_search  ★   │ │  web_fetch   ★    │   ← 新增       │ │
│  │  └──────────────────┘ └───────────────────┘                 │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                       │                           ↑              │
│                       ▼                           │              │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │              Middleware 中间件链 (复用)                        │ │
│  │  ┌───────────┐  ┌───────────┐  ┌────────────────────────┐  │ │
│  │  │ 危险命令  │→│ 飞书审批  │→│  联网安全 ★ (新增)      │  │ │
│  │  │ 拦截      │  │ 门禁      │  │  SSRF/域名/IP 检查     │  │ │
│  │  └───────────┘  └───────────┘  └────────────────────────┘  │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                       │                           ↑              │
│                       ▼                           │              │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                   SearchProvider 层 (新增)                    │ │
│  │  ┌───────────┐  ┌──────────┐  ┌──────────┐                 │ │
│  │  │DuckDuckGo │  │  Tavily  │  │ SearXNG  │  ...            │ │
│  │  │(免费默认) │  │(高质量)  │  │(自托管)  │                 │ │
│  │  └───────────┘  └──────────┘  └──────────┘                 │ │
│  └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

### 4.2 新增文件规划

```
internal/
├── tools/
│   ├── web_search.go          # web_search 工具实现
│   ├── web_fetch.go           # web_fetch 工具实现
│   ├── web_middleware.go      # 联网安全中间件 (SSRF防护等)
│   ├── web_search_test.go     # 单元测试
│   └── web_fetch_test.go      # 单元测试
├── search/                    # 新增 search 包
│   ├── interface.go           # SearchProvider 接口定义
│   ├── duckduckgo.go          # DuckDuckGo 实现
│   ├── tavily.go              # Tavily 实现
│   ├── brave.go               # Brave Search 实现
│   ├── cache.go               # 搜索结果缓存
│   ├── fetch.go               # HTTP 抓取 + 正文提取 + 安全校验
│   └── extract.go             # HTML → Markdown/纯文本 提取
└── config/
    └── search_config.go       # 搜索相关配置结构
```

---

## 5. 工具拆分设计

### 5.1 `web_search` 工具

**职责**: 搜索互联网，返回候选 URL 列表及摘要

**工具定义**:

```json
{
  "name": "web_search",
  "description": "搜索互联网获取最新信息。返回包含标题、URL 和摘要的搜索结果列表。
用于查找实时信息、最新文档、新闻事件等。注意：此工具只返回搜索结果摘要，
如需深入阅读某个页面，请使用 web_fetch 工具。",
  "input_schema": {
    "type": "object",
    "properties": {
      "query": {
        "type": "string",
        "description": "搜索关键词。保持简短精准，3-8 个词为佳。
对于复杂问题，建议分多次搜索而非一次宽泛搜索。"
      },
      "max_results": {
        "type": "integer",
        "description": "最大返回结果数，默认 5，最大 10",
        "default": 5
      },
      "time_range": {
        "type": "string",
        "enum": ["day", "week", "month", "year", "all"],
        "description": "时间范围过滤，默认 all",
        "default": "all"
      }
    },
    "required": ["query"]
  }
}
```

**输出格式**:

```
搜索结果 (共 5 条，耗时 0.3s):

1. [理解 Web Search 工具的设计模式](https://example.com/web-search-design)
   摘要: 本文详细介绍了 AI Agent 中 web_search 和 web_fetch 的拆分设计模式...
   时间: 2026-06-15

2. [Agent 联网搜索最佳实践](https://example.com/agent-search-best-practices)
   摘要: 从安全、性能、成本三个维度分析 Agent 联网搜索的工程化方案...
   时间: 2026-05-20

...
```

### 5.2 `web_fetch` 工具

**职责**: 抓取指定 URL 的网页内容，提取并返回可读文本

**工具定义**:

```json
{
  "name": "web_fetch",
  "description": "抓取指定 URL 的网页内容，提取正文并以 Markdown/纯文本格式返回。
用于深入阅读 web_search 返回的搜索结果页面。
注意：此工具只用于获取具体页面内容，不适合作为搜索工具使用。",
  "input_schema": {
    "type": "object",
    "properties": {
      "url": {
        "type": "string",
        "description": "要抓取的网页 URL。必须包含协议 (http/https)。"
      },
      "extract_mode": {
        "type": "string",
        "enum": ["markdown", "text"],
        "description": "提取模式：markdown 保留结构，text 纯文本。默认 markdown",
        "default": "markdown"
      },
      "max_length": {
        "type": "integer",
        "description": "返回内容的最大字符数，默认 8000，最大 32000",
        "default": 8000
      }
    },
    "required": ["url"]
  }
}
```

**输出格式**:

```
[web_fetch] https://example.com/web-search-design (13608 bytes → 截断至 8000 chars)

# 理解 Web Search 工具的设计模式

本文详细介绍了 AI Agent 中...

## 为什么需要拆分

...

> 引用块保留

...（返回完整 8000 字符的 Markdown）
```

### 5.3 工具协作流程

典型的 LLM 调用序列：

```
Turn 1:
  LLM: "用户问最新 Go 1.24 的泛型改进，我需要搜索"
  → web_search("Go 1.24 generics improvements 2026", max_results=5)

Turn 2 (收到搜索结果):
  LLM: "前 3 条结果看起来相关，让我深入阅读"
  → web_fetch("https://go.dev/blog/go1.24")       ─┐
  → web_fetch("https://example.com/go-generics")   ─┤ 并发执行
  → web_fetch("https://blog.golang.org/1.24")      ─┘

Turn 3 (收到页面内容):
  LLM: "综合三个来源，回答用户问题"
  → (无工具调用，输出带引用的最终回答，ReAct 循环结束)
```

---

## 6. 搜索后端选型

### 6.1 候选方案对比

| 方案 | 费用 | API Key | 结果质量 | 中文支持 | Agent优化 | 隐私 |
|------|------|---------|---------|---------|----------|------|
| **DuckDuckGo** | 免费 | 不需要 | ★★★ | ★★★ | ★★ | ★★★★ |
| **Tavily** | 免费1000/月 | 需要 | ★★★★★ | ★★★★ | ★★★★★ | ★★★ |
| **Brave Search** | 免费2000/月 | 需要 | ★★★★ | ★★★★ | ★★★ | ★★★★★ |
| **SerpAPI** | 付费 (~$50/月) | 需要 | ★★★★★ | ★★★★★ | ★★ | ★★ |
| **Bing API** | 付费 | 需要 | ★★★★ | ★★★★ | ★★ | ★ |
| **SearXNG** | 自托管免费 | 不需要 | ★★★ | ★★★ | ★★★ | ★★★★★ |
| **Google Custom Search** | 免费100/天 | 需要 | ★★★★★ | ★★★★ | ★★ | ★ |

### 6.2 推荐策略：分层默认

```
Phase 1 (MVP):    DuckDuckGo (免费，零配置，开箱即用)
                   ↓
Phase 2 (增强):    + Tavily (高质量 Agent 专用搜索，需要 API Key)
                   ↓
Phase 3 (生产):    + SearXNG (自托管，完全隐私可控)
                   + Brave Search (备用/补充)
```

**默认推荐 DuckDuckGo** 作为 MVP 首选：
- 完全免费，无需注册
- API 简单 (`duckduckgo-search` 库)
- 隐私友好
- 中文搜索可用
- 零配置启动

### 6.3 SearchProvider 接口设计

```go
// internal/search/interface.go

// SearchResult 单条搜索结果
type SearchResult struct {
    Title       string    `json:"title"`
    URL         string    `json:"url"`
    Snippet     string    `json:"snippet"`
    PublishedAt *time.Time `json:"published_at,omitempty"`
    Source      string    `json:"source"` // 来源引擎标识
}

// SearchResponse 搜索响应
type SearchResponse struct {
    Query       string         `json:"query"`
    Results     []SearchResult `json:"results"`
    TotalCount  int            `json:"total_count"`
    EngineName  string         `json:"engine_name"`
    CostMs      int64          `json:"cost_ms"`
}

// SearchProvider 搜索后端接口
type SearchProvider interface {
    // Name 返回搜索引擎名称 (用于日志和输出标识)
    Name() string

    // Search 执行搜索，按分数排序返回结果
    Search(ctx context.Context, query string, opts SearchOptions) (*SearchResponse, error)
}

// SearchOptions 搜索选项
type SearchOptions struct {
    MaxResults int       `json:"max_results"`  // 最大返回数 (1-20)
    TimeRange  string    `json:"time_range"`   // day/week/month/year/all
    Country    string    `json:"country"`      // 地区偏好 (如 "cn", "us")
    Language   string    `json:"language"`     // 语言偏好 (如 "zh", "en")
}
```

---

## 7. 安全与风控

### 7.1 `web_fetch` 安全三层防护

**第 1 层: 工具层参数校验**
- 只允许 `http://` 和 `https://` 协议
- 拒绝 `file://`、`ftp://`、`gopher://` 等
- URL 格式合法性校验

**第 2 层: 网络层 SSRF 防护**
- DNS 解析后检查 IP 是否为内网地址
- 拦截列表: `127.0.0.0/8`、`10.0.0.0/8`、`172.16.0.0/12`、`192.168.0.0/16`、`169.254.0.0/16`
- 可选：通过 `ALLOW_PRIVATE_IP` 环境变量开关（开发环境允许访问 localhost 服务）

**第 3 层: 中间件层审计 + 审批**
- 复用现有的 Middleware 机制
- 对 `web_fetch` 调用记录完整审计日志
- 可在飞书审批流中加入敏感 URL 拦截规则

### 7.2 `web_search` 安全考虑

- 搜索查询本身风险较低，不需要特别沙箱
- 主要防护在 `web_fetch` 层（因为抓取 URL 来自搜索结果，不可信）

### 7.3 内容安全

```
┌─────────────┐     ┌──────────────┐     ┌─────────────┐
│  HTTP 响应   │ →   │ 正文提取      │ →   │ 长度截断     │
│  (原始HTML)  │     │ (HTML→Markdown)│     │ (max 8000)  │
└─────────────┘     └──────────────┘     └─────────────┘
                            │
                            ▼
                     ┌──────────────┐
                     │ 注入防护检查  │  ← 可选：检测内容是否包含
                     │ (Prompt Inj) │     prompt injection 模式
                     └──────────────┘
```

---

## 8. 成本与性能控制

### 8.1 缓存策略

| 缓存对象 | TTL | 策略 |
|---------|-----|------|
| 搜索结果 | 15 分钟 | 以 query + engine 为 key 的内存缓存 |
| 网页内容 | 15 分钟 | 以 URL 为 key 的内存缓存 |
| DNS 解析 | 系统默认 | 使用操作系统 DNS 缓存 |

> 注意: 当前项目规模，内存缓存足够（使用 `sync.Map` 或带 TTL 的小型缓存库如 `go-cache`）。不需要引入 Redis。

### 8.2 资源限制

| 限制项 | 默认值 | 说明 |
|--------|-------|------|
| `web_fetch` 超时 | 30s | 单个页面抓取的最大等待时间 |
| `web_search` 超时 | 15s | 单次搜索的最大等待时间 |
| 抓取内容上限 | 8,000 chars | 返回给 LLM 的最大字符数 |
| 最大重定向次数 | 3 | 超过则返回错误 |
| 并发抓取限制 | 5 | engine 层已支持并发，无需额外限制 |
| 页面最大下载 | 5MB | 超过此大小的页面截断处理 |
| 搜索速率限制 | 10次/分钟 | 保护搜索后端不被封禁 |

### 8.3 Token 成本估算

以项目当前使用的 GLM-4.5 为例：

| 场景 | Token 消耗 |
|------|-----------|
| 搜索结果的 context 占用 | ~500-1000 tokens (5 条结果) |
| 单个网页抓取 context 占用 | ~1500-3000 tokens (8000 chars markdown) |
| 典型一次搜索+抓取 | ~3000-5000 tokens |
| 深度研究（3次搜索+10次抓取） | ~15000-25000 tokens |

**对策**:
- System Prompt 中明确引导模型"先用已有知识，再搜索"
- 搜索结果默认返回摘要（不包含完整正文）
- `web_fetch` 默认截断至 8000 字符
- 压缩器(Compactor) 会自动压缩历史上下文

---

## 9. 与现有架构的集成

### 9.1 工具注册（与现有模式完全一致）

```go
// main.go 中的注册方式（与现有工具无异）
registry.Register(tools.NewWebSearchTool(searchProvider))
registry.Register(tools.NewWebFetchTool())

// 子 Agent 的只读注册表也可以加上搜索
readOnlyRegistry.Register(tools.NewWebSearchTool(searchProvider))
readOnlyRegistry.Register(tools.NewWebFetchTool())
```

### 9.2 中间件集成

```go
// 复用现有 Middleware 机制挂载联网安全中间件
registry.Use(func(ctx context.Context, call schema.ToolCall) (bool, string) {
    if call.Name == "web_fetch" {
        // SSRF 检查、协议检查等
        if blocked, reason := webMiddleware.CheckURL(call.Arguments); blocked {
            return false, reason
        }
    }
    return true, ""
})

// 环绕中间件：统计搜索/抓取耗时和成功率
registry.UseAround(func(ctx context.Context, call schema.ToolCall, next func() (string, error)) (string, error) {
    start := time.Now()
    result, err := next()
    log.Printf("[WebTool] %s 耗时: %v, 错误: %v", call.Name, time.Since(start), err)
    return result, err
})
```

### 9.3 System Prompt 注入

建议在 `PromptComposer.Build()` 中注入联网搜索的使用指导：

```markdown
## 联网搜索能力

你可以使用以下工具访问互联网：
- `web_search`: 搜索最新信息，返回候选页面列表
- `web_fetch`: 抓取指定网页的完整内容

使用原则：
1. 优先使用你的训练知识回答问题
2. 只有当需要实时信息、最新文档或你知识截止日期之后的信息时，才使用搜索
3. 采用多次精准搜索而非单次宽泛搜索
4. 搜索后使用 web_fetch 深入阅读相关页面
5. 始终引用信息来源 URL
```

### 9.4 与 SubAgent 的配合

SubAgent 的只读注册表加入搜索工具后，可以实现：

```
主 Agent: "调查一下 Go 1.24 的泛型改进"
    ↓
SubAgent (拿到搜索只读权限):
    → web_search("Go 1.24 generics type parameters improvements")
    → web_fetch(result_url_1)
    → web_fetch(result_url_2)
    → 汇总报告给主 Agent
    ↓
主 Agent: 基于 SubAgent 的报告，进行代码修改
```

---

## 10. 分阶段实施建议

### Phase 1: MVP（最小可行，1-2 天）

**目标**: 能搜、能抓、能返回

- [ ] 实现 `SearchProvider` 接口
- [ ] 实现 DuckDuckGo 搜索后端
- [ ] 实现 `web_search` 工具（BaseTool 接口）
- [ ] 实现 `web_fetch` 工具（HTTP GET + HTML→文本提取）
- [ ] 集成到 main.go 注册工具
- [ ] 基本测试：搜索 "今天天气"，抓取 Wikipedia 页面

**技术选型**:
- DuckDuckGo: 直接 HTTP 调用其 Instant Answer API 或使用 `duckduckgo-search` 的 scrape 方式
- HTML 提取: 使用 `github.com/JohannesKaufmann/html-to-markdown` 或 `github.com/PuerkitoBio/goquery`

### Phase 2: 安全增强（1 天）

**目标**: 生产可用

- [ ] SSRF 防护（内网 IP 拦截）
- [ ] 协议白名单检查
- [ ] 超时、重定向、大小限制
- [ ] 内容截断策略
- [ ] 缓存实现（内存 + TTL）
- [ ] 搜索速率限制

### Phase 3: 扩展与优化（按需）

**目标**: 企业级可用

- [ ] Tavily 搜索后端集成（高质量 Agent 搜索）
- [ ] Brave Search 后端集成（隐私场景）
- [ ] SearXNG 后端集成（自托管）
- [ ] 搜索结果缓存
- [ ] 搜索质量监控（成功率、延迟、命中率）
- [ ] 可配置的域名黑白名单
- [ ] Prompt Injection 检测

---

## 11. 待讨论的开放问题

以下是需要做决策的开放问题，建议逐项讨论：

### Q1: DuckDuckGo 作为默认后端的合规性？

DuckDuckGo 没有官方搜索 API。社区通常通过解析其 HTML 页面或使用 Instant Answer API 来获取结果。需要考虑：
- 使用条款是否允许程序化调用？
- 是否需要增加 User-Agent 标识？
- 速率限制如何设定？

**备选**: 如果合规性存疑，可以直接从 Brave Search（免费 2000次/月，有官方 API）起步。

### Q2: 是否需要搜图/搜视频能力？

当前方案只覆盖文本搜索。如果未来需要：
- 图片搜索 → 需要支持图片 URL 返回 + 多模态模型配合
- 视频搜索 → 需要额外的视频搜索后端

**建议**: Phase 1-2 只做文本搜索，图片搜索在需求明确后再扩展。

### Q3: 是否需要 "Search-and-Answer" 合并工具？

某些平台（如 Tavily、OpenAI）提供了 `search_and_answer` 的合并工具，一次调用同时完成 "搜索+LLM总结"。

**建议**: 暂不提供。保持工具语义纯粹（工具做检索，LLM 做总结），这样更灵活且 Token 更省（不用每次搜索都触发 LLM 总结）。

### Q4: 是否给 SubAgent 开放搜索能力？

给 SubAgent 开放搜索可能带来：
- **优势**: SubAgent 可以自主做深度调研
- **风险**: 搜索调用次数不可控，可能消耗过多资源

**建议**: 默认不给 SubAgent 开放，但允许通过配置开启。也可以在 System Prompt 中限制 SubAgent 的搜索次数。

### Q5: 搜索结果是否需要去重/排序？

不同搜索后端返回的结果质量参差不齐。是否需要：
- 跨引擎去重（如果配置了多个后端）
- 域名去重（同一个域名只保留最高分的一条）
- 内容去重（相似摘要合并）

**建议**: Phase 1 不做。DuckDuckGo 返回的结果本身质量可接受。多后端场景（Phase 3）再做。

### Q6: 配置方式？

配置项包括：搜索引擎选择、API Key、缓存 TTL、速率限制等。是：
- A) 环境变量方式（`WEB_SEARCH_ENGINE=dockerduckgo`）
- B) YAML 配置文件方式（`search.yaml`）
- C) 复用现有的 `config.yaml` 热加载机制

**建议**: 采用 C，复用现有 `config.yaml` + 热加载机制。API Key 类的敏感信息用环境变量。

---

## 附录

### A. 参考资料

| 资料 | 链接 |
|------|------|
| Claude Code Tool System Architecture | https://github.com/openedclaude/claude-reviews-claude |
| Claude Code Web Search MCP Ecosystem | https://github.com/WaynePluto/local-web-search-agent |
| OpenAI Web Search Tool Docs | https://developers.openai.com/api/docs/guides/tools-web-search |
| OpenClaw Web Search Guide | https://docs.openclaw.ai/tools/web |
| Tavily Agent Toolkit | https://docs.tavily.com/examples/agent-toolkit/tools |
| 微信 Agent 联网检索工程化指南 | https://mp.weixin.qq.com/s?__biz=MzYyNTM4MDcyMA==&mid=2247485130 |
| OpenRouter Consistent Web Tools | https://openrouter.ai/blog/announcements/agentic-web-tools/ |
| LobeHub Web Search Guidelines | https://lobehub.com/skills/mcerqua-jam-skills-web-search-guidelines |
| Gemini CLI Web Tools Tutorial | https://github.com/google-gemini/gemini-cli/blob/main/docs/cli/tutorials/web-tools.md |

### B. 关键术语

| 术语 | 说明 |
|------|------|
| Split Pattern | 将 web_search 和 web_fetch 拆分为两个独立工具的设计模式 |
| SSRF | Server-Side Request Forgery，服务端请求伪造攻击 |
| Search Provider | 搜索后端实现，如 DuckDuckGo、Tavily 等 |
| ReAct | Reasoning + Acting，推理-行动循环 Agent 范式 |
| Funnel Strategy | 从宽到窄的信息筛选漏斗策略 |
