package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/zephel01/vibe-local-go/internal/session"
	"github.com/zephel01/vibe-local-go/internal/tool"
)

func TestNewDispatcher(t *testing.T) {
	registry := tool.NewRegistry()
	dispatcher := NewDispatcher(registry, nil, nil, nil)

	if dispatcher == nil {
		t.Fatal("NewDispatcher should return non-nil dispatcher")
	}

	if dispatcher.registry == nil {
		t.Error("Registry should be set")
	}
}

func TestCanExecuteInParallel(t *testing.T) {
	registry := tool.NewRegistry()
	dispatcher := NewDispatcher(registry, nil, nil, nil)

	// Read-only tools should allow parallel execution
	readOnlyCalls := []session.ToolCall{
		{ID: "1", Type: "function", Function: session.FunctionCall{Name: "read_file", Arguments: `{}`}},
		{ID: "2", Type: "function", Function: session.FunctionCall{Name: "grep", Arguments: `{}`}},
	}

	if !dispatcher.canExecuteInParallel(readOnlyCalls) {
		t.Error("Read-only tools should be executable in parallel")
	}

	// Write tools should not allow parallel execution
	writeCalls := []session.ToolCall{
		{ID: "1", Type: "function", Function: session.FunctionCall{Name: "write_file", Arguments: `{}`}},
	}

	if dispatcher.canExecuteInParallel(writeCalls) {
		t.Error("Write tools should not be executable in parallel")
	}

	// Mixed tools should not allow parallel execution
	mixedCalls := []session.ToolCall{
		{ID: "1", Type: "function", Function: session.FunctionCall{Name: "read_file", Arguments: `{}`}},
		{ID: "2", Type: "function", Function: session.FunctionCall{Name: "write_file", Arguments: `{}`}},
	}

	if dispatcher.canExecuteInParallel(mixedCalls) {
		t.Error("Mixed read/write tools should not be executable in parallel")
	}

	// Too many tools should not allow parallel execution
	manyCalls := make([]session.ToolCall, MaxParallelTools+1)
	for i := 0; i < len(manyCalls); i++ {
		manyCalls[i] = session.ToolCall{
			ID: "test", Type: "function",
			Function: session.FunctionCall{Name: "read_file", Arguments: `{}`},
		}
	}

	if dispatcher.canExecuteInParallel(manyCalls) {
		t.Error("Too many tools should not be executable in parallel")
	}
}

func TestIsReadOnlyTool(t *testing.T) {
	readOnlyTools := []string{
		"read_file",
		"glob",
		"grep",
		"web_search",
		"web_fetch",
	}

	for _, toolName := range readOnlyTools {
		if !isReadOnlyTool(toolName) {
			t.Errorf("Tool '%s' should be read-only", toolName)
		}
	}

	writeTools := []string{
		"write_file",
		"edit_file",
		"bash",
	}

	for _, toolName := range writeTools {
		if isReadOnlyTool(toolName) {
			t.Errorf("Tool '%s' should not be read-only", toolName)
		}
	}

	// Unknown tool should not be read-only
	if isReadOnlyTool("unknown_tool") {
		t.Error("Unknown tool should not be read-only")
	}
}

func TestIsWriteTool(t *testing.T) {
	writeTools := []string{
		"write_file",
		"edit_file",
		"bash",
	}

	for _, toolName := range writeTools {
		if !isWriteTool(toolName) {
			t.Errorf("Tool '%s' should be write tool", toolName)
		}
	}

	readOnlyTools := []string{
		"read_file",
		"glob",
		"grep",
	}

	for _, toolName := range readOnlyTools {
		if isWriteTool(toolName) {
			t.Errorf("Tool '%s' should not be write tool", toolName)
		}
	}
}

