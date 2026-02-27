package session

import (
	"encoding/json"
	"sync"
)

const (
	// MaxMessages is the maximum number of messages in a session
	MaxMessages = 500
)

// MessageRole represents a message role
type MessageRole string

const (
	RoleSystem    MessageRole = "system"
	RoleUser      MessageRole = "user"
	RoleAssistant MessageRole = "assistant"
	RoleTool      MessageRole = "tool"
)

// Message represents a chat message
type Message struct {
	Role       MessageRole   `json:"role"`
	Content    string        `json:"content"`
	ToolCalls  []ToolCall   `json:"tool_calls,omitempty"`
	ToolID     string        `json:"tool_id,omitempty"`
	TokenCount int           `json:"token_count,omitempty"`
}

// ToolCall represents a tool call within a message
type ToolCall struct {
	ID       string        `json:"id"`
	Type     string        `json:"type"`
	Function FunctionCall  `json:"function"`
}

// FunctionCall represents a function call
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Session represents a chat session with message history
type Session struct {
	ID             string
	Messages       []Message
	SystemPrompt   string
	TokenEstimate  int
	mu             sync.RWMutex

	// Cache for GetMessagesForLLM (avoid O(n) rebuild every call)
	cachedLLMMessages []map[string]interface{}
	llmCacheDirty     bool // true when messages changed since last cache build
}

// NewSession creates a new session
func NewSession(id string, systemPrompt string) *Session {
	return &Session{
		ID:            id,
		Messages:      make([]Message, 0, 100),
		SystemPrompt:  systemPrompt,
		TokenEstimate: len(systemPrompt), // Rough estimate
		llmCacheDirty: true,
	}
}

// AddUserMessage adds a user message to the session
func (s *Session) AddUserMessage(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg := Message{
		Role:    RoleUser,
		Content: content,
	}

	s.Messages = append(s.Messages, msg)
	s.llmCacheDirty = true
	s.compactIfNeeded()
}

// AddAssistantMessage adds an assistant message to the session
func (s *Session) AddAssistantMessage(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg := Message{
		Role:    RoleAssistant,
		Content: content,
	}

	s.Messages = append(s.Messages, msg)
	s.llmCacheDirty = true
	s.compactIfNeeded()
}

// AddToolCall adds an assistant message with tool calls
func (s *Session) AddToolCall(toolCalls []ToolCall) {
	s.mu.Lock()
	defer s.mu.Unlock()

	msg := Message{
		Role:      RoleAssistant,
		Content:   "",
		ToolCalls:  toolCalls,
	}

	s.Messages = append(s.Messages, msg)
	s.llmCacheDirty = true
	s.compactIfNeeded()
}

// AddToolResults adds tool result messages
func (s *Session) AddToolResults(results []ToolResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, result := range results {
		msg := Message{
			Role:    RoleTool,
			Content: result.Content,
			ToolID:  result.ToolCallID,
		}

		s.Messages = append(s.Messages, msg)
	}

	s.llmCacheDirty = true
	s.compactIfNeeded()
}

// ToolResult represents a tool execution result
type ToolResult struct {
	Content   string
	ToolCallID string
}

// GetMessages returns all messages in the session
func (s *Session) GetMessages() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	messages := make([]Message, len(s.Messages))
	copy(messages, s.Messages)
	return messages
}

// GetMessagesForLLM returns messages formatted for LLM API.
// Uses a dirty-flag cache to avoid O(n) rebuild when messages haven't changed.
func (s *Session) GetMessagesForLLM() []map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.llmCacheDirty && s.cachedLLMMessages != nil {
		return s.cachedLLMMessages
	}

	// Rebuild cache
	messages := make([]map[string]interface{}, 0, len(s.Messages)+1)

	if s.SystemPrompt != "" {
		messages = append(messages, map[string]interface{}{
			"role":    string(RoleSystem),
			"content": s.SystemPrompt,
		})
	}

	for _, msg := range s.Messages {
		msgMap := map[string]interface{}{
			"role":    string(msg.Role),
			"content": msg.Content,
		}

		if len(msg.ToolCalls) > 0 {
			msgMap["tool_calls"] = msg.ToolCalls
		}

		if msg.ToolID != "" {
			msgMap["tool_call_id"] = msg.ToolID
		}

		messages = append(messages, msgMap)
	}

	s.cachedLLMMessages = messages
	s.llmCacheDirty = false
	return messages
}

