package session

import (
	"strings"
	"testing"
)

func TestCompactIfNeeded_BelowThreshold(t *testing.T) {
	session := NewSession("test", "")

	// Add few messages (< 300)
	for i := 0; i < 50; i++ {
		session.AddUserMessage("Short message")
	}

	session.UpdateTokenCount()

	result := session.CompactIfNeeded()

	if result != nil {
		t.Error("CompactIfNeeded should return nil when below threshold")
	}
}

func TestCompactIfNeeded_AboveMessageThreshold(t *testing.T) {
	session := NewSession("test", "")

	// Add many messages (> 300)
	for i := 0; i < 350; i++ {
		session.AddUserMessage("Message " + string(rune('a'+(i%26))))
	}

	session.UpdateTokenCount()

	result := session.CompactIfNeeded()

	if result == nil {
		t.Error("CompactIfNeeded should return result when above threshold")
	}

	if result.CompactedMessages <= 0 {
		t.Error("CompactedMessages should be > 0")
	}
}

func TestCompact(t *testing.T) {
	session := NewSession("test", "")

	// Add 350 messages
	for i := 0; i < 350; i++ {
		session.AddUserMessage("Message")
	}

	session.UpdateTokenCount()
	originalTokenCount := session.TokenEstimate

	result := session.Compact()

	if result == nil {
		t.Fatal("Compact() should return result")
	}

	// Should keep 30 messages + 1 summary message = 31
	if len(session.Messages) != 31 {
		t.Errorf("Message count after compaction = %v, want 31", len(session.Messages))
	}

	if result.RemainingMessages != 31 {
		t.Errorf("Result.RemainingMessages = %v, want 31", result.RemainingMessages)
	}

	if result.CompactedMessages != 319 {
		t.Errorf("CompactedMessages = %v, want 319", result.CompactedMessages)
	}

	if session.TokenEstimate >= originalTokenCount {
		t.Error("TokenEstimate should be reduced after compaction")
	}
}

func TestCompact_SmallSession(t *testing.T) {
	session := NewSession("test", "")
	session.AddUserMessage("Hello")

	originalTokenCount := session.TokenEstimate
	originalMessages := len(session.Messages)

	result := session.Compact()

	// Should not compact if less than 300 messages
	if len(session.Messages) != originalMessages {
		t.Error("Small session should not be compacted")
	}

	if session.TokenEstimate != originalTokenCount {
		t.Error("TokenEstimate should not change for small session")
	}

	if result == nil {
		t.Fatal("Compact() should return result even for small session")
	}

	if result.CompactedMessages != 0 {
		t.Error("CompactedMessages should be 0 for small session")
	}
}

func TestCompact_WithSummary(t *testing.T) {
	session := NewSession("test", "")

	// Add messages with tool calls
	for i := 0; i < 350; i++ {
		session.AddUserMessage("User message")
		if i%2 == 0 {
			session.AddToolCall([]ToolCall{
				{ID: "call_1", Type: "function", Function: FunctionCall{Name: "bash", Arguments: "{}"}},
			})
		}
	}

	result := session.Compact()

	if result == nil {
		t.Fatal("Compact() should return result")
	}

	// Should include a summary message
	if len(session.Messages) < 31 {
		t.Errorf("Should have at least 30 messages + 1 summary, got %d", len(session.Messages))
	}

	// First message should be system role with summary
	if session.Messages[0].Role != RoleSystem {
		t.Errorf("First message should be system (summary), got %v", session.Messages[0].Role)
	}

	if result.Summary == "" {
		t.Error("Summary should be generated")
	}

	if !contains(result.Summary, "Summary of previous conversation") {
		t.Error("Summary should contain header")
	}
}

func TestSummarizeMessages_Empty(t *testing.T) {
	summary, err := summarizeMessages([]Message{})

	if err != nil {
		t.Fatalf("summarizeMessages() error = %v", err)
	}

	if summary != "" {
		t.Errorf("summarizeMessages() = %v, want empty string for empty list", summary)
	}
}

