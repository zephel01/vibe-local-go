package session

import (
	"testing"
)

func TestNewSession(t *testing.T) {
	session := NewSession("test-id", "system prompt")

	if session.ID != "test-id" {
		t.Errorf("ID = %v, want test-id", session.ID)
	}

	if session.SystemPrompt != "system prompt" {
		t.Errorf("SystemPrompt = %v, want 'system prompt'", session.SystemPrompt)
	}

	if len(session.Messages) != 0 {
		t.Errorf("Messages = %v, want 0", len(session.Messages))
	}

	if session.TokenEstimate != len("system prompt") {
		t.Errorf("TokenEstimate = %v, want %v", session.TokenEstimate, len("system prompt"))
	}
}

func TestAddUserMessage(t *testing.T) {
	session := NewSession("test-id", "")

	session.AddUserMessage("Hello, world!")

	if len(session.Messages) != 1 {
		t.Fatalf("len(Messages) = %v, want 1", len(session.Messages))
	}

	msg := session.Messages[0]
	if msg.Role != RoleUser {
		t.Errorf("Role = %v, want user", msg.Role)
	}

	if msg.Content != "Hello, world!" {
		t.Errorf("Content = %v, want 'Hello, world!'", msg.Content)
	}
}

func TestAddAssistantMessage(t *testing.T) {
	session := NewSession("test-id", "")

	session.AddAssistantMessage("Hi there!")

	if len(session.Messages) != 1 {
		t.Fatalf("len(Messages) = %v, want 1", len(session.Messages))
	}

	msg := session.Messages[0]
	if msg.Role != RoleAssistant {
		t.Errorf("Role = %v, want assistant", msg.Role)
	}

	if msg.Content != "Hi there!" {
		t.Errorf("Content = %v, want 'Hi there!'", msg.Content)
	}
}

func TestAddToolCall(t *testing.T) {
	session := NewSession("test-id", "")

	toolCalls := []ToolCall{
		{
			ID:   "call_1",
			Type: "function",
			Function: FunctionCall{
				Name:      "bash",
				Arguments: `{"command": "ls"}`,
			},
		},
	}

	session.AddToolCall(toolCalls)

	if len(session.Messages) != 1 {
		t.Fatalf("len(Messages) = %v, want 1", len(session.Messages))
	}

	msg := session.Messages[0]
	if msg.Role != RoleAssistant {
		t.Errorf("Role = %v, want assistant", msg.Role)
	}

	if len(msg.ToolCalls) != 1 {
		t.Errorf("ToolCalls = %v, want 1", len(msg.ToolCalls))
	}

	if msg.ToolCalls[0].ID != "call_1" {
		t.Errorf("ToolCall.ID = %v, want call_1", msg.ToolCalls[0].ID)
	}
}

func TestAddToolResults(t *testing.T) {
	session := NewSession("test-id", "")

	results := []ToolResult{
		{
			Content:   "file1.txt\nfile2.txt",
			ToolCallID: "call_1",
		},
		{
			Content:   "success",
			ToolCallID: "call_2",
		},
	}

	session.AddToolResults(results)

	if len(session.Messages) != 2 {
		t.Fatalf("len(Messages) = %v, want 2", len(session.Messages))
	}

	if session.Messages[0].Role != RoleTool {
		t.Errorf("Role = %v, want tool", session.Messages[0].Role)
	}

	if session.Messages[0].ToolID != "call_1" {
		t.Errorf("ToolID = %v, want call_1", session.Messages[0].ToolID)
	}
}

func TestGetMessages(t *testing.T) {
	session := NewSession("test-id", "")

	session.AddUserMessage("Hello")
	session.AddAssistantMessage("Hi")

	messages := session.GetMessages()

	if len(messages) != 2 {
		t.Fatalf("len(messages) = %v, want 2", len(messages))
	}

	if messages[0].Content != "Hello" {
		t.Errorf("First message = %v, want 'Hello'", messages[0].Content)
	}

	if messages[1].Content != "Hi" {
		t.Errorf("Second message = %v, want 'Hi'", messages[1].Content)
	}
}

func TestGetMessages_IsolatedCopy(t *testing.T) {
	session := NewSession("test-id", "")
	session.AddUserMessage("Hello")

	messages1 := session.GetMessages()
	messages2 := session.GetMessages()

	if &messages1[0] == &messages2[0] {
		t.Error("GetMessages should return independent copies")
	}
}