// UpdateTokenCount updates the token estimate for all messages
func (s *Session) UpdateTokenCount() {
	s.mu.Lock()
	defer s.mu.Unlock()

	total := 0
	for i := range s.Messages {
		s.Messages[i].TokenCount = EstimateTokens(s.Messages[i].Content)
		total += s.Messages[i].TokenCount
	}

	s.TokenEstimate = total
}

// GetTokenCount returns the current token estimate
func (s *Session) GetTokenCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.TokenEstimate
}

// GetMessageCount returns the number of messages
func (s *Session) GetMessageCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.Messages)
}

// Clear clears all messages from the session
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Messages = make([]Message, 0, 100)
	s.TokenEstimate = len(s.SystemPrompt)
	s.llmCacheDirty = true
	s.cachedLLMMessages = nil
}

// compactIfNeeded compacts messages if we're approaching limits
func (s *Session) compactIfNeeded() {
	// Compact if we have too many messages
	if len(s.Messages) >= MaxMessages {
		s.compact()
	}
}

// compact removes old messages while keeping recent ones
func (s *Session) compact() {
	// Keep recent 300 messages
	if len(s.Messages) > 300 {
		oldMessages := s.Messages[:len(s.Messages)-300]
		s.Messages = s.Messages[len(s.Messages)-300:]

		// Update token estimate
		for _, msg := range oldMessages {
			s.TokenEstimate -= msg.TokenCount
		}
		if s.TokenEstimate < 0 {
			s.TokenEstimate = 0
		}
		s.llmCacheDirty = true
		s.cachedLLMMessages = nil
	}
}

// SetSystemPrompt sets the system prompt
func (s *Session) SetSystemPrompt(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.SystemPrompt = prompt
	s.llmCacheDirty = true
}

// GetLastNMessages returns the last N messages
func (s *Session) GetLastNMessages(n int) []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.Messages) <= n {
		return s.Messages
	}

	return s.Messages[len(s.Messages)-n:]
}

// Clone creates a deep copy of the session
func (s *Session) Clone() *Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	messages := make([]Message, len(s.Messages))
	copy(messages, s.Messages)

	return &Session{
		ID:            s.ID,
		Messages:      messages,
		SystemPrompt:  s.SystemPrompt,
		TokenEstimate: s.TokenEstimate,
	}
}

// ToJSON converts session to JSON
func (s *Session) ToJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return json.MarshalIndent(s, "", "  ")
}

// FromJSON loads session from JSON
func (s *Session) FromJSON(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return err
	}

	s.ID = session.ID
	s.Messages = session.Messages
	s.SystemPrompt = session.SystemPrompt
	s.TokenEstimate = session.TokenEstimate
	s.llmCacheDirty = true
	s.cachedLLMMessages = nil

	return nil
}

// GetID returns the session ID
func (s *Session) GetID() string {
	return s.ID
}

// SetID sets the session ID
func (s *Session) SetID(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ID = id
}

// HasToolCalls checks if the last message has tool calls
func (s *Session) HasToolCalls() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.Messages) == 0 {
		return false
	}

	lastMsg := s.Messages[len(s.Messages)-1]
	return len(lastMsg.ToolCalls) > 0
}

// GetLastAssistantMessage returns the last assistant message
func (s *Session) GetLastAssistantMessage() (*Message, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role == RoleAssistant {
			return &s.Messages[i], true
		}
	}

	return nil, false
}
