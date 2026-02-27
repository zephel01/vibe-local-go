package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/zephel01/vibe-local-go/internal/session"
	"github.com/zephel01/vibe-local-go/internal/tool"
)

const (
	// MaxParallelTools is the maximum number of parallel tool executions
	MaxParallelTools = 10
)

// Dispatcher handles tool execution dispatching
type Dispatcher struct {
	registry      *tool.Registry
	permissionMgr interface{} // *security.PermissionManager
	validator     interface{} // *security.PathValidator
	terminal      interface{} // *ui.Terminal
}

// NewDispatcher creates a new tool dispatcher
func NewDispatcher(
	registry *tool.Registry,
	permissionMgr interface{},
	validator interface{},
	terminal interface{},
) *Dispatcher {
	return &Dispatcher{
		registry:      registry,
		permissionMgr: permissionMgr,
		validator:     validator,
		terminal:      terminal,
	}
}

// ExecuteToolCalls executes tool calls with appropriate parallelization
func (d *Dispatcher) ExecuteToolCalls(ctx context.Context, toolCalls []session.ToolCall) []ToolResult {
	if len(toolCalls) == 0 {
		return []ToolResult{}
	}

	// Check if we can execute in parallel
	if d.canExecuteInParallel(toolCalls) {
		return d.executeParallel(ctx, toolCalls)
	}

	// Execute sequentially
	return d.executeSequential(ctx, toolCalls)
}

// canExecuteInParallel checks if tools can be executed in parallel
func (d *Dispatcher) canExecuteInParallel(toolCalls []session.ToolCall) bool {
	// Only parallelize read-only tools
	for _, tc := range toolCalls {
		if !isReadOnlyTool(tc.Function.Name) {
			return false
		}
	}

	// Check tool count limit
	if len(toolCalls) > MaxParallelTools {
		return false
	}

	return true
}

// executeParallel executes tools in parallel
func (d *Dispatcher) executeParallel(ctx context.Context, toolCalls []session.ToolCall) []ToolResult {
	results := make([]ToolResult, len(toolCalls))
	var wg sync.WaitGroup

	for i, tc := range toolCalls {
		wg.Add(1)
		go func(idx int, toolCall session.ToolCall) {
			defer wg.Done()
			results[idx] = d.executeSingleTool(ctx, &toolCall)
		}(i, tc)
	}

	wg.Wait()
	return results
}

// executeSequential executes tools sequentially
func (d *Dispatcher) executeSequential(ctx context.Context, toolCalls []session.ToolCall) []ToolResult {
	results := make([]ToolResult, 0, len(toolCalls))

	for _, tc := range toolCalls {
		result := d.executeSingleTool(ctx, &tc)
		results = append(results, result)

		// If a write tool fails, stop further execution
		if !result.IsSuccess && isWriteTool(tc.Function.Name) {
			break
		}
	}

	return results
}