func TestGetMessagesForLLM(t *testing.T) {
	session := NewSession("test-id", "System prompt")

	session.AddUserMessage("Hello")
	session.AddAssistantMessage("Hi")

	messages := session.GetMessagesForLLM()

	if len(messages) != 3 { // system + 2 messages
		t.Fatalf("len(messages) = %v, want 3", len(messages))
	}

	systemMsg := messages[0]
	if systemMsg["role"] != string(RoleSystem) {
		t.Errorf("First message role = %v, want system", systemMsg["role"])
	}

	if systemMsg["content"] != "System prompt" {
		t.Errorf("System content = %v, want 'System prompt'", systemMsg["content"])
	}
}

func TestGetMessagesForLLM_WithToolCalls(t *testing.T) {
	session := NewSession("test-id", "")

	toolCalls := []ToolCall{
		{
			ID:   "call_1",
			Type: "function",
			Function: FunctionCall{
				Name:      "bash",
				Arguments: `{}`,
			},
		},
	}

	session.AddToolCall(toolCalls)
	session.AddToolResults([]ToolResult{{Content: "result", ToolCallID: "call_1"}})

	messages := session.GetMessagesForLLM()

	// Check assistant message has tool_calls
	asstMsg := messages[0]
	if asstMsg["role"] != string(RoleAssistant) {
		t.Errorf("Assistant role = %v, want assistant", asstMsg["role"])
	}

	if _, ok := asstMsg["tool_calls"]; !ok {
		t.Error("Assistant message should have tool_calls")
	}

	// Check tool message has tool_call_id
	toolMsg := messages[1]
	if toolMsg["role"] != string(RoleTool) {
		t.Errorf("Tool role = %v, want tool", toolMsg["role"])
	}

	if toolMsg["tool_call_id"] != "call_1" {
		t.Errorf("Tool call ID = %v, want call_1", toolMsg["tool_call_id"])
	}
}

func TestUpdateTokenCount(t *testing.T) {
	session := NewSession("test-id", "system")

	session.AddUserMessage("Hello world")
	session.AddAssistantMessage("Hi there")

	session.UpdateTokenCount()

	if session.TokenEstimate == 0 {
		t.Error("TokenEstimate should be updated")
	}
}

func TestGetTokenCount(t *testing.T) {
	session := NewSession("test-id", "system prompt")

	count := session.GetTokenCount()

	if count != len("system prompt") {
		t.Errorf("TokenCount = %v, want %v", count, len("system prompt"))
	}
}

func TestGetMessageCount(t *testing.T) {
	session := NewSession("test-id", "")

	if session.GetMessageCount() != 0 {
		t.Errorf("MessageCount = %v, want 0", session.GetMessageCount())
	}

	session.AddUserMessage("Hello")
	session.AddUserMessage("World")

	if session.GetMessageCount() != 2 {
		t.Errorf("MessageCount = %v, want 2", session.GetMessageCount())
	}
}

func TestClear(t *testing.T) {
	session := NewSession("test-id", "system prompt")

	session.AddUserMessage("Hello")
	session.AddAssistantMessage("Hi")

	session.Clear()

	if len(session.Messages) != 0 {
		t.Errorf("Messages = %v, want 0 after Clear", len(session.Messages))
	}

	if session.TokenEstimate != len("system prompt") {
		t.Errorf("TokenEstimate = %v, want %v after Clear", session.TokenEstimate, len("system prompt"))
	}
}

func TestSetSystemPrompt(t *testing.T) {
	session := NewSession("test-id", "old prompt")

	session.SetSystemPrompt("new prompt")

	if session.SystemPrompt != "new prompt" {
		t.Errorf("SystemPrompt = %v, want 'new prompt'", session.SystemPrompt)
	}
}

func TestGetLastNMessages(t *testing.T) {
	session := NewSession("test-id", "")

	for i := 0; i < 10; i++ {
		session.AddUserMessage(string(rune('a' + i)))
	}

	last3 := session.GetLastNMessages(3)

	if len(last3) != 3 {
		t.Fatalf("len(last3) = %v, want 3", len(last3))
	}

	if last3[0].Content != "h" {
		t.Errorf("First of last 3 = %v, want h", last3[0].Content)
	}

	if last3[2].Content != "j" {
		t.Errorf("Last of last 3 = %v, want j", last3[2].Content)
	}
}

func TestGetLastNMessages_LessThanAvailable(t *testing.T) {
	session := NewSession("test-id", "")

	session.AddUserMessage("Hello")

	messages := session.GetLastNMessages(10)

	if len(messages) != 1 {
		t.Errorf("len(messages) = %v, want 1", len(messages))
	}
}

