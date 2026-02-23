package llm

import (
	"encoding/json"
	"testing"
	"time"
)

func TestExtractToolCallsFromText_InvokePattern(t *testing.T) {
	text := `<invoke name="bash_tool">{"command": "ls -la"}</invoke>`
	knownTools := []string{"bash_tool"}

	calls, err := ExtractToolCallsFromText(text, knownTools)
	if err != nil {
		t.Fatalf("ExtractToolCallsFromText() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %v, want 1", len(calls))
	}

	call := calls[0]
	if call.Function.Name != "bash_tool" {
		t.Errorf("Function.Name = %v, want bash_tool", call.Function.Name)
	}

	var args map[string]interface{}
	if err := json.Unmarshal(call.Function.Arguments, &args); err != nil {
		t.Fatalf("Failed to unmarshal arguments: %v", err)
	}
	if args["command"] != "ls -la" {
		t.Errorf("command = %v, want 'ls -la'", args["command"])
	}
}

func TestExtractToolCallsFromText_FunctionPattern(t *testing.T) {
	text := `<function>{"name": "read_file", "arguments": {"path": "test.txt"}}</function>`
	knownTools := []string{"read_file"}

	calls, err := ExtractToolCallsFromText(text, knownTools)
	if err != nil {
		t.Fatalf("ExtractToolCallsFromText() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %v, want 1", len(calls))
	}

	call := calls[0]
	if call.Function.Name != "read_file" {
		t.Errorf("Function.Name = %v, want read_file", call.Function.Name)
	}

	var args map[string]interface{}
	if err := json.Unmarshal(call.Function.Arguments, &args); err != nil {
		t.Fatalf("Failed to unmarshal arguments: %v", err)
	}
	if args["path"] != "test.txt" {
		t.Errorf("path = %v, want 'test.txt'", args["path"])
	}
}

func TestExtractToolCallsFromText_SimplePattern(t *testing.T) {
	text := `<use_tool name="grep_tool">{"pattern": "test"}</use_tool>`
	knownTools := []string{"grep_tool"}

	calls, err := ExtractToolCallsFromText(text, knownTools)
	if err != nil {
		t.Fatalf("ExtractToolCallsFromText() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %v, want 1", len(calls))
	}

	call := calls[0]
	if call.Function.Name != "grep_tool" {
		t.Errorf("Function.Name = %v, want grep_tool", call.Function.Name)
	}
}

func TestExtractToolCallsFromText_KeyValueArgs(t *testing.T) {
	text := `<invoke name="bash_tool">command=ls path=/tmp</invoke>`
	knownTools := []string{"bash_tool"}

	calls, err := ExtractToolCallsFromText(text, knownTools)
	if err != nil {
		t.Fatalf("ExtractToolCallsFromText() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %v, want 1", len(calls))
	}

	var args map[string]string
	if err := json.Unmarshal(calls[0].Function.Arguments, &args); err != nil {
		t.Fatalf("Failed to unmarshal arguments: %v", err)
	}
	if args["command"] != "ls" {
		t.Errorf("command = %v, want 'ls'", args["command"])
	}
	if args["path"] != "/tmp" {
		t.Errorf("path = %v, want '/tmp'", args["path"])
	}
}

