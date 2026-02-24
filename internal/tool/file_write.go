package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// MaxWriteFileSize is the maximum file size for writing
	MaxWriteFileSize = 10 * 1024 * 1024 // 10MB
	// MaxUndoStack is the maximum number of undo entries
	MaxUndoStack = 20
)

// SandboxStager はサンドボックスへのステージングインターフェース
// sandbox パッケージへの循環参照を避けるためインターフェースで定義
type SandboxStager interface {
	IsEnabled() bool
	Stage(originalPath string, content []byte) error
}

// WriteTool writes content to files
type WriteTool struct {
	baseDir    string
	undoStack  []UndoEntry
	undoMutex  sync.Mutex
	sandbox    SandboxStager
}

// NewWriteTool creates a new write tool
func NewWriteTool() *WriteTool {
	return &WriteTool{
		undoStack: make([]UndoEntry, 0),
	}
}

// SetSandbox はサンドボックスマネージャーを設定する
func (t *WriteTool) SetSandbox(sb SandboxStager) {
	t.sandbox = sb
}

// Name returns the tool name
func (t *WriteTool) Name() string {
	return "write_file"
}

// Schema returns the tool schema
func (t *WriteTool) Schema() *FunctionSchema {
	return &FunctionSchema{
		Name:        "write_file",
		Description: "Write content to a file",
		Parameters: &ParameterSchema{
			Type: "object",
			Properties: map[string]*PropertyDef{
				"path": {
					Type:        "string",
					Description: "The file path to write to",
				},
				"content": {
					Type:        "string",
					Description: "The content to write",
				},
			},
			Required: []string{"path", "content"},
		},
	}
}

// Execute writes content to a file
func (t *WriteTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}

	if err := json.Unmarshal(params, &args); err != nil {
		return NewErrorResult(err), nil
	}

	if args.Path == "" {
		return NewErrorResult(fmt.Errorf("path cannot be empty")), nil
	}

	// Check file size
	if len(args.Content) > MaxWriteFileSize {
		return NewErrorResult(fmt.Errorf("content too large (%d bytes, max %d)", len(args.Content), MaxWriteFileSize)), nil
	}

	// Resolve path
	resolvedPath, err := resolvePath(args.Path)
	if err != nil {
		return NewErrorResult(err), nil
	}

	// Check for protected paths
	if isProtectedPath(resolvedPath) {
		return NewErrorResult(fmt.Errorf("cannot write to protected path: %s", args.Path)), nil
	}

	// Check for managed/dependency directories (仮想環境・依存関係ディレクトリへの誤書き込みを防ぐ)
	if managedDir := getManagedDirWarning(resolvedPath); managedDir != "" {
		return NewErrorResult(fmt.Errorf("cannot write to managed directory %s: %s\nHint: write to the project root or a subdirectory you created", managedDir, args.Path)), nil
	}

	// Check if it's a symlink
	if isSymlink(args.Path) {
		return NewErrorResult(fmt.Errorf("cannot write to symlink: %s", args.Path)), nil
	}

	// Fix escaped newlines (\\n -> \n) - handle cases where LLM double-escapes
	content := args.Content
	// Replace literal backslash-n with actual newlines
	// This handles cases where LLM returns "\\n" instead of "\n"
	for {
		newContent := strings.ReplaceAll(content, "\\n", "\n")
		// Also handle other common escapes
		newContent = strings.ReplaceAll(newContent, "\\t", "\t")
		newContent = strings.ReplaceAll(newContent, "\\r", "\r")
		if newContent == content {
			break
		}
		content = newContent
	}

	// サンドボックスモードの場合はステージングにリダイレクト
	if t.sandbox != nil && t.sandbox.IsEnabled() {
		if err := t.sandbox.Stage(resolvedPath, []byte(content)); err != nil {
			return NewErrorResult(fmt.Errorf("sandbox staging failed: %w", err)), nil
		}
		return NewResult(fmt.Sprintf("[sandbox] Staged %d bytes → %s (use /commit to apply, /diff to review)", len(content), args.Path)), nil
	}

	// 通常モード: 直接書き込み

	// Create parent directories
	parentDir := filepath.Dir(resolvedPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return NewErrorResult(err), nil
	}

	// Save old content for undo
	oldContent := ""
	if fileExists(resolvedPath) {
		oldData, err := os.ReadFile(resolvedPath)
		if err != nil {
			return NewErrorResult(err), nil
		}
		oldContent = string(oldData)
	}

	// Write to temp file first (atomic write)
	tmpFile := resolvedPath + ".tmp"
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		return NewErrorResult(err), nil
	}

	// Rename temp file to target (atomic on Unix)
	if err := os.Rename(tmpFile, resolvedPath); err != nil {
		// Clean up temp file on error
		os.Remove(tmpFile)
		return NewErrorResult(err), nil
	}

	// Add to undo stack
	t.addToUndoStack(UndoEntry{
		Path:      resolvedPath,
		OldContent: oldContent,
		NewContent: content,
	})

	return NewResult(fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), args.Path)), nil
}

