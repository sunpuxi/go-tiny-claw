package config

import (
	"log"
	"os"
	"regexp"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// dangerConfig 管理危险命令正则列表，支持运行时热加载。
// 读操作加 RLock，写操作加 Lock，保证并发安全。
type dangerConfig struct {
	mu         sync.RWMutex
	patterns   []*regexp.Regexp
	configPath string
	lastMod    time.Time
	stopCh     chan struct{}
}

// GlobalDangerConfig 全局实例，在 main.go 中初始化。
var GlobalDangerConfig *dangerConfig

// InitDangerConfig 加载配置文件并启动热加载轮询。
func InitDangerConfig(path string) *dangerConfig {
	dc := &dangerConfig{
		configPath: path,
		stopCh:     make(chan struct{}),
	}
	if err := dc.load(); err != nil {
		log.Printf("[config] 加载 %s 失败: %v，使用内置默认规则", path, err)
	}
	dc.ensureDefault()
	GlobalDangerConfig = dc
	return dc
}

// StartWatching 每 interval 轮询检查配置文件变更。
func (dc *dangerConfig) StartWatching(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				fi, err := os.Stat(dc.configPath)
				if err != nil {
					continue
				}
				if fi.ModTime().After(dc.lastMod) {
					log.Println("[config] 配置文件变更，热加载中...")
					if err := dc.load(); err != nil {
						log.Printf("[config] 热加载失败: %v，保留上次配置", err)
					} else {
						log.Println("[config] 热加载成功")
					}
				}
			case <-dc.stopCh:
				return
			}
		}
	}()
}

// Stop 停止热加载轮询。
func (dc *dangerConfig) Stop() { close(dc.stopCh) }

// Match 检查 args 是否命中任意危险规则。并发安全。
func (dc *dangerConfig) Match(args string) bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	for _, p := range dc.patterns {
		if p.MatchString(args) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------

type configFile struct {
	Patterns []string `yaml:"patterns"`
}

func (dc *dangerConfig) load() error {
	data, err := os.ReadFile(dc.configPath)
	if err != nil {
		return err
	}

	var cf configFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return err
	}

	compiled := make([]*regexp.Regexp, 0, len(cf.Patterns))
	for _, raw := range cf.Patterns {
		re, err := regexp.Compile(raw)
		if err != nil {
			log.Printf("[config] 跳过无效正则 [%s]: %v", raw, err)
			continue
		}
		compiled = append(compiled, re)
	}

	log.Printf("[config] 已加载 %d 条危险命令规则", len(compiled))

	dc.mu.Lock()
	dc.patterns = compiled
	dc.mu.Unlock()

	if fi, err := os.Stat(dc.configPath); err == nil {
		dc.lastMod = fi.ModTime()
	}
	return nil
}

func (dc *dangerConfig) ensureDefault() {
	dc.mu.RLock()
	has := len(dc.patterns) > 0
	dc.mu.RUnlock()
	if has {
		return
	}

	defaults := []string{
		`rm\s+-r`,
		`sudo\s+`,
		`drop\s+`,
		`>.*\.go`,
	}
	compiled := make([]*regexp.Regexp, len(defaults))
	for i, raw := range defaults {
		compiled[i] = regexp.MustCompile(raw)
	}

	dc.mu.Lock()
	dc.patterns = compiled
	dc.mu.Unlock()
	log.Println("[config] 配置文件无有效规则，已启用内置默认规则")
}
