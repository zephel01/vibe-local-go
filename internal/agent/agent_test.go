package agent

import (
	"encoding/json"
	"testing"

	"github.com/zephel01/vibe-local-go/internal/config"
	"github.com/zephel01/vibe-local-go/internal/llm"
	"github.com/zephel01/vibe-local-go/internal/security"
	"github.com/zephel01/vibe-local-go/internal/session"
	"github.com/zephel01/vibe-local-go/internal/tool"
	"github.com/zephel01/vibe-local-go/internal/ui"
)

// Helper to create a test agent with real dependencies
func createSimpleTestAgent() *Agent {
	cfg := &config.Config{}
	cfg.Model = "test-model"
	cfg.OllamaHost = "http://localhost:11434"

	provider := llm.NewOllamaProvider(cfg.OllamaHost, cfg.Model)
	registry := tool.NewRegistry()
	permMgr, _ := security.NewPermissionManager(true)
	validator := security.NewPathValidator(".")
	sess := session.NewSession("test-session", "")
	term := ui.NewTerminal()

	return NewAgent(
		provider,
		registry,
		permMgr,
		validator,
		sess,
		term,
		cfg,
	)
}

func TestNewAgent(t *testing.T) {
	cfg := &config.Config{}
	cfg.Model = "test-model"
	cfg.OllamaHost = "http://localhost:11434"

	provider := llm.NewOllamaProvider(cfg.OllamaHost, cfg.Model)
	registry := tool.NewRegistry()
	permMgr, _ := security.NewPermissionManager(true)
	validator := security.NewPathValidator(".")
	sess := session.NewSession("test-session", "")
	term := ui.NewTerminal()

	agent := NewAgent(
		provider,
		registry,
		permMgr,
		validator,
		sess,
		term,
		cfg,
	)

	if agent == nil {
		t.Fatal("NewAgent should return non-nil agent")
	}

	if agent.provider == nil {
		t.Error("Provider should be set")
	}

	if agent.registry == nil {
		t.Error("Registry should be set")
	}

	if agent.permissionMgr == nil {
		t.Error("Permission manager should be set")
	}

	if agent.validator == nil {
		t.Error("Validator should be set")
	}

	if agent.session == nil {
		t.Error("Session should be set")
	}

	if agent.terminal == nil {
		t.Error("Terminal should be set")
	}

	if agent.config == nil {
		t.Error("Config should be set")
	}

	if agent.loopDetector == nil {
		t.Error("Loop detector should be initialized")
	}
}

func TestGetIterationCount(t *testing.T) {
	agent := createSimpleTestAgent()

	count := agent.GetIterationCount()
	if count != 0 {
		t.Errorf("Initial iteration count should be 0, got %d", count)
	}
}

func TestStop(t *testing.T) {
	agent := createSimpleTestAgent()

	agent.Stop()

	// Stop doesn't have much effect in current implementation
	// Just verify it doesn't panic
}

func TestGetStatus(t *testing.T) {
	agent := createSimpleTestAgent()

	status := agent.GetStatus()
	if status == "" {
		t.Error("Status should not be empty")
	}
}

func TestCompactSession(t *testing.T) {
	cfg := &config.Config{}
	cfg.Model = "test-model"
	cfg.OllamaHost = "http://localhost:11434"

	provider := llm.NewOllamaProvider(cfg.OllamaHost, cfg.Model)
	registry := tool.NewRegistry()
	permMgr, _ := security.NewPermissionManager(true)
	validator := security.NewPathValidator(".")

	sess := session.NewSession("test-session", "")
	term := ui.NewTerminal()

	agent := NewAgent(
		provider,
		registry,
		permMgr,
		validator,
		sess,
		term,
		cfg,
	)

	agent.CompactSession()

	// Just verify the session still exists
	if agent.GetSession() == nil {
		t.Error("Session should still exist after compaction")
	}
}

func TestGetSession(t *testing.T) {
	agent := createSimpleTestAgent()

	retrieved := agent.GetSession()

	if retrieved == nil {
		t.Error("Session should not be nil")
	}

	if retrieved.ID == "" {
		t.Error("Session should have an ID")
	}
}

