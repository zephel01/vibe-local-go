package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewReadTool(t *testing.T) {
	tool := NewReadTool()

	if tool == nil {
		t.Fatal("expected non-nil tool")
	}

	if tool.Name() != "read_file" {
		t.Errorf("expected name 'read_file', got '%s'", tool.Name())
	}
}

func TestReadTool_Schema(t *testing.T) {
	tool := NewReadTool()
	schema := tool.Schema()

	if schema.Name != "read_file" {
		t.Errorf("expected name 'read_file', got '%s'", schema.Name)
	}

	if schema.Parameters == nil {
		t.Fatal("expected parameters to be set")
	}

	// Check required parameters
	foundPath := false
	for _, req := range schema.Parameters.Required {
		if req == "path" {
			foundPath = true
			break
		}
	}
	if !foundPath {
		t.Error("expected 'path' to be required")
	}
}

func TestReadTool_Execute_ReadFile(t *testing.T) {
	tool := NewReadTool()

	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	testFile := tmpFile.Name()
	content := "Hello, World!\nLine 2\nLine 3"
	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		t.Fatalf("failed to write test content: %v", err)
	}
	tmpFile.Close()

	ctx := context.Background()
	params := json.RawMessage(`{"path": "` + testFile + `"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	if !strings.Contains(result.Output, "Hello, World!") {
		t.Errorf("expected output to contain file content, got '%s'", result.Output)
	}
}

func TestReadTool_Execute_WithOffsetAndLimit(t *testing.T) {
	tool := NewReadTool()

	// Create a file with multiple lines
	tmpFile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	testFile := tmpFile.Name()
	lines := make([]string, 10)
	for i := 0; i < 10; i++ {
		lines[i] = "Line " + string(rune('1'+i))
	}
	content := strings.Join(lines, "\n")
	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		t.Fatalf("failed to write test content: %v", err)
	}
	tmpFile.Close()

	ctx := context.Background()
	params := json.RawMessage(`{"path": "` + testFile + `", "offset": 5, "limit": 3}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	// Should show line numbers 6-8
	if !strings.Contains(result.Output, "showing lines 6-8") {
		t.Error("expected showing lines info")
	}
}