func TestExtractToolCallsFromText_MultiplePatterns(t *testing.T) {
	text := `<invoke name="bash_tool">{"command": "ls"}</invoke><function>{"name": "read_file", "arguments": {"path": "test.txt"}}</function>`
	knownTools := []string{"bash_tool", "read_file"}

	calls, err := ExtractToolCallsFromText(text, knownTools)
	if err != nil {
		t.Fatalf("ExtractToolCallsFromText() error = %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("len(calls) = %v, want 2", len(calls))
	}
}

func TestExtractToolCallsFromText_RemoveCodeBlocks(t *testing.T) {
	text := "```tool\n<invoke name=\"bash_tool\">{\"command\": \"ls\"}</invoke>\n```"
	knownTools := []string{"bash_tool"}

	calls, err := ExtractToolCallsFromText(text, knownTools)
	if err != nil {
		t.Fatalf("ExtractToolCallsFromText() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %v, want 1", len(calls))
	}
}

func TestExtractToolCallsFromText_FilterUnknownTools(t *testing.T) {
	text := `<invoke name="bash_tool">{"command": "ls"}</invoke><invoke name="unknown_tool">{"arg": "val"}</invoke>`
	knownTools := []string{"bash_tool"}

	calls, err := ExtractToolCallsFromText(text, knownTools)
	if err != nil {
		t.Fatalf("ExtractToolCallsFromText() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %v, want 1", len(calls))
	}
	if calls[0].Function.Name != "bash_tool" {
		t.Errorf("Function.Name = %v, want bash_tool", calls[0].Function.Name)
	}
}

func TestExtractToolCallsFromText_RemoveDuplicates(t *testing.T) {
	text := `<invoke name="bash_tool">{"command": "ls"}</invoke><invoke name="bash_tool">{"command": "ls"}</invoke>`
	knownTools := []string{"bash_tool"}

	calls, err := ExtractToolCallsFromText(text, knownTools)
	if err != nil {
		t.Fatalf("ExtractToolCallsFromText() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("len(calls) = %v, want 1 (duplicates should be removed)", len(calls))
	}
}

func TestExtractToolCallsFromText_NoMatches(t *testing.T) {
	text := "Just plain text without any tool calls"
	knownTools := []string{"bash_tool"}

	calls, err := ExtractToolCallsFromText(text, knownTools)
	if err == nil {
		t.Fatal("expected error for no matches, got nil")
	}
	if calls != nil {
		t.Fatal("expected nil calls on error")
	}
}

func TestRemoveCodeBlocks(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "tool code block",
			input:    "```tool\n<invoke name=\"test\">arg</invoke>\n```",
			expected: "<invoke name=\"test\">arg</invoke>",
		},
		{
			name:     "xml code block",
			input:    "```xml\n<invoke name=\"test\">arg</invoke>\n```",
			expected: "<invoke name=\"test\">arg</invoke>",
		},
		{
			name:     "function code block",
			input:    "```function\n<invoke name=\"test\">arg</invoke>\n```",
			expected: "<invoke name=\"test\">arg</invoke>",
		},
		{
			name:     "no code block",
			input:    "<invoke name=\"test\">arg</invoke>",
			expected: "<invoke name=\"test\">arg</invoke>",
		},
		{
			name:     "mixed content",
			input:    "text ```tool\ncode\n``` more text",
			expected: "text code more text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeCodeBlocks(tt.input)
			if result != tt.expected {
				t.Errorf("removeCodeBlocks() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestParseKeyValueArgs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single pair",
			input:    "command=ls",
			expected: `{"command":"ls"}`,
		},
		{
			name:     "multiple pairs",
			input:    "command=ls path=/tmp",
			expected: `{"command":"ls","path":"/tmp"}`,
		},
		{
			name:     "empty string",
			input:    "",
			expected: `{}`,
		},
		{
			name:     "whitespace only",
			input:    "   ",
			expected: `{}`,
		},
		{
			name:     "complex values",
			input:    "key1=value1 key2=value2_with_underscores",
			expected: `{"key1":"value1","key2":"value2_with_underscores"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseKeyValueArgs(tt.input)
			var resultJSON map[string]interface{}
			var expectedJSON map[string]interface{}

			if err := json.Unmarshal(result, &resultJSON); err != nil {
				t.Fatalf("Failed to unmarshal result: %v", err)
			}
			if err := json.Unmarshal([]byte(tt.expected), &expectedJSON); err != nil {
				t.Fatalf("Failed to unmarshal expected: %v", err)
			}

			if len(resultJSON) != len(expectedJSON) {
				t.Errorf("len(result) = %v, want %v", len(resultJSON), len(expectedJSON))
			}

			for key, val := range expectedJSON {
				if resultJSON[key] != val {
					t.Errorf("result[%s] = %v, want %v", key, resultJSON[key], val)
				}
			}
		})
	}
}

func TestFilterKnownTools(t *testing.T) {
	calls := []ToolCall{
		{
			Function: FunctionCall{Name: "bash_tool", Arguments: json.RawMessage(`{"cmd":"ls"}`)},
		},
		{
			Function: FunctionCall{Name: "read_file", Arguments: json.RawMessage(`{"path":"test"}`)},
		},
		{
			Function: FunctionCall{Name: "unknown_tool", Arguments: json.RawMessage(`{"arg":"val"}`)},
		},
	}

	knownTools := []string{"bash_tool", "read_file"}

	filtered := filterKnownTools(calls, knownTools)
	if len(filtered) != 2 {
		t.Fatalf("len(filtered) = %v, want 2", len(filtered))
	}

	for _, call := range filtered {
		if call.Function.Name == "unknown_tool" {
			t.Error("unknown_tool should be filtered out")
		}
	}
}

func TestFilterKnownTools_AllKnown(t *testing.T) {
	calls := []ToolCall{
		{
			Function: FunctionCall{Name: "bash_tool", Arguments: json.RawMessage(`{}`)},
		},
		{
			Function: FunctionCall{Name: "read_file", Arguments: json.RawMessage(`{}`)},
		},
	}

	knownTools := []string{"bash_tool", "read_file"}

	filtered := filterKnownTools(calls, knownTools)
	if len(filtered) != 2 {
		t.Errorf("len(filtered) = %v, want 2", len(filtered))
	}
}

func TestFilterKnownTools_EmptyKnown(t *testing.T) {
	calls := []ToolCall{
		{
			Function: FunctionCall{Name: "bash_tool", Arguments: json.RawMessage(`{}`)},
		},
	}

	knownTools := []string{}

	filtered := filterKnownTools(calls, knownTools)
	if len(filtered) != 1 {
		t.Errorf("len(filtered) = %v, want 1 (all tools when known is empty)", len(filtered))
	}
}

func TestRemoveDuplicates(t *testing.T) {
	calls := []ToolCall{
		{
			Function: FunctionCall{Name: "bash_tool", Arguments: json.RawMessage(`{"cmd":"ls"}`)},
		},
		{
			Function: FunctionCall{Name: "bash_tool", Arguments: json.RawMessage(`{"cmd":"ls"}`)},
		},
		{
			Function: FunctionCall{Name: "read_file", Arguments: json.RawMessage(`{"path":"test"}`)},
		},
	}

	unique := removeDuplicates(calls)
	if len(unique) != 2 {
		t.Fatalf("len(unique) = %v, want 2", len(unique))
	}

	bashCount := 0
	readCount := 0
	for _, call := range unique {
		if call.Function.Name == "bash_tool" {
			bashCount++
		}
		if call.Function.Name == "read_file" {
			readCount++
		}
	}

	if bashCount != 1 {
		t.Errorf("bash_tool count = %v, want 1", bashCount)
	}
	if readCount != 1 {
		t.Errorf("read_file count = %v, want 1", readCount)
	}
}

func TestGenerateCallID(t *testing.T) {
	toolName := "bash_tool"

	id1 := generateCallID(toolName)
	time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	id2 := generateCallID(toolName)

	if id1 == id2 {
		t.Error("generateCallID() should generate unique IDs")
	}

	if !startsWith(id1, "call_bash_tool_") {
		t.Errorf("ID = %v, want to start with 'call_bash_tool_'", id1)
	}

	if !startsWith(id2, "call_bash_tool_") {
		t.Errorf("ID = %v, want to start with 'call_bash_tool_'", id2)
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
