package ui

import (
	"fmt"
	"regexp"
	"strings"
)

// MarkdownRenderer マークダウンレンダラー
type MarkdownRenderer struct {
	terminal *Terminal
	width    int
}

// NewMarkdownRenderer 新しいマークダウンレンダラーを作成
func NewMarkdownRenderer(terminal *Terminal, width int) *MarkdownRenderer {
	if width == 0 {
		width = 80
	}
	return &MarkdownRenderer{
		terminal: terminal,
		width:    width,
	}
}

// Render マークダウンをレンダリング
func (mr *MarkdownRenderer) Render(text string) {
	// コードブロックを検出
	blocks := mr.parseCodeBlocks(text)

	// コードブロック以外の部分をレンダリング
	remaining := text
	for _, block := range blocks {
		// コードブロック前のテキストをレンダリング
		before := remaining[:block.Start]
		mr.renderInline(before)

		// コードブロックをレンダリング
		mr.renderCodeBlock(block)

		// 残りを更新
		remaining = remaining[block.End:]
	}

	// 最後の部分をレンダリング
	mr.renderInline(remaining)
}

// CodeBlock コードブロック
type CodeBlock struct {
	Start   int
	End     int
	Lang    string
	Content string
}

// parseCodeBlocks コードブロックを解析
func (mr *MarkdownRenderer) parseCodeBlocks(text string) []CodeBlock {
	var blocks []CodeBlock

	// ```で囲まれたコードブロックを検出
	re := regexp.MustCompile("```([a-zA-Z0-9+-]*)?\n(.*?)```")
	matches := re.FindAllStringSubmatchIndex(text, -1)

	for _, match := range matches {
		start := match[0]
		end := match[1]

		// 言語を抽出
		fullMatch := text[start:end]
		lines := strings.SplitN(fullMatch, "\n", 2)
		lang := ""
		if len(lines) > 0 {
			lang = strings.TrimPrefix(lines[0], "```")
		}

		// コンテンツを抽出
		content := ""
		if len(lines) > 1 {
			content = strings.TrimSuffix(lines[1], "```")
		}

		blocks = append(blocks, CodeBlock{
			Start:   start,
			End:     end,
			Lang:    lang,
			Content: content,
		})
	}

	return blocks
}

// renderCodeBlock コードブロックをレンダリング
func (mr *MarkdownRenderer) renderCodeBlock(block CodeBlock) {
	// コードブロックの境界線
	separator := strings.Repeat("─", mr.width-4)

	// 安全な長さを計算
	borderLen := mr.width - 4 - len(block.Lang) - 5
	if borderLen < 0 {
		borderLen = 0
	}
	if borderLen > len(separator) {
		borderLen = len(separator)
	}

	mr.terminal.PrintColored(ColorGray, fmt.Sprintf("┌─ %s ─%s\n", block.Lang, separator[:borderLen]))

	// コンテンツを行ごとに表示
	lines := strings.Split(block.Content, "\n")
	for _, line := range lines {
		mr.terminal.PrintColored(ColorGray, fmt.Sprintf("│ %s\n", line))
	}

	mr.terminal.PrintColored(ColorGray, fmt.Sprintf("└%s\n", separator))
}

// renderInline インライン要素をレンダリング
func (mr *MarkdownRenderer) renderInline(text string) {
	// コードスパン（`code`）を処理
	re := regexp.MustCompile("`([^`]+)`")
	matches := re.FindAllStringSubmatchIndex(text, -1)

	if len(matches) == 0 {
		// コードスパンがない場合はそのまま表示
		mr.renderTextStyles(text)
		return
	}

	// コードスパンを分割してレンダリング
	remaining := text
	for _, match := range matches {
		start := match[0]
		end := match[1]

		// コードスパン前のテキスト
		before := remaining[:start]
		mr.renderTextStyles(before)

		// コードスパン
		code := text[match[2]:match[3]]
		mr.terminal.PrintColored(ColorGreen, fmt.Sprintf("`%s`", code))

		// 残りを更新
		remaining = remaining[end:]
	}

	// 最後の部分
	mr.renderTextStyles(remaining)
}

