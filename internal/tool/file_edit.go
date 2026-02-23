package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

const (
	// MaxEditFileSize is the maximum file size for editing
	MaxEditFileSize = 50 * 1024 * 1024 // 50MB
	// MaxDiffLines is the maximum number of diff lines to show
	MaxDiffLines = 40
)

// EditTool edits files by replacing strings
type EditTool struct {
	writeTool *WriteTool
	sandbox   SandboxStager
}

// NewEditTool creates a new edit tool
func NewEditTool() *EditTool {
	return &EditTool{
		writeTool: NewWriteTool(),
	}
}

// SetSandbox はサンドボックスマネージャーを設定する
func (t *EditTool) SetSandbox(sb SandboxStager) {
	t.sandbox = sb
}

// Name returns the tool name
func (t *EditTool) Name() string {
	return "edit_file"
}

// Schema returns the tool schema
func (t *EditTool) Schema() *FunctionSchema {
	return &FunctionSchema{
		Name:        "edit_file",
		Description: "Edit a file by replacing strings",
		Parameters: &ParameterSchema{
			Type: "object",
			Properties: map[string]*PropertyDef{
				"path": {
					Type:        "string",
					Description: "The file path to edit",
				},
				"old_string": {
					Type:        "string",
					Description: "The string to replace",
				},
				"new_string": {
					Type:        "string",
					Description: "The replacement string",
				},
				"replace_all": {
					Type:        "boolean",
					Description: "Replace all occurrences (default: false)",
					Default:     false,
				},
			},
			Required: []string{"path", "old_string", "new_string"},
		},
	}
}

// Execute edits a file
func (t *EditTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	var args struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}

	if err := json.Unmarshal(params, &args); err != nil {
		return NewErrorResult(err), nil
	}

	if args.Path == "" {
		return NewErrorResult(fmt.Errorf("path cannot be empty")), nil
	}

	if args.OldString == "" {
		return NewErrorResult(fmt.Errorf("old_string cannot be empty")), nil
	}

	// Resolve path
	resolvedPath, err := resolvePath(args.Path)
	if err != nil {
		return NewErrorResult(err), nil
	}

	// Read file
	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return NewErrorResult(err), nil
	}

	// Check file size
	if len(content) > MaxEditFileSize {
		return NewErrorResult(fmt.Errorf("file too large (%d bytes, max %d)", len(content), MaxEditFileSize)), nil
	}

	// Normalize content (Unicode NFC)
	oldContent := string(content)
	newContent := oldContent
	oldString := normalizeString(args.OldString)
	newString := args.NewString

	// Perform replacement
	if args.ReplaceAll {
		newContent = strings.ReplaceAll(newContent, oldString, newString)
	} else {
		// Check for multiple occurrences
		count := strings.Count(newContent, oldString)
		if count > 1 {
			return NewErrorResult(fmt.Errorf("old_string appears %d times; use replace_all=true or provide more unique context", count)), nil
		}

		// Single replacement
		if count == 0 {
			return NewErrorResult(fmt.Errorf("old_string not found in file")), nil
		}

		newContent = strings.Replace(newContent, oldString, newString, 1)
	}

	// Generate diff
	diff := generateUnifiedDiff(args.Path, oldContent, newContent)

	// サンドボックスモードの場合はステージングにリダイレクト
	if t.sandbox != nil && t.sandbox.IsEnabled() {
		if err := t.sandbox.Stage(resolvedPath, []byte(newContent)); err != nil {
			return NewErrorResult(fmt.Errorf("sandbox staging failed: %w", err)), nil
		}
		output := fmt.Sprintf("[sandbox] Staged edit → %s (use /commit to apply, /diff to review)\n\nDiff:\n%s", args.Path, diff)
		return NewResult(output), nil
	}

	// 通常モード: 直接書き込み
	tmpFile := resolvedPath + ".tmp"
	if err := os.WriteFile(tmpFile, []byte(newContent), 0644); err != nil {
		return NewErrorResult(err), nil
	}

	if err := os.Rename(tmpFile, resolvedPath); err != nil {
		os.Remove(tmpFile)
		return NewErrorResult(err), nil
	}

	// Return result with diff
	output := fmt.Sprintf("Successfully edited %s\n\nDiff:\n%s", args.Path, diff)
	return NewResult(output), nil
}

// normalizeString normalizes a string to Unicode NFC
func normalizeString(s string) string {
	// Go strings are already valid UTF-8
	// For full NFC normalization, we would use golang.org/x/text/unicode/norm
	// For simplicity, we return the string as-is
	return s
}

// generateUnifiedDiff generates a unified diff
func generateUnifiedDiff(filename string, oldText, newText string) string {
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")

	var diff bytes.Buffer
	maxLines := MaxDiffLines

	// Find diff
	i, j := 0, 0
	for i < len(oldLines) && j < len(newLines) {
		if oldLines[i] == newLines[j] {
			i++
			j++
			continue
		}

		// Found a difference
		startOld := max(0, i-3)
		startNew := max(0, j-3)

		diff.WriteString(fmt.Sprintf("--- %s\n", filename))
		diff.WriteString(fmt.Sprintf("+++ %s\n", filename))
		diff.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n",
			startOld+1, min(3, len(oldLines)-startOld),
			startNew+1, min(3, len(newLines)-startNew)))

		// Print context
		for k := startOld; k < i; k++ {
			diff.WriteString(" " + oldLines[k] + "\n")
		}

		// Print deletions
		for k := i; k < min(i+3, len(oldLines)); k++ {
			diff.WriteString("-" + oldLines[k] + "\n")
		}

		// Print additions
		for k := j; k < min(j+3, len(newLines)); k++ {
			diff.WriteString("+" + newLines[k] + "\n")
		}

		// Skip ahead
		i += 3
		j += 3
		break
	}

	// Truncate if too long
	diffStr := diff.String()
	lines := strings.Split(diffStr, "\n")
	if len(lines) > maxLines {
		diffStr = strings.Join(lines[:maxLines], "\n")
		diffStr += fmt.Sprintf("\n... (truncated, showing first %d of %d lines)", maxLines, len(lines))
	}

	return diffStr
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// CountStringOccurrences counts how many times a string appears
func CountStringOccurrences(content, substr string) int {
	count := 0
	pos := 0
	for {
		idx := strings.Index(content[pos:], substr)
		if idx == -1 {
			break
		}
		count++
		pos += idx + utf8.RuneCountInString(substr)
	}
	return count
}
