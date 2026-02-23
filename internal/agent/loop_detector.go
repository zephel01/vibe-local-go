package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const (
	// MaxSameToolRepeat is the maximum number of times to repeat the same tool
	MaxSameToolRepeat = 3
	// LoopHistorySize is the number of recent tool calls to track
	LoopHistorySize = 10
)

// ToolCallRecord represents a recorded tool call
type ToolCallRecord struct {
	ToolName   string
	Arguments  string
	Timestamp  int64
}

// LoopDetector detects repeated tool call patterns
type LoopDetector struct {
	history     []ToolCallRecord
	toolCounts  map[string]int
	historySize int
}

// NewLoopDetector creates a new loop detector
func NewLoopDetector() *LoopDetector {
	return &LoopDetector{
		history:     make([]ToolCallRecord, 0, LoopHistorySize),
		toolCounts:  make(map[string]int),
		historySize: LoopHistorySize,
	}
}

// RecordToolCall records a tool call for loop detection
func (ld *LoopDetector) RecordToolCall(toolName string, arguments string) {
	record := ToolCallRecord{
		ToolName:  toolName,
		Arguments: arguments,
		Timestamp: getCurrentTimestamp(),
	}

	// Add to history
	if len(ld.history) >= ld.historySize {
		ld.history = ld.history[1:]
	}
	ld.history = append(ld.history, record)

	// Update tool count
	ld.toolCounts[toolName]++
}

// DetectLoop checks if a loop pattern is detected
func (ld *LoopDetector) DetectLoop() bool {
	if len(ld.history) < 3 {
		return false
	}

	// Check for same tool repeated too many times
	for _, count := range ld.toolCounts {
		if count >= MaxSameToolRepeat {
			return true
		}
	}

	// Check for identical tool calls in sequence
	if ld.hasIdenticalSequence() {
		return true
	}

	// Check for repeating patterns
	if ld.hasRepeatingPattern() {
		return true
	}

	return false
}

// hasIdenticalSequence checks for identical tool calls in sequence
func (ld *LoopDetector) hasIdenticalSequence() bool {
	if len(ld.history) < 2 {
		return false
	}

	// Check last N calls
	last := ld.history[len(ld.history)-1]
	previous := ld.history[len(ld.history)-2]

	// If same tool and same arguments, potential loop
	if last.ToolName == previous.ToolName &&
		last.Arguments == previous.Arguments {
		return true
	}

	return false
}

// hasRepeatingPattern checks for repeating patterns in tool calls
func (ld *LoopDetector) hasRepeatingPattern() bool {
	if len(ld.history) < 3 {
		return false
	}

	// Check for ABA pattern (Tool A, Tool B, Tool A again)
	lastThree := ld.history[len(ld.history)-3:]
	if lastThree[0].ToolName == lastThree[2].ToolName {
		return true
	}

	// Check for simple repetition of same tool
	recentCount := 0
	lastToolName := ld.history[len(ld.history)-1].ToolName

	for i := len(ld.history) - 1; i >= 0; i-- {
		if ld.history[i].ToolName == lastToolName {
			recentCount++
		} else {
			break
		}
	}

	if recentCount >= MaxSameToolRepeat {
		return true
	}

	return false
}

// GetLoopInfo returns information about detected loops
func (ld *LoopDetector) GetLoopInfo() *LoopInfo {
	if !ld.DetectLoop() {
		return &LoopInfo{
			LoopDetected: false,
		}
	}

	// Find the repeating pattern
	pattern := ld.findRepeatingPattern()

	return &LoopInfo{
		LoopDetected:  true,
		ToolName:      pattern.ToolName,
		RepeatCount:   ld.toolCounts[pattern.ToolName],
		LastSeen:      pattern.Timestamp,
		Description:   ld.getDescription(pattern),
	}
}