func TestClone(t *testing.T) {
	session := NewSession("test-id", "system prompt")
	session.AddUserMessage("Hello")

	clone := session.Clone()

	if clone.ID != session.ID {
		t.Errorf("Clone ID = %v, want %v", clone.ID, session.ID)
	}

	if clone.SystemPrompt != session.SystemPrompt {
		t.Errorf("Clone SystemPrompt = %v, want %v", clone.SystemPrompt, session.SystemPrompt)
	}

	if len(clone.Messages) != len(session.Messages) {
		t.Errorf("Clone message count = %v, want %v", len(clone.Messages), len(session.Messages))
	}

	// Verify they're independent
	clone.AddUserMessage("Clone message")

	if len(session.Messages) != 1 {
		t.Error("Original session should not be affected by clone modification")
	}

	if len(clone.Messages) != 2 {
		t.Error("Clone should have 2 messages after modification")
	}
}

func TestToJSON(t *testing.T) {
	session := NewSession("test-id", "system prompt")
	session.AddUserMessage("Hello")

	data, err := session.ToJSON()

	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("ToJSON() should return non-empty data")
	}

	str := string(data)
	if !contains(str, "test-id") {
		t.Error("JSON should contain session ID")
	}

	if !contains(str, "Hello") {
		t.Error("JSON should contain message content")
	}
}

func TestFromJSON(t *testing.T) {
	session := NewSession("test-id", "")

	jsonData := []byte(`{
		"ID": "loaded-id",
		"SystemPrompt": "loaded prompt",
		"Messages": [
			{"role": "user", "content": "test message"}
		],
		"TokenEstimate": 100
	}`)

	err := session.FromJSON(jsonData)

	if err != nil {
		t.Fatalf("FromJSON() error = %v", err)
	}

	if session.ID != "loaded-id" {
		t.Errorf("ID = %v, want loaded-id", session.ID)
	}

	if session.SystemPrompt != "loaded prompt" {
		t.Errorf("SystemPrompt = %v, want 'loaded prompt'", session.SystemPrompt)
	}

	if len(session.Messages) != 1 {
		t.Errorf("Message count = %v, want 1", len(session.Messages))
	}

	if session.TokenEstimate != 100 {
		t.Errorf("TokenEstimate = %v, want 100", session.TokenEstimate)
	}
}

func TestGetID(t *testing.T) {
	session := NewSession("test-id", "")

	if session.GetID() != "test-id" {
		t.Errorf("GetID() = %v, want test-id", session.GetID())
	}
}

func TestSetID(t *testing.T) {
	session := NewSession("old-id", "")

	session.SetID("new-id")

	if session.ID != "new-id" {
		t.Errorf("ID = %v, want new-id", session.ID)
	}
}

func TestHasToolCalls(t *testing.T) {
	session := NewSession("test-id", "")

	if session.HasToolCalls() {
		t.Error("HasToolCalls should return false for empty session")
	}

	session.AddUserMessage("Hello")
	if session.HasToolCalls() {
		t.Error("HasToolCalls should return false for user message")
	}

	toolCalls := []ToolCall{
		{ID: "call_1", Type: "function", Function: FunctionCall{Name: "bash", Arguments: "{}"}},
	}
	session.AddToolCall(toolCalls)

	if !session.HasToolCalls() {
		t.Error("HasToolCalls should return true after adding tool call")
	}
}

func TestGetLastAssistantMessage(t *testing.T) {
	session := NewSession("test-id", "")

	session.AddUserMessage("User 1")
	session.AddAssistantMessage("Assistant 1")
	session.AddUserMessage("User 2")

	msg, found := session.GetLastAssistantMessage()

	if !found {
		t.Fatal("Should find last assistant message")
	}

	if msg.Content != "Assistant 1" {
		t.Errorf("Message = %v, want 'Assistant 1'", msg.Content)
	}
}

func TestGetLastAssistantMessage_NotFound(t *testing.T) {
	session := NewSession("test-id", "")
	session.AddUserMessage("Hello")

	msg, found := session.GetLastAssistantMessage()

	if found {
		t.Error("Should not find assistant message when none exists")
	}

	if msg != nil {
		t.Error("Message should be nil when not found")
	}
}

func TestGetContextWindow(t *testing.T) {
	session := NewSession("test-id", "")

	window := session.GetContextWindow()

	if window != 32768 {
		t.Errorf("GetContextWindow() = %v, want 32768", window)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