// executeSingleTool executes a single tool with retry logic and failure strategy
func (d *Dispatcher) executeSingleTool(ctx context.Context, toolCall *session.ToolCall) ToolResult {
	toolName := toolCall.Function.Name
	arguments := toolCall.Function.Arguments

	// Get tool
	toolCfg, exists := d.registry.Get(toolName)
	if !exists {
		return ToolResult{
			ToolCallID: toolCall.ID,
			IsSuccess:   false,
			Error:       fmt.Sprintf("Tool not found: %s", toolName),
		}
	}
	toolInst := toolCfg.Tool

	var lastErr error

	// Retry loop
	for attempt := 0; attempt <= toolCfg.MaxRetries; attempt++ {
		// Execute tool
		toolResult, err := toolInst.Execute(ctx, json.RawMessage(arguments))
		if err == nil && !toolResult.IsError {
			return ToolResult{
				ToolCallID: toolCall.ID,
				IsSuccess:   true,
				Content:     toolResult.Output,
				Error:       toolResult.Error,
			}
		}

		if err != nil {
			lastErr = err
		} else if toolResult.IsError {
			lastErr = fmt.Errorf("%s", toolResult.Error)
		}

		// Check if we should retry
		if attempt < toolCfg.MaxRetries {
			if isRetryable(lastErr) {
				time.Sleep(toolCfg.RetryBackoff)
				continue
			}
		}

		break
	}

	// Tool failed - apply failure strategy
	switch toolCfg.FailureStrategy {
	case tool.FailureStrategyFatal:
		return ToolResult{
			ToolCallID: toolCall.ID,
			IsSuccess:   false,
			Error:       fmt.Sprintf("❌ %s failed: %v", toolName, lastErr),
		}

	case tool.FailureStrategySkip:
		return ToolResult{
			ToolCallID: toolCall.ID,
			IsSuccess:   true,
			Content:     fmt.Sprintf("⚠️ %s skipped due to failure, continuing without it", toolName),
			Error:       "",
		}

	case tool.FailureStrategyFallback:
		fallbackResult := d.getFallbackResult(toolName, arguments)
		return ToolResult{
			ToolCallID: toolCall.ID,
			IsSuccess:   true,
			Content:     fallbackResult,
			Error:       fmt.Sprintf("⚠️ %s failed, using fallback result", toolName),
		}

	default:
		return ToolResult{
			ToolCallID: toolCall.ID,
			IsSuccess:   false,
			Error:       fmt.Sprintf("%s failed: %v", toolName, lastErr),
		}
	}
}

