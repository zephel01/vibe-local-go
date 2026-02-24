package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// NotebookEditTool edits Jupyter notebook (.ipynb) cells
type NotebookEditTool struct{}

// NewNotebookEditTool creates a new notebook edit tool
func NewNotebookEditTool() *NotebookEditTool {
	return &NotebookEditTool{}
}

// Name returns the tool name
func (t *NotebookEditTool) Name() string {
	return "notebook_edit"
}

// Schema returns the tool schema
func (t *NotebookEditTool) Schema() *FunctionSchema {
	return &FunctionSchema{
		Name:        "notebook_edit",
		Description: "Edit a Jupyter notebook (.ipynb) cell. Supports replace, insert, and delete operations.",
		Parameters: &ParameterSchema{
			Type: "object",
			Properties: map[string]*PropertyDef{
				"path": {
					Type:        "string",
					Description: "Path to the .ipynb notebook file",
				},
				"cell_number": {
					Type:        "integer",
					Description: "0-based index of the cell to edit",
				},
				"edit_mode": {
					Type:        "string",
					Description: "Edit mode: replace, insert, or delete",
					Enum:        []string{"replace", "insert", "delete"},
					Default:     "replace",
				},
				"new_source": {
					Type:        "string",
					Description: "New source content for the cell (required for replace/insert, ignored for delete)",
				},
				"cell_type": {
					Type:        "string",
					Description: "Cell type: code or markdown (required for insert, optional for replace)",
					Enum:        []string{"code", "markdown"},
				},
			},
			Required: []string{"path", "cell_number"},
		},
	}
}

// notebookCell represents a single cell in a Jupyter notebook
type notebookCell struct {
	CellType string        `json:"cell_type"`
	Source   []string       `json:"source"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
	// code cell fields
	ExecutionCount *int            `json:"execution_count,omitempty"`
	Outputs        json.RawMessage `json:"outputs,omitempty"`
	// id field (nbformat 4.5+)
	ID string `json:"id,omitempty"`
}

// notebook represents a Jupyter notebook structure
type notebook struct {
	Cells         []notebookCell  `json:"cells"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	NBFormat      int             `json:"nbformat"`
	NBFormatMinor int             `json:"nbformat_minor"`
}

// Execute edits a notebook cell
func (t *NotebookEditTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	var args struct {
		Path       string `json:"path"`
		CellNumber int    `json:"cell_number"`
		EditMode   string `json:"edit_mode"`
		NewSource  string `json:"new_source"`
		CellType   string `json:"cell_type"`
	}

	if err := json.Unmarshal(params, &args); err != nil {
		return NewErrorResult(fmt.Errorf("invalid parameters: %v", err)), nil
	}

	if args.Path == "" {
		return NewErrorResult(fmt.Errorf("path is required")), nil
	}

	if args.EditMode == "" {
		args.EditMode = "replace"
	}

	// Validate edit_mode
	switch args.EditMode {
	case "replace", "insert", "delete":
		// valid
	default:
		return NewErrorResult(fmt.Errorf("invalid edit_mode: %s (must be replace, insert, or delete)", args.EditMode)), nil
	}

	// Resolve path
	resolvedPath, err := resolvePath(args.Path)
	if err != nil {
		return NewErrorResult(fmt.Errorf("cannot resolve path: %v", err)), nil
	}

	// Read notebook
	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return NewErrorResult(fmt.Errorf("cannot read notebook: %v", err)), nil
	}

	var nb notebook
	if err := json.Unmarshal(data, &nb); err != nil {
		return NewErrorResult(fmt.Errorf("invalid notebook format: %v", err)), nil
	}

	// Execute operation
	switch args.EditMode {
	case "replace":
		return t.replaceCell(&nb, resolvedPath, args.CellNumber, args.NewSource, args.CellType)
	case "insert":
		return t.insertCell(&nb, resolvedPath, args.CellNumber, args.NewSource, args.CellType)
	case "delete":
		return t.deleteCell(&nb, resolvedPath, args.CellNumber)
	}

	return NewErrorResult(fmt.Errorf("unexpected edit_mode: %s", args.EditMode)), nil
}

