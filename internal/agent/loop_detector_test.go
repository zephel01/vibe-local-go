package agent

import (
	"testing"
)

func TestNewLoopDetector(t *testing.T) {
	ld := NewLoopDetector()

	if ld == nil {
		t.Fatal("NewLoopDetector should return non-nil detector")
	}

	if ld.history == nil {
		t.Error("History should be initialized")
	}

	if ld.toolCounts == nil {
		t.Error("Tool counts should be initialized")
	}

	if ld.historySize != LoopHistorySize {
		t.Errorf("History size = %v, want %v", ld.historySize, LoopHistorySize)
	}
}

func TestRecordToolCall(t *testing.T) {
	ld := NewLoopDetector()

	// Record first tool call
	ld.RecordToolCall("read_file", `{"path": "test.txt"}`)

	if len(ld.history) != 1 {
		t.Errorf("History length = %v, want 1", len(ld.history))
	}

	if ld.history[0].ToolName != "read_file" {
		t.Errorf("Tool name = %v, want 'read_file'", ld.history[0].ToolName)
	}

	if ld.toolCounts["read_file"] != 1 {
		t.Errorf("Tool count = %v, want 1", ld.toolCounts["read_file"])
	}

	// Record second tool call
	ld.RecordToolCall("grep", `{"pattern": "test"}`)

	if len(ld.history) != 2 {
		t.Errorf("History length = %v, want 2", len(ld.history))
	}

	if ld.toolCounts["read_file"] != 1 {
		t.Error("Read file count should still be 1")
	}

	if ld.toolCounts["grep"] != 1 {
		t.Error("Grep count should be 1")
	}
}

func TestRecordToolCall_HistoryOverflow(t *testing.T) {
	ld := NewLoopDetector()

	// Fill history beyond capacity
	for i := 0; i < LoopHistorySize+5; i++ {
		ld.RecordToolCall("tool", `{}`)
	}

	// History should be capped at LoopHistorySize
	if len(ld.history) != LoopHistorySize {
		t.Errorf("History length = %v, want %v", len(ld.history), LoopHistorySize)
	}

	// Tool count should reflect all calls
	if ld.toolCounts["tool"] != LoopHistorySize+5 {
		t.Errorf("Tool count = %v, want %v", ld.toolCounts["tool"], LoopHistorySize+5)
	}
}

func TestDetectLoop_NoLoop(t *testing.T) {
	ld := NewLoopDetector()

	// Record different tool calls
	ld.RecordToolCall("read_file", `{"path": "a.txt"}`)
	ld.RecordToolCall("grep", `{"pattern": "test"}`)
	ld.RecordToolCall("bash", `{"command": "ls"}`)

	if ld.DetectLoop() {
		t.Error("Should not detect loop for different tools")
	}
}

func TestDetectLoop_SameToolRepeated(t *testing.T) {
	ld := NewLoopDetector()

	// Record same tool 3 times (threshold)
	ld.RecordToolCall("read_file", `{"path": "test.txt"}`)
	ld.RecordToolCall("read_file", `{"path": "test.txt"}`)
	ld.RecordToolCall("read_file", `{"path": "test.txt"}`)

	if !ld.DetectLoop() {
		t.Error("Should detect loop when same tool repeated 3 times")
	}
}

func TestDetectLoop_IdenticalSequence(t *testing.T) {
	ld := NewLoopDetector()

	// Record identical calls - need 3 to trigger detection
	ld.RecordToolCall("read_file", `{"path": "test.txt"}`)
	ld.RecordToolCall("read_file", `{"path": "test.txt"}`)
	ld.RecordToolCall("read_file", `{"path": "test.txt"}`)

	if !ld.DetectLoop() {
		t.Error("Should detect identical sequence")
	}
}

func TestDetectLoop_RepeatingPattern(t *testing.T) {
	ld := NewLoopDetector()

	// ABA pattern
	ld.RecordToolCall("read_file", `{"path": "a.txt"}`)
	ld.RecordToolCall("grep", `{"pattern": "test"}`)
	ld.RecordToolCall("read_file", `{"path": "a.txt"}`)

	if !ld.DetectLoop() {
		t.Error("Should detect ABA pattern")
	}
}

func TestHasRepeatingPattern(t *testing.T) {
	ld := NewLoopDetector()

	// Not enough calls
	ld.RecordToolCall("read_file", `{}`)
	if ld.hasRepeatingPattern() {
		t.Error("Should not detect pattern with < 3 calls")
	}

	// Different tools
	ld.RecordToolCall("read_file", `{}`)
	ld.RecordToolCall("grep", `{}`)
	ld.RecordToolCall("bash", `{}`)
	if ld.hasRepeatingPattern() {
		t.Error("Should not detect pattern for different tools")
	}

	// ABA pattern
	ld.RecordToolCall("read_file", `{}`)
	ld.RecordToolCall("grep", `{}`)
	ld.RecordToolCall("read_file", `{}`)
	if !ld.hasRepeatingPattern() {
		t.Error("Should detect ABA pattern")
	}
}

