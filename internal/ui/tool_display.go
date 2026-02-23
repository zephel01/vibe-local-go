package ui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zephel01/vibe-local-go/internal/tool"
)

const (
	// MaxToolOutputLength is the maximum length of tool output to display
	MaxToolOutputLength = 30000
	// TruncatePreviewLength is the length to show when truncating output
	TruncatePreviewLength = 15000
)

// ShowToolCall displays a tool call with its parameters (Python版準拠: ⚡ Tool → summary)
func (t *Terminal) ShowToolCall(toolName string, params json.RawMessage) {
	// パラメータを解析してサマリーを作成
	summary := formatToolSummary(toolName, params)

	t.PrintColored(ColorCyan, fmt.Sprintf("  ⚡ %s", toolName))
	if summary != "" {
		t.PrintColored(ColorWhite, " → ")
		t.Print(summary)
	}
	t.Println("")
}

// formatToolSummary ツールコールの要約を生成
func formatToolSummary(toolName string, params json.RawMessage) string {
	var paramsMap map[string]interface{}
	if err := json.Unmarshal(params, &paramsMap); err != nil {
		return ""
	}

	switch toolName {
	case "Bash", "bash":
		if cmd, ok := paramsMap["command"].(string); ok {
			// 長いコマンドは短縮
			if len(cmd) > 80 {
				return cmd[:77] + "..."
			}
			return cmd
		}
	case "read_file", "ReadFile":
		if path, ok := paramsMap["path"].(string); ok {
			return path
		}
	case "write_file", "WriteFile":
		if path, ok := paramsMap["path"].(string); ok {
			contentLen := 0
			if content, ok := paramsMap["content"].(string); ok {
				contentLen = len(content)
			}
			return fmt.Sprintf("%s (%d bytes)", path, contentLen)
		}
	case "edit_file", "EditFile":
		if path, ok := paramsMap["path"].(string); ok {
			return path
		}
	case "glob", "Glob":
		if pattern, ok := paramsMap["pattern"].(string); ok {
			return pattern
		}
	case "grep", "Grep":
		if pattern, ok := paramsMap["pattern"].(string); ok {
			if path, ok := paramsMap["path"].(string); ok {
				return fmt.Sprintf("%s in %s", pattern, path)
			}
			return pattern
		}
	}

	return ""
}

// ShowToolResult displays the result of a tool execution (Python版準拠)
func (t *Terminal) ShowToolResult(result *tool.Result) {
	if result.IsError {
		t.PrintColored(ColorRed, fmt.Sprintf("  ┃ Error: %s\n", result.Error))
		if result.Output != "" {
			// インデント付きで出力
			for _, line := range strings.Split(result.Output, "\n") {
				t.Printf("  ┃ %s\n", line)
			}
		}
	} else {
		output := result.Output
		if output == "" {
			return
		}

		// Truncate if too long
		if len(output) > MaxToolOutputLength {
			prefix := output[:TruncatePreviewLength]
			suffix := output[len(output)-TruncatePreviewLength:]

			if lastNewline := strings.LastIndex(prefix, "\n"); lastNewline > 0 {
				prefix = prefix[:lastNewline]
			}
			if firstNewline := strings.Index(suffix, "\n"); firstNewline > 0 {
				suffix = suffix[firstNewline+1:]
			}

			omittedChars := len(output) - len(prefix) - len(suffix)

			for _, line := range strings.Split(prefix, "\n") {
				t.Printf("  ┃ %s\n", line)
			}
			t.PrintColored(ColorYellow, fmt.Sprintf("  ┃ ... [%d characters omitted] ...\n", omittedChars))
			for _, line := range strings.Split(suffix, "\n") {
				t.Printf("  ┃ %s\n", line)
			}
		} else {
			for _, line := range strings.Split(output, "\n") {
				t.Printf("  ┃ %s\n", line)
			}
		}
	}
}

// ShowBackgroundTask バックグラウンドタスク開始を表示（Python版準拠）
func (t *Terminal) ShowBackgroundTask(taskID string) {
	t.PrintColored(ColorGray, fmt.Sprintf("  ┃ Background task started: %s\n", taskID))
	t.PrintColored(ColorGray, fmt.Sprintf("  ┃ Use Bash(command='bg_status %s') to check result.\n", taskID))
}

// ShowToolResult displays the result of a tool execution (standalone function)
func ShowToolResult(result *tool.Result) {
	term := NewTerminal()
	term.ShowToolResult(result)
}

// ShowToolCall displays a tool call with its parameters (standalone function)
func ShowToolCall(toolName string, params json.RawMessage) {
	term := NewTerminal()
	term.ShowToolCall(toolName, params)
}
