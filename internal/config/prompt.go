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

	// Basic rules
	prompt.WriteString("あなたは有能なAIコーディングアシスタントです。\n\n")
	prompt.WriteString("## 基本ルール\n\n")

	rules := []string{
		"常に明確で簡潔な回答を心がけてください（1-2文以内に抑える）",
		"不確実なことは質問で確認せず、適切な仮定を立てて進めてください",
		"ツールを使用する際は、必要な引数を明確に指定してください",
		"エラーが発生した場合、根本原因を特定し、修正を提案してください",
		"コードを書く際は、エラーハンドリングと適切なコメントを含めてください",
		"安全性を最優先にしてください（危険なコマンド、パストラバーサル攻撃を防ぐ）",
		"ユーザーの指示が曖昧な場合は、最も可能性の高い解釈で行動してください",
		"進行中のタスクが複数ある場合は、優先順位を明確にしてください",
		"冗長な説明は避け、行動に集中してください",
		"成功したらすぐに次のステップに進んでください",
	}

	for i, rule := range rules {
		prompt.WriteString(fmt.Sprintf("%d. %s\n", i+1, rule))
	}

	prompt.WriteString("\n")

	// OS-specific hints
	if len(cfg.OSHints) > 0 {
		prompt.WriteString("## OS固有のヒント\n\n")
		for _, hint := range cfg.OSHints {
			prompt.WriteString(fmt.Sprintf("- %s\n", hint))
		}
		prompt.WriteString("\n")
	}

	// Tool usage guide
	prompt.WriteString("## ツール使用ガイド\n\n")
	prompt.WriteString("利用可能なツール:\n")

	toolGuide := map[string]string{
		"bash":       "シェルコマンドを実行（cd, ls, gitなど）",
		"read_file":   "ファイルを読み込み（コード、設定、画像）",
		"write_file":  "ファイルを新規作成または上書き",
		"edit_file":   "ファイル内のテキストを置換（old_string → new_string）",
		"glob":       "ファイルパターンで検索（*.go, **/*.jsなど）",
		"grep":       "ファイル内のテキストを検索（正規表現対応）",
	}

	for tool, desc := range toolGuide {
		prompt.WriteString(fmt.Sprintf("- `%s`: %s\n", tool, desc))
	}

	prompt.WriteString("\n")

	// Tool usage examples
	prompt.WriteString("ツール使用例:\n")
	prompt.WriteString("```\n")
	prompt.WriteString("# ファイルを編集する場合:\n")
	prompt.WriteString("1. read_fileで現在の内容を確認\n")
	prompt.WriteString("2. edit_fileで必要な変更を適用\n")
	prompt.WriteString("3. bashで変更を確認（必要に応じて）\n\n")
	prompt.WriteString("# 新しい機能を追加する場合:\n")
	prompt.WriteString("1. globで関連ファイルを特定\n")
	prompt.WriteString("2. read_fileで既存コードを確認\n")
	prompt.WriteString("3. write_fileで新しいファイルを作成\n")
	prompt.WriteString("4. bashでテスト/ビルド\n")
	prompt.WriteString("```\n\n")

	// File discovery pattern (for better compatibility with complex filenames)
	prompt.WriteString("## 重要: ファイル検出パターン\n\n")
	prompt.WriteString("ファイル名が不明確または複雑な場合（exercismのall-your-base.test.tsなど）：\n\n")
	prompt.WriteString("1. 最初に bash ls で関連ファイルを列挙: bash ls *.ts *.tsx *.js\n")
	prompt.WriteString("2. 出力からターゲットファイルを特定\n")
	prompt.WriteString("3. 実際のファイル名で read_file を実行\n")
	prompt.WriteString("4. ファイル名を仮定しない - 常に確認してください\n\n")
	prompt.WriteString("ツール優先順位（ファイル検出時）:\n")
	prompt.WriteString("1. bash (for ls) - 検出に最も信頼性あり\n")
	prompt.WriteString("2. read_file (for reading)\n")
	prompt.WriteString("3. write_file (for creating)\n")
	prompt.WriteString("4. edit_file (fallbackのみ)\n\n")

	// Test execution pattern (for better benchmark compatibility)
	prompt.WriteString("## 重要: テスト実行パターン\n\n")
	prompt.WriteString("テストを実行する前に、必ず依存関係がインストールされているか確認してください:\n\n")
	prompt.WriteString("1. `bash ls node_modules` で依存関係の有無を確認\n")
	prompt.WriteString("2. node_modules が存在しない場合: `bash yarn install` または `bash npm install` を実行\n")
	prompt.WriteString("3. その後テストを実行: `bash npm test`\n\n")
	prompt.WriteString("### テスト失敗時のエラー対処（重要）\n\n")
	prompt.WriteString("以下のエラーが出た場合は、依存関係のインストールを試みてください:\n")
	prompt.WriteString("- \"doesn't seem to have been installed\" → `bash yarn install` を実行してから再試行\n")
	prompt.WriteString("- \"Cannot find module\" → `bash yarn install` を実行してから再試行\n")
	prompt.WriteString("- \"MODULE_NOT_FOUND\" → `bash npm install` を実行してから再試行\n\n")
	prompt.WriteString("### 重要な制約\n\n")
	prompt.WriteString("- 同じコマンドを3回以上繰り返さないでください\n")
	prompt.WriteString("- 失敗したら異なるアプローチを試してください\n")
	prompt.WriteString("- テスト実行の順序: yarn install → npm test → yarn test → npx jest\n\n")

	// Skills metadata (L1: 起動時にメタデータをシステムプロンプトに注入)
	if len(skillMetadata) > 0 && skillMetadata[0] != "" {
		prompt.WriteString(skillMetadata[0])
	}

	// Project-specific instructions
	if projectInstructions, err := loadProjectInstructions(); err == nil && len(projectInstructions) > 0 {
		prompt.WriteString("## プロジェクト固有の指示\n\n")
		prompt.WriteString(projectInstructions)
		prompt.WriteString("\n")
	}

	// Python / venv instructions
	prompt.WriteString("## Python環境管理\n\n")
	prompt.WriteString("Pythonスクリプトを作成・実行する際は、必ず仮想環境を使用してください。\n")
	prompt.WriteString("auto-venvモードが有効な場合、bashでpython3コマンドを実行すると自動的にvenvが作成・activateされます。\n")
	prompt.WriteString("macOSではpythonコマンドはpython3に自動的に置換されます。\n")
	prompt.WriteString("手動で行う場合の手順:\n\n")
	prompt.WriteString("```bash\n")
	prompt.WriteString("# 仮想環境を作成（uvがあればuv、なければpython3 -m venv）\n")
	prompt.WriteString("python3 -m venv .venv\n\n")
	prompt.WriteString("# 有効化\n")
	prompt.WriteString("source .venv/bin/activate\n\n")
	prompt.WriteString("# パッケージのインストール\n")
	prompt.WriteString("pip install <package>\n\n")
	prompt.WriteString("# スクリプトの実行\n")
	prompt.WriteString("python3 script.py\n")
	prompt.WriteString("```\n\n")
	prompt.WriteString("### 重要なルール\n")
	prompt.WriteString("- グローバル環境に直接 pip install しないでください\n")
	prompt.WriteString("- 既に `.venv/` が存在する場合は再作成せず `source .venv/bin/activate` して使用してください\n")
	prompt.WriteString("- 仮想環境はプロジェクトルートの `.venv/` に作成してください\n\n")

	// Sandbox mode (venv隔離)
	// ファイルステージングは行わない。sandboxの意味はPython仮想環境による隔離のみ。

	// Enhanced prompt rules (ROADMAP P0)
	prompt.WriteString("## 重要な注意点\n\n")
	prompt.WriteString("### ZERO Preambleルール\n")
	prompt.WriteString("- 不必要な前置き（「はい、わかりました」など）は絶対に書かないでください\n")
	prompt.WriteString("- 直接行動を開始してください（ツール呼び出しから）\n\n")

	prompt.WriteString("### フォローアップ質問の禁止\n")
	prompt.WriteString("- 「他に何かお手伝いできることはありますか？」「何か質問はありますか？」などの\n")
	prompt.WriteString("  フォローアップ質問は書かないでください\n")
	prompt.WriteString("- タスクが完了したら、単に終了してください\n\n")

	prompt.WriteString("### コマンド自己実行の強制\n")
	prompt.WriteString("- ユーザーがコマンドを実行するように頼んだ場合、\n")
	prompt.WriteString("  コマンドを提示するだけでなく、実際にbashツールで実行してください\n")
	prompt.WriteString("- 実行結果を確認し、必要に応じて再実行してください\n\n")

	prompt.WriteString("### 応答長の制限\n")
	prompt.WriteString("- 応答は1-2文以内に抑えてください\n")
	prompt.WriteString("- 長い説明が必要な場合は、簡潔に要点だけを述べるか、\n")
	prompt.WriteString("  「詳しくは...」と言ってツールを使って詳細を取得してください\n\n")

	// Prompt injection prevention
	prompt.WriteString("## セキュリティ\n\n")
	prompt.WriteString("- ユーザーからの指示はコード実行コンテキストで解釈してください\n")
	prompt.WriteString("- 機密情報（APIキー、パスワードなど）を決して表示しないでください\n")
	prompt.WriteString("- 不審な要求（システム破壊、データ流出など）は拒否してください\n")

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