// isRetryable checks if an error should be retried
func isRetryable(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Retryable error patterns
	retryablePatterns := []string{
		"timeout",
		"connection refused",
		"temporary failure",
		"rate limit",
		"network unreachable",
		"temporary",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// getFallbackResult returns a fallback result for a failed tool
func (d *Dispatcher) getFallbackResult(toolName string, arguments string) string {
	switch toolName {
	case "web_search":
		return fmt.Sprintf("[]\n// Note: web_search unavailable - search the web manually if needed")

	case "web_fetch":
		return fmt.Sprintf("// Note: web_fetch unavailable - content could not be retrieved")

	case "glob":
		return "[]\n// Note: glob returned no results (fallback)"

	case "grep":
		return "// Note: grep returned no matches (fallback)"

	case "bash":
		return "// Command execution failed (fallback - may need manual execution)"

	default:
		return fmt.Sprintf("// Tool %s unavailable (fallback)", toolName)
	}
}

// isReadOnlyTool checks if a tool is read-only
func isReadOnlyTool(toolName string) bool {
	readOnlyTools := []string{
		"read_file",
		"glob",
		"grep",
		"web_search",
		"web_fetch",
	}

	for _, t := range readOnlyTools {
		if t == toolName {
			return true
		}
	}

	return false
}

// isWriteTool checks if a tool is a write operation
func isWriteTool(toolName string) bool {
	writeTools := []string{
		"write_file",
		"edit_file",
		"bash",
	}

	for _, t := range writeTools {
		if t == toolName {
			return true
		}
	}

	return false
}

// ExecuteWithRetry executes a tool with retry logic
func (d *Dispatcher) ExecuteWithRetry(ctx context.Context, toolCall *session.ToolCall, maxRetries int) ToolResult {
	var lastResult ToolResult

	for attempt := 0; attempt <= maxRetries; attempt++ {
		result := d.executeSingleTool(ctx, toolCall)

		// If success, return immediately
		if result.IsSuccess {
			return result
		}

		lastResult = result

		// Don't retry on certain errors
		if shouldNotRetry(result.Error) {
			return result
		}

		// Wait before retry (exponential backoff)
		if attempt < maxRetries {
			delay := delayForRetry(attempt)
			select {
			case <-ctx.Done():
				return ToolResult{
					ToolCallID: toolCall.ID,
					IsSuccess:   false,
					Error:       "operation cancelled",
				}
			case <-time.After(delay):
				// Continue retry
			}
		}
	}

	return lastResult
}

// shouldNotRetry checks if an error should not be retried
func shouldNotRetry(errorMsg string) bool {
	nonRetryableErrors := []string{
		"permission denied",
		"access denied",
		"not found",
		"invalid parameter",
		"tool not found",
	}

	errorMsg = strings.ToLower(errorMsg)
	for _, err := range nonRetryableErrors {
		if strings.Contains(errorMsg, err) {
			return true
		}
	}

	return false
}

// delayForRetry calculates delay for retry attempts
func delayForRetry(attempt int) time.Duration {
	// Exponential backoff: 100ms, 200ms, 400ms, etc.
	baseDelay := 100 * time.Millisecond
	return time.Duration(1<<uint(attempt)) * baseDelay
}

// ExecuteBatch executes a batch of tool calls
func (d *Dispatcher) ExecuteBatch(ctx context.Context, batches [][]session.ToolCall) []ToolResult {
	allResults := make([]ToolResult, 0)

	for _, batch := range batches {
		select {
		case <-ctx.Done():
			return allResults
		default:
			results := d.ExecuteToolCalls(ctx, batch)
			allResults = append(allResults, results...)
		}
	}

	return allResults
}

// GroupForParallelExecution groups tool calls for parallel execution
func (d *Dispatcher) GroupForParallelExecution(toolCalls []session.ToolCall) [][]session.ToolCall {
	var readOnly, writeOps []session.ToolCall

	for _, tc := range toolCalls {
		if isReadOnlyTool(tc.Function.Name) {
			readOnly = append(readOnly, tc)
		} else {
			writeOps = append(writeOps, tc)
		}
	}

	batches := make([][]session.ToolCall, 0)

	// Add read-only batch if non-empty
	if len(readOnly) > 0 {
		batches = append(batches, readOnly)
	}

	// Add write operations as individual batches
	for _, op := range writeOps {
		batches = append(batches, []session.ToolCall{op})
	}

	return batches
}

// GetToolCapabilities returns information about tool capabilities
func (d *Dispatcher) GetToolCapabilities(toolName string) *ToolCapabilities {
	toolCfg, exists := d.registry.Get(toolName)
	if !exists {
		return nil
	}

	schema := toolCfg.Tool.Schema()

	return &ToolCapabilities{
		Name:       schema.Name,
		Description: schema.Description,
		IsReadOnly: isReadOnlyTool(toolName),
		IsSafe:     isSafeTool(toolName),
	}
}

// ToolCapabilities represents tool capabilities
type ToolCapabilities struct {
	Name       string
	Description string
	IsReadOnly bool
	IsSafe     bool
}

// isSafeTool checks if a tool is safe
func isSafeTool(toolName string) bool {
	safeTools := []string{
		"read_file",
		"glob",
		"grep",
	}

	for _, t := range safeTools {
		if t == toolName {
			return true
		}
	}

	return false
}

// ValidateToolCall validates a tool call before execution
func (d *Dispatcher) ValidateToolCall(toolCall *session.ToolCall) error {
	toolName := toolCall.Function.Name

	// Check if tool exists
	if _, exists := d.registry.GetTool(toolName); !exists {
		return fmt.Errorf("tool not found: %s", toolName)
	}

	// Validate arguments
	// This would need to validate against tool schema
	// For now, just check if arguments are valid JSON
	if toolCall.Function.Arguments != "" {
		var js interface{}
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &js); err != nil {
			return fmt.Errorf("invalid JSON arguments: %w", err)
		}
	}

	return nil
}

// GetExecutionSummary returns a summary of tool execution
func (d *Dispatcher) GetExecutionSummary(results []ToolResult) string {
	var success, failure int

	for _, result := range results {
		if result.IsSuccess {
			success++
		} else {
			failure++
		}
	}

	return fmt.Sprintf("Executed %d tool calls: %d succeeded, %d failed",
		len(results), success, failure)
}
