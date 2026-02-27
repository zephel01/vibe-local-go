package session

import (
	"fmt"
	"strings"
)

const (
	// CompactThreshold is the percentage at which to compact
	CompactThreshold = 0.5
	// CompactMessageThreshold is the minimum messages to trigger compaction
	CompactMessageThreshold = 100
)

// CompactionResult represents the result of a compaction operation
type CompactionResult struct {
	OriginalTokenCount int
	NewTokenCount      int
	CompactedMessages  int
	RemainingMessages   int
	Summary            string
}

// CompactIfNeeded compacts the session if needed
func (s *Session) CompactIfNeeded() *CompactionResult {
	// Check if compaction is needed
	tokenCount := s.GetTokenCount()
	messageCount := s.GetMessageCount()

	// Compact if we exceed 70% of context or have 300+ messages
	if float64(tokenCount) > float64(s.GetContextWindow())*CompactThreshold ||
		messageCount >= CompactMessageThreshold {
		return s.Compact()
	}

	return nil
}

// Compact compacts the session by summarizing old messages
func (s *Session) Compact() *CompactionResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	originalCount := s.TokenEstimate
	originalMessages := len(s.Messages)

	var summary string

	// Keep recent 30 messages
	keepCount := 30
	if len(s.Messages) > keepCount {
		oldMessages := s.Messages[:len(s.Messages)-keepCount]
		s.Messages = s.Messages[len(s.Messages)-keepCount:]

		// Try to summarize old messages
		var err error
		summary, err = summarizeMessages(oldMessages)
		if err != nil {
			// If summarization fails, just delete old messages
			for _, msg := range oldMessages {
				s.TokenEstimate -= msg.TokenCount
			}
			if s.TokenEstimate < 0 {
				s.TokenEstimate = 0
			}
		} else {
			// Create a summary message
			summaryMsg := Message{
				Role:    RoleSystem,
				Content: summary,
			}
			s.Messages = append([]Message{summaryMsg}, s.Messages...)
			s.TokenEstimate += EstimateTokens(summary)
		}
	}

	// Update token counts manually (don't call UpdateTokenCount to avoid deadlock)
	// We already have the lock
	total := 0
	for _, msg := range s.Messages {
		total += msg.TokenCount
	}
	s.TokenEstimate = total

	result := &CompactionResult{
		OriginalTokenCount: originalCount,
		NewTokenCount:      s.TokenEstimate,
		CompactedMessages:  originalMessages - len(s.Messages),
		RemainingMessages:  len(s.Messages),
		Summary:            summary,
	}

	return result
}

// summarizeMessages creates a summary of messages
func summarizeMessages(messages []Message) (string, error) {
	// Build summary from messages
	// This is a simple implementation - in production you'd use the LLM to summarize

	if len(messages) == 0 {
		return "", nil
	}

	var summaryBuilder strings.Builder
	summaryBuilder.WriteString("Summary of previous conversation:\n")

	// Count tool calls and key actions
	toolCalls := make(map[string]int)
	userActions := 0
	assistantResponses := 0

	for _, msg := range messages {
		switch msg.Role {
		case RoleUser:
			userActions++
		case RoleAssistant:
			assistantResponses++
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					toolCalls[tc.Function.Name]++
				}
			}
		case RoleTool:
			// Tool result messages
		}
	}

	// Add action summary
	if userActions > 0 || assistantResponses > 0 {
		summaryBuilder.WriteString(fmt.Sprintf("- User actions: %d\n", userActions))
		summaryBuilder.WriteString(fmt.Sprintf("- Assistant responses: %d\n", assistantResponses))
	}

	// Add tool usage summary
	if len(toolCalls) > 0 {
		summaryBuilder.WriteString("- Tools used:\n")
		for tool, count := range toolCalls {
			summaryBuilder.WriteString(fmt.Sprintf("  * %s: %d time(s)\n", tool, count))
		}
	}

	// Add recent context from last few messages
	if len(messages) > 5 {
		recentMessages := messages[len(messages)-5:]
		summaryBuilder.WriteString("\nRecent context:\n")
		for i, msg := range recentMessages {
			switch msg.Role {
			case RoleUser:
				summaryBuilder.WriteString(fmt.Sprintf("%d. User: %s\n", i+1, truncateForSummary(msg.Content, 100)))
			case RoleAssistant:
				if msg.Content != "" {
					summaryBuilder.WriteString(fmt.Sprintf("%d. Assistant: %s\n", i+1, truncateForSummary(msg.Content, 100)))
				}
				if len(msg.ToolCalls) > 0 {
					toolNames := make([]string, 0, len(msg.ToolCalls))
					for _, tc := range msg.ToolCalls {
						toolNames = append(toolNames, tc.Function.Name)
					}
					summaryBuilder.WriteString(fmt.Sprintf("%d. Assistant called: %s\n", i+1, strings.Join(toolNames, ", ")))
				}
			}
		}
	}

	return summaryBuilder.String(), nil
}

