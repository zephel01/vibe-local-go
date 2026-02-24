package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// createTestNotebook creates a minimal .ipynb for testing
func createTestNotebook(t *testing.T, dir string) string {
	t.Helper()
	nb := notebook{
		Cells: []notebookCell{
			{
				CellType: "code",
				Source:   []string{"print(\"hello\")\n"},
				Metadata: json.RawMessage("{}"),
				Outputs:  json.RawMessage("[]"),
			},
			{
				CellType: "markdown",
				Source:   []string{"# Title\n"},
				Metadata: json.RawMessage("{}"),
			},
			{
				CellType: "code",
				Source:   []string{"x = 1\n", "y = 2\n"},
				Metadata: json.RawMessage("{}"),
				Outputs:  json.RawMessage("[]"),
			},
		},
		Metadata:      json.RawMessage(`{"kernelspec":{"display_name":"Python 3","language":"python","name":"python3"}}`),
		NBFormat:      4,
		NBFormatMinor: 5,
	}

	path := filepath.Join(dir, "test.ipynb")
	data, err := json.MarshalIndent(nb, "", " ")
	if err != nil {
		t.Fatalf("failed to marshal test notebook: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write test notebook: %v", err)
	}
	return path
}

// readNotebookFile reads and parses a notebook from disk
func readNotebookFile(t *testing.T, path string) notebook {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read notebook: %v", err)
	}
	var nb notebook
	if err := json.Unmarshal(data, &nb); err != nil {
		t.Fatalf("failed to parse notebook: %v", err)
	}
	return nb
}

func TestNotebookEditTool_Name(t *testing.T) {
	tool := NewNotebookEditTool()
	if tool.Name() != "notebook_edit" {
		t.Errorf("expected name 'notebook_edit', got '%s'", tool.Name())
	}
}

func TestNotebookEditTool_Schema(t *testing.T) {
	tool := NewNotebookEditTool()
	schema := tool.Schema()
	if schema.Name != "notebook_edit" {
		t.Errorf("expected schema name 'notebook_edit', got '%s'", schema.Name)
	}
	if schema.Parameters == nil {
		t.Fatal("schema parameters should not be nil")
	}
	if _, ok := schema.Parameters.Properties["path"]; !ok {
		t.Error("schema should have 'path' property")
	}
	if _, ok := schema.Parameters.Properties["cell_number"]; !ok {
		t.Error("schema should have 'cell_number' property")
	}
}

func TestNotebookEditTool_ReplaceCell(t *testing.T) {
	dir := t.TempDir()
	path := createTestNotebook(t, dir)
	tool := NewNotebookEditTool()

	params, _ := json.Marshal(map[string]interface{}{
		"path":        path,
		"cell_number": 0,
		"edit_mode":   "replace",
		"new_source":  "print(\"world\")",
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Error)
	}

	nb := readNotebookFile(t, path)
	if len(nb.Cells) != 3 {
		t.Errorf("expected 3 cells, got %d", len(nb.Cells))
	}
	got := joinSource(nb.Cells[0].Source)
	if got != "print(\"world\")" {
		t.Errorf("expected 'print(\"world\")', got '%s'", got)
	}
}

func TestNotebookEditTool_ReplaceCellWithType(t *testing.T) {
	dir := t.TempDir()
	path := createTestNotebook(t, dir)
	tool := NewNotebookEditTool()

	// Change code cell to markdown
	params, _ := json.Marshal(map[string]interface{}{
		"path":        path,
		"cell_number": 0,
		"edit_mode":   "replace",
		"new_source":  "# Now markdown",
		"cell_type":   "markdown",
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Error)
	}

	nb := readNotebookFile(t, path)
	if nb.Cells[0].CellType != "markdown" {
		t.Errorf("expected cell type 'markdown', got '%s'", nb.Cells[0].CellType)
	}
}

func TestNotebookEditTool_InsertCell(t *testing.T) {
	dir := t.TempDir()
	path := createTestNotebook(t, dir)
	tool := NewNotebookEditTool()

	params, _ := json.Marshal(map[string]interface{}{
		"path":        path,
		"cell_number": 1,
		"edit_mode":   "insert",
		"new_source":  "# Inserted cell",
		"cell_type":   "markdown",
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Error)
	}

	nb := readNotebookFile(t, path)
	if len(nb.Cells) != 4 {
		t.Errorf("expected 4 cells after insert, got %d", len(nb.Cells))
	}
	if nb.Cells[1].CellType != "markdown" {
		t.Errorf("expected inserted cell type 'markdown', got '%s'", nb.Cells[1].CellType)
	}
	got := joinSource(nb.Cells[1].Source)
	if got != "# Inserted cell" {
		t.Errorf("expected '# Inserted cell', got '%s'", got)
	}
}

func TestNotebookEditTool_InsertAtEnd(t *testing.T) {
	dir := t.TempDir()
	path := createTestNotebook(t, dir)
	tool := NewNotebookEditTool()

	params, _ := json.Marshal(map[string]interface{}{
		"path":        path,
		"cell_number": 3, // insert at end (after all 3 cells)
		"edit_mode":   "insert",
		"new_source":  "# End",
		"cell_type":   "markdown",
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Error)
	}

	nb := readNotebookFile(t, path)
	if len(nb.Cells) != 4 {
		t.Errorf("expected 4 cells, got %d", len(nb.Cells))
	}
	got := joinSource(nb.Cells[3].Source)
	if got != "# End" {
		t.Errorf("expected '# End', got '%s'", got)
	}
}

