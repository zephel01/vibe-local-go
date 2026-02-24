package agent

import (
	"testing"
)

func TestNewParallelOrchestrator(t *testing.T) {
	po := NewParallelOrchestrator(nil, nil)
	if po == nil {
		t.Fatal("NewParallelOrchestrator returned nil")
	}
	if po.maxAgents != MaxParallelAgents {
		t.Errorf("expected maxAgents=%d, got %d", MaxParallelAgents, po.maxAgents)
	}
}

func TestBuildSubAgentPrompt_ReadOnly(t *testing.T) {
	prompt := buildSubAgentPrompt("analyze code", false)
	if prompt == "" {
		t.Error("prompt should not be empty")
	}
	if len(prompt) < 50 {
		t.Error("prompt seems too short")
	}
}

func TestBuildSubAgentPrompt_ReadWrite(t *testing.T) {
	prompt := buildSubAgentPrompt("fix bug", true)
	if prompt == "" {
		t.Error("prompt should not be empty")
	}
}

func TestDetectWriteConflicts_NoConflicts(t *testing.T) {
	files := map[string][]string{
		"main.go": {"agent-1"},
		"test.go": {"agent-2"},
	}

	conflicts := detectWriteConflicts(files)
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %d", len(conflicts))
	}
}

func TestDetectWriteConflicts_WithConflicts(t *testing.T) {
	files := map[string][]string{
		"main.go": {"agent-1", "agent-2"},
		"test.go": {"agent-2"},
	}

	conflicts := detectWriteConflicts(files)
	if len(conflicts) != 1 {
		t.Errorf("expected 1 conflict, got %d", len(conflicts))
	}
}

func TestJoinStrings(t *testing.T) {
	tests := []struct {
		input    []string
		expected string
	}{
		{[]string{}, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a, b"},
		{[]string{"a", "b", "c"}, "a, b, c"},
	}

	for _, tt := range tests {
		result := joinStrings(tt.input)
		if result != tt.expected {
			t.Errorf("joinStrings(%v) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestRunParallel_EmptyTasks(t *testing.T) {
	po := NewParallelOrchestrator(nil, nil)
	results := po.RunParallel(nil, []AgentTask{})
	if results != nil {
		t.Error("expected nil results for empty tasks")
	}
}
