package tool

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// MaxTextFileSize is the maximum file size for text files
	MaxTextFileSize = 100 * 1024 * 1024 // 100MB
	// MaxImageFileSize is the maximum file size for images
	MaxImageFileSize = 10 * 1024 * 1024 // 10MB
	// DefaultLineLimit is the default number of lines to read
	DefaultLineLimit = 2000
	// MaxLineLimit is the maximum number of lines to read
	MaxLineLimit = 20000
)

// ReadTool reads file contents
type ReadTool struct {
	baseDir string
}

// NewReadTool creates a new read tool
func NewReadTool() *ReadTool {
	return &ReadTool{}
}

// Name returns the tool name
func (t *ReadTool) Name() string {
	return "read_file"
}

// Schema returns the tool schema
func (t *ReadTool) Schema() *FunctionSchema {
	return &FunctionSchema{
		Name:        "read_file",
		Description: "Read the contents of a file",
		Parameters: &ParameterSchema{
			Type: "object",
			Properties: map[string]*PropertyDef{
				"path": {
					Type:        "string",
					Description: "The file path to read",
				},
				"offset": {
					Type:        "integer",
					Description: "Starting line number (0-based)",
					Default:     0,
				},
				"limit": {
					Type:        "integer",
					Description: "Maximum number of lines to read",
					Default:     DefaultLineLimit,
				},
			},
			Required: []string{"path"},
		},
	}
}

// Execute reads a file
func (t *ReadTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	var args struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}

	if err := json.Unmarshal(params, &args); err != nil {
		return NewErrorResult(err), nil
	}

	if args.Path == "" {
		return NewErrorResult(fmt.Errorf("path cannot be empty")), nil
	}

	// Set default limit
	if args.Limit <= 0 {
		args.Limit = DefaultLineLimit
	}
	if args.Limit > MaxLineLimit {
		args.Limit = MaxLineLimit
	}

	// Resolve path
	resolvedPath, err := resolvePath(args.Path)
	if err != nil {
		return NewErrorResult(err), nil
	}

	// Get file info
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return NewErrorResult(err), nil
	}

	// Check if directory
	if info.IsDir() {
		return NewErrorResult(fmt.Errorf("path is a directory: %s", args.Path)), nil
	}

	// Check file size
	if info.Size() > MaxTextFileSize {
		return NewErrorResult(fmt.Errorf("file too large (%d bytes, max %d)", info.Size(), MaxTextFileSize)), nil
	}

	// Determine file type
	ext := strings.ToLower(filepath.Ext(args.Path))

	// Check if image
	if isImageFile(ext) {
		return t.readImage(resolvedPath)
	}

	// Check if Jupyter notebook
	if ext == ".ipynb" {
		return t.readJSON(resolvedPath)
	}

	// Read as text
	return t.readText(resolvedPath, args.Offset, args.Limit)
}

// readText reads a text file
func (t *ReadTool) readText(path string, offset, limit int) (*Result, error) {
	file, err := os.Open(path)
	if err != nil {
		return NewErrorResult(err), nil
	}
	defer file.Close()

	// Check for binary content
	if isBinary(file) {
		return NewErrorResult(fmt.Errorf("file appears to be binary")), nil
	}

	// Reset file position
	file.Seek(0, io.SeekStart)

	// Read lines
	var lines []string
	scanner := readLines(file, offset, limit)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return NewErrorResult(err), nil
	}

	// Format output with line numbers
	var output strings.Builder
	if offset > 0 {
		output.WriteString(fmt.Sprintf("(showing lines %d-%d)\n", offset+1, offset+len(lines)))
	}

	for i, line := range lines {
		output.WriteString(fmt.Sprintf("%5d | %s\n", offset+i+1, line))
	}

	return NewResult(output.String()), nil
}

