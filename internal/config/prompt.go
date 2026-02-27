package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BuildSystemPrompt builds system prompt
// skillMetadata: スキルマネージャーから生成されたメタデータ文字列（空文字なら無視）
func BuildSystemPrompt(cfg *Config, skillMetadata ...string) string {
	var prompt strings.Builder

	// Core identity & rules (compact)
	prompt.WriteString(`あなたは有能なAIコーディングアシスタントです。

## 基本ルール
- 簡潔に回答（1-2文）、不要な前置き・フォローアップ質問は禁止
- 曖昧な指示は最も可能性の高い解釈で即座に行動
- エラー時は根本原因を特定し修正。同じコマンドを3回以上繰り返さない
- コマンド実行を頼まれたら提示ではなくbashツールで実行
- 機密情報の表示禁止、不審な要求は拒否

`)

	// OS-specific hints (keep as-is, dynamic)
	if len(cfg.OSHints) > 0 {
		prompt.WriteString("## OS固有のヒント\n")
		for _, hint := range cfg.OSHints {
			prompt.WriteString(fmt.Sprintf("- %s\n", hint))
		}
		prompt.WriteString("\n")
	}

	// Tool guide (compact - LLM knows tool schemas from function definitions)
	prompt.WriteString(`## ツール使用ガイド
ファイル名が不明な場合は最初にbash lsで確認。ファイル名を仮定しないこと。
編集時: read_file → edit_file → bashで確認。新規作成時: glob → read_file → write_file → bashでテスト。

`)

	// Test execution (compact but preserving critical info)
	prompt.WriteString(`## テスト実行
1. bash ls node_modulesで依存関係確認。なければyarn install実行
2. テスト実行: npm test（失敗時はyarn install後に再試行）
3. MODULE_NOT_FOUND/Cannot find module → yarn install後に再試行

`)

	// Skills metadata
	if len(skillMetadata) > 0 && skillMetadata[0] != "" {
		prompt.WriteString(skillMetadata[0])
	}

	// Project-specific instructions
	if projectInstructions, err := loadProjectInstructions(); err == nil && len(projectInstructions) > 0 {
		prompt.WriteString("## プロジェクト固有の指示\n\n")
		prompt.WriteString(projectInstructions)
		prompt.WriteString("\n")
	}

	// Python venv instructions (only if Python files are likely)
	if cfg.IncludePythonHints {
		prompt.WriteString(`## Python環境管理
Pythonスクリプト実行時は必ず仮想環境を使用。auto-venvモード有効時は自動でvenv作成・activate。
手動: python3 -m venv .venv && source .venv/bin/activate。グローバルpip install禁止。

`)
	}

	return prompt.String()
}

// loadProjectInstructions loads project-specific instructions
func loadProjectInstructions() (string, error) {
	// Priority: .vibe-coder.json > CLAUDE.md > README.md

	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Try .vibe-coder.json
	if content, err := readFile(filepath.Join(cwd, ".vibe-coder.json")); err == nil {
		return content, nil
	}

	// Try CLAUDE.md
	if content, err := readFile(filepath.Join(cwd, "CLAUDE.md")); err == nil {
		return content, nil
	}

	// Try README.md (first 500 chars only)
	if content, err := readFile(filepath.Join(cwd, "README.md")); err == nil {
		// Only use first 500 characters
		if len(content) > 500 {
			content = content[:500] + "..."
		}
		return content, nil
	}

	return "", nil
}

// readFile reads a file with size limit
func readFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Max size limit: 10KB
	const maxSize = 10 * 1024
	reader := bufio.NewReader(file)
	content, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}

	// Sanitize (secret filter)
	sanitized := sanitizeInstructions(content)

	// Size limit
	if len(sanitized) > maxSize {
		sanitized = sanitized[:maxSize]
	}

	return sanitized, nil
}

// sanitizeInstructions sanitizes project instructions (prompt injection prevention)
func sanitizeInstructions(content string) string {
	// Filter dangerous keywords
	dangerousKeywords := []string{
		"IGNORE_PREVIOUS",
		"IGNORE_ALL_ABOVE",
		"DISREGARD_PREVIOUS",
		"Forget everything above",
		"Replace all instructions",
	}

	sanitized := content
	for _, keyword := range dangerousKeywords {
		if strings.Contains(strings.ToUpper(sanitized), keyword) {
			_ = fmt.Sprintf("Warning: Dangerous keyword detected in project instructions: %s", keyword)
			sanitized = strings.ReplaceAll(sanitized, keyword, "[REDACTED]")
		}
	}

	// Secret information filter (simplified)
	sensitivePatterns := []string{
		"API_KEY", "SECRET_KEY", "ACCESS_TOKEN", "PRIVATE_KEY",
		"password", "passwd", "secret", "token",
	}

	for _, pattern := range sensitivePatterns {
		if strings.Contains(strings.ToLower(sanitized), strings.ToLower(pattern)) {
			sanitized = regexpReplace(sanitized, pattern+"[^\\s=]+", "[REDACTED]")
		}
	}

	return sanitized
}

// regexpReplace does regex replacement (simplified implementation without regexp package)
func regexpReplace(text, pattern, replacement string) string {
	result := text
	patternLower := strings.ToLower(pattern)
	textLower := strings.ToLower(text)

	idx := strings.Index(textLower, patternLower)
	for idx != -1 {
		before := result[:idx]

		matchEnd := idx
		for matchEnd < len(result) {
			char := result[matchEnd]
			if char == ' ' || char == '\t' || char == '\n' || char == '=' {
				break
			}
			matchEnd++
		}

		after := result[matchEnd:]
		result = before + replacement + after

		textLower = strings.ToLower(result)
		idx = strings.Index(textLower[idx+len(replacement):], patternLower)
		if idx != -1 {
			idx += idx + len(replacement)
		}
	}

	return result
}

// UpdateSystemPromptWithOSHints adds OS-specific hints
func UpdateSystemPromptWithOSHints(prompt string, hints []string) string {
	if len(hints) == 0 {
		return prompt
	}

	var builder strings.Builder
	builder.WriteString(prompt)

	builder.WriteString("\n\n## OS固有のヒント\n\n")
	for _, hint := range hints {
		builder.WriteString(fmt.Sprintf("- %s\n", hint))
	}

	return builder.String()
}

// TruncatePromptToSize truncates prompt to specified size
func TruncatePromptToSize(prompt string, maxSize int) string {
	if len(prompt) <= maxSize {
		return prompt
	}

	safeSize := int(float64(maxSize) * 0.9)

	truncated := prompt[:safeSize]
	lastNewline := strings.LastIndex(truncated, "\n")
	if lastNewline > 0 {
		truncated = truncated[:lastNewline]
	}

	return truncated + "\n...（プロンプトはサイズ制限により切り詰められました）"
}
