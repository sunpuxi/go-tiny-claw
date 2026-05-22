package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAllSkillName(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".claw", "skills")
	setupSkillFile(t, skillsDir, "git-workflow", "git-workflow", "Git 工作流规范")
	setupSkillFile(t, skillsDir, "code-review", "code-review", "代码审查流程")
	setupSkillFile(t, skillsDir, "deploy", "deploy", "部署流程")

	loader := NewSkillLoader(tmpDir)
	result := loader.LoadAllSkillName()

	for _, name := range []string{"git-workflow", "code-review", "deploy"} {
		if !strings.Contains(result, name) {
			t.Errorf("结果中应包含 %q，实际: %s", name, result)
		}
	}
}

func TestLoadAllSkillName_DirNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	loader := NewSkillLoader(tmpDir)
	result := loader.LoadAllSkillName()

	if result != "" {
		t.Errorf("目录不存在时应返回空字符串，实际得到: %q", result)
	}
}

func TestLoadAllSkillName_NoSkillFiles(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".claw", "skills")
	err := os.MkdirAll(skillsDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(skillsDir, "readme.txt"), []byte("hello"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	loader := NewSkillLoader(tmpDir)
	result := loader.LoadAllSkillName()

	if result != "目前可供使用的Skill列表为：" {
		t.Errorf("没有 SKILL.md 时应返回只有前缀的空列表，实际: %q", result)
	}
}

func TestLoadAllSkillName_NoFrontmatter(t *testing.T) {
	tmpDir := t.TempDir()
	skillPath := filepath.Join(tmpDir, ".claw", "skills", "default", "SKILL.md")
	err := os.MkdirAll(filepath.Dir(skillPath), 0755)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(skillPath, []byte("# Just a title\nsome content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	loader := NewSkillLoader(tmpDir)
	result := loader.LoadAllSkillName()

	if !strings.Contains(result, "Unknown Skill") {
		t.Errorf("无 frontmatter 应包含 'Unknown Skill'，实际: %q", result)
	}
}

func TestReadSkill_Found(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".claw", "skills")
	setupSkillFile(t, skillsDir, "git-workflow", "git-workflow", "Git 工作流规范")

	loader := NewSkillLoader(tmpDir)
	skill, err := loader.ReadSkill("git-workflow")
	if err != nil {
		t.Fatalf("ReadSkill 执行失败: %v", err)
	}

	if skill.Name != "git-workflow" {
		t.Errorf("期望 name = 'git-workflow'，实际: %q", skill.Name)
	}
	if skill.Description != "Git 工作流规范" {
		t.Errorf("期望 description = 'Git 工作流规范'，实际: %q", skill.Description)
	}
	if !strings.Contains(skill.Body, "执行前请先拉取最新代码") {
		t.Errorf("Body 应包含 skill 正文，实际: %s", skill.Body)
	}
}

func TestReadSkill_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	skillsDir := filepath.Join(tmpDir, ".claw", "skills")
	setupSkillFile(t, skillsDir, "git-workflow", "git-workflow", "Git 工作流规范")

	loader := NewSkillLoader(tmpDir)
	_, err := loader.ReadSkill("non-existent-skill")
	if err == nil {
		t.Fatal("应返回错误，实际为 nil")
	}
	if !strings.Contains(err.Error(), "未找到") {
		t.Errorf("错误信息应包含'未找到'，实际: %v", err)
	}
}

func TestReadSkill_DirNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	loader := NewSkillLoader(tmpDir)
	_, err := loader.ReadSkill("anything")
	if err == nil {
		t.Fatal("目录不存在时应返回错误，实际为 nil")
	}
}

// setupSkillFile 创建技能目录和 SKILL.md 文件的辅助函数
func setupSkillFile(t *testing.T, baseDir, dirName, name, description string) {
	t.Helper()
	skillPath := filepath.Join(baseDir, dirName, "SKILL.md")
	err := os.MkdirAll(filepath.Dir(skillPath), 0755)
	if err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n# " + name + "\n执行前请先拉取最新代码\n"
	err = os.WriteFile(skillPath, []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}
}
