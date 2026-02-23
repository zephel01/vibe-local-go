package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultBashTimeout is the default timeout for bash commands
	DefaultBashTimeout = 120 * time.Second
	// MaxBashTimeout is the maximum timeout for bash commands
	MaxBashTimeout = 600 * time.Second
	// MaxOutputLength is the maximum output length to return
	MaxOutputLength = 30000
	// TruncatePrefixLength is the prefix length when truncating
	TruncatePrefixLength = 15000
	// MaxBgTasks is the maximum number of background tasks
	MaxBgTasks = 50
	// BgTaskCleanupInterval is the interval for cleaning up old tasks
	BgTaskCleanupInterval = 1 * time.Hour
)

// BashTool executes bash commands
type BashTool struct {
	baseDir string
}

// NewBashTool creates a new bash tool
func NewBashTool() *BashTool {
	return &BashTool{}
}

// Name returns the tool name
func (t *BashTool) Name() string {
	return "bash"
}

// Schema returns the tool schema
func (t *BashTool) Schema() *FunctionSchema {
	return &FunctionSchema{
		Name:        "bash",
		Description: "Execute a bash command in the shell",
		Parameters: &ParameterSchema{
			Type: "object",
			Properties: map[string]*PropertyDef{
				"command": {
					Type:        "string",
					Description: "The bash command to execute",
				},
				"timeout": {
					Type:        "integer",
					Description: "Timeout in seconds (default: 120, max: 600)",
					Default:     120,
				},
				"run_in_background": {
					Type:        "boolean",
					Description: "Run command in background (returns task ID)",
					Default:     false,
				},
			},
			Required: []string{"command"},
		},
	}
}

// Execute executes a bash command
func (t *BashTool) Execute(ctx context.Context, params json.RawMessage) (*Result, error) {
	var args struct {
		Command          string `json:"command"`
		Timeout         int    `json:"timeout"`
		RunInBackground  bool   `json:"run_in_background"`
	}

	if err := json.Unmarshal(params, &args); err != nil {
		return NewErrorResult(err), nil
	}

	// Sanitize command (remove dangerous patterns if needed)
	args.Command = strings.TrimSpace(args.Command)
	if args.Command == "" {
		return NewErrorResult(fmt.Errorf("command cannot be empty")), nil
	}

	// Set timeout
	timeout := DefaultBashTimeout
	if args.Timeout > 0 && args.Timeout <= int(MaxBashTimeout.Seconds()) {
		timeout = time.Duration(args.Timeout) * time.Second
	}

	// Check for background execution
	if args.RunInBackground {
		return t.executeInBackground(args.Command, timeout)
	}

	// Execute command synchronously
	return t.executeSync(ctx, args.Command, timeout)
}

// executeSync executes a command synchronously
func (t *BashTool) executeSync(ctx context.Context, command string, timeout time.Duration) (*Result, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Determine shell based on OS
	var shellCmd string
	var shellArgs []string

	switch runtime.GOOS {
	case "windows":
		shellCmd = "cmd.exe"
		shellArgs = []string{"/c", command}
	default:
		shellCmd = "sh"
		shellArgs = []string{"-c", command}
	}

	// Create command with sanitized environment
	cmd := exec.CommandContext(ctx, shellCmd, shellArgs...)
	cmd.Env = sanitizeEnv()

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute
	err := cmd.Run()

	// Combine output
	output := stdout.String()
	stderrStr := stderr.String()
	if stderrStr != "" {
		if output != "" {
			output += "\n"
		}
		output += stderrStr
	}

	// Truncate output if too long
	output = truncateOutput(output)

	// Check if command failed
	if err != nil {
		return NewErrorResultWithID("", fmt.Errorf("Command failed: %v\nOutput:\n%s", err, output)), nil
	}

	return NewResultWithID("", output), nil
}

// truncateOutput truncates output to maximum length
func truncateOutput(output string) string {
	if len(output) <= MaxOutputLength {
		return output
	}

	prefix := output[:TruncatePrefixLength]
	suffix := output[len(output)-TruncatePrefixLength:]

	// Try to truncate at newline boundaries for cleaner display
	if lastNewline := strings.LastIndex(prefix, "\n"); lastNewline > 0 {
		prefix = prefix[:lastNewline]
	}
	if firstNewline := strings.Index(suffix, "\n"); firstNewline > 0 {
		suffix = suffix[firstNewline+1:]
	}

	omittedChars := len(output) - len(prefix) - len(suffix)
	return fmt.Sprintf("%s\n\n... [%d characters omitted] ...\n\n%s", prefix, omittedChars, suffix)
}