func TestUpdateSystemPrompt(t *testing.T) {
	cfg := &config.Config{}
	cfg.Model = "test-model"
	cfg.OllamaHost = "http://localhost:11434"

	provider := llm.NewOllamaProvider(cfg.OllamaHost, cfg.Model)
	registry := tool.NewRegistry()
	permMgr, _ := security.NewPermissionManager(true)
	validator := security.NewPathValidator(".")

	sess := session.NewSession("test-session", "")
	term := ui.NewTerminal()

	agent := NewAgent(
		provider,
		registry,
		permMgr,
		validator,
		sess,
		term,
		cfg,
	)

	newPrompt := "New system prompt"
	agent.UpdateSystemPrompt(newPrompt)

	// Verify session was updated
	if sess.SystemPrompt != newPrompt {
		t.Errorf("System prompt should be '%s', got '%s'", newPrompt, sess.SystemPrompt)
	}
}

func TestClear(t *testing.T) {
	cfg := &config.Config{}
	cfg.Model = "test-model"
	cfg.OllamaHost = "http://localhost:11434"

	provider := llm.NewOllamaProvider(cfg.OllamaHost, cfg.Model)
	registry := tool.NewRegistry()
	permMgr, _ := security.NewPermissionManager(true)
	validator := security.NewPathValidator(".")

	sess := session.NewSession("test-session", "")
	sess.AddUserMessage("test")

	term := ui.NewTerminal()

	agent := NewAgent(
		provider,
		registry,
		permMgr,
		validator,
		sess,
		term,
		cfg,
	)

	// Verify messages were added
	if sess.GetMessageCount() == 0 {
		t.Error("Should have messages before clear")
	}

	agent.Clear()

	if sess.GetMessageCount() != 0 {
		t.Errorf("Session should be cleared, but has %d messages", sess.GetMessageCount())
	}
}

func TestLoopDetectorInAgent(t *testing.T) {
	agent := createSimpleTestAgent()

	if agent.loopDetector == nil {
		t.Error("Loop detector should be initialized")
	}

	// Test loop detection works
	agent.loopDetector.RecordToolCall("test_tool", `{}`)
	agent.loopDetector.RecordToolCall("test_tool", `{}`)
	agent.loopDetector.RecordToolCall("test_tool", `{}`)

	if !agent.loopDetector.DetectLoop() {
		t.Error("Should detect loop after 3 identical calls")
	}

	// Test reset
	agent.loopDetector.Reset()
	if agent.loopDetector.DetectLoop() {
		t.Error("Should not detect loop after reset")
	}
}

func TestLoopDetectorHistoryOverflow(t *testing.T) {
	agent := createSimpleTestAgent()

	// Add more calls than the history size can hold
	for i := 0; i < 100; i++ {
		agent.loopDetector.RecordToolCall("test_tool", `{}`)
	}

	// Should handle overflow gracefully
	historySize := agent.loopDetector.GetHistorySize()
	if historySize > LoopHistorySize {
		t.Errorf("History size should be capped at %d, got %d", LoopHistorySize, historySize)
	}
}

func TestLoopDetectorRepeatingPattern(t *testing.T) {
	agent := createSimpleTestAgent()

	// Create a repeating pattern: A, B, A, B, A, B
	pattern := []string{"{op:\"A\"}", "{op:\"B\"}"}
	for i := 0; i < 3; i++ {
		for _, arg := range pattern {
			agent.loopDetector.RecordToolCall("test_tool", arg)
		}
	}

	// The pattern detection is internal, so we can't test it directly
	// Just verify the loop detector doesn't crash
	loopInfo := agent.loopDetector.GetLoopInfo()
	if loopInfo == nil {
		t.Error("Loop info should not be nil")
	}
}

func TestLoopDetectorDescription(t *testing.T) {
	agent := createSimpleTestAgent()

	// Create a simple loop
	for i := 0; i < 3; i++ {
		agent.loopDetector.RecordToolCall("test_tool", `{}`)
	}

	loopInfo := agent.loopDetector.GetLoopInfo()
	if loopInfo == nil {
		t.Error("Loop info should not be nil")
	}

	if loopInfo.Description == "" {
		t.Error("Description should not be empty")
	}
}

func TestLoopDetectorIterationCount(t *testing.T) {
	agent := createSimpleTestAgent()

	// Create a loop
	for i := 0; i < 3; i++ {
		agent.loopDetector.RecordToolCall("test_tool", `{}`)
	}

	iteration := agent.loopDetector.GetCurrentLoopIteration("test_tool")
	if iteration == 0 {
		t.Error("Loop iteration should be greater than 0")
	}

	// Add same tool call again - iteration should increase
	agent.loopDetector.RecordToolCall("test_tool", `{}`)
	newIteration := agent.loopDetector.GetCurrentLoopIteration("test_tool")
	if newIteration <= iteration {
		t.Error("Loop iteration should increase with same tool call")
	}
}

