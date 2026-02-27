package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zephel01/vibe-local-go/internal/config"
	"github.com/zephel01/vibe-local-go/internal/llm"
	"github.com/zephel01/vibe-local-go/internal/session"
	"github.com/zephel01/vibe-local-go/internal/tool"
)

const (
	// SubAgentMaxTurns is the hard limit for sub-agent iterations
	SubAgentMaxTurns = 20

	// SubAgentTimeout is the default timeout for a sub-agent task
	SubAgentTimeout = 5 * time.Minute
)

// SubAgent is a lightweight agent that runs independently with its own session
type SubAgent struct {
	id            string
	provider      llm.LLMProvider
	registry      *tool.Registry
	session       *session.Session
	maxTurns      int
	allowWrites   bool
	loopDetector  *LoopDetector
}

// SubAgentConfig holds configuration for creating a SubAgent
type SubAgentConfig struct {
	ID           string
	Provider     llm.LLMProvider
	Registry     *tool.Registry
	SystemPrompt string
	MaxTurns     int
	AllowWrites  bool
}

// NewSubAgent creates a new sub-agent
func NewSubAgent(cfg SubAgentConfig) *SubAgent {
	if cfg.MaxTurns <= 0 || cfg.MaxTurns > SubAgentMaxTurns {
		cfg.MaxTurns = SubAgentMaxTurns
	}

	sessID := fmt.Sprintf("subagent-%s", cfg.ID)
	sess := session.NewSession(sessID, cfg.SystemPrompt)

	return &SubAgent{
		id:           cfg.ID,
		provider:     cfg.Provider,
		registry:     cfg.Registry,
		session:      sess,
		maxTurns:     cfg.MaxTurns,
		allowWrites:  cfg.AllowWrites,
		loopDetector: NewLoopDetector(),
	}
}

// SubAgentResult holds the result of a sub-agent execution
type SubAgentResult struct {
	ID       string        `json:"id"`
	Output   string        `json:"output"`
	Error    error         `json:"error,omitempty"`
	Duration time.Duration `json:"duration"`
	Turns    int           `json:"turns"`
}

// Run executes the sub-agent with the given task
func (sa *SubAgent) Run(ctx context.Context, task string) SubAgentResult {
	startTime := time.Now()

	// Apply timeout
	ctx, cancel := context.WithTimeout(ctx, SubAgentTimeout)
	defer cancel()

	// Add task as user message
	sa.session.AddUserMessage(task)

	var lastOutput string
	turns := 0

	for turns < sa.maxTurns {
		select {
		case <-ctx.Done():
			return SubAgentResult{
				ID:       sa.id,
				Output:   lastOutput,
				Error:    fmt.Errorf("sub-agent timed out or cancelled"),
				Duration: time.Since(startTime),
				Turns:    turns,
			}
		default:
		}

		turns++

		// Loop detection
		if sa.loopDetector.DetectLoop() {
			return SubAgentResult{
				ID:       sa.id,
				Output:   lastOutput,
				Error:    fmt.Errorf("sub-agent detected loop, stopping"),
				Duration: time.Since(startTime),
				Turns:    turns,
			}
		}

		// Prepare messages and tools
		messages := sa.session.GetMessagesForLLM()
		tools := sa.getFilteredSchemas()

		// Call LLM
		response, err := sa.callLLM(ctx, messages, tools)
		if err != nil {
			return SubAgentResult{
				ID:       sa.id,
				Output:   lastOutput,
				Error:    fmt.Errorf("LLM call failed: %v", err),
				Duration: time.Since(startTime),
				Turns:    turns,
			}
		}

		// No tool calls — agent is done
		if len(response.ToolCalls) == 0 {
			sa.session.AddAssistantMessage(response.Content)
			return SubAgentResult{
				ID:       sa.id,
				Output:   response.Content,
				Duration: time.Since(startTime),
				Turns:    turns,
			}
		}

		// Execute tool calls
		sa.session.AddToolCall(response.ToolCalls)
		results := sa.executeSubAgentTools(ctx, response.ToolCalls)
		sa.session.AddToolResults(results)

		// Track last meaningful output
		if response.Content != "" {
			lastOutput = response.Content
		}
	}

	return SubAgentResult{
		ID:       sa.id,
		Output:   lastOutput,
		Error:    fmt.Errorf("sub-agent reached max turns (%d)", sa.maxTurns),
		Duration: time.Since(startTime),
		Turns:    turns,
	}
}