func TestSummarizeMessages_Simple(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAssistant, Content: "Hi there"},
	}

	summary, err := summarizeMessages(messages)

	if err != nil {
		t.Fatalf("summarizeMessages() error = %v", err)
	}

	if summary == "" {
		t.Error("Summary should not be empty")
	}

	if !contains(summary, "User actions: 1") {
		t.Error("Summary should count user actions")
	}

	if !contains(summary, "Assistant responses: 1") {
		t.Error("Summary should count assistant responses")
	}
}

func TestSummarizeMessages_WithToolCalls(t *testing.T) {
	messages := []Message{
		{Role: RoleUser, Content: "List files"},
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{ID: "call_1", Type: "function", Function: FunctionCall{Name: "bash", Arguments: "{}"}},
				{ID: "call_2", Type: "function", Function: FunctionCall{Name: "grep", Arguments: "{}"}},
			},
		},
	}

	summary, err := summarizeMessages(messages)

	if err != nil {
		t.Fatalf("summarizeMessages() error = %v", err)
	}

	if !contains(summary, "Tools used:") {
		t.Error("Summary should include tool usage")
	}

	if !contains(summary, "bash: 1 time(s)") {
		t.Error("Summary should count bash tool calls")
	}

	if !contains(summary, "grep: 1 time(s)") {
		t.Error("Summary should count grep tool calls")
	}
}

func TestTruncateForSummary_Short(t *testing.T) {
	text := "Short text"
	result := truncateForSummary(text, 100)

	if result != text {
		t.Errorf("truncateForSummary() = %v, want %v", result, text)
	}
}

func TestTruncateForSummary_Long(t *testing.T) {
	text := "This is a very long text that should be truncated"
	result := truncateForSummary(text, 20)

	expected := "This is a very long ..."

	if result != expected {
		t.Errorf("truncateForSummary() = %v, want %v", result, expected)
	}
}

func TestCompactWithLLM(t *testing.T) {
	session := NewSession("test", "")

	for i := 0; i < 350; i++ {
		session.AddUserMessage("Message")
	}

	result := session.CompactWithLLM(nil)

	if result == nil {
		t.Error("CompactWithLLM() should return result")
	}

	// For now, CompactWithLLM does the same as Compact
	// Should compact the session
	if len(session.Messages) > 30 {
		t.Errorf("Messages = %v, want <= 30 after compaction", len(session.Messages))
	}
}

func TestNeedsCompaction_BelowThreshold(t *testing.T) {
	session := NewSession("test", "")
	session.AddUserMessage("Hello")

	if session.NeedsCompaction() {
		t.Error("Small session should not need compaction")
	}
}

func TestNeedsCompaction_AboveMessageThreshold(t *testing.T) {
	session := NewSession("test", "")

	// Add 350 messages
	for i := 0; i < 350; i++ {
		session.AddUserMessage("Message")
	}

	if !session.NeedsCompaction() {
		t.Error("Session with 350 messages should need compaction")
	}
}

func TestNeedsCompaction_AboveTokenThreshold(t *testing.T) {
	session := NewSession("test", "")

	// Add enough content to exceed 70% of context window
	longText := strings.Repeat("word ", 25000) // ~250k chars = ~62.5k tokens
	session.AddUserMessage(longText)
	session.UpdateTokenCount()

	if !session.NeedsCompaction() {
		t.Error("Session exceeding 70% of context window should need compaction")
	}
}

func TestGetCompactionThreshold(t *testing.T) {
	threshold := GetCompactionThreshold()

	if threshold != CompactThreshold {
		t.Errorf("GetCompactionThreshold() = %v, want %v", threshold, CompactThreshold)
	}
}

func TestGetCompactMessageThreshold(t *testing.T) {
	threshold := GetCompactMessageThreshold()

	if threshold != CompactMessageThreshold {
		t.Errorf("GetCompactMessageThreshold() = %v, want %v", threshold, CompactMessageThreshold)
	}
}

