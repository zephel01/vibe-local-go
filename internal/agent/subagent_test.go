package agent

import (
	"strings"
	"testing"
	"time"
)

func TestSubAgentConfig_Defaults(t *testing.T) {
	sa := NewSubAgent(SubAgentConfig{
		ID:       "test-1",
		MaxTurns: 0, // Should default to SubAgentMaxTurns
	})

	if sa.maxTurns != SubAgentMaxTurns {
		t.Errorf("expected maxTurns=%d, got %d", SubAgentMaxTurns, sa.maxTurns)
	}

	if sa.id != "test-1" {
		t.Errorf("expected id='test-1', got '%s'", sa.id)
	}
}

func TestSubAgentConfig_MaxTurnsLimit(t *testing.T) {
	sa := NewSubAgent(SubAgentConfig{
		ID:       "test-2",
		MaxTurns: 100, // Over limit
	})

	if sa.maxTurns != SubAgentMaxTurns {
		t.Errorf("expected maxTurns=%d (capped), got %d", SubAgentMaxTurns, sa.maxTurns)
	}
}

func TestSubAgentConfig_CustomMaxTurns(t *testing.T) {
	sa := NewSubAgent(SubAgentConfig{
		ID:       "test-3",
		MaxTurns: 5,
	})

	if sa.maxTurns != 5 {
		t.Errorf("expected maxTurns=5, got %d", sa.maxTurns)
	}
}

func TestSubAgent_ReadOnlyFiltering(t *testing.T) {
	sa := NewSubAgent(SubAgentConfig{
		ID:          "test-readonly",
		AllowWrites: false,
	})

	if sa.allowWrites {
		t.Error("expected allowWrites=false for read-only agent")
	}
}

func TestSubAgent_WriteEnabled(t *testing.T) {
	sa := NewSubAgent(SubAgentConfig{
		ID:          "test-write",
		AllowWrites: true,
	})

	if !sa.allowWrites {
		t.Error("expected allowWrites=true")
	}
}

func TestIsWriteTool_SubAgent(t *testing.T) {
	// isWriteTool is defined in dispatch.go: write_file, edit_file, bash
	tests := []struct {
		name     string
		expected bool
	}{
		{"write_file", true},
		{"edit_file", true},
		{"bash", true},
		{"read_file", false},
		{"glob", false},
		{"grep", false},
		{"notebook_edit", false},
	}

	for _, tt := range tests {
		result := isWriteTool(tt.name)
		if result != tt.expected {
			t.Errorf("isWriteTool(%s) = %v, want %v", tt.name, result, tt.expected)
		}
	}
}

func TestFormatResults(t *testing.T) {
	results := []SubAgentResult{
		{
			ID:       "agent-1",
			Output:   "Found 3 files",
			Duration: 1500 * time.Millisecond,
			Turns:    5,
		},
		{
			ID:       "agent-2",
			Output:   "Analysis complete",
			Error:    nil,
			Duration: 2300 * time.Millisecond,
			Turns:    8,
		},
	}

	formatted := FormatResults(results)

	if !strings.Contains(formatted, "Parallel Agent Results") {
		t.Error("should contain header")
	}
	if !strings.Contains(formatted, "agent-1") {
		t.Error("should contain agent-1 ID")
	}
	if !strings.Contains(formatted, "agent-2") {
		t.Error("should contain agent-2 ID")
	}
	if !strings.Contains(formatted, "Found 3 files") {
		t.Error("should contain agent-1 output")
	}
	if !strings.Contains(formatted, "Complete") {
		t.Error("should contain 'Complete' status")
	}
}

func TestFormatResults_WithError(t *testing.T) {
	results := []SubAgentResult{
		{
			ID:       "agent-1",
			Error:    errTimeout,
			Duration: 5 * time.Second,
			Turns:    10,
		},
	}

	formatted := FormatResults(results)
	if !strings.Contains(formatted, "Error") {
		t.Error("should contain error status")
	}
}

var errTimeout = &timeoutError{}

type timeoutError struct{}

func (e *timeoutError) Error() string { return "timeout" }