// getManagedDirWarning checks if path is inside a managed/dependency directory.
// Returns the managed dir name if the path should not be written, empty string otherwise.
// 仮想環境・依存関係・バージョン管理ディレクトリへの誤書き込みを防ぐ
func getManagedDirWarning(path string) string {
	path = filepath.Clean(path)
	// Check each path component for managed directory names
	parts := strings.Split(path, string(filepath.Separator))
	managedDirs := []string{
		".venv",        // Python virtual environment
		"venv",         // Python virtual environment (alternative name)
		"__pycache__",  // Python bytecode cache
		"node_modules", // Node.js dependencies
		".git",         // Git internals
		".tox",         // Python tox testing
		"site-packages", // Python installed packages
		"dist-packages", // Python system packages
	}
	for _, part := range parts {
		for _, managed := range managedDirs {
			if part == managed {
				return managed
			}
		}
	}
	return ""
}

// isProtectedPath checks if path is protected
func isProtectedPath(path string) bool {
	path = filepath.Clean(path)

	protectedPaths := []string{
		"/",
		"/bin",
		"/sbin",
		"/usr/bin",
		"/usr/sbin",
		"/etc/passwd",
		"/etc/shadow",
	}

	for _, pp := range protectedPaths {
		if path == pp || strings.HasPrefix(path, pp+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

// isSymlink checks if path is a symlink
func isSymlink(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSymlink != 0
}

// fileExists checks if file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// addToUndoStack adds an entry to the undo stack
func (t *WriteTool) addToUndoStack(entry UndoEntry) {
	t.undoMutex.Lock()
	defer t.undoMutex.Unlock()

	// Limit stack size
	if len(t.undoStack) >= MaxUndoStack {
		t.undoStack = t.undoStack[1:]
	}

	t.undoStack = append(t.undoStack, entry)
}

// Undo reverts the last write operation
func (t *WriteTool) Undo() error {
	t.undoMutex.Lock()
	defer t.undoMutex.Unlock()

	if len(t.undoStack) == 0 {
		return fmt.Errorf("nothing to undo")
	}

	entry := t.undoStack[len(t.undoStack)-1]
	t.undoStack = t.undoStack[:len(t.undoStack)-1]

	// Write old content back
	if entry.OldContent == "" {
		// File was new, delete it
		return os.Remove(entry.Path)
	}

	// Atomic write
	tmpFile := entry.Path + ".tmp"
	if err := os.WriteFile(tmpFile, []byte(entry.OldContent), 0644); err != nil {
		return err
	}

	if err := os.Rename(tmpFile, entry.Path); err != nil {
		os.Remove(tmpFile)
		return err
	}

	return nil
}

// GetUndoStack returns the current undo stack
func (t *WriteTool) GetUndoStack() []UndoEntry {
	t.undoMutex.Lock()
	defer t.undoMutex.Unlock()

	stack := make([]UndoEntry, len(t.undoStack))
	copy(stack, t.undoStack)
	return stack
}

// UndoEntry represents an undo entry
type UndoEntry struct {
	Path       string
	OldContent string
	NewContent string
}
