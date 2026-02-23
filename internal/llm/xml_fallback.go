package llm

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ExtractToolCallsFromText extracts tool calls from XML-formatted text
// This is a fallback for models that don't support function calling (e.g., Qwen)
func ExtractToolCallsFromText(text string, knownToolNames []string) ([]ToolCall, error) {
	var toolCalls []ToolCall

	// Remove code blocks (injection prevention)
	text = removeCodeBlocks(text)

	// Try multiple XML patterns
	patterns := []struct {
		name     string
		regex    string
		parser   func(matches [][]string, knownTools []string) ([]ToolCall, error)
	}{
		{
			name:  "invoke",
			regex: `<invoke\s+name="([^"]+)">([^<]*)</invoke>`,
			parser: parseInvokePattern,
		},
		{
			name:  "function",
			regex: `<function[^>]*>([^<]*)</function>`,
			parser: parseFunctionPattern,
		},
		{
			name:  "use_tool",
			regex: `<use_tool\s+name="([^"]+)">([^<]*)</use_tool>`,
			parser: parseSimplePattern,
		},
		{
			name:  "tool_call",
			regex: `<tool_call\s+name="([^"]+)">([^<]*)</tool_call>`,
			parser: parseSimplePattern,
		},
		{
			name:  "execute",
			regex: `<execute\s+name="([^"]+)">([^<]*)</execute>`,
			parser: parseSimplePattern,
		},
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern.regex)
		matches := re.FindAllStringSubmatch(text, -1)

		if len(matches) > 0 {
			calls, err := pattern.parser(matches, knownToolNames)
			if err != nil {
				continue
			}
			toolCalls = append(toolCalls, calls...)
		}
	}

	// Filter to only known tools
	toolCalls = filterKnownTools(toolCalls, knownToolNames)

	// Remove duplicates
	toolCalls = removeDuplicates(toolCalls)

	if len(toolCalls) == 0 {
		return nil, fmt.Errorf("no tool calls found in XML text")
	}

	return toolCalls, nil
}

func removeCodeBlocks(text string) string {
	// Remove ```tool, ```xml, and ```function code blocks
	// Capture the content without leading/trailing newlines
	re := regexp.MustCompile("```(?:tool|xml|function)\\n([\\s\\S]*?)\\n```")
	text = re.ReplaceAllString(text, "$1")
	return text
}

func parseInvokePattern(matches [][]string, knownTools []string) ([]ToolCall, error) {
	var calls []ToolCall

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		toolName := match[1]
		arguments := match[2]

		// Parse arguments as JSON
		var args json.RawMessage
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			// Try to parse as plain text or key=value pairs
			args = parseKeyValueArgs(arguments)
		}

		calls = append(calls, ToolCall{
			ID:   generateCallID(toolName),
			Type: "function",
			Function: FunctionCall{
				Name:      toolName,
				Arguments: args,
			},
		})
	}

	return calls, nil
}

func parseFunctionPattern(matches [][]string, knownTools []string) ([]ToolCall, error) {
	var calls []ToolCall

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		// Expect JSON format: {"name": "tool_name", "arguments": {...}}
		var data struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}

		if err := json.Unmarshal([]byte(match[1]), &data); err != nil {
			continue
		}

		calls = append(calls, ToolCall{
			ID:   generateCallID(data.Name),
			Type: "function",
			Function: FunctionCall{
				Name:      data.Name,
				Arguments: data.Arguments,
			},
		})
	}

	return calls, nil
}

func parseSimplePattern(matches [][]string, knownTools []string) ([]ToolCall, error) {
	var calls []ToolCall

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}

		toolName := match[1]
		arguments := match[2]

		// Parse arguments as JSON
		var args json.RawMessage
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			args = parseKeyValueArgs(arguments)
		}

		calls = append(calls, ToolCall{
			ID:   generateCallID(toolName),
			Type: "function",
			Function: FunctionCall{
				Name:      toolName,
				Arguments: args,
			},
		})
	}

	return calls, nil
}

func parseKeyValueArgs(text string) json.RawMessage {
	text = strings.TrimSpace(text)
	if text == "" {
		return json.RawMessage("{}")
	}

	// Try to parse as key=value pairs
	pairs := strings.Fields(text)
	result := make(map[string]string)
	re := regexp.MustCompile(`^([^=]+)=(.*)$`)

	for _, pair := range pairs {
		if match := re.FindStringSubmatch(pair); len(match) == 3 {
			result[match[1]] = match[2]
		}
	}

	jsonBytes, _ := json.Marshal(result)
	return json.RawMessage(jsonBytes)
}

func filterKnownTools(calls []ToolCall, knownTools []string) []ToolCall {
	if len(knownTools) == 0 {
		return calls
	}

	knownSet := make(map[string]bool)
	for _, name := range knownTools {
		knownSet[name] = true
	}

	var filtered []ToolCall
	for _, call := range calls {
		if knownSet[call.Function.Name] {
			filtered = append(filtered, call)
		}
	}

	return filtered
}

func removeDuplicates(calls []ToolCall) []ToolCall {
	seen := make(map[string]bool)
	var unique []ToolCall

	for _, call := range calls {
		key := fmt.Sprintf("%s-%s", call.Function.Name, call.Function.Arguments)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, call)
		}
	}

	return unique
}

func generateCallID(toolName string) string {
	return fmt.Sprintf("call_%s_%d", toolName, time.Now().UnixNano())
}