func TestGetLoopInfo_NoLoop(t *testing.T) {
	ld := NewLoopDetector()

	info := ld.GetLoopInfo()

	if info.LoopDetected {
		t.Error("LoopDetected should be false")
	}

	if info.ToolName != "" {
		t.Errorf("ToolName = %v, want empty", info.ToolName)
	}
}

func TestGetLoopInfo_WithLoop(t *testing.T) {
	ld := NewLoopDetector()

	// Create a loop
	ld.RecordToolCall("read_file", `{}`)
	ld.RecordToolCall("read_file", `{}`)
	ld.RecordToolCall("read_file", `{}`)

	info := ld.GetLoopInfo()

	if !info.LoopDetected {
		t.Error("LoopDetected should be true")
	}

	if info.ToolName != "read_file" {
		t.Errorf("ToolName = %v, want 'read_file'", info.ToolName)
	}

	if info.RepeatCount != 3 {
		t.Errorf("RepeatCount = %v, want 3", info.RepeatCount)
	}

	if info.Description == "" {
		t.Error("Description should not be empty")
	}
}

func TestFindRepeatingPattern(t *testing.T) {
	ld := NewLoopDetector()

	ld.RecordToolCall("read_file", `{}`)
	ld.RecordToolCall("grep", `{}`)
	ld.RecordToolCall("read_file", `{}`)

	pattern := ld.findRepeatingPattern()

	if pattern.ToolName != "read_file" {
		t.Errorf("ToolName = %v, want 'read_file'", pattern.ToolName)
	}
}

func TestGetDescription(t *testing.T) {
	ld := NewLoopDetector()

	// Test with max repeat
	pattern := ToolCallRecord{
		ToolName:  "read_file",
		Arguments: `{}`,
	}
	ld.toolCounts["read_file"] = MaxSameToolRepeat

	desc := ld.getDescription(pattern)

	if desc == "" {
		t.Error("Description should not be empty")
	}
}

func TestReset(t *testing.T) {
	ld := NewLoopDetector()

	// Add some history
	ld.RecordToolCall("read_file", `{}`)
	ld.RecordToolCall("grep", `{}`)

	// Reset
	ld.Reset()

	if len(ld.history) != 0 {
		t.Errorf("History should be empty after reset, got %v", len(ld.history))
	}

	if len(ld.toolCounts) != 0 {
		t.Errorf("Tool counts should be empty after reset, got %v", len(ld.toolCounts))
	}
}

func TestGetHistorySize(t *testing.T) {
	ld := NewLoopDetector()

	// Empty history
	if size := ld.GetHistorySize(); size != 0 {
		t.Errorf("History size = %v, want 0", size)
	}

	// Add some calls
	ld.RecordToolCall("read_file", `{}`)
	ld.RecordToolCall("grep", `{}`)

	if size := ld.GetHistorySize(); size != 2 {
		t.Errorf("History size = %v, want 2", size)
	}
}

func TestGetToolCounts(t *testing.T) {
	ld := NewLoopDetector()

	ld.RecordToolCall("read_file", `{}`)
	ld.RecordToolCall("read_file", `{}`)
	ld.RecordToolCall("grep", `{}`)

	counts := ld.GetToolCounts()

	if len(counts) != 2 {
		t.Errorf("Tool counts length = %v, want 2", len(counts))
	}

	if counts["read_file"] != 2 {
		t.Errorf("read_file count = %v, want 2", counts["read_file"])
	}

	if counts["grep"] != 1 {
		t.Errorf("grep count = %v, want 1", counts["grep"])
	}
}

func TestGetRecentCalls(t *testing.T) {
	ld := NewLoopDetector()

	// Add calls
	ld.RecordToolCall("read_file", `{}`)
	ld.RecordToolCall("grep", `{}`)
	ld.RecordToolCall("bash", `{}`)

	// Get recent 2 calls
	recent := ld.GetRecentCalls(2)

	if len(recent) != 2 {
		t.Errorf("Recent calls length = %v, want 2", len(recent))
	}

	if recent[0].ToolName != "grep" {
		t.Errorf("First recent call = %v, want 'grep'", recent[0].ToolName)
	}

	if recent[1].ToolName != "bash" {
		t.Errorf("Second recent call = %v, want 'bash'", recent[1].ToolName)
	}

	// Get more than available
	all := ld.GetRecentCalls(10)

	if len(all) != 3 {
		t.Errorf("All calls length = %v, want 3", len(all))
	}
}

