package context

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Skill 定义了从 SKILL.md 中解析出的标准化技能结构
type Skill struct {
	Name        string
	Description string
	Body        string // Markdown 正文指令
}

// SkillLoader 负责从本地文件系统中加载并解析符合规范的技能模板
type SkillLoader struct {
	workDir       string
	skillCacheMap map[string]Skill
	mu            sync.RWMutex
}

func NewSkillLoader(workDir string) *SkillLoader {
	return &SkillLoader{
		workDir:       workDir,
		skillCacheMap: make(map[string]Skill),
	}
}

// LoadAllSkillName 扫描 .claw/skills 目录，返回所有 SKILL.md 中定义的技能名称列表
func (s *SkillLoader) LoadAllSkillName() string {
	skillBaseDir := filepath.Join(s.workDir, ".claw", "skills")

	if _, err := os.Stat(skillBaseDir); os.IsNotExist(err) {
		return ""
	}

	var names []string

	filepath.WalkDir(skillBaseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "SKILL.md" {
			content, err := os.ReadFile(path)
			if err == nil {
				skill := parseSkillMD(string(content))
				names = append(names, skill.Name)
			}
		}
		return nil
	})

	return "目前可供使用的Skill列表为：" + strings.Join(names, ",")
}

// ReadSkill 按技能名称查找并解析对应的 SKILL.md 文件，返回完整的 Skill 结构
// 使用 double-checked locking 模式：先读缓存（RLock），未命中则升级为写锁（Lock）再扫描文件系统
func (s *SkillLoader) ReadSkill(name string) (Skill, error) {
	// 1. 读锁检查缓存（允许并发读）
	s.mu.RLock()
	if skill, ok := s.skillCacheMap[name]; ok {
		s.mu.RUnlock()
		return skill, nil
	}
	s.mu.RUnlock()

	// 2. 缓存未命中，获取写锁进行文件扫描
	s.mu.Lock()
	defer s.mu.Unlock()

	// 3. Double-check：在获取到写锁后再次检查，防止其他 goroutine 已加载
	if skill, ok := s.skillCacheMap[name]; ok {
		return skill, nil
	}

	// 4. 遍历 .claw/skills 查找匹配的 SKILL.md
	skillBaseDir := filepath.Join(s.workDir, ".claw", "skills")
	if _, err := os.Stat(skillBaseDir); os.IsNotExist(err) {
		return Skill{}, fmt.Errorf("skills 目录不存在: %s", skillBaseDir)
	}

	var found Skill
	err := filepath.WalkDir(skillBaseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "SKILL.md" {
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil // 跳过读取失败的文件
			}
			skill := parseSkillMD(string(content))
			if skill.Name == name {
				found = skill
				s.skillCacheMap[name] = found // 写锁已持有，直接写入缓存
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return Skill{}, fmt.Errorf("遍历 skills 目录失败: %w", err)
	}

	if found.Name == "" {
		return Skill{}, fmt.Errorf("未找到名称为 '%s' 的技能", name)
	}

	return found, nil
}

// parseSkillMD 极简解析带有 YAML Frontmatter 的 Markdown 内容
func parseSkillMD(content string) Skill {
	skill := Skill{
		Name:        "Unknown Skill",
		Description: "No description provided.",
		Body:        content, // 默认将全量内容作为 body
	}

	// 简单解析 YAML Frontmatter (以 --- 包裹)
	if strings.HasPrefix(content, "---\n") || strings.HasPrefix(content, "---\r\n") {
		parts := strings.SplitN(content, "---", 3)
		if len(parts) == 3 {
			frontmatter := parts[1]
			skill.Body = strings.TrimSpace(parts[2])

			// 逐行提取 metadata
			lines := strings.Split(frontmatter, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "name:") {
					skill.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
				} else if strings.HasPrefix(line, "description:") {
					skill.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
				}
			}
		}
	}

	return skill
}
