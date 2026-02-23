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

// ShowToolCall displays a tool call with its parameters
func (t *Terminal) ShowToolCall(toolName string, params json.RawMessage) {
	t.PrintColoredf(ColorCyan, "🔧 Tool: %s", toolName)

	// Format parameters nicely if not empty
	if len(params) > 0 {
		var paramsMap map[string]interface{}
		if err := json.Unmarshal(params, &paramsMap); err == nil {
			// Pretty print JSON
			prettyJSON, _ := json.MarshalIndent(paramsMap, "  ", "  ")
			t.Printf("  Parameters:\n%s", string(prettyJSON))
		} else {
			// Raw JSON if parsing fails
			t.Printf("  Parameters: %s", string(params))
		}
	}
	t.Println("")
}

// ShowToolResult displays the result of a tool execution
func (t *Terminal) ShowToolResult(result *tool.Result) {
	if result.IsError {
		t.PrintColoredf(ColorRed, "❌ Error: %s\n", result.Error)
		if result.Output != "" {
			t.Printf("  %s\n", result.Output)
		}
	} else {
		// Truncate output if too long
		output := result.Output
		if len(output) > MaxToolOutputLength {
			prefix := output[:TruncatePreviewLength]
			suffix := output[len(output)-TruncatePreviewLength:]

			// Try to truncate at newline boundaries for cleaner display
			if lastNewline := strings.LastIndex(prefix, "\n"); lastNewline > 0 {
				prefix = prefix[:lastNewline]
			}
			if firstNewline := strings.Index(suffix, "\n"); firstNewline > 0 {
				suffix = suffix[firstNewline+1:]
			}

			omittedChars := len(output) - len(prefix) - len(suffix)
			t.Printf("✓ Output (%d characters, showing first %d and last %d):\n",
				len(output), len(prefix), len(suffix))
			t.Printf("%s\n\n", prefix)
			t.PrintColored(ColorYellow, fmt.Sprintf("  ... [%d characters omitted] ...\n\n", omittedChars))
			t.Printf("%s\n", suffix)
		} else {
			t.Printf("✓ Output:\n%s\n", output)
		}
	}
	t.Println("")
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