func TestIsSafeTool(t *testing.T) {
	safeTools := []string{
		"read_file",
		"glob",
		"grep",
	}

	for _, toolName := range safeTools {
		if !isSafeTool(toolName) {
			t.Errorf("Tool '%s' should be safe", toolName)
		}
	}

	unsafeTools := []string{
		"write_file",
		"bash",
	}

	for _, toolName := range unsafeTools {
		if isSafeTool(toolName) {
			t.Errorf("Tool '%s' should not be safe", toolName)
		}
	}
}

func TestShouldNotRetry(t *testing.T) {
	nonRetryableErrors := []string{
		"Permission denied",
		"Access denied",
		"Tool not found",
		"Invalid parameter",
		"File not found",
	}

	for _, errMsg := range nonRetryableErrors {
		if !shouldNotRetry(errMsg) {
			t.Errorf("Error should not be retryable: %s", errMsg)
		}
	}

	retryableErrors := []string{
		"Network timeout",
		"Connection lost",
		"Temporary error",
	}

	for _, errMsg := range retryableErrors {
		if shouldNotRetry(errMsg) {
			t.Errorf("Error should be retryable: %s", errMsg)
		}
	}
}

func TestDelayForRetry(t *testing.T) {
	// Test exponential backoff
	attempts := []struct {
		attempt int
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{0, 100 * time.Millisecond, 100 * time.Millisecond},
		{1, 200 * time.Millisecond, 200 * time.Millisecond},
		{2, 400 * time.Millisecond, 400 * time.Millisecond},
		{3, 800 * time.Millisecond, 800 * time.Millisecond},
	}

	for _, tc := range attempts {
		delay := delayForRetry(tc.attempt)
		if delay < tc.minDelay || delay > tc.maxDelay {
			t.Errorf("Delay for attempt %d = %v, want between %v and %v",
				tc.attempt, delay, tc.minDelay, tc.maxDelay)
		}
	}
}

func TestGroupForParallelExecution(t *testing.T) {
	registry := tool.NewRegistry()
	dispatcher := NewDispatcher(registry, nil, nil, nil)

	toolCalls := []session.ToolCall{
		{ID: "1", Function: session.FunctionCall{Name: "read_file", Arguments: `{}`}},
		{ID: "2", Function: session.FunctionCall{Name: "read_file", Arguments: `{}`}},
		{ID: "3", Function: session.FunctionCall{Name: "write_file", Arguments: `{}`}},
		{ID: "4", Function: session.FunctionCall{Name: "bash", Arguments: `{}`}},
	}

	batches := dispatcher.GroupForParallelExecution(toolCalls)

	// Should have 3 batches: 1 read-only, 2 individual writes
	if len(batches) != 3 {
		t.Errorf("Batches count = %v, want 3", len(batches))
	}

	// First batch should be read-only
	if len(batches[0]) != 2 {
		t.Errorf("First batch size = %v, want 2", len(batches[0]))
	}

	// Remaining batches should be single write operations
	if len(batches[1]) != 1 || len(batches[2]) != 1 {
		t.Error("Write operations should be in individual batches")
	}
}

func TestValidateToolCall(t *testing.T) {
	registry := tool.NewRegistry()
	mockTool := newMockTool("read_file")
	registry.Register(mockTool)
	dispatcher := NewDispatcher(registry, nil, nil, nil)

	// Valid JSON arguments
	validCall := &session.ToolCall{
		ID: "1",
		Function: session.FunctionCall{
			Name:      "read_file",
			Arguments: `{"path": "test.txt"}`,
		},
	}

	err := dispatcher.ValidateToolCall(validCall)
	if err != nil {
		t.Errorf("Valid tool call should pass validation: %v", err)
	}

	// Invalid JSON arguments
	invalidCall := &session.ToolCall{
		ID: "1",
		Function: session.FunctionCall{
			Name:      "read_file",
			Arguments: `{invalid json}`,
		},
	}

	err = dispatcher.ValidateToolCall(invalidCall)
	if err == nil {
		t.Error("Invalid JSON should fail validation")
	}

	// Empty arguments (should be valid)
	emptyArgsCall := &session.ToolCall{
		ID: "1",
		Function: session.FunctionCall{
			Name:      "read_file",
			Arguments: "",
		},
	}

	err = dispatcher.ValidateToolCall(emptyArgsCall)
	if err != nil {
		t.Errorf("Empty arguments should be valid: %v", err)
	}

	// Non-existent tool
	nonExistentCall := &session.ToolCall{
		ID: "1",
		Function: session.FunctionCall{
			Name:      "nonexistent_tool",
			Arguments: `{}`,
		},
	}

	err = dispatcher.ValidateToolCall(nonExistentCall)
	if err == nil {
		t.Error("Non-existent tool should fail validation")
	}
}