func TestNotebookEditTool_DeleteCell(t *testing.T) {
	dir := t.TempDir()
	path := createTestNotebook(t, dir)
	tool := NewNotebookEditTool()

	params, _ := json.Marshal(map[string]interface{}{
		"path":        path,
		"cell_number": 1,
		"edit_mode":   "delete",
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Error)
	}

	nb := readNotebookFile(t, path)
	if len(nb.Cells) != 2 {
		t.Errorf("expected 2 cells after delete, got %d", len(nb.Cells))
	}
	// Cell 1 should now be the old cell 2
	if nb.Cells[1].CellType != "code" {
		t.Errorf("expected remaining cell 1 to be code, got '%s'", nb.Cells[1].CellType)
	}
}

func TestNotebookEditTool_DeleteLastCell(t *testing.T) {
	dir := t.TempDir()
	// Create notebook with only 1 cell
	nb := notebook{
		Cells: []notebookCell{
			{CellType: "code", Source: []string{"x = 1\n"}, Metadata: json.RawMessage("{}"), Outputs: json.RawMessage("[]")},
		},
		NBFormat:      4,
		NBFormatMinor: 5,
	}
	path := filepath.Join(dir, "single.ipynb")
	data, _ := json.MarshalIndent(nb, "", " ")
	data = append(data, '\n')
	os.WriteFile(path, data, 0644)

	tool := NewNotebookEditTool()
	params, _ := json.Marshal(map[string]interface{}{
		"path":        path,
		"cell_number": 0,
		"edit_mode":   "delete",
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error when deleting last cell")
	}
}

func TestNotebookEditTool_OutOfRange(t *testing.T) {
	dir := t.TempDir()
	path := createTestNotebook(t, dir)
	tool := NewNotebookEditTool()

	tests := []struct {
		name     string
		mode     string
		cellNum  int
	}{
		{"replace out of range", "replace", 10},
		{"replace negative", "replace", -1},
		{"delete out of range", "delete", 5},
		{"insert out of range", "insert", 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, _ := json.Marshal(map[string]interface{}{
				"path":        path,
				"cell_number": tt.cellNum,
				"edit_mode":   tt.mode,
				"new_source":  "test",
			})
			result, err := tool.Execute(context.Background(), params)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Errorf("expected error for %s with cell %d", tt.mode, tt.cellNum)
			}
		})
	}
}

func TestNotebookEditTool_InvalidMode(t *testing.T) {
	dir := t.TempDir()
	path := createTestNotebook(t, dir)
	tool := NewNotebookEditTool()

	params, _ := json.Marshal(map[string]interface{}{
		"path":        path,
		"cell_number": 0,
		"edit_mode":   "invalid",
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for invalid edit_mode")
	}
}

func TestNotebookEditTool_NonExistentFile(t *testing.T) {
	tool := NewNotebookEditTool()
	params, _ := json.Marshal(map[string]interface{}{
		"path":        "/nonexistent/test.ipynb",
		"cell_number": 0,
		"edit_mode":   "replace",
		"new_source":  "test",
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error for non-existent file")
	}
}

func TestNotebookEditTool_DefaultMode(t *testing.T) {
	dir := t.TempDir()
	path := createTestNotebook(t, dir)
	tool := NewNotebookEditTool()

	// No edit_mode specified — should default to "replace"
	params, _ := json.Marshal(map[string]interface{}{
		"path":        path,
		"cell_number": 0,
		"new_source":  "replaced",
	})

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Error)
	}

	nb := readNotebookFile(t, path)
	got := joinSource(nb.Cells[0].Source)
	if got != "replaced" {
		t.Errorf("expected 'replaced', got '%s'", got)
	}
}

func TestSplitSource(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"", []string{}},
		{"hello", []string{"hello"}},
		{"line1\nline2", []string{"line1\n", "line2"}},
		{"line1\nline2\nline3", []string{"line1\n", "line2\n", "line3"}},
		{"line1\n", []string{"line1\n", ""}},
	}

	for _, tt := range tests {
		result := splitSource(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("splitSource(%q): expected %d parts, got %d", tt.input, len(tt.expected), len(result))
			continue
		}
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("splitSource(%q)[%d]: expected %q, got %q", tt.input, i, tt.expected[i], result[i])
			}
		}
	}
}

func TestNotebookEditTool_PreservesMetadata(t *testing.T) {
	dir := t.TempDir()
	path := createTestNotebook(t, dir)
	tool := NewNotebookEditTool()

	params, _ := json.Marshal(map[string]interface{}{
		"path":        path,
		"cell_number": 0,
		"edit_mode":   "replace",
		"new_source":  "updated",
	})

	tool.Execute(context.Background(), params)

	nb := readNotebookFile(t, path)
	// Verify notebook-level metadata is preserved
	if nb.NBFormat != 4 {
		t.Errorf("expected nbformat 4, got %d", nb.NBFormat)
	}
	if nb.NBFormatMinor != 5 {
		t.Errorf("expected nbformat_minor 5, got %d", nb.NBFormatMinor)
	}
	if nb.Metadata == nil {
		t.Error("notebook metadata should be preserved")
	}
}

// joinSource joins source lines back into a single string (strips trailing newlines from intermediate lines)
func joinSource(source []string) string {
	if len(source) == 0 {
		return ""
	}
	var result string
	for _, s := range source {
		result += s
	}
	// Trim trailing newline for comparison
	return trimTrailingNewline(result)
}

func trimTrailingNewline(s string) string {
	if len(s) > 0 && s[len(s)-1] == '\n' {
		return s[:len(s)-1]
	}
	return s
}