// replaceCell replaces the content of an existing cell
func (t *NotebookEditTool) replaceCell(nb *notebook, path string, cellNum int, source string, cellType string) (*Result, error) {
	if cellNum < 0 || cellNum >= len(nb.Cells) {
		return NewErrorResult(fmt.Errorf("cell_number %d out of range (notebook has %d cells)", cellNum, len(nb.Cells))), nil
	}

	// Update source
	nb.Cells[cellNum].Source = splitSource(source)

	// Optionally update cell type
	if cellType != "" {
		nb.Cells[cellNum].CellType = cellType
		if cellType == "markdown" {
			// Clear code-specific fields
			nb.Cells[cellNum].ExecutionCount = nil
			nb.Cells[cellNum].Outputs = json.RawMessage("[]")
		} else if cellType == "code" {
			// Ensure code cell has outputs array
			if nb.Cells[cellNum].Outputs == nil {
				nb.Cells[cellNum].Outputs = json.RawMessage("[]")
			}
		}
	}

	// Clear execution state on replace
	if nb.Cells[cellNum].CellType == "code" {
		nb.Cells[cellNum].ExecutionCount = nil
		nb.Cells[cellNum].Outputs = json.RawMessage("[]")
	}

	if err := writeNotebook(nb, path); err != nil {
		return NewErrorResult(err), nil
	}

	return NewResult(fmt.Sprintf("Replaced cell %d in %s", cellNum, path)), nil
}

// insertCell inserts a new cell at the specified position
func (t *NotebookEditTool) insertCell(nb *notebook, path string, cellNum int, source string, cellType string) (*Result, error) {
	if cellType == "" {
		cellType = "code"
	}

	if cellNum < 0 || cellNum > len(nb.Cells) {
		return NewErrorResult(fmt.Errorf("cell_number %d out of range for insert (valid: 0-%d)", cellNum, len(nb.Cells))), nil
	}

	newCell := notebookCell{
		CellType: cellType,
		Source:   splitSource(source),
		Metadata: json.RawMessage("{}"),
	}

	if cellType == "code" {
		newCell.Outputs = json.RawMessage("[]")
	}

	// Insert at position
	cells := make([]notebookCell, 0, len(nb.Cells)+1)
	cells = append(cells, nb.Cells[:cellNum]...)
	cells = append(cells, newCell)
	cells = append(cells, nb.Cells[cellNum:]...)
	nb.Cells = cells

	if err := writeNotebook(nb, path); err != nil {
		return NewErrorResult(err), nil
	}

	return NewResult(fmt.Sprintf("Inserted new %s cell at position %d in %s", cellType, cellNum, path)), nil
}

// deleteCell deletes a cell at the specified position
func (t *NotebookEditTool) deleteCell(nb *notebook, path string, cellNum int) (*Result, error) {
	if cellNum < 0 || cellNum >= len(nb.Cells) {
		return NewErrorResult(fmt.Errorf("cell_number %d out of range (notebook has %d cells)", cellNum, len(nb.Cells))), nil
	}

	if len(nb.Cells) <= 1 {
		return NewErrorResult(fmt.Errorf("cannot delete the last remaining cell")), nil
	}

	nb.Cells = append(nb.Cells[:cellNum], nb.Cells[cellNum+1:]...)

	if err := writeNotebook(nb, path); err != nil {
		return NewErrorResult(err), nil
	}

	return NewResult(fmt.Sprintf("Deleted cell %d from %s (now %d cells)", cellNum, path, len(nb.Cells))), nil
}

// splitSource splits source string into lines for Jupyter format
// Jupyter stores source as array of lines, each ending with \n except possibly the last
func splitSource(source string) []string {
	if source == "" {
		return []string{}
	}

	lines := strings.Split(source, "\n")
	result := make([]string, len(lines))
	for i, line := range lines {
		if i < len(lines)-1 {
			result[i] = line + "\n"
		} else {
			result[i] = line
		}
	}
	return result
}

// writeNotebook writes a notebook back to disk with proper formatting
func writeNotebook(nb *notebook, path string) error {
	data, err := json.MarshalIndent(nb, "", " ")
	if err != nil {
		return fmt.Errorf("failed to marshal notebook: %v", err)
	}

	// Jupyter notebooks end with a newline
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write notebook: %v", err)
	}

	return nil
}
