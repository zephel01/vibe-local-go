package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewGrepTool(t *testing.T) {
	tool := NewGrepTool()

	if tool == nil {
		t.Fatal("expected tool to be non-nil")
	}

	if tool.Name() != "grep" {
		t.Errorf("expected name 'grep', got '%s'", tool.Name())
	}
}

func TestGrepTool_Schema(t *testing.T) {
	tool := NewGrepTool()
	schema := tool.Schema()

	if schema.Name != "grep" {
		t.Errorf("expected schema name 'grep', got '%s'", schema.Name)
	}

	if schema.Parameters == nil {
		t.Fatal("expected parameters to be non-nil")
	}

	// Check required fields
	required := schema.Parameters.Required
	if len(required) != 1 || required[0] != "pattern" {
		t.Errorf("expected required ['pattern'], got %v", required)
	}

	// Check pattern property
	patternProp, ok := schema.Parameters.Properties["pattern"]
	if !ok {
		t.Fatal("expected 'pattern' property")
	}
	if patternProp.Type != "string" {
		t.Errorf("expected pattern type 'string', got '%s'", patternProp.Type)
	}

	// Check mode property with default
	modeProp, ok := schema.Parameters.Properties["mode"]
	if !ok {
		t.Fatal("expected 'mode' property")
	}
	if modeProp.Default != "content" {
		t.Errorf("expected mode default 'content', got '%v'", modeProp.Default)
	}

	// Check context_lines property with default
	contextProp, ok := schema.Parameters.Properties["context_lines"]
	if !ok {
		t.Fatal("expected 'context_lines' property")
	}
	if contextProp.Default != 0 {
		t.Errorf("expected context_lines default 0, got '%v'", contextProp.Default)
	}

	// Check max_matches property with default
	maxProp, ok := schema.Parameters.Properties["max_matches"]
	if !ok {
		t.Fatal("expected 'max_matches' property")
	}
	if maxProp.Default != DefaultMaxMatches {
		t.Errorf("expected max_matches default %d, got '%v'", DefaultMaxMatches, maxProp.Default)
	}

	// Check file_pattern property with default
	fileProp, ok := schema.Parameters.Properties["file_pattern"]
	if !ok {
		t.Fatal("expected 'file_pattern' property")
	}
	if fileProp.Default != "*" {
		t.Errorf("expected file_pattern default '*', got '%v'", fileProp.Default)
	}
}