// findRepeatingPattern finds the repeating tool call pattern
func (ld *LoopDetector) findRepeatingPattern() ToolCallRecord {
	if len(ld.history) == 0 {
		return ToolCallRecord{}
	}

	// Find the most frequently called tool
	maxCount := 0
	var topTool string

	for toolName, count := range ld.toolCounts {
		if count > maxCount {
			maxCount = count
			topTool = toolName
		}
	}

	// Find the most recent call to this tool
	for i := len(ld.history) - 1; i >= 0; i-- {
		if ld.history[i].ToolName == topTool {
			return ld.history[i]
		}
	}

	return ld.history[len(ld.history)-1]
}

// getDescription returns a description of the loop
func (ld *LoopDetector) getDescription(pattern ToolCallRecord) string {
	count := ld.toolCounts[pattern.ToolName]

	if count >= MaxSameToolRepeat {
		return fmt.Sprintf("Tool '%s' called %d times consecutively", pattern.ToolName, count)
	}

	return fmt.Sprintf("Repeated pattern detected with tool '%s'", pattern.ToolName)
}

// Reset resets the loop detector
func (ld *LoopDetector) Reset() {
	ld.history = make([]ToolCallRecord, 0, ld.historySize)
	ld.toolCounts = make(map[string]int)
}

// GetHistorySize returns the current history size
func (ld *LoopDetector) GetHistorySize() int {
	return len(ld.history)
}

// GetToolCounts returns the count of each tool called
func (ld *LoopDetector) GetToolCounts() map[string]int {
	counts := make(map[string]int)
	for k, v := range ld.toolCounts {
		counts[k] = v
	}
	return counts
}

// GetRecentCalls returns the N most recent tool calls
func (ld *LoopDetector) GetRecentCalls(n int) []ToolCallRecord {
	if len(ld.history) <= n {
		return ld.history
	}

	return ld.history[len(ld.history)-n:]
}

// GenerateToolCallHash generates a hash of a tool call
func GenerateToolCallHash(toolName string, arguments string) string {
	data := fmt.Sprintf("%s:%s", toolName, arguments)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// CheckForStuckLoop checks if the agent is stuck in a loop
func (ld *LoopDetector) CheckForStuckLoop() bool {
	if len(ld.history) < MaxSameToolRepeat {
		return false
	}

	// Check if all recent calls are the same tool
	lastCalls := ld.history[len(ld.history)-MaxSameToolRepeat:]
	firstTool := lastCalls[0].ToolName

	for _, call := range lastCalls {
		if call.ToolName != firstTool {
			return false
		}
	}

	return true
}

// GetCurrentLoopIteration returns the current iteration count
func (ld *LoopDetector) GetCurrentLoopIteration(toolName string) int {
	count, exists := ld.toolCounts[toolName]
	if !exists {
		return 0
	}
	return count
}

// ShouldAbort checks if execution should be aborted due to looping
func (ld *LoopDetector) ShouldAbort() bool {
	return ld.CheckForStuckLoop()
}

// GetLoopStatus returns the current loop status
func (ld *LoopDetector) GetLoopStatus() string {
	if !ld.DetectLoop() {
		return "No loop detected"
	}

	info := ld.GetLoopInfo()
	return fmt.Sprintf("Loop detected: %s", info.Description)
}

// LoopInfo represents information about a detected loop
type LoopInfo struct {
	LoopDetected bool
	ToolName    string
	RepeatCount int
	LastSeen    int64
	Description  string
}

// getCurrentTimestamp returns the current timestamp
func getCurrentTimestamp() int64 {
	// In production, use time.Now().Unix()
	// For now, return a dummy value
	return 0
}

// ClearToolCount clears the count for a specific tool
func (ld *LoopDetector) ClearToolCount(toolName string) {
	delete(ld.toolCounts, toolName)
}

// GetMostCalledTool returns the most called tool
func (ld *LoopDetector) GetMostCalledTool() (string, int) {
	maxCount := 0
	var topTool string

	for toolName, count := range ld.toolCounts {
		if count > maxCount {
			maxCount = count
			topTool = toolName
		}
	}

	return topTool, maxCount
}
