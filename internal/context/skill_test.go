package context

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestLoadAllSkillName(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()

	// 创建 .claw/skills 目录结构
	skillsDir := filepath.Join(tmpDir, ".claw", "skills")
	err := os.MkdirAll(skillsDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// 创建 SKILL.md 文件
	skillFiles := map[string]string{
		filepath.Join(skillsDir, "git-workflow", "SKILL.md"): `---
name: git-workflow
description: Git 工作流规范
---
# Git Workflow
执行前请先拉取最新代码
`,
		filepath.Join(skillsDir, "code-review", "SKILL.md"): `---
name: code-review
description: 代码审查流程
---
# Code Review
检查代码风格和测试覆盖
`,
		filepath.Join(skillsDir, "deploy", "SKILL.md"): `---
name: deploy
description: 部署流程
---
# Deploy
执行构建和发布
`,
	}

	for path, content := range skillFiles {
		dir := filepath.Dir(path)
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			t.Fatal(err)
		}
		err = os.WriteFile(path, []byte(content), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	// 执行测试
	loader := NewSkillLoader(tmpDir)
	names := loader.LoadAllSkillName()

	// 验证结果
	sort.Strings(names)
	expected := []string{"code-review", "deploy", "git-workflow"}
	if len(names) != len(expected) {
		t.Fatalf("期望 %d 个技能名，实际得到 %d: %v", len(expected), len(names), names)
	}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("索引 %d: 期望 %q, 实际 %q", i, name, names[i])
		}
	}
}

func TestLoadAllSkillName_DirNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	loader := NewSkillLoader(tmpDir)
	names := loader.LoadAllSkillName()

	if names != nil {
		t.Errorf("目录不存在时应返回 nil，实际得到: %v", names)
	}
}

func TestLoadAllSkillName_NoSkillFiles(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".claw", "skills")
	err := os.MkdirAll(skillsDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// 在 skills 目录下放一个无关的文件，确保不被误解析
	err = os.WriteFile(filepath.Join(skillsDir, "readme.txt"), []byte("hello"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	loader := NewSkillLoader(tmpDir)
	names := loader.LoadAllSkillName()

	if len(names) != 0 {
		t.Errorf("没有 SKILL.md 时应返回空切片，实际得到: %v", names)
	}
}

func TestLoadAllSkillName_NoFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	skillPath := filepath.Join(tmpDir, ".claw", "skills", "default", "SKILL.md")
	err := os.MkdirAll(filepath.Dir(skillPath), 0755)
	if err != nil {
		t.Fatal(err)
	}

	// SKILL.md 没有 frontmatter，应返回默认名称 "Unknown Skill"
	err = os.WriteFile(skillPath, []byte("# Just a title\nsome content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	loader := NewSkillLoader(tmpDir)
	names := loader.LoadAllSkillName()

	if len(names) != 1 || names[0] != "Unknown Skill" {
		t.Errorf("无 frontmatter 应返回 'Unknown Skill'，实际得到: %v", names)
	}
}