func TestGetCompactionStats(t *testing.T) {
	session := NewSession("test", "")
	session.AddUserMessage("Hello")
	session.UpdateTokenCount()

	stats := session.GetCompactionStats()

	if stats.MessageCount != 1 {
		t.Errorf("MessageCount = %v, want 1", stats.MessageCount)
	}

	if stats.CurrentTokens == 0 {
		t.Error("CurrentTokens should be non-zero")
	}

	if stats.ContextWindow != 32768 {
		t.Errorf("ContextWindow = %v, want 32768", stats.ContextWindow)
	}

	if stats.UsagePercent <= 0 {
		t.Error("UsagePercent should be > 0 for non-empty session")
	}

	if stats.NeedsCompaction {
		t.Error("Small session should not need compaction")
	}
}

func TestGetCompactionStats_Empty(t *testing.T) {
	session := NewSession("test", "")

	stats := session.GetCompactionStats()

	if stats.MessageCount != 0 {
		t.Errorf("MessageCount = %v, want 0", stats.MessageCount)
	}

	if stats.CurrentTokens < 0 {
		t.Errorf("CurrentTokens = %v, want >= 0", stats.CurrentTokens)
	}
}

func TestCompactToTarget(t *testing.T) {
	session := NewSession("test", "")

	// Add many messages
	for i := 0; i < 100; i++ {
		session.AddUserMessage(strings.Repeat("word ", 100))
	}

	session.UpdateTokenCount()
	targetTokens := 5000

	result := session.CompactToTarget(targetTokens)

	if result == nil {
		t.Fatal("CompactToTarget() should return result")
	}

	if result.Summary != "Compacted to target token count" {
		t.Errorf("Summary = %v, want 'Compacted to target token count'", result.Summary)
	}

	// Should be at or below target
	if session.TokenEstimate > targetTokens+100 { // Allow small overshoot
		t.Errorf("TokenEstimate = %v, want <= %v", session.TokenEstimate, targetTokens+100)
	}
}

func TestCompactToTarget_AlreadyBelow(t *testing.T) {
	session := NewSession("test", "")
	session.AddUserMessage("Hello")

	session.UpdateTokenCount()
	targetTokens := 10000

	result := session.CompactToTarget(targetTokens)

	if result.CompactedMessages != 0 {
		t.Error("Should not compact if already below target")
	}

	if len(session.Messages) != 1 {
		t.Error("Should keep all messages if below target")
	}
}

func TestCompactToTarget_VerifyRemovalOrder(t *testing.T) {
	session := NewSession("test", "")

	// Add messages in order
	for i := 0; i < 50; i++ {
		session.AddUserMessage("Message " + string(rune('0'+i)))
	}

	session.UpdateTokenCount()
	targetTokens := 0 // Compact to minimum

	session.CompactToTarget(targetTokens)

	// All messages should be removed
	if len(session.Messages) != 0 {
		t.Errorf("Messages = %v, want 0", len(session.Messages))
	}
}

func TestSummary(t *testing.T) {
	session := NewSession("test-id", "")
	session.AddUserMessage("Hello")

	summary := session.Summary()

	if summary == "" {
		t.Error("Summary should not be empty")
	}

	if !contains(summary, "test-id") {
		t.Error("Summary should contain session ID")
	}

	if !contains(summary, "messages") {
		t.Error("Summary should contain message count")
	}

	if !contains(summary, "tokens") {
		t.Error("Summary should contain token estimate")
	}
}

func TestSummary_Format(t *testing.T) {
	session := NewSession("test-id", "")

	for i := 0; i < 10; i++ {
		session.AddUserMessage("Message")
	}

	summary := session.Summary()

	// Check format
	if !contains(summary, "Session test-id:") {
		t.Error("Summary should start with 'Session test-id:'")
	}

	if !contains(summary, "%") {
		t.Error("Summary should contain percentage")
	}
}