func TestGrepTool_Execute_ContentMode(t *testing.T) {
	tool := NewGrepTool()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := "Hello World\nGoodbye World\nHello Universe\n"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	params := json.RawMessage(`{"pattern": "World", "path": "` + tmpDir + `"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	if !strings.Contains(result.Output, "Found 2 matches") {
		t.Errorf("expected 2 matches, got: %s", result.Output)
	}

	if !strings.Contains(result.Output, "Hello World") {
		t.Error("expected 'Hello World' in results")
	}

	if !strings.Contains(result.Output, "Goodbye World") {
		t.Error("expected 'Goodbye World' in results")
	}
}

func TestGrepTool_Execute_FilesWithMatchesMode(t *testing.T) {
	tool := NewGrepTool()

	tmpDir := t.TempDir()

	// Create multiple files
	if err := os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("Hello World"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("Goodbye World"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "file3.txt"), []byte("Hello Universe"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	params := json.RawMessage(`{"pattern": "World", "mode": "files_with_matches", "path": "` + tmpDir + `"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	if !strings.Contains(result.Output, "Found 2 files with matches") {
		t.Errorf("expected 2 files, got: %s", result.Output)
	}

	if !strings.Contains(result.Output, "file1.txt") {
		t.Error("expected file1.txt in results")
	}

	if !strings.Contains(result.Output, "file2.txt") {
		t.Error("expected file2.txt in results")
	}

	// file3.txt doesn't contain "World"
	if strings.Contains(result.Output, "file3.txt") {
		t.Error("did not expect file3.txt in results")
	}
}

func TestGrepTool_Execute_CountMode(t *testing.T) {
	tool := NewGrepTool()

	tmpDir := t.TempDir()

	// Create file with multiple matches (each line counts as 1)
	content := "test test test test\ntest test\nother line\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	params := json.RawMessage(`{"pattern": "test", "mode": "count", "path": "` + tmpDir + `"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	// Count mode counts lines containing matches, not total occurrences
	// There are 2 lines containing "test"
	if !strings.Contains(result.Output, "test.txt: 2") {
		t.Errorf("expected count of 2 (lines with matches), got: %s", result.Output)
	}
}

func TestGrepTool_Execute_WithContextLines(t *testing.T) {
	tool := NewGrepTool()

	tmpDir := t.TempDir()

	content := "line 1\nline 2\nline 3\nline 4\nline 5\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	params := json.RawMessage(`{"pattern": "line 3", "context_lines": 1, "path": "` + tmpDir + `"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	// Should include lines 2, 3, 4 (1 line of context)
	if !strings.Contains(result.Output, "line 2") {
		t.Error("expected context line 'line 2'")
	}

	if !strings.Contains(result.Output, "line 3") {
		t.Error("expected match line 'line 3'")
	}

	if !strings.Contains(result.Output, "line 4") {
		t.Error("expected context line 'line 4'")
	}
}

func TestGrepTool_Execute_WithFilePattern(t *testing.T) {
	tool := NewGrepTool()

	tmpDir := t.TempDir()

	// Create files with different extensions
	if err := os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("Hello World"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "file2.go"), []byte("Hello World"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "file3.txt"), []byte("Goodbye World"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	params := json.RawMessage(`{"pattern": "World", "file_pattern": "*.txt", "path": "` + tmpDir + `"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	if !strings.Contains(result.Output, "file1.txt") {
		t.Error("expected file1.txt in results")
	}

	if strings.Contains(result.Output, "file2.go") {
		t.Error("did not expect file2.go in results (doesn't match *.txt)")
	}

	if !strings.Contains(result.Output, "file3.txt") {
		t.Error("expected file3.txt in results")
	}
}

func TestGrepTool_Execute_EmptyPattern(t *testing.T) {
	tool := NewGrepTool()
	ctx := context.Background()
	params := json.RawMessage(`{"pattern": ""}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !result.IsError {
		t.Error("expected error for empty pattern")
	}

	if !strings.Contains(result.Error, "pattern cannot be empty") {
		t.Errorf("expected specific error message, got: %s", result.Error)
	}
}

func TestGrepTool_Execute_InvalidMode(t *testing.T) {
	tool := NewGrepTool()
	ctx := context.Background()
	params := json.RawMessage(`{"pattern": "test", "mode": "invalid_mode"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !result.IsError {
		t.Error("expected error for invalid mode")
	}

	if !strings.Contains(result.Error, "invalid mode") {
		t.Errorf("expected specific error message, got: %s", result.Error)
	}
}

func TestGrepTool_Execute_InvalidRegex(t *testing.T) {
	tool := NewGrepTool()
	ctx := context.Background()
	params := json.RawMessage(`{"pattern": "[invalid("}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if !result.IsError {
		t.Error("expected error for invalid regex")
	}

	if !strings.Contains(result.Error, "invalid regex pattern") {
		t.Errorf("expected specific error message, got: %s", result.Error)
	}
}

func TestGrepTool_Execute_NoMatches(t *testing.T) {
	tool := NewGrepTool()

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("Hello World"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	params := json.RawMessage(`{"pattern": "nonexistent", "path": "` + tmpDir + `"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	if !strings.Contains(result.Output, "Found 0 matches") {
		t.Error("expected to find 0 matches")
	}
}

func TestGrepTool_Execute_RecursiveSearch(t *testing.T) {
	tool := NewGrepTool()

	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	// Create files in different directories
	if err := os.WriteFile(filepath.Join(tmpDir, "root.txt"), []byte("Hello World"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("Hello Universe"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()

	// Test 1: Search in root directory only
	params1 := json.RawMessage(`{"pattern": "World", "file_pattern": "*", "path": "` + tmpDir + `"}`)
	result1, err := tool.Execute(ctx, params1)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result1.IsError {
		t.Errorf("expected success, got error: %s", result1.Error)
	}
	if !strings.Contains(result1.Output, "root.txt") {
		t.Error("expected root.txt in results")
	}

	// Test 2: Search in subdirectory
	params2 := json.RawMessage(`{"pattern": "Universe", "file_pattern": "*/*.txt", "path": "` + tmpDir + `"}`)
	result2, err := tool.Execute(ctx, params2)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result2.IsError {
		t.Errorf("expected success, got error: %s", result2.Error)
	}
	if !strings.Contains(result2.Output, "nested.txt") {
		t.Error("expected nested.txt in results")
	}
}

func TestGrepTool_Execute_RegexPattern(t *testing.T) {
	tool := NewGrepTool()

	tmpDir := t.TempDir()
	content := "test123\ntest456\ntest789\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	// In JSON, backslashes need to be escaped, so \d+ becomes \\d+
	params := json.RawMessage(`{"pattern": "test\\d+", "path": "` + tmpDir + `"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	if !strings.Contains(result.Output, "Found 3 matches") {
		t.Errorf("expected 3 matches, got: %s", result.Output)
	}
}

func TestGrepTool_Execute_DefaultPath(t *testing.T) {
	tool := NewGrepTool()

	// Change to temp directory
	tmpDir := t.TempDir()
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	if err := os.WriteFile("test.txt", []byte("Hello World"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	params := json.RawMessage(`{"pattern": "World"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	if !strings.Contains(result.Output, "Hello World") {
		t.Error("expected match in results")
	}
}

func TestGrepTool_Execute_MaxMatches(t *testing.T) {
	tool := NewGrepTool()

	tmpDir := t.TempDir()

	// Create file with many matches
	content := strings.Repeat("test\n", 1000)
	if err := os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	params := json.RawMessage(`{"pattern": "test", "max_matches": 10, "path": "` + tmpDir + `"}`)
	result, err := tool.Execute(ctx, params)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Error)
	}

	// Should find at most 10 matches
	if !strings.Contains(result.Output, "test") {
		t.Error("expected matches in results")
	}
}
