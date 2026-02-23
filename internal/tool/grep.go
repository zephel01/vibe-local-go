package tool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	// MaxGrepFileSize is the maximum file size for grep
	MaxGrepFileSize = 50 * 1024 * 1024 // 50MB
	// DefaultMaxMatches is the default maximum number of matches
	DefaultMaxMatches = 500
)

// GrepTool searches for text patterns in files
type GrepTool struct {
	baseDir string
}

// NewGrepTool creates a new grep tool
func NewGrepTool() *GrepTool {
	return &GrepTool{}
}

// Name returns the tool name
func (t *GrepTool) Name() string {
	return "grep"
}

// Schema returns the tool schema
func (t *GrepTool) Schema() *FunctionSchema {
	return &FunctionSchema{
		Name:        "grep",
		Description: "Search for text patterns in files",
		Parameters: &ParameterSchema{
			Type: "object",
			Properties: map[string]*PropertyDef{
				"pattern": {
					Type:        "string",
					Description: "Regular expression pattern to search for",
				},
				"path": {
					Type:        "string",
					Description: "Directory to search in (default: current directory)",
					Default:     ".",
				},
				"mode": {
					Type:        "string",
					Description: "Search mode: content (default), files_with_matches, count",
					Default:     "content",
				},
				"context_lines": {
					Type:        "integer",
					Description: "Number of context lines to show (default: 0)",
					Default:     0,
				},
				"max_matches": {
					Type:        "integer",
					Description: "Maximum number of matches (default: 500)",
					Default:     DefaultMaxMatches,
				},
				"file_pattern": {
					Type:        "string",
					Description: "Glob pattern to filter files (e.g., '*.go')",
					Default:     "*",
				},
			},
			Required: []string{"pattern"},
		},
	}
}

// Execute searches for patterns
func (t *GrepTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	var args struct {
		Pattern      string `json:"pattern"`
		Path         string `json:"path"`
		Mode         string `json:"mode"`
		ContextLines int    `json:"context_lines"`
		MaxMatches   int    `json:"max_matches"`
		FilePattern  string `json:"file_pattern"`
	}

	if err := json.Unmarshal(params, &args); err != nil {
		return NewErrorResult(err), nil
	}

	if args.Pattern == "" {
		return NewErrorResult(fmt.Errorf("pattern cannot be empty")), nil
	}

	// Set defaults
	if args.Path == "" {
		args.Path = "."
	}
	if args.Mode == "" {
		args.Mode = "content"
	}
	if args.FilePattern == "" {
		args.FilePattern = "*"
	}
	if args.MaxMatches <= 0 {
		args.MaxMatches = DefaultMaxMatches
	}

	// Validate mode
	switch args.Mode {
	case "content", "files_with_matches", "count":
		// Valid modes
	default:
		return NewErrorResult(fmt.Errorf("invalid mode: %s (must be content, files_with_matches, or count)", args.Mode)), nil
	}

	// Compile regex
	re, err := regexp.Compile(args.Pattern)
	if err != nil {
		return NewErrorResult(fmt.Errorf("invalid regex pattern: %w", err)), nil
	}

	// Perform search
	results, err := t.grepSearch(args.Path, args.FilePattern, re, args.Mode, args.ContextLines, args.MaxMatches)
	if err != nil {
		return NewErrorResult(err), nil
	}

	// Format output based on mode
	var output strings.Builder
	switch args.Mode {
	case "content":
		output.WriteString(fmt.Sprintf("Found %d matches:\n\n", len(results)))
		for _, match := range results {
			output.WriteString(fmt.Sprintf("%s:%d:%s\n", match.FilePath, match.LineNumber, match.Line))
		}
	case "files_with_matches":
		output.WriteString(fmt.Sprintf("Found %d files with matches:\n\n", len(results)))
		for _, match := range results {
			output.WriteString(match.FilePath + "\n")
		}
	case "count":
		output.WriteString(fmt.Sprintf("Match counts:\n\n"))
		for _, match := range results {
			output.WriteString(fmt.Sprintf("%s: %d\n", match.FilePath, match.Count))
		}
	}

	return NewResult(output.String()), nil
}