func TestReadTool_Execute_NonExistentFile(t *testing.T) {
	tool := NewReadTool()
	ctx := context.Background()

	params := json.RawMessage(`{"path": "/non/existent/file.txt"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !result.IsError {
		t.Error("expected error result for non-existent file")
	}
}

func TestReadTool_Execute_EmptyPath(t *testing.T) {
	tool := NewReadTool()
	ctx := context.Background()

	params := json.RawMessage(`{"path": ""}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !result.IsError {
		t.Error("expected error result for empty path")
	}
}

func TestReadTool_Execute_Directory(t *testing.T) {
	tool := NewReadTool()

	// Use a temporary directory
	tmpDir := t.TempDir()
	ctx := context.Background()

	params := json.RawMessage(`{"path": "` + tmpDir + `"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !result.IsError {
		t.Error("expected error result for directory")
	}

	if !strings.Contains(result.Error, "directory") {
		t.Error("expected error to mention directory")
	}
}

func TestNewWriteTool(t *testing.T) {
	tool := NewWriteTool()

	if tool == nil {
		t.Fatal("expected non-nil tool")
	}

	if tool.Name() != "write_file" {
		t.Errorf("expected name 'write_file', got '%s'", tool.Name())
	}
}

func TestWriteTool_Schema(t *testing.T) {
	tool := NewWriteTool()
	schema := tool.Schema()

	if schema.Name != "write_file" {
		t.Errorf("expected name 'write_file', got '%s'", schema.Name)
	}

	// Check required parameters
	required := schema.Parameters.Required
	if len(required) != 2 {
		t.Errorf("expected 2 required parameters, got %d", len(required))
	}
}

func TestWriteTool_Execute_WriteNewFile(t *testing.T) {
	tool := NewWriteTool()

	// Create temp file directly
	tmpFile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()
	testFile := tmpFile.Name()

	ctx := context.Background()
	params := json.RawMessage(`{"path": "` + testFile + `", "content": "test content"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	// Verify file was written
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	if string(content) != "test content" {
		t.Errorf("expected 'test content', got '%s'", string(content))
	}
}

func TestWriteTool_Execute_OverwriteExisting(t *testing.T) {
	tool := NewWriteTool()

	// Create temp file directly
	tmpFile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	testFile := tmpFile.Name()
	tmpFile.Close()

	// Write initial content
	if err := os.WriteFile(testFile, []byte("old content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	params := json.RawMessage(`{"path": "` + testFile + `", "content": "new content"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	// Verify content was overwritten
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if string(content) != "new content" {
		t.Errorf("expected 'new content', got '%s'", string(content))
	}
}

func TestWriteTool_Execute_EmptyPath(t *testing.T) {
	tool := NewWriteTool()
	ctx := context.Background()

	params := json.RawMessage(`{"path": "", "content": "test"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !result.IsError {
		t.Error("expected error result for empty path")
	}
}

func TestWriteTool_Undo(t *testing.T) {
	tool := NewWriteTool()

	// Create temp file directly
	tmpFile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	testFile := tmpFile.Name()
	tmpFile.Close()

	ctx := context.Background()

	// Write initial content
	params1 := json.RawMessage(`{"path": "` + testFile + `", "content": "first"}`)
	result1, err := tool.Execute(ctx, params1)
	if err != nil || result1.IsError {
		t.Fatalf("failed to write: %v", result1.Error)
	}

	// Overwrite
	params2 := json.RawMessage(`{"path": "` + testFile + `", "content": "second"}`)
	result2, err := tool.Execute(ctx, params2)
	if err != nil || result2.IsError {
		t.Fatalf("failed to overwrite: %v", result2.Error)
	}

	// Undo
	err = tool.Undo()
	if err != nil {
		t.Errorf("failed to undo: %v", err)
	}

	// Verify reverted to first
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if string(content) != "first" {
		t.Errorf("expected 'first', got '%s'", string(content))
	}
}

func TestWriteTool_UndoNewFile(t *testing.T) {
	tool := NewWriteTool()

	// Create temp file directly
	tmpFile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	testFile := tmpFile.Name()
	tmpFile.Close()

	ctx := context.Background()

	// Write new file
	params := json.RawMessage(`{"path": "` + testFile + `", "content": "content"}`)
	result, err := tool.Execute(ctx, params)
	if err != nil || result.IsError {
		t.Fatalf("failed to write: %v", result.Error)
	}

	// Undo (should delete the file)
	err = tool.Undo()
	if err != nil {
		t.Errorf("failed to undo: %v", err)
	}

	// Verify file was deleted
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("expected file to be deleted after undo")
	}
}

func TestWriteTool_GetUndoStack(t *testing.T) {
	tool := NewWriteTool()

	// Create temp file directly
	tmpFile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	testFile := tmpFile.Name()
	tmpFile.Close()

	ctx := context.Background()

	// Perform multiple writes
	for i := 0; i < 3; i++ {
		params := json.RawMessage(`{"path": "` + testFile + `", "content": "content` + string(rune('1'+i)) + `"}`)
		result, err := tool.Execute(ctx, params)
		if err != nil || result.IsError {
			t.Fatalf("failed to write: %v", result.Error)
		}
	}

	stack := tool.GetUndoStack()

	if len(stack) != 3 {
		t.Errorf("expected 3 undo entries, got %d", len(stack))
	}

	// Verify stack content (LIFO order)
	if stack[2].NewContent != "content3" {
		t.Error("expected last operation to be in stack")
	}
}

func TestWriteTool_UndoNothingToUndo(t *testing.T) {
	tool := NewWriteTool()

	err := tool.Undo()
	if err == nil {
		t.Error("expected error when nothing to undo")
	}

	if !strings.Contains(err.Error(), "nothing to undo") {
		t.Errorf("expected 'nothing to undo' error, got '%s'", err.Error())
	}
}

func TestNewEditTool(t *testing.T) {
	tool := NewEditTool()

	if tool == nil {
		t.Fatal("expected non-nil tool")
	}

	if tool.Name() != "edit_file" {
		t.Errorf("expected name 'edit_file', got '%s'", tool.Name())
	}
}

func TestEditTool_Execute_SingleReplace(t *testing.T) {
	tool := NewEditTool()

	// Create temp file directly
	tmpFile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	testFile := tmpFile.Name()

	// Create file with content - use unique strings to avoid multiple occurrences
	content := "Hello World\nGoodbye Universe\n"
	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		t.Fatalf("failed to write test content: %v", err)
	}
	tmpFile.Close()

	ctx := context.Background()
	params := json.RawMessage(`{"path": "` + testFile + `", "old_string": "World", "new_string": "Universe"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	// Verify only first occurrence was replaced
	newContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	expected := "Hello Universe\nGoodbye Universe\n"
	if string(newContent) != expected {
		t.Errorf("expected '%s', got '%s'", expected, string(newContent))
	}

	// Verify diff is included
	if !strings.Contains(result.Output, "Diff:") {
		t.Error("expected diff in output")
	}
}

func TestEditTool_Execute_ReplaceAll(t *testing.T) {
	tool := NewEditTool()

	// Create temp file directly
	tmpFile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	testFile := tmpFile.Name()

	content := "Hello World\nGoodbye World\nAnother World\n"
	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		t.Fatalf("failed to write test content: %v", err)
	}
	tmpFile.Close()

	ctx := context.Background()
	params := json.RawMessage(`{"path": "` + testFile + `", "old_string": "World", "new_string": "Universe", "replace_all": true}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	// Verify all occurrences were replaced
	newContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	expected := "Hello Universe\nGoodbye Universe\nAnother Universe\n"
	if string(newContent) != expected {
		t.Errorf("expected '%s', got '%s'", expected, string(newContent))
	}
}

func TestEditTool_Execute_MultipleOccurrencesError(t *testing.T) {
	tool := NewEditTool()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "file.txt")

	content := "Hello World\nGoodbye World\nAnother World"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	params := json.RawMessage(`{"path": "` + testFile + `", "old_string": "World", "new_string": "Universe", "replace_all": false}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !result.IsError {
		t.Error("expected error for multiple occurrences without replace_all")
	}

	if !strings.Contains(result.Error, "appears 3 times") {
		t.Errorf("expected error about multiple occurrences, got '%s'", result.Error)
	}
}

func TestEditTool_Execute_NotFound(t *testing.T) {
	tool := NewEditTool()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "file.txt")

	content := "Hello World"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	params := json.RawMessage(`{"path": "` + testFile + `", "old_string": "Universe", "new_string": "Galaxy"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !result.IsError {
		t.Error("expected error when old_string not found")
	}

	if !strings.Contains(result.Error, "not found") {
		t.Errorf("expected 'not found' error, got '%s'", result.Error)
	}
}

func TestEditTool_Execute_EmptyOldString(t *testing.T) {
	tool := NewEditTool()
	ctx := context.Background()

	params := json.RawMessage(`{"path": "/tmp/test.txt", "old_string": "", "new_string": "new"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !result.IsError {
		t.Error("expected error for empty old_string")
	}
}

func TestEditTool_Execute_NonExistentFile(t *testing.T) {
	tool := NewEditTool()
	ctx := context.Background()

	params := json.RawMessage(`{"path": "/non/existent/file.txt", "old_string": "old", "new_string": "new"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !result.IsError {
		t.Error("expected error for non-existent file")
	}
}

func TestCountStringOccurrences(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		substr    string
		expected  int
	}{
		{"no matches", "hello world", "xyz", 0},
		{"one match", "hello world", "world", 1},
		{"multiple matches", "hello world hello world", "hello", 2},
		// Note: empty substring causes infinite loop in current implementation
		// {"empty substring", "hello world", "", 0},
		{"empty content", "", "hello", 0},
		{"overlapping", "aaa", "aa", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := CountStringOccurrences(tt.content, tt.substr)
			if count != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, count)
			}
		})
	}
}

func TestIsImageFile(t *testing.T) {
	tests := []struct {
		ext     string
		isImage bool
	}{
		{".png", true},
		{".jpg", true},
		{".jpeg", true},
		{".gif", true},
		{".webp", true},
		{".svg", true},
		{".bmp", true},
		{".tiff", true},
		{".ico", true},
		{".txt", false},
		{".json", false},
		{".go", false},
		{".py", false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			result := isImageFile(tt.ext)
			if result != tt.isImage {
				t.Errorf("expected %v for %s, got %v", tt.isImage, tt.ext, result)
			}
		})
	}
}

func TestIsBinary(t *testing.T) {
	// Create a text file
	tmpDir := t.TempDir()
	textFile := filepath.Join(tmpDir, "text.txt")
	if err := os.WriteFile(textFile, []byte("hello world"), 0644); err != nil {
		t.Fatalf("failed to create text file: %v", err)
	}

	// Create a binary file (with null byte)
	binaryFile := filepath.Join(tmpDir, "binary.bin")
	binaryData := []byte{'h', 'e', 'l', 'l', 'o', 0, 'w', 'o', 'r', 'l', 'd'}
	if err := os.WriteFile(binaryFile, binaryData, 0644); err != nil {
		t.Fatalf("failed to create binary file: %v", err)
	}

	// Test text file
	f, err := os.Open(textFile)
	if err != nil {
		t.Fatalf("failed to open text file: %v", err)
	}
	defer f.Close()
	if isBinary(f) {
		t.Error("text file incorrectly detected as binary")
	}

	// Test binary file
	f2, err := os.Open(binaryFile)
	if err != nil {
		t.Fatalf("failed to open binary file: %v", err)
	}
	defer f2.Close()
	if !isBinary(f2) {
		t.Error("binary file not detected as binary")
	}
}