// truncateForSummary truncates content for summary display
func truncateForSummary(content string, maxLength int) string {
	if len(content) <= maxLength {
		return content
	}

	return content[:maxLength] + "..."
}

// CompactWithLLM compacts using an LLM for better summaries
// This is a placeholder - in production you'd use the LLM client
func (s *Session) CompactWithLLM(llmClient interface{}) *CompactionResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	// For now, just do simple compaction
	return s.Compact()
}

// GetContextWindow returns the context window size
func (s *Session) GetContextWindow() int {
	// This should be set from config
	// Default to 32768
	return 32768
}

// SetContextWindow sets the context window size
func (s *Session) SetContextWindow(size int) {
	// Store context window size
	// This could be a field in the Session struct
}

// NeedsCompaction checks if session needs compaction
func (s *Session) NeedsCompaction() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	contextWindow := s.GetContextWindow()
	return float64(s.TokenEstimate) > float64(contextWindow)*CompactThreshold ||
		len(s.Messages) >= CompactMessageThreshold
}

// GetCompactionThreshold returns the compaction threshold
func GetCompactionThreshold() float64 {
	return CompactThreshold
}

// GetCompactMessageThreshold returns the message threshold
func GetCompactMessageThreshold() int {
	return CompactMessageThreshold
}

// GetCompactionStats returns compaction statistics
func (s *Session) GetCompactionStats() CompactionStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	contextWindow := s.GetContextWindow()
	usage := EstimateContextUsage(s.TokenEstimate, contextWindow)

	return CompactionStats{
		CurrentTokens:   s.TokenEstimate,
		ContextWindow:   contextWindow,
		UsagePercent:    usage,
		MessageCount:    len(s.Messages),
		NeedsCompaction: s.NeedsCompaction(),
	}
}

// CompactionStats represents compaction statistics
type CompactionStats struct {
	CurrentTokens   int
	ContextWindow   int
	UsagePercent    float64
	MessageCount    int
	NeedsCompaction bool
}

// Summary returns a string summary of the session
func (s *Session) Summary() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := s.GetCompactionStats()

	return fmt.Sprintf("Session %s: %d messages, ~%d tokens (%.1f%% of %d)",
		s.ID, stats.MessageCount, stats.CurrentTokens,
		stats.UsagePercent, stats.ContextWindow)
}

// CompactToTarget compacts to a target token count
func (s *Session) CompactToTarget(targetTokens int) *CompactionResult {
	s.mu.Lock()
	defer s.mu.Unlock()

	originalCount := s.TokenEstimate
	originalMessages := len(s.Messages)

	// Remove messages until we're at target
	for len(s.Messages) > 0 && s.TokenEstimate > targetTokens {
		// Remove oldest messages first
		removed := s.Messages[0]
		s.Messages = s.Messages[1:]
		s.TokenEstimate -= removed.TokenCount
	}

	if s.TokenEstimate < 0 {
		s.TokenEstimate = 0
	}

	result := &CompactionResult{
		OriginalTokenCount: originalCount,
		NewTokenCount:      s.TokenEstimate,
		CompactedMessages:  originalMessages - len(s.Messages),
		RemainingMessages:   len(s.Messages),
		Summary:            "Compacted to target token count",
	}

	return result
}