// renderTextStyles テキストスタイルをレンダリング
func (mr *MarkdownRenderer) renderTextStyles(text string) {
	// **太字** を処理
	re := regexp.MustCompile("\\*\\*([^*]+)\\*\\*")
	matches := re.FindAllStringSubmatchIndex(text, -1)

	if len(matches) == 0 {
		// スタイルがない場合はそのまま表示
		mr.terminal.Print(text)
		return
	}

	// 太字を分割してレンダリング
	remaining := text
	for _, match := range matches {
		start := match[0]
		end := match[1]

		// 太字前のテキスト
		before := remaining[:start]
		mr.terminal.Print(before)

		// 太字部分
		bold := text[match[2]:match[3]]
		mr.terminal.PrintColored(Bold+ColorYellow, bold)

		// 残りを更新
		remaining = remaining[end:]
	}

	// 最後の部分
	mr.terminal.Print(remaining)
}

// RenderTable テーブルをレンダリング
func (mr *MarkdownRenderer) RenderTable(headers []string, rows [][]string) {
	if len(headers) == 0 || len(rows) == 0 {
		return
	}

	// 列幅を計算
	colWidths := make([]int, len(headers))
	for i, header := range headers {
		colWidths[i] = len(header)
	}

	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) {
				colWidths[i] = max(colWidths[i], len(cell))
			}
		}
	}

	// 区切り線を生成
	separator := make([]string, len(colWidths))
	for i, width := range colWidths {
		separator[i] = strings.Repeat("─", width+2)
	}

	// ヘッダー行
	mr.terminal.PrintColored(ColorCyan, "│ ")
	for i, header := range headers {
		mr.terminal.Printf("%-*s │ ", colWidths[i], header)
	}
	mr.terminal.Print("\n")

	// 区切り線
	mr.terminal.PrintColored(ColorCyan, "├")
	for i, sep := range separator {
		if i > 0 {
			mr.terminal.PrintColored(ColorCyan, "┼")
		}
		mr.terminal.PrintColored(ColorCyan, sep)
	}
	mr.terminal.PrintColored(ColorCyan, "┤\n")

	// データ行
	for _, row := range rows {
		mr.terminal.Print("│ ")
		for i, cell := range row {
			if i < len(colWidths) {
				mr.terminal.Printf("%-*s │ ", colWidths[i], cell)
			}
		}
		mr.terminal.Print("\n")
	}

	// 下部区切り線
	mr.terminal.PrintColored(ColorCyan, "└")
	for i, sep := range separator {
		if i > 0 {
			mr.terminal.PrintColored(ColorCyan, "┴")
		}
		mr.terminal.PrintColored(ColorCyan, sep)
	}
	mr.terminal.PrintColored(ColorCyan, "┘\n")
}

// RenderList リストをレンダリング
func (mr *MarkdownRenderer) RenderList(items []string) {
	for i, item := range items {
		if strings.HasPrefix(item, "- ") || strings.HasPrefix(item, "* ") {
			// マークダウンの箇条書き
			mr.terminal.Printf("  • %s\n", strings.TrimPrefix(strings.TrimPrefix(item, "- "), "* "))
		} else if strings.HasPrefix(item, "  - ") || strings.HasPrefix(item, "  * ") {
			// ネストされた箇条書き
			mr.terminal.Printf("    ◦ %s\n", strings.TrimPrefix(strings.TrimPrefix(item, "  - "), "  * "))
		} else if strings.Contains(item, fmt.Sprintf("%d.", i+1)) {
			// 番号付きリスト
			mr.terminal.Printf("  %s\n", item)
		} else {
			mr.terminal.Printf("  • %s\n", item)
		}
	}
}

// RenderQuote 引用をレンダリング
func (mr *MarkdownRenderer) RenderQuote(text string) {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		mr.terminal.PrintColored(ColorGray, fmt.Sprintf("│ %s\n", line))
	}
}

// RenderHorizontalLine 水平線をレンダリング
func (mr *MarkdownRenderer) RenderHorizontalLine() {
	line := strings.Repeat("─", mr.width)
	mr.terminal.PrintColored(ColorGray, line+"\n")
}

// RenderHeading 見出しをレンダリング
func (mr *MarkdownRenderer) RenderHeading(level int, text string) {
	prefix := strings.Repeat("#", level)

	switch level {
	case 1:
		mr.terminal.PrintColored(ColorCyan, fmt.Sprintf("\n%s %s %s\n\n", prefix, text, prefix))
	case 2:
		mr.terminal.PrintColored(ColorGreen, fmt.Sprintf("\n%s %s\n\n", prefix, text))
	case 3:
		mr.terminal.PrintColored(ColorYellow, fmt.Sprintf("\n%s %s\n\n", prefix, text))
	default:
		mr.terminal.PrintColored(ColorWhite, fmt.Sprintf("\n%s %s\n\n", prefix, text))
	}
}

// max 最大値を返す
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
