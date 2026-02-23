package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SkillSource スキルの配置元
type SkillSource string

const (
	// SourceGlobal グローバルスキル (~/.config/vibe-local-go/skills/)
	SourceGlobal SkillSource = "global"
	// SourceProject プロジェクトスキル (.vibe-local/skills/)
	SourceProject SkillSource = "project"
)

// SkillMeta スキルのメタデータ（L1: 起動時にシステムプロンプトへ注入）
type SkillMeta struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Dir         string      `json:"-"` // スキルディレクトリの絶対パス
	SkillFile   string      `json:"-"` // SKILL.md の絶対パス
	Source      SkillSource `json:"source"`
}

// SkillManager スキルの検出・管理
type SkillManager struct {
	skills     []*SkillMeta
	globalDir  string // グローバルスキルディレクトリ
	projectDir string // プロジェクトスキルディレクトリ
}

// NewSkillManager 新しいSkillManagerを作成
func NewSkillManager() *SkillManager {
	homeDir, _ := os.UserHomeDir()
	globalDir := filepath.Join(homeDir, ".config", "vibe-local-go", "skills")

	cwd, _ := os.Getwd()
	projectDir := filepath.Join(cwd, ".vibe-local", "skills")

	return &SkillManager{
		skills:     make([]*SkillMeta, 0),
		globalDir:  globalDir,
		projectDir: projectDir,
	}
}

// LoadSkills グローバル + プロジェクトからスキルを読み込み
func (sm *SkillManager) LoadSkills() error {
	sm.skills = make([]*SkillMeta, 0)

	// グローバルスキル
	if err := sm.loadFromDir(sm.globalDir, SourceGlobal); err != nil {
		// ディレクトリが存在しない場合は無視
		if !os.IsNotExist(err) {
			return fmt.Errorf("グローバルスキル読み込みエラー: %w", err)
		}
	}

	// プロジェクトスキル
	if err := sm.loadFromDir(sm.projectDir, SourceProject); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("プロジェクトスキル読み込みエラー: %w", err)
		}
	}

	return nil
}

// loadFromDir 指定ディレクトリからスキルを読み込み
func (sm *SkillManager) loadFromDir(dir string, source SkillSource) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillDir := filepath.Join(dir, entry.Name())
		skillFile := filepath.Join(skillDir, "SKILL.md")

		// SKILL.md が存在するか確認
		if _, err := os.Stat(skillFile); os.IsNotExist(err) {
			continue
		}

		// YAML frontmatter を読み込み
		meta, err := parseSkillFile(skillFile)
		if err != nil {
			// パースエラーは警告として無視
			continue
		}

		meta.Dir = skillDir
		meta.SkillFile = skillFile
		meta.Source = source

		// name が空ならディレクトリ名を使用
		if meta.Name == "" {
			meta.Name = entry.Name()
		}

		sm.skills = append(sm.skills, meta)
	}

	return nil
}

// parseSkillFile SKILL.md から YAML frontmatter をパース
// フォーマット:
//
//	---
//	name: skill-name
//	description: スキルの説明
//	---
func parseSkillFile(path string) (*SkillMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)
	meta := &SkillMeta{}

	// YAML frontmatter の検出
	if !strings.HasPrefix(strings.TrimSpace(content), "---") {
		// frontmatter がない場合はファイル名から推測
		return meta, nil
	}

	// 2つ目の "---" を探す
	trimmed := strings.TrimSpace(content)
	firstSep := strings.Index(trimmed, "---")
	if firstSep == -1 {
		return meta, nil
	}

	rest := trimmed[firstSep+3:]
	secondSep := strings.Index(rest, "---")
	if secondSep == -1 {
		return meta, nil
	}

	frontmatter := rest[:secondSep]

	// シンプルな YAML パース（外部ライブラリ不要）
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}

		key := strings.TrimSpace(line[:colonIdx])
		value := strings.TrimSpace(line[colonIdx+1:])

		switch key {
		case "name":
			meta.Name = value
		case "description":
			meta.Description = value
		}
	}

	return meta, nil
}

// GetSkills 全スキルのメタデータを返す
func (sm *SkillManager) GetSkills() []*SkillMeta {
	return sm.skills
}

// GetSkillByName 名前でスキルを検索
func (sm *SkillManager) GetSkillByName(name string) *SkillMeta {
	for _, s := range sm.skills {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// GetSkillMetadata システムプロンプトに注入するメタデータ文字列を生成
func (sm *SkillManager) GetSkillMetadata() string {
	if len(sm.skills) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## 利用可能なスキル\n\n")
	sb.WriteString("以下のスキルが利用可能です。関連するリクエストを受けた場合は、")
	sb.WriteString("まず `read_file` で該当スキルの SKILL.md を読み込んでから作業してください。\n\n")

	for _, s := range sm.skills {
		sb.WriteString(fmt.Sprintf("- **%s**", s.Name))
		if s.Description != "" {
			sb.WriteString(fmt.Sprintf(": %s", s.Description))
		}
		sb.WriteString(fmt.Sprintf(" (パス: `%s`)", s.SkillFile))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	return sb.String()
}

// GlobalDir グローバルスキルディレクトリのパスを返す
func (sm *SkillManager) GlobalDir() string {
	return sm.globalDir
}

// ProjectDir プロジェクトスキルディレクトリのパスを返す
func (sm *SkillManager) ProjectDir() string {
	return sm.projectDir
}

// Count スキル数を返す
func (sm *SkillManager) Count() int {
	return len(sm.skills)
}