// readImage reads an image file and returns base64
func (t *ReadTool) readImage(path string) (*Result, error) {
	file, err := os.Open(path)
	if err != nil {
		return NewErrorResult(err), nil
	}
	defer file.Close()

	// Check file size
	info, err := file.Stat()
	if err != nil {
		return NewErrorResult(err), nil
	}

	if info.Size() > MaxImageFileSize {
		return NewErrorResult(fmt.Errorf("image too large (%d bytes, max %d)", info.Size(), MaxImageFileSize)), nil
	}

	// Read file
	data, err := io.ReadAll(file)
	if err != nil {
		return NewErrorResult(err), nil
	}

	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(data)
	ext := strings.ToLower(filepath.Ext(path))

	output := fmt.Sprintf("Image file (base64 encoded)\nFormat: %s\nSize: %d bytes\n\n%s",
		ext, info.Size(), encoded)

	return NewResult(output), nil
}

// readJSON reads a JSON file (e.g., .ipynb)
func (t *ReadTool) readJSON(path string) (*Result, error) {
	file, err := os.Open(path)
	if err != nil {
		return NewErrorResult(err), nil
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return NewErrorResult(err), nil
	}

	// Pretty print JSON
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "", "  "); err != nil {
		// Invalid JSON, return raw
		return NewResult(string(data)), nil
	}

	return NewResult(pretty.String()), nil
}

// isBinary checks if a file is binary
func isBinary(file *os.File) bool {
	// Read first 512 bytes
	buf := make([]byte, 512)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return false
	}

	// Check for null byte
	for i := 0; i < n; i++ {
		if buf[i] == 0 {
			return true
		}
	}

	// Reset file position
	file.Seek(0, io.SeekStart)
	return false
}

// isImageFile checks if file extension indicates an image
func isImageFile(ext string) bool {
	imageExts := []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".tiff", ".bmp"}
	for _, imgExt := range imageExts {
		if strings.EqualFold(ext, imgExt) {
			return true
		}
	}
	return false
}

// resolvePath resolves symlinks and validates path.
// For paths that don't exist yet (e.g. write_file creating new files),
// it resolves the closest existing ancestor and appends the remaining components.
func resolvePath(path string) (string, error) {
	// Clean path
	path = filepath.Clean(path)

	// Make absolute if relative
	if !filepath.IsAbs(path) {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return "", err
		}
		path = absPath
	}

	// Resolve symlinks
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		// If the path doesn't exist yet (common for write operations),
		// try to resolve the closest existing ancestor directory instead.
		if os.IsNotExist(err) {
			return resolveNonExistentPath(path)
		}
		return "", err
	}

	return resolved, nil
}

// resolveNonExistentPath resolves a path where the target (and possibly some
// parent directories) don't exist yet. It finds the closest existing ancestor,
// resolves symlinks on that, and appends the remaining path components.
func resolveNonExistentPath(path string) (string, error) {
	current := path
	var missingParts []string

	for {
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root — just return the cleaned path
			return path, nil
		}

		missingParts = append([]string{filepath.Base(current)}, missingParts...)
		current = parent

		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			// Found existing ancestor, reconstruct full path
			return filepath.Join(append([]string{resolved}, missingParts...)...), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
	}
}

// lineScanner implements a scanner with offset and limit
type lineScanner struct {
	lines   []string
	offset  int
	limit   int
	current int
}

func readLines(file *os.File, offset, limit int) *lineScanner {
	lines := make([]string, 0, limit)
	scanner := bufio.NewScanner(file)

	current := 0
	for scanner.Scan() && len(lines) < limit {
		if current >= offset {
			lines = append(lines, scanner.Text())
		}
		current++
	}

	return &lineScanner{
		lines:   lines,
		offset:  offset,
		limit:   limit,
		current: 0,
	}
}

// Scan implements scanner interface
func (ls *lineScanner) Scan() bool {
	return ls.current < len(ls.lines)
}

// Text returns current line
func (ls *lineScanner) Text() string {
	line := ls.lines[ls.current]
	ls.current++
	return line
}

// Err returns error
func (ls *lineScanner) Err() error {
	return nil
}
