// internal/mcp/manager.go
// MCP 服务器管理器：加载配置、启动所有 MCP Server、聚合工具、注册到 Registry
package mcp

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/sunpuxi/go-tiny-claw/internal/tools"
)

// ============================================================================
// 配置类型
// ============================================================================

// ServerConfig 单个 MCP 服务器的启动配置
type ServerConfig struct {
	Name    string            `yaml:"name"`    // 服务器标识名，用于日志和工具名前缀
	Command string            `yaml:"command"` // 启动命令，如 "npx"、"python"、"node"
	Args    []string          `yaml:"args"`    // 命令参数
	Env     map[string]string `yaml:"env"`     // 额外的环境变量
}

// MCPConfig MCP 配置的顶层结构（对应 config/mcp_servers.yaml）
type MCPConfig struct {
	Servers []ServerConfig `yaml:"mcp_servers"`
}

// ============================================================================
// 配置加载
// ============================================================================

// LoadMCPConfig 从 YAML 文件加载 MCP 服务器配置
// 如果文件不存在，返回 nil config + nil error（非致命，跳过 MCP）
func LoadMCPConfig(path string) (*MCPConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("[MCP] 配置文件 '%s' 不存在，跳过 MCP 工具集成", path)
			return nil, nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg MCPConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 展开环境变量引用（如 ${GITHUB_TOKEN}）
	for i := range cfg.Servers {
		for k, v := range cfg.Servers[i].Env {
			cfg.Servers[i].Env[k] = os.ExpandEnv(v)
		}
	}

	return &cfg, nil
}

// ============================================================================
// MCP 管理器
// ============================================================================

// Manager 管理所有 MCP 服务器的生命周期和工具注册
type Manager struct {
	config  *MCPConfig
	clients []*Client
	tools   []*MCPTool
}

// NewManager 创建 MCP 管理器
func NewManager(cfg *MCPConfig) *Manager {
	return &Manager{
		config:  cfg,
		clients: make([]*Client, 0, len(cfg.Servers)),
		tools:   make([]*MCPTool, 0),
	}
}

// Start 启动所有 MCP 服务器并发现工具
// 单个服务器启动失败不影响其他服务器（graceful degradation）
func (m *Manager) Start(ctx context.Context) error {
	if len(m.config.Servers) == 0 {
		log.Println("[MCP] 没有配置任何 MCP 服务器，跳过启动")
		return nil
	}

	var lastErr error
	successCount := 0

	for _, srvCfg := range m.config.Servers {
		if srvCfg.Name == "" || srvCfg.Command == "" {
			log.Printf("[MCP] 跳过无效配置: name=%q command=%q", srvCfg.Name, srvCfg.Command)
			continue
		}

		log.Printf("[MCP] 正在启动服务器 '%s' (command=%s)...", srvCfg.Name, srvCfg.Command)

		// 创建并初始化 MCP 客户端
		client, err := NewClient(srvCfg.Name, srvCfg.Command, srvCfg.Args, srvCfg.Env)
		if err != nil {
			log.Printf("[MCP] ⚠️ 服务器 '%s' 启动失败: %v", srvCfg.Name, err)
			lastErr = err
			continue
		}

		// 发现工具
		discoverCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		toolDefs, err := client.ListTools(discoverCtx)
		cancel()
		if err != nil {
			log.Printf("[MCP] ⚠️ 服务器 '%s' 工具发现失败: %v", srvCfg.Name, err)
			client.Close()
			lastErr = err
			continue
		}

		// 为每个工具创建适配器
		for _, def := range toolDefs {
			mt := NewMCPTool(srvCfg.Name, def, client)
			m.tools = append(m.tools, mt)
			log.Printf("[MCP]   发现工具: %s", mt.Name())
		}

		m.clients = append(m.clients, client)
		successCount++
	}

	log.Printf("[MCP] 启动完成: %d/%d 个服务器成功，共发现 %d 个工具",
		successCount, len(m.config.Servers), len(m.tools))

	if successCount == 0 && lastErr != nil {
		return fmt.Errorf("所有 MCP 服务器启动失败，最后错误: %w", lastErr)
	}

	return nil
}

// RegisterAll 将所有 MCP 工具注册到给定的 Registry
func (m *Manager) RegisterAll(registry tools.Registry) {
	for _, tool := range m.tools {
		registry.Register(tool)
	}
	log.Printf("[MCP] 已将 %d 个 MCP 工具注册到 Registry", len(m.tools))
}

// ToolCount 返回发现的工具总数
func (m *Manager) ToolCount() int {
	return len(m.tools)
}

// Shutdown 关闭所有 MCP 服务器连接，回收子进程资源
func (m *Manager) Shutdown() {
	for _, client := range m.clients {
		if err := client.Close(); err != nil {
			log.Printf("[MCP] 关闭服务器 '%s' 时出错: %v", client.ServerName(), err)
		}
	}
	log.Println("[MCP] 所有 MCP 服务器已关闭")
}
