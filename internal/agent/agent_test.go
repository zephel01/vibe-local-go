package agent

import (
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

	client := llm.NewClient(cfg.OllamaHost)
	registry := tool.NewRegistry()
	permMgr, _ := security.NewPermissionManager(true)
	validator := security.NewPathValidator(".")
	sess := session.NewSession("test-session", "")
	term := ui.NewTerminal()

	return NewAgent(
		client,
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

	client := llm.NewClient(cfg.OllamaHost)
	registry := tool.NewRegistry()
	permMgr, _ := security.NewPermissionManager(true)
	validator := security.NewPathValidator(".")
	sess := session.NewSession("test-session", "")
	term := ui.NewTerminal()

	agent := NewAgent(
		client,
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

	if agent.client == nil {
		t.Error("Client should be set")
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

	client := llm.NewClient(cfg.OllamaHost)
	registry := tool.NewRegistry()
	permMgr, _ := security.NewPermissionManager(true)
	validator := security.NewPathValidator(".")
	
	sess := session.NewSession("test-session", "")
	term := ui.NewTerminal()

	agent := NewAgent(
		client,
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

	client := llm.NewClient(cfg.OllamaHost)
	registry := tool.NewRegistry()
	permMgr, _ := security.NewPermissionManager(true)
	validator := security.NewPathValidator(".")
	
	sess := session.NewSession("test-session", "")
	term := ui.NewTerminal()

	agent := NewAgent(
		client,
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

	client := llm.NewClient(cfg.OllamaHost)
	registry := tool.NewRegistry()
	permMgr, _ := security.NewPermissionManager(true)
	validator := security.NewPathValidator(".")
	
	sess := session.NewSession("test-session", "")
	sess.AddUserMessage("test")
	
	term := ui.NewTerminal()

	agent := NewAgent(
		client,
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
