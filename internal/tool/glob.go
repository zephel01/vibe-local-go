package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

const (
	// MaxResults is the maximum number of results to return
	MaxResults = 200
)

// GlobTool searches for files matching patterns
type GlobTool struct {
	baseDir string
}

// NewGlobTool creates a new glob tool
func NewGlobTool() *GlobTool {
	return &GlobTool{}
}

// Name returns the tool name
func (t *GlobTool) Name() string {
	return "glob"
}

// Schema returns the tool schema
func (t *GlobTool) Schema() *FunctionSchema {
	return &FunctionSchema{
		Name:        "glob",
		Description: "Find files matching glob patterns",
		Parameters: &ParameterSchema{
			Type: "object",
			Properties: map[string]*PropertyDef{
				"pattern": {
					Type:        "string",
					Description: "Glob pattern (e.g., '*.go', 'src/**/*.js', 'test?')",
				},
				"path": {
					Type:        "string",
					Description: "Directory to search in (default: current directory)",
					Default:     ".",
				},
			},
			Required: []string{"pattern"},
		},
	}
}

// Execute searches for files
func (t *GlobTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	var args struct {
		Pattern string `json:"pattern"`
		Path    string `json:"path"`
	}

	if err := json.Unmarshal(params, &args); err != nil {
		return NewErrorResult(err), nil
	}

	if args.Pattern == "" {
		return NewErrorResult(fmt.Errorf("pattern cannot be empty")), nil
	}

	// Set default path
	if args.Path == "" {
		args.Path = "."
	}

	// Resolve path
	searchPath, err := filepath.Abs(args.Path)
	if err != nil {
		return NewErrorResult(err), nil
	}

	// Convert glob pattern
	matches, err := t.globSearch(searchPath, args.Pattern)
	if err != nil || len(matches) == 0 {
		suggestedPattern := inferFilePattern(args.Pattern)
		return NewErrorResult(fmt.Errorf("no files match '%s'. Try: bash ls %s", args.Pattern, suggestedPattern)), nil
	}

	// Format output
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Found %d files matching '%s':\n\n", len(matches), args.Pattern))

	for i, match := range matches {
		if i >= MaxResults {
			output.WriteString(fmt.Sprintf("... (showing first %d of %d results)", MaxResults, len(matches)))
			break
		}
		output.WriteString(match.Path + "\n")
	}

	return NewResult(output.String()), nil
}

// globSearch performs the actual glob search
func (t *GlobTool) globSearch(basePath, pattern string) ([]FileMatch, error) {
	var matches []FileMatch

	// Handle recursive patterns (**)
	if strings.Contains(pattern, "**") {
		return t.globRecursive(basePath, pattern)
	}

	// Non-recursive pattern
	files, err := filepath.Glob(filepath.Join(basePath, pattern))
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if info, err := os.Stat(file); err == nil && !info.IsDir() {
			matches = append(matches, FileMatch{
				Path:    file,
				ModTime: info.ModTime(),
				Size:    info.Size(),
			})
		}
	}

	return matches, nil
}

// globRecursive handles recursive glob patterns
func (t *GlobTool) globRecursive(basePath, pattern string) ([]FileMatch, error) {
	var matches []FileMatch

	// Split pattern by **
	parts := strings.Split(pattern, "**")
	if len(parts) < 2 {
		// Fallback to non-recursive
		return t.globSearch(basePath, pattern)
	}

	// Walk directory tree
	err := filepath.WalkDir(basePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// Skip directories in skip list
		if d.IsDir() && isSkipDir(path) {
			return filepath.SkipDir
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(basePath, path)
		if err != nil {
			return nil
		}

		// Check if matches pattern
		matched, err := matchPattern(relPath, pattern)
		if err != nil {
			return nil
		}

		if matched {
			if info, err := os.Stat(path); err == nil {
				matches = append(matches, FileMatch{
					Path:    path,
					ModTime: info.ModTime(),
					Size:    info.Size(),
				})
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort by modification time (newest first)
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].ModTime.After(matches[j].ModTime)
	})

	return matches, nil
}

// matchPattern checks if a path matches a glob pattern
func matchPattern(path, pattern string) (bool, error) {
	// Use doublestar.Match for proper ** pattern support
	matched, err := doublestar.Match(pattern, path)
	return matched, err
}

// isSkipDir checks if a directory should be skipped
func isSkipDir(path string) bool {
	basename := filepath.Base(path)

	skipDirs := []string{
		".git",
		".svn",
		".hg",
		".bzr",
		"node_modules",
		"__pycache__",
		".pytest_cache",
		"venv",
		"env",
		".venv",
		".idea",
		".vscode",
		"dist",
		"build",
		"target",
		"bin",
		"obj",
		"out",
	}

	for _, skip := range skipDirs {
		if basename == skip {
			return true
		}
	}

	return false
}

// FileMatch represents a matched file
type FileMatch struct {
	Path    string
	ModTime time.Time
	Size    int64
}

// inferFilePattern infers a simpler file pattern from the input pattern
func inferFilePattern(pattern string) string {
	if strings.Contains(pattern, "*") {
		// Extract extension from pattern
		if strings.Contains(pattern, ".") {
			extStart := strings.LastIndex(pattern, ".")
			if extStart > 0 {
				ext := pattern[extStart:]
				if strings.Contains(ext, "*") {
					return "*" + strings.TrimSuffix(ext, "*")
				}
				return "*" + ext
			}
		}
		return strings.TrimSuffix(pattern, "*") + "*"
	}
	if strings.Contains(pattern, ".") {
		extStart := strings.LastIndex(pattern, ".")
		if extStart > 0 {
			return "*" + pattern[extStart:]
		}
	}
	return "*.ts"
}
