package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	baseDir    string
	sandboxDir string // サンドボックスディレクトリのパス（PATH参照用、cmd.Dirには使わない）
	autoVenv   bool   // Python実行時に自動で.venvをactivateするか
	venvDir    string // 仮想環境ディレクトリパス（デフォルト: .venv）
}

// NewBashTool creates a new bash tool
func NewBashTool() *BashTool {
	return &BashTool{
		autoVenv: false,
		venvDir:  ".venv",
	}
}

// SetSandboxDir はサンドボックスディレクトリのパスを設定する
// ※ bashの作業ディレクトリは変更しない（常にプロジェクトルートで実行）
func (t *BashTool) SetSandboxDir(dir string) {
	t.sandboxDir = dir
}

// SetAutoVenv は自動venv機能を設定する
func (t *BashTool) SetAutoVenv(enabled bool, venvDir string) {
	t.autoVenv = enabled
	if venvDir != "" {
		t.venvDir = venvDir
	}
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

	// python を python3 に置換（macOS互換性）
	command := replacePythonWithPython3(args.Command)

	// Python自動venv: コマンドがPython関連なら.venvのactivateを前置
	command = t.wrapWithVenvIfNeeded(command)

	// Execute command synchronously
	return t.executeSync(ctx, command, timeout)
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
		// bashを使用（sourceコマンド、venv activateに必要）
		shellCmd = "bash"
		shellArgs = []string{"-c", command}
	}

	// Create command with sanitized environment
	cmd := exec.CommandContext(ctx, shellCmd, shellArgs...)
	cmd.Env = sanitizeEnv()
	// 作業ディレクトリは常にプロジェクトルート（プロセスのcwd）を使用
	// sandboxモードでもbashはプロジェクトルートで実行する

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

// wrapWithVenvIfNeeded はPythonコマンドを検出した場合に.venvのactivateを前置する
func (t *BashTool) wrapWithVenvIfNeeded(command string) string {
	if !t.autoVenv {
		return command
	}

	// Python関連コマンドでなければそのまま
	if !isPythonCommand(command) {
		return command
	}

	// 既にactivateやvenv作成を含んでいる場合はそのまま
	if strings.Contains(command, "activate") ||
		strings.Contains(command, "uv venv") ||
		strings.Contains(command, "python3 -m venv") ||
		strings.Contains(command, "python -m venv") {
		return command
	}

	// venvは常にプロジェクトルート（cwd）に作成
	workDir, _ := os.Getwd()
	venvActivate := filepath.Join(workDir, t.venvDir, "bin", "activate")

	// .venvが既に存在する場合: activateして実行
	if _, err := os.Stat(venvActivate); err == nil {
		return fmt.Sprintf("source %s && %s", venvActivate, command)
	}

	// .venvがない場合: 作成してからactivate
	// uv があれば uv venv、なければ python3 -m venv にフォールバック
	venvPath := filepath.Join(workDir, t.venvDir)
	createVenv := fmt.Sprintf(
		"if command -v uv >/dev/null 2>&1; then uv venv %s; else python3 -m venv %s; fi",
		venvPath, venvPath,
	)

	return fmt.Sprintf("%s && source %s && %s", createVenv, venvActivate, command)
}

// isPythonCommand はコマンドがPython関連かどうかを判定する
func isPythonCommand(command string) bool {
	cmd := strings.TrimSpace(command)

	// 先頭のコマンド名で判定
	pythonPrefixes := []string{
		"python3 ", "python3\n", "python ",  "python\n",
		"pip ", "pip3 ", "pip install",
		"uv pip", "uv run",
		"pytest", "mypy", "ruff", "black", "isort", "flake8",
		"flask", "django", "uvicorn", "gunicorn", "streamlit",
	}
	for _, prefix := range pythonPrefixes {
		if strings.HasPrefix(cmd, prefix) {
			return true
		}
	}
	// "python3" 単体（改行やスペースなし）
	if cmd == "python3" || cmd == "python" {
		return true
	}

	// パイプやセミコロン、&&の後にpythonが来るケース
	if strings.Contains(cmd, "| python") || strings.Contains(cmd, "&& python") ||
		strings.Contains(cmd, "; python") {
		return true
	}

	// .pyファイルを実行するケース（python foo.py や python3 script.py）
	if strings.Contains(cmd, ".py") {
		fields := strings.Fields(cmd)
		if len(fields) > 0 {
			first := fields[0]
			if first == "python" || first == "python3" || strings.HasSuffix(first, ".py") {
				return true
			}
		}
	}

	return false
}

// replacePythonWithPython3 はコマンド内の python を python3 に置換する
// 引用符内の文字列やパス名は置換しないように注意が必要
func replacePythonWithPython3(command string) string {
	// すでに python3 を含む場合は置換不要
	if strings.Contains(command, "python3") {
		return command
	}

	// 簡単な置換: コマンドの先頭やトークンの先頭にある python を置換
	// 正確なパースは複雑になるため、基本的なパターンで対応

	// パターン: "python " -> "python3 "
	result := strings.ReplaceAll(command, "python ", "python3 ")
	// パターン: "python\n" -> "python3\n"
	result = strings.ReplaceAll(result, "python\n", "python3\n")
	// パターン: "python\t" -> "python3\t"
	result = strings.ReplaceAll(result, "python\t", "python3\t")

	// パターン: "python|" -> "python3|"
	result = strings.ReplaceAll(result, "python|", "python3|")
	// パターン: "python;" -> "python3;"
	result = strings.ReplaceAll(result, "python;", "python3;")
	// パターン: "python&" -> "python3&"
	result = strings.ReplaceAll(result, "python&", "python3&")
	// パターン: "(python)" -> "(python3)"
	result = strings.ReplaceAll(result, "(python)", "(python3)")

	// パターン: "| python" -> "| python3"
	result = strings.ReplaceAll(result, "| python", "| python3")
	// パターン: "&& python" -> "&& python3"
	result = strings.ReplaceAll(result, "&& python", "&& python3")
	// パターン: "; python" -> "; python3"
	result = strings.ReplaceAll(result, "; python", "; python3")

	// パターン: "python." で終わる場合は置換しない（パス名の可能性）
	// 例: mypython.exe

	return result
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