// sanitizeEnv removes sensitive environment variables
func sanitizeEnv() []string {
	env := os.Environ()

	// Patterns for sensitive variables
	sensitivePatterns := []string{
		"TOKEN",
		"SECRET",
		"PASSWORD",
		"KEY",
		"API_KEY",
		"AUTH",
		"PRIVATE",
	}

	// Filter out sensitive variables
	var cleanEnv []string
	for _, e := range env {
		keep := true
		for _, pattern := range sensitivePatterns {
			if strings.Contains(strings.ToUpper(e), pattern) {
				keep = false
				break
			}
		}
		if keep {
			cleanEnv = append(cleanEnv, e)
		}
	}

	return cleanEnv
}

// BackgroundTask represents a background task
type BackgroundTask struct {
	ID        string
	Command   string
	StartTime time.Time
	Output    string
	Error     error
	Done      bool
}

// backgroundTaskManager manages background tasks
var (
	bgTaskMap   = &sync.Map{} // map[string]*BackgroundTask
	bgTaskMutex sync.Mutex
)

// executeInBackground executes a command in the background
func (t *BashTool) executeInBackground(command string, timeout time.Duration) (*Result, error) {
	bgTaskMutex.Lock()
	count := 0
	bgTaskMap.Range(func(_, _ interface{}) bool {
		count++
		return true
	})

	if count >= MaxBgTasks {
		bgTaskMutex.Unlock()
		return NewErrorResult(fmt.Errorf("maximum background tasks reached (%d)", MaxBgTasks)), nil
	}
	bgTaskMutex.Unlock()

	// Generate task ID
	taskID := generateTaskID()

	// Create task
	task := &BackgroundTask{
		ID:        taskID,
		Command:   command,
		StartTime: time.Now(),
		Done:      false,
	}
	bgTaskMap.Store(taskID, task)

	// Execute in goroutine
	go func() {
		ctx := context.Background()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		// Determine shell
		var shellCmd string
		var shellArgs []string
		switch runtime.GOOS {
		case "windows":
			shellCmd = "cmd.exe"
			shellArgs = []string{"/c", command}
		default:
			shellCmd = "sh"
			shellArgs = []string{"-c", command}
		}

		cmd := exec.CommandContext(ctx, shellCmd, shellArgs...)
		cmd.Env = sanitizeEnv()

		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output

		err := cmd.Run()

		task.Output = truncateOutput(output.String())
		task.Error = err
		task.Done = true
	}()

	return NewResult(fmt.Sprintf("Background task started with ID: %s", taskID)), nil
}

// generateTaskID generates a unique task ID
func generateTaskID() string {
	return fmt.Sprintf("bg_%d", time.Now().UnixNano())
}

// GetBackgroundTask retrieves a background task
func GetBackgroundTask(taskID string) (*BackgroundTask, bool) {
	val, ok := bgTaskMap.Load(taskID)
	if !ok {
		return nil, false
	}
	return val.(*BackgroundTask), true
}

// cleanupOldBackgroundTasks removes tasks older than 1 hour
func cleanupOldBackgroundTasks() {
	now := time.Now()
	bgTaskMap.Range(func(key, value interface{}) bool {
		task := value.(*BackgroundTask)
		if now.Sub(task.StartTime) > BgTaskCleanupInterval && task.Done {
			bgTaskMap.Delete(key)
		}
		return true
	})
}

// CheckDangerousCommand checks if a command contains dangerous patterns
func CheckDangerousCommand(command string) (bool, string) {
	dangerousPatterns := []struct {
		pattern string
		reason  string
	}{
		{`rm\s+-rf\s+\/`, "Attempting to delete root filesystem"},
		{`rm\s+-rf\s+\.\.\.?`, "Attempting to delete parent directories"},
		{`sudo\s+rm\s+`, "Attempting to delete files with sudo"},
		{`sudo\s+dd\s+`, "Attempting to run dd with sudo"},
		{`dd.*of=/dev/`, "Attempting to write to device file"},
		{`mkfs\.\w+`, "Attempting to format filesystem"},
		{`\s*>\s*\/dev\/`, "Attempting to redirect to device file"},
		{`curl.*\|.*sh`, "Executing downloaded script (potential security risk)"},
		{`wget.*\|.*sh`, "Executing downloaded script (potential security risk)"},
		{`curl.*\|.*bash`, "Executing downloaded script (potential security risk)"},
		{`wget.*\|.*bash`, "Executing downloaded script (potential security risk)"},
		{`chmod\s+777\s+\/`, "Making root world-writable"},
		{`chown\s+-[Rr]`, "Recursive ownership change on root"},
	}

	commandLower := strings.ToLower(command)

	for _, dp := range dangerousPatterns {
		matched, _ := regexp.MatchString(dp.pattern, commandLower)
		if matched {
			return true, dp.reason
		}
	}

	return false, ""
}

// InitBashToolCleanup starts the background task cleanup goroutine
func InitBashToolCleanup() {
	go func() {
		for {
			time.Sleep(BgTaskCleanupInterval)
			cleanupOldBackgroundTasks()
		}
	}()
}