func TestLoopDetectorStatus(t *testing.T) {
	agent := createSimpleTestAgent()

	status := agent.loopDetector.GetLoopStatus()
	if status != "No loop detected" {
		t.Errorf("Should not be in active loop initially, got: %s", status)
	}

	// Create a loop
	for i := 0; i < 3; i++ {
		agent.loopDetector.RecordToolCall("test_tool", `{}`)
	}

	status = agent.loopDetector.GetLoopStatus()
	if !contains(status, "Loop detected") {
		t.Errorf("Should be in active loop, got status: %s", status)
	}

	// Reset
	agent.loopDetector.Reset()
	status = agent.loopDetector.GetLoopStatus()
	if status != "No loop detected" {
		t.Error("Should not be in active loop after reset")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || len(s) > len(substr) && (s[len(s)-len(substr):] == substr)
}

func TestLoopDetectorToolCounts(t *testing.T) {
	agent := createSimpleTestAgent()

	// Add different tools
	agent.loopDetector.RecordToolCall("tool_a", `{}`)
	agent.loopDetector.RecordToolCall("tool_b", `{}`)
	agent.loopDetector.RecordToolCall("tool_a", `{}`)

	counts := agent.loopDetector.GetToolCounts()
	if len(counts) != 2 {
		t.Errorf("Should have 2 unique tools, got %d", len(counts))
	}

	if counts["tool_a"] != 2 {
		t.Errorf("Tool A should be called 2 times, got %d", counts["tool_a"])
	}

	if counts["tool_b"] != 1 {
		t.Errorf("Tool B should be called 1 time, got %d", counts["tool_b"])
	}

	mostCalled, count := agent.loopDetector.GetMostCalledTool()
	if mostCalled != "tool_a" {
		t.Errorf("Most called tool should be 'tool_a', got '%s'", mostCalled)
	}

	if count != 2 {
		t.Errorf("Tool A should be called 2 times, got %d", count)
	}
}

func TestLoopDetectorRecentCalls(t *testing.T) {
	agent := createSimpleTestAgent()

	// Add some calls
	agent.loopDetector.RecordToolCall("tool_a", `{arg:"1"}`)
	agent.loopDetector.RecordToolCall("tool_b", `{arg:"2"}`)
	agent.loopDetector.RecordToolCall("tool_a", `{arg:"3"}`)

	recent := agent.loopDetector.GetRecentCalls(2)
	if len(recent) != 2 {
		t.Errorf("Should get 2 recent calls, got %d", len(recent))
	}

	// Most recent should be tool_a with arg:3 (last in the returned slice)
	if recent[len(recent)-1].ToolName != "tool_a" {
		t.Errorf("Most recent call should be 'tool_a', got '%s'", recent[len(recent)-1].ToolName)
	}
}

func TestLoopDetectorClearToolCount(t *testing.T) {
	agent := createSimpleTestAgent()

	// Add tool calls
	agent.loopDetector.RecordToolCall("tool_a", `{}`)
	agent.loopDetector.RecordToolCall("tool_a", `{}`)
	agent.loopDetector.RecordToolCall("tool_b", `{}`)

	// Clear tool_a count
	agent.loopDetector.ClearToolCount("tool_a")

	counts := agent.loopDetector.GetToolCounts()
	if counts["tool_a"] != 0 {
		t.Errorf("Tool A count should be 0 after clear, got %d", counts["tool_a"])
	}

	if counts["tool_b"] != 1 {
		t.Errorf("Tool B count should still be 1, got %d", counts["tool_b"])
	}
}

func TestNormalizeJSONArgs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "already a JSON object",
			input:    `{"command":"ls"}`,
			expected: `{"command":"ls"}`,
		},
		{
			name:     "single string encoding",
			input:    `"{\"command\":\"ls\"}"`,
			expected: `{"command":"ls"}`,
		},
		{
			name:     "double string encoding",
			input:    `"\"{\\\"command\\\":\\\"ls\\\"}\""`,
			expected: `{"command":"ls"}`,
		},
		{
			name:     "string with unicode escapes",
			input:    `"{\"command\":\"mkdir tetris \\u0026\\u0026 cd tetris\"}"`,
			expected: `{"command":"mkdir tetris \u0026\u0026 cd tetris"}`,
		},
		{
			name:     "complex object with newlines",
			input:    `"{\"path\":\"test.py\",\"content\":\"print(\\\"hello\\\")\\n\"}"`,
			expected: `{"content":"print(\"hello\")\n","path":"test.py"}`,
		},
		{
			name:     "empty object",
			input:    `{}`,
			expected: `{}`,
		},
		{
			name:     "empty string encoded",
			input:    `"{}"`,
			expected: `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeJSONArgs([]byte(tt.input))
			// Parse both expected and result as JSON for comparison
			// (key ordering may differ)
			var expectedMap, resultMap map[string]interface{}
			if err1 := json.Unmarshal([]byte(tt.expected), &expectedMap); err1 == nil {
				if err2 := json.Unmarshal([]byte(result), &resultMap); err2 == nil {
					// Compare maps
					expectedBytes, _ := json.Marshal(expectedMap)
					resultBytes, _ := json.Marshal(resultMap)
					if string(expectedBytes) != string(resultBytes) {
						t.Errorf("normalizeJSONArgs(%s) = %s, want %s", tt.input, result, tt.expected)
					}
					return
				}
			}
			// Fallback: string comparison
			if result != tt.expected {
				t.Errorf("normalizeJSONArgs(%s) = %s, want %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseChatResponseWithStringArgs(t *testing.T) {
	// Simulate LLM returning arguments as a JSON string (common with Ollama)
	resp := &llm.ChatResponse{
		Choices: []llm.Choice{
			{
				Message: llm.Message{
					Content: "",
					ToolCalls: []llm.ToolCall{
						{
							ID:   "call_1",
							Type: "function",
							Function: llm.FunctionCall{
								Name:      "bash",
								Arguments: []byte(`"{\"command\":\"ls -la\"}"`),
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}

	result, err := parseChatResponse(resp)
	if err != nil {
		t.Fatalf("parseChatResponse failed: %v", err)
	}

	if len(result.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(result.ToolCalls))
	}

	tc := result.ToolCalls[0]
	if tc.Function.Name != "bash" {
		t.Errorf("Expected tool name 'bash', got '%s'", tc.Function.Name)
	}

	// The arguments should be a valid JSON object string
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		t.Errorf("Failed to unmarshal arguments: %v (arguments = %s)", err, tc.Function.Arguments)
	}

	if args.Command != "ls -la" {
		t.Errorf("Expected command 'ls -la', got '%s'", args.Command)
	}
}

func TestParseChatResponseWithObjectArgs(t *testing.T) {
	// Simulate LLM returning arguments as a JSON object (standard format)
	resp := &llm.ChatResponse{
		Choices: []llm.Choice{
			{
				Message: llm.Message{
					Content: "",
					ToolCalls: []llm.ToolCall{
						{
							ID:   "call_1",
							Type: "function",
							Function: llm.FunctionCall{
								Name:      "write_file",
								Arguments: []byte(`{"path":"test.py","content":"print('hello')"}`),
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}

	result, err := parseChatResponse(resp)
	if err != nil {
		t.Fatalf("parseChatResponse failed: %v", err)
	}

	if len(result.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(result.ToolCalls))
	}

	// Should be parseable as JSON
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(result.ToolCalls[0].Function.Arguments), &args); err != nil {
		t.Errorf("Failed to unmarshal arguments: %v (arguments = %s)", err, result.ToolCalls[0].Function.Arguments)
	}

	if args.Path != "test.py" {
		t.Errorf("Expected path 'test.py', got '%s'", args.Path)
	}
}

func TestDynamicMaxTokens(t *testing.T) {
	base := 8192

	tests := []struct {
		name      string
		iteration int
		expected  int
	}{
		{"iteration 1 - full tokens", 1, 8192},
		{"iteration 3 - full tokens", 3, 8192},
		{"iteration 4 - half tokens", 4, 4096},
		{"iteration 10 - half tokens", 10, 4096},
		{"iteration 11 - quarter tokens", 11, 2048},
		{"iteration 30 - quarter tokens", 30, 2048},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := dynamicMaxTokens(base, tt.iteration)
			if result != tt.expected {
				t.Errorf("dynamicMaxTokens(%d, %d) = %d, want %d", base, tt.iteration, result, tt.expected)
			}
		})
	}
}

func TestLoopDetectorStuckLoop(t *testing.T) {
	agent := createSimpleTestAgent()

	// Create a stuck loop scenario: same tool called many times
	for i := 0; i < 10; i++ {
		agent.loopDetector.RecordToolCall("stuck_tool", `{}`)
	}

	// Should detect stuck loop
	stuck := agent.loopDetector.CheckForStuckLoop()
	if !stuck {
		t.Error("Should detect stuck loop after 10 identical calls")
	}

	// Should suggest abort
	abort := agent.loopDetector.ShouldAbort()
	if !abort {
		t.Error("Should suggest abort for stuck loop")
	}
}