// getFilteredSchemas returns tool schemas, optionally filtering write tools
func (sa *SubAgent) getFilteredSchemas() []*tool.FunctionSchema {
	allSchemas := sa.registry.GetSchemas()

	if sa.allowWrites {
		return allSchemas
	}

	// Read-only mode: filter out write tools
	writeToolNames := map[string]bool{
		"write_file":    true,
		"edit_file":     true,
		"notebook_edit": true,
	}

	filtered := make([]*tool.FunctionSchema, 0, len(allSchemas))
	for _, schema := range allSchemas {
		if writeToolNames[schema.Name] {
			continue
		}
		filtered = append(filtered, schema)
	}

	return filtered
}

// callLLM calls the LLM provider using the same pattern as the main Agent
func (sa *SubAgent) callLLM(ctx context.Context, messages []map[string]interface{}, tools []*tool.FunctionSchema) (*ChatResponse, error) {
	// Convert messages to llm.Message format
	llmMessages := make([]llm.Message, len(messages))
	for i, msg := range messages {
		llmMessages[i] = llm.Message{
			Role:    msg["role"].(string),
			Content: msg["content"].(string),
			ToolID:  getString(msg, "tool_id"),
		}
	}

	// Build request
	req := &llm.ChatRequest{
		Model:       "",
		Messages:    llmMessages,
		Tools:       convertTools(tools),
		Stream:      false,
		Temperature: config.DefaultTemperature,
	}

	// Call LLM via provider
	resp, err := sa.provider.Chat(ctx, req)
	if err != nil {
		return nil, err
	}

	return parseChatResponse(resp)
}

// executeSubAgentTools executes tool calls and returns results (named differently to avoid conflict with dispatch.go)
func (sa *SubAgent) executeSubAgentTools(ctx context.Context, toolCalls []session.ToolCall) []session.ToolResult {
	results := make([]session.ToolResult, 0, len(toolCalls))

	for _, tc := range toolCalls {
		toolName := tc.Function.Name

		// Block write tools in read-only mode
		if !sa.allowWrites && isWriteTool(toolName) {
			results = append(results, session.ToolResult{
				Content:    "Error: write operations are not allowed in read-only sub-agent mode",
				ToolCallID: tc.ID,
			})
			continue
		}

		// Get tool from registry
		t, ok := sa.registry.GetTool(toolName)
		if !ok {
			results = append(results, session.ToolResult{
				Content:    fmt.Sprintf("Error: unknown tool '%s'", toolName),
				ToolCallID: tc.ID,
			})
			continue
		}

		// Record for loop detection
		sa.loopDetector.RecordToolCall(toolName, tc.Function.Arguments)

		// Execute tool
		toolCtx, cancel := context.WithTimeout(ctx, ToolExecutionTimeout)
		result, err := t.Execute(toolCtx, json.RawMessage(tc.Function.Arguments))
		cancel()

		if err != nil {
			results = append(results, session.ToolResult{
				Content:    fmt.Sprintf("Error executing %s: %v", toolName, err),
				ToolCallID: tc.ID,
			})
			continue
		}

		output := result.Output
		if result.IsError {
			output = fmt.Sprintf("Error: %s", result.Error)
		}

		results = append(results, session.ToolResult{
			Content:    output,
			ToolCallID: tc.ID,
		})
	}

	return results
}

// FormatResults formats multiple sub-agent results into a summary string
func FormatResults(results []SubAgentResult) string {
	var sb strings.Builder

	sb.WriteString("=== Parallel Agent Results ===\n\n")

	for i, result := range results {
		sb.WriteString(fmt.Sprintf("--- Agent %d [%s] ---\n", i+1, result.ID))
		sb.WriteString(fmt.Sprintf("Duration: %s | Turns: %d\n", result.Duration.Round(time.Millisecond), result.Turns))

		if result.Error != nil {
			sb.WriteString(fmt.Sprintf("Status: Error - %v\n", result.Error))
		} else {
			sb.WriteString("Status: Complete\n")
		}

		if result.Output != "" {
			sb.WriteString(fmt.Sprintf("\n%s\n", result.Output))
		}

		sb.WriteString("\n")
	}

	return sb.String()
}