// grepSearch performs the actual grep search
func (t *GrepTool) grepSearch(searchPath, filePattern string, re *regexp.Regexp, mode string, contextLines, maxMatches int) ([]GrepMatch, error) {
	var results []GrepMatch

	// Walk directory
	err := filepath.WalkDir(searchPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		// Skip directories
		if d.IsDir() {
			if isSkipDir(path) {
				return filepath.SkipDir
			}
			return nil
		}

		// Check file pattern
		relPath, err := filepath.Rel(searchPath, path)
		if err != nil {
			return nil
		}

		matched, err := filepath.Match(filePattern, relPath)
		if err != nil || !matched {
			return nil
		}

		// Check file size
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Size() > MaxGrepFileSize {
			return nil
		}

		// Search file
		matches, err := t.searchFile(path, re, mode, contextLines)
		if err != nil {
			return nil
		}

		results = append(results, matches...)

		// Check max matches
		if mode == "content" && len(results) >= maxMatches {
			return fmt.Errorf("maximum matches reached")
		}

		return nil
	})

	// If we hit max matches, truncate
	if err != nil && err.Error() == "maximum matches reached" {
		if len(results) > maxMatches {
			results = results[:maxMatches]
		}
		return results, nil
	}

	return results, err
}

// searchFile searches a single file
func (t *GrepTool) searchFile(path string, re *regexp.Regexp, mode string, contextLines int) ([]GrepMatch, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var matches []GrepMatch
	scanner := bufio.NewScanner(file)

	// Read all lines for context
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Search lines
	for i, line := range lines {
		if re.MatchString(line) {
			switch mode {
			case "content":
				// Get context lines
				start := max(0, i-contextLines)
				end := min(len(lines), i+contextLines+1)

				for j := start; j < end; j++ {
					matches = append(matches, GrepMatch{
						FilePath:   path,
						LineNumber: j + 1,
						Line:       lines[j],
						Match:      re.FindString(lines[j]),
					})
				}
			case "files_with_matches":
				matches = append(matches, GrepMatch{
					FilePath: path,
					Count:    1,
				})
				return matches, nil // Only need one match
			case "count":
				// Count matches in this file
				if len(matches) == 0 || matches[len(matches)-1].FilePath != path {
					matches = append(matches, GrepMatch{
						FilePath: path,
						Count:    0,
					})
				}
				matches[len(matches)-1].Count++
			}
		}
	}

	return matches, nil
}

// GrepMatch represents a grep match result
type GrepMatch struct {
	FilePath   string
	LineNumber int
	Line       string
	Match      string
	Count      int
}

// parseContextLines parses context lines from parameters
func parseContextLines(params json.RawMessage) (int, error) {
	var args struct {
		ContextLines int `json:"context_lines"`
	}
	if err := json.Unmarshal(params, &args); err != nil {
		return 0, nil
	}
	if args.ContextLines < 0 {
		args.ContextLines = 0
	}
	return args.ContextLines, nil
}

// parseMaxMatches parses max_matches from parameters
func parseMaxMatches(params json.RawMessage) (int, error) {
	var args struct {
		MaxMatches int `json:"max_matches"`
	}
	if err := json.Unmarshal(params, &args); err != nil {
		return DefaultMaxMatches, nil
	}
	if args.MaxMatches <= 0 {
		return DefaultMaxMatches, nil
	}
	return args.MaxMatches, nil
}

// parseMode parses search mode from parameters
func parseMode(params json.RawMessage) string {
	var args struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(params, &args); err != nil {
		return "content"
	}
	switch args.Mode {
	case "content", "files_with_matches", "count":
		return args.Mode
	default:
		return "content"
	}
}

// parseFilePattern parses file pattern from parameters
func parseFilePattern(params json.RawMessage) string {
	var args struct {
		FilePattern string `json:"file_pattern"`
	}
	if err := json.Unmarshal(params, &args); err != nil || args.FilePattern == "" {
		return "*"
	}
	return args.FilePattern
}

// parseInt parses an integer from JSON parameters
func parseInt(params json.RawMessage, key string, defaultValue int) (int, error) {
	var args map[string]interface{}
	if err := json.Unmarshal(params, &args); err != nil {
		return defaultValue, nil
	}

	if val, ok := args[key]; ok {
		switch v := val.(type) {
		case float64:
			return int(v), nil
		case string:
			i, err := strconv.Atoi(v)
			if err != nil {
				return defaultValue, nil
			}
			return i, nil
		}
	}

	return defaultValue, nil
}