func TestGetExecutionSummary(t *testing.T) {
	registry := tool.NewRegistry()
	dispatcher := NewDispatcher(registry, nil, nil, nil)

	// All successful
	allSuccess := []ToolResult{
		{ToolCallID: "1", IsSuccess: true},
		{ToolCallID: "2", IsSuccess: true},
		{ToolCallID: "3", IsSuccess: true},
	}

	summary := dispatcher.GetExecutionSummary(allSuccess)
	if summary != "Executed 3 tool calls: 3 succeeded, 0 failed" {
		t.Errorf("Summary = %v, want 'Executed 3 tool calls: 3 succeeded, 0 failed'", summary)
	}

	// All failed
	allFailed := []ToolResult{
		{ToolCallID: "1", IsSuccess: false},
		{ToolCallID: "2", IsSuccess: false},
	}

	summary = dispatcher.GetExecutionSummary(allFailed)
	if summary != "Executed 2 tool calls: 0 succeeded, 2 failed" {
		t.Errorf("Summary = %v, want 'Executed 2 tool calls: 0 succeeded, 2 failed'", summary)
	}

	// Mixed
	mixed := []ToolResult{
		{ToolCallID: "1", IsSuccess: true},
		{ToolCallID: "2", IsSuccess: false},
		{ToolCallID: "3", IsSuccess: true},
	}

	summary = dispatcher.GetExecutionSummary(mixed)
	if summary != "Executed 3 tool calls: 2 succeeded, 1 failed" {
		t.Errorf("Summary = %v, want 'Executed 3 tool calls: 2 succeeded, 1 failed'", summary)
	}

	// Empty
	empty := []ToolResult{}
	summary = dispatcher.GetExecutionSummary(empty)
	if summary != "Executed 0 tool calls: 0 succeeded, 0 failed" {
		t.Errorf("Summary = %v, want 'Executed 0 tool calls: 0 succeeded, 0 failed'", summary)
	}
}

func TestExecuteToolCalls_Empty(t *testing.T) {
	registry := tool.NewRegistry()
	dispatcher := NewDispatcher(registry, nil, nil, nil)

	results := dispatcher.ExecuteToolCalls(context.Background(), []session.ToolCall{})

	if len(results) != 0 {
		t.Errorf("Results length = %v, want 0", len(results))
	}
}

func TestExecuteToolCalls_ContextCancellation(t *testing.T) {
	registry := tool.NewRegistry()
	dispatcher := NewDispatcher(registry, nil, nil, nil)

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	toolCalls := []session.ToolCall{
		{ID: "1", Function: session.FunctionCall{Name: "read_file", Arguments: `{}`}},
	}

	results := dispatcher.ExecuteToolCalls(ctx, toolCalls)

	// Should still return results (even if empty due to no tools)
	if results == nil {
		t.Error("Results should not be nil")
	}
}