func TestGenerateToolCallHash(t *testing.T) {
	hash1 := GenerateToolCallHash("read_file", `{"path": "test.txt"}`)
	hash2 := GenerateToolCallHash("read_file", `{"path": "test.txt"}`)
	hash3 := GenerateToolCallHash("read_file", `{"path": "other.txt"}`)

	// Same call should have same hash
	if hash1 != hash2 {
		t.Error("Same tool call should have same hash")
	}

	// Different call should have different hash
	if hash1 == hash3 {
		t.Error("Different tool calls should have different hashes")
	}

	if hash1 == "" {
		t.Error("Hash should not be empty")
	}
}

func TestCheckForStuckLoop(t *testing.T) {
	ld := NewLoopDetector()

	// Not enough calls
	if ld.CheckForStuckLoop() {
		t.Error("Should not detect stuck loop with < MaxSameToolRepeat calls")
	}

	// All different tools
	for i := 0; i < MaxSameToolRepeat; i++ {
		ld.RecordToolCall("tool"+string(rune('a'+i)), `{}`)
	}

	if ld.CheckForStuckLoop() {
		t.Error("Should not detect stuck loop with different tools")
	}

	ld.Reset()

	// All same tool
	for i := 0; i < MaxSameToolRepeat; i++ {
		ld.RecordToolCall("read_file", `{}`)
	}

	if !ld.CheckForStuckLoop() {
		t.Error("Should detect stuck loop with same tool")
	}
}

func TestGetCurrentLoopIteration(t *testing.T) {
	ld := NewLoopDetector()

	if iter := ld.GetCurrentLoopIteration("read_file"); iter != 0 {
		t.Errorf("Initial iteration = %v, want 0", iter)
	}

	ld.RecordToolCall("read_file", `{}`)
	ld.RecordToolCall("read_file", `{}`)

	if iter := ld.GetCurrentLoopIteration("read_file"); iter != 2 {
		t.Errorf("Iteration count = %v, want 2", iter)
	}

	if iter := ld.GetCurrentLoopIteration("grep"); iter != 0 {
		t.Errorf("Non-existent tool iteration = %v, want 0", iter)
	}
}

func TestShouldAbort(t *testing.T) {
	ld := NewLoopDetector()

	// Normal operation
	if ld.ShouldAbort() {
		t.Error("Should not abort with no calls")
	}

	// Stuck loop
	for i := 0; i < MaxSameToolRepeat; i++ {
		ld.RecordToolCall("read_file", `{}`)
	}

	if !ld.ShouldAbort() {
		t.Error("Should abort when stuck in loop")
	}
}

func TestGetLoopStatus(t *testing.T) {
	ld := NewLoopDetector()

	// No loop
	status := ld.GetLoopStatus()
	if status != "No loop detected" {
		t.Errorf("Status = %v, want 'No loop detected'", status)
	}

	// With loop
	for i := 0; i < MaxSameToolRepeat; i++ {
		ld.RecordToolCall("read_file", `{}`)
	}

	status = ld.GetLoopStatus()
	if status == "No loop detected" {
		t.Error("Status should indicate loop detected")
	}
}

func TestClearToolCount(t *testing.T) {
	ld := NewLoopDetector()

	ld.RecordToolCall("read_file", `{}`)
	ld.RecordToolCall("read_file", `{}`)

	if ld.toolCounts["read_file"] != 2 {
		t.Errorf("Tool count = %v, want 2", ld.toolCounts["read_file"])
	}

	ld.ClearToolCount("read_file")

	if _, exists := ld.toolCounts["read_file"]; exists {
		t.Error("Tool count should be cleared")
	}
}

func TestGetMostCalledTool(t *testing.T) {
	ld := NewLoopDetector()

	// No calls
	tool, count := ld.GetMostCalledTool()
	if tool != "" || count != 0 {
		t.Errorf("Most called tool = (%v, %v), want ('', 0)", tool, count)
	}

	// Add calls
	ld.RecordToolCall("read_file", `{}`)
	ld.RecordToolCall("read_file", `{}`)
	ld.RecordToolCall("read_file", `{}`)
	ld.RecordToolCall("grep", `{}`)

	tool, count = ld.GetMostCalledTool()
	if tool != "read_file" {
		t.Errorf("Most called tool = %v, want 'read_file'", tool)
	}

	if count != 3 {
		t.Errorf("Count = %v, want 3", count)
	}
}