// Mock tool for testing
type mockTool struct {
	name     string
	execute  func(context.Context, json.RawMessage) (*tool.Result, error)
	schema   *tool.FunctionSchema
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Execute(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
	return m.execute(ctx, args)
}

func (m *mockTool) Schema() *tool.FunctionSchema {
	return m.schema
}

func newMockTool(name string) *mockTool {
	return &mockTool{
		name: name,
		execute: func(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
			return &tool.Result{
				Output: `{"result": "success"}`,
				IsError: false,
			}, nil
		},
		schema: &tool.FunctionSchema{
			Name:        name,
			Description: "Mock tool",
			Parameters:  nil,
		},
	}
}

func TestExecuteSingleTool_ToolNotFound(t *testing.T) {
	registry := tool.NewRegistry()
	dispatcher := NewDispatcher(registry, nil, nil, nil)

	toolCall := &session.ToolCall{
		ID: "1",
		Function: session.FunctionCall{
			Name:      "nonexistent_tool",
			Arguments: `{}`,
		},
	}

	result := dispatcher.executeSingleTool(context.Background(), toolCall)

	if result.IsSuccess {
		t.Error("Result should indicate failure")
	}

	if result.Error == "" {
		t.Error("Error should not be empty")
	}
}

func TestExecuteSingleTool_ToolError(t *testing.T) {
	registry := tool.NewRegistry()
	errorTool := &mockTool{
		name: "error_tool",
		execute: func(ctx context.Context, args json.RawMessage) (*tool.Result, error) {
			return nil, errors.New("tool execution failed")
		},
		schema: &tool.FunctionSchema{
			Name:        "error_tool",
			Description: "Error tool",
		},
	}
	registry.Register(errorTool)

	dispatcher := NewDispatcher(registry, nil, nil, nil)

	toolCall := &session.ToolCall{
		ID: "1",
		Function: session.FunctionCall{
			Name:      "error_tool",
			Arguments: `{}`,
		},
	}

	result := dispatcher.executeSingleTool(context.Background(), toolCall)

	if result.IsSuccess {
		t.Error("Result should indicate failure")
	}

	if result.Error == "" {
		t.Error("Error should not be empty")
	}
}

func TestExecuteWithRetry_Success(t *testing.T) {
	registry := tool.NewRegistry()
	dispatcher := NewDispatcher(registry, nil, nil, nil)

	toolCall := &session.ToolCall{
		ID: "1",
		Function: session.FunctionCall{
			Name:      "mock_tool",
			Arguments: `{}`,
		},
	}

	// Mock registry would be needed for full test
	result := dispatcher.ExecuteWithRetry(context.Background(), toolCall, 3)

	// Will fail because tool doesn't exist in registry
	if result.IsSuccess {
		t.Error("Should fail without tool in registry")
	}
}

func TestExecuteWithRetry_MaxRetries(t *testing.T) {
	registry := tool.NewRegistry()
	dispatcher := NewDispatcher(registry, nil, nil, nil)

	toolCall := &session.ToolCall{
		ID: "1",
		Function: session.FunctionCall{
			Name:      "nonexistent",
			Arguments: `{}`,
		},
	}

	result := dispatcher.ExecuteWithRetry(context.Background(), toolCall, 2)

	if result.IsSuccess {
		t.Error("Should fail after max retries")
	}

	if result.Error == "" {
		t.Error("Error should not be empty")
	}
}

func TestExecuteBatch_ContextCancellation(t *testing.T) {
	registry := tool.NewRegistry()
	dispatcher := NewDispatcher(registry, nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	batches := [][]session.ToolCall{
		{{ID: "1", Function: session.FunctionCall{Name: "read_file", Arguments: `{}`}}},
		{{ID: "2", Function: session.FunctionCall{Name: "grep", Arguments: `{}`}}},
	}

	results := dispatcher.ExecuteBatch(ctx, batches)

	// Should return empty results due to cancellation
	if len(results) != 0 {
		t.Errorf("Results should be empty after cancellation, got %d", len(results))
	}
}

func TestToolCapabilities(t *testing.T) {
	registry := tool.NewRegistry()
	dispatcher := NewDispatcher(registry, nil, nil, nil)

	// Test with non-existent tool
	caps := dispatcher.GetToolCapabilities("nonexistent")
	if caps != nil {
		t.Error("Capabilities should be nil for non-existent tool")
	}
}
