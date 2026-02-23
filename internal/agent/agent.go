package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zephel01/vibe-local-go/internal/config"
	"github.com/zephel01/vibe-local-go/internal/llm"
	"github.com/zephel01/vibe-local-go/internal/security"
	"github.com/zephel01/vibe-local-go/internal/session"
	"github.com/zephel01/vibe-local-go/internal/tool"
	"github.com/zephel01/vibe-local-go/internal/ui"
)

const (
	// MaxIterations is the maximum number of agent iterations
	MaxIterations = 50
	// MaxRetries is the maximum number of retries for failed tool calls
	MaxRetries = 2
	// ToolExecutionTimeout is the timeout for tool execution
	ToolExecutionTimeout = 30 * time.Second
)

// Agent represents the main agent loop
type Agent struct {
	client         *llm.Client
	registry       *tool.Registry
	permissionMgr  *security.PermissionManager
	validator      *security.PathValidator
	session        *session.Session
	terminal       *ui.Terminal
	config         *config.Config
	loopDetector   *LoopDetector
	spinner        *ui.ToolSpinner
}

// NewAgent creates a new agent
func NewAgent(
	client *llm.Client,
	registry *tool.Registry,
	permissionMgr *security.PermissionManager,
	validator *security.PathValidator,
	sess *session.Session,
	term *ui.Terminal,
	cfg *config.Config,
) *Agent {
	return &Agent{
		client:        client,
		registry:      registry,
		permissionMgr: permissionMgr,
		validator:     validator,
		session:       sess,
		terminal:      term,
		config:        cfg,
		loopDetector:  NewLoopDetector(),
		spinner:       ui.NewToolSpinner(term),
	}
}

// Run executes the agent loop
func (a *Agent) Run(ctx context.Context, userInput string) error {
	// Add user input to session
	a.session.AddUserMessage(userInput)

	// ReAct loop
	iteration := 0
	for iteration < MaxIterations {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		iteration++

		// Check for loop
		if a.loopDetector.DetectLoop() {
			a.terminal.PrintWarning("Detected repeated tool calls, stopping to prevent infinite loop")
			break
		}

		// Prepare chat request
		messages := a.session.GetMessagesForLLM()
		tools := a.registry.GetSchemas()

		// Call LLM (スピナー表示)
		a.spinner.Start("🧠 Thinking...")
		response, err := a.callLLM(ctx, messages, tools)
		a.spinner.Stop()
		if err != nil {
			return fmt.Errorf("LLM call failed: %w", err)
		}

		// トークン使用量を表示（Python版準拠）
		a.terminal.ShowTokenUsage(response.PromptTokens, response.CompletionTokens, a.config.ContextWindow)

		// Check for tool calls
		if len(response.ToolCalls) == 0 {
			// No tool calls, just assistant response
			a.session.AddAssistantMessage(response.Content)
			a.terminal.Println(response.Content)
			break
		}

		// Add assistant message with tool calls
		a.session.AddToolCall(response.ToolCalls)

		// Execute tools
		results, agentResults, err := a.executeToolCallsWithResults(ctx, response.ToolCalls)
		if err != nil {
			return fmt.Errorf("tool execution failed: %w", err)
		}

		// Add tool results to session
		a.session.AddToolResults(results)

		// If all tools succeeded, continue loop
		allSuccess := true
		for _, result := range agentResults {
			if !result.IsSuccess {
				allSuccess = false
				break
			}
		}

		if !allSuccess {
			// Inform LLM about failures
			errorMsg := "Some tool calls failed. Please review the error messages and try again."
			a.session.AddAssistantMessage(errorMsg)
			a.terminal.PrintError(errorMsg)
		}
	}

	return nil
}

// callLLM calls the LLM with the current messages
func (a *Agent) callLLM(ctx context.Context, messages []map[string]interface{}, tools []*tool.FunctionSchema) (*ChatResponse, error) {
	// Convert messages to llm.Message format
	llmMessages := make([]llm.Message, len(messages))
	for i, msg := range messages {
		llmMessages[i] = llm.Message{
			Role:      msg["role"].(string),
			Content:   msg["content"].(string),
			ToolID:    getString(msg, "tool_id"),
		}
	}

	// Build request
	req := &llm.ChatRequest{
		Model:       a.config.Model,
		Messages:    llmMessages,
		Tools:       convertTools(tools),
		Stream:      false,
		Temperature: 0.7,
		MaxTokens:   a.config.MaxTokens,
	}

	// Call LLM
	resp, err := a.client.ChatSync(ctx, req)
	if err != nil {
		return nil, err
	}

	// Parse response
	return parseChatResponse(resp)
}

// executeToolCalls executes tool calls
func (a *Agent) executeToolCalls(ctx context.Context, toolCalls []session.ToolCall) ([]session.ToolResult, error) {
	sessionResults := make([]session.ToolResult, 0, len(toolCalls))

	for _, tc := range toolCalls {
		result := a.executeSingleTool(ctx, &tc)
		sessionResults = append(sessionResults, session.ToolResult{
			Content:   result.Content,
			ToolCallID: result.ToolCallID,
		})

		// Track tool calls for loop detection
		a.loopDetector.RecordToolCall(tc.Function.Name, tc.Function.Arguments)
	}

	return sessionResults, nil
}

// executeToolCallsWithResults executes tool calls and returns agent ToolResults for error checking
func (a *Agent) executeToolCallsWithResults(ctx context.Context, toolCalls []session.ToolCall) ([]session.ToolResult, []ToolResult, error) {
	sessionResults := make([]session.ToolResult, 0, len(toolCalls))
	agentResults := make([]ToolResult, 0, len(toolCalls))

	for _, tc := range toolCalls {
		result := a.executeSingleTool(ctx, &tc)
		sessionResults = append(sessionResults, session.ToolResult{
			Content:   result.Content,
			ToolCallID: result.ToolCallID,
		})
		agentResults = append(agentResults, result)

		// Track tool calls for loop detection
		a.loopDetector.RecordToolCall(tc.Function.Name, tc.Function.Arguments)
	}

	return sessionResults, agentResults, nil
}

// executeSingleTool executes a single tool
func (a *Agent) executeSingleTool(ctx context.Context, toolCall *session.ToolCall) ToolResult {
	toolName := toolCall.Function.Name
	arguments := toolCall.Function.Arguments

	// Get tool
	toolInst, exists := a.registry.Get(toolName)
	if !exists {
		return ToolResult{
			ToolCallID: toolCall.ID,
			IsSuccess:   false,
			Error:       fmt.Sprintf("Tool not found: %s", toolName),
		}
	}

	// Check permission
	allowed, reason, err := a.permissionMgr.CheckPermission(toolName, nil)
	if err != nil {
		return ToolResult{
			ToolCallID: toolCall.ID,
			IsSuccess:   false,
			Error:       fmt.Sprintf("Permission error: %v", err),
		}
	}

	if !allowed {
		// Ask user
		a.terminal.Printf("Tool: %s (Reason: %s)\n", toolName, reason)
		allowed, err = a.askUserPermission(toolName, arguments)
		if err != nil {
			return ToolResult{
				ToolCallID: toolCall.ID,
				IsSuccess:   false,
				Error:       fmt.Sprintf("Permission denied: %v", err),
			}
		}
		if !allowed {
			return ToolResult{
				ToolCallID: toolCall.ID,
				IsSuccess:   false,
				Error:       "User denied permission",
			}
		}
	}

	// Show tool call
	a.terminal.ShowToolCall(toolName, json.RawMessage(arguments))

	// Execute tool with timeout (スピナー表示)
	ctx, cancel := context.WithTimeout(ctx, ToolExecutionTimeout)
	defer cancel()

	a.spinner.Start(fmt.Sprintf("⚡ %s...", toolName))
	toolResult, err := toolInst.Execute(ctx, json.RawMessage(arguments))
	a.spinner.Stop()
	if err != nil {
		return ToolResult{
			ToolCallID: toolCall.ID,
			IsSuccess:   false,
			Error:       err.Error(),
		}
	}

	// Show tool result
	a.terminal.ShowToolResult(toolResult)

	return ToolResult{
		ToolCallID: toolCall.ID,
		IsSuccess:   !toolResult.IsError,
		Content:     toolResult.Output,
		Error:       toolResult.Error,
	}
}

// askUserPermission asks user for permission
func (a *Agent) askUserPermission(toolName string, arguments string) (bool, error) {
	if a.config.AutoApprove {
		return true, nil
	}

	permResult, err := a.terminal.AskPermission(toolName, arguments)
	if err != nil {
		return false, err
	}

	// Save permission if always/deny
	if permResult.Remember == ui.PermissionAlways ||
		permResult.Remember == ui.PermissionDeny {
		secPermType := security.PermissionType(permResult.Remember)
		if err := a.permissionMgr.SetPermission(toolName, secPermType); err != nil {
			a.terminal.Printf("Warning: failed to save permission: %v\n", err)
		}
	}

	return permResult.Allowed, nil
}

// ChatResponse represents a chat response
type ChatResponse struct {
	Content          string
	ToolCalls        []session.ToolCall
	PromptTokens     int
	CompletionTokens int
}

// normalizeJSONArgs normalizes tool call arguments to a valid JSON object string.
// Handles multiple levels of JSON string encoding that some LLMs produce.
// e.g., `{"command":"ls"}` → `{"command":"ls"}` (already correct)
// e.g., `"{\"command\":\"ls\"}"` → `{"command":"ls"}` (single string encoding)
// e.g., `"\"{\\\"command\\\":\\\"ls\\\"}\""` → `{"command":"ls"}` (double string encoding)
func normalizeJSONArgs(raw json.RawMessage) string {
	data := string(raw)

	// Try to unwrap string encoding layers (max 3 levels)
	for i := 0; i < 3; i++ {
		// Check if it's already a valid JSON object
		trimmed := strings.TrimSpace(data)
		if len(trimmed) > 0 && trimmed[0] == '{' {
			// Validate it's proper JSON
			var obj map[string]interface{}
			if err := json.Unmarshal([]byte(trimmed), &obj); err == nil {
				// Re-marshal to normalize (clean up any encoding artifacts)
				argsBytes, _ := json.Marshal(obj)
				return string(argsBytes)
			}
		}

		// Try to unquote one layer of JSON string encoding
		var unquoted string
		if err := json.Unmarshal([]byte(data), &unquoted); err == nil {
			data = unquoted
			continue
		}

		// Can't unquote further, return what we have
		break
	}

	// Return the best we got
	return data
}

// parseChatResponse parses LLM response
func parseChatResponse(resp *llm.ChatResponse) (*ChatResponse, error) {
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := resp.Choices[0]

	result := &ChatResponse{
		Content:          choice.Message.Content,
		ToolCalls:        make([]session.ToolCall, 0),
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
	}

	// Parse tool calls from message
	for _, tc := range choice.Message.ToolCalls {
		argsStr := normalizeJSONArgs(tc.Function.Arguments)

		result.ToolCalls = append(result.ToolCalls, session.ToolCall{
			ID:   tc.ID,
			Type:  tc.Type,
			Function: session.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: argsStr,
			},
		})
	}

	return result, nil
}

// ToolResult represents a tool execution result
type ToolResult struct {
	ToolCallID string
	IsSuccess   bool
	Content     string
	Error       string
}

// convertTools converts tool schemas to LLM format
func convertTools(schemas []*tool.FunctionSchema) []llm.ToolDef {
	tools := make([]llm.ToolDef, 0, len(schemas))
	for _, schema := range schemas {
		params := make(map[string]interface{})
		if schema.Parameters != nil {
			params = convertParameterSchema(schema.Parameters)
		}
		tools = append(tools, llm.ToolDef{
			Type: "function",
			Function: llm.FunctionDef{
				Name:        schema.Name,
				Description: schema.Description,
				Parameters:  params,
			},
		})
	}
	return tools
}

// HandleToolCallError handles tool call errors
func (a *Agent) HandleToolCallError(toolName string, err error) {
	errorMsg := fmt.Sprintf("Tool execution failed for %s: %v", toolName, err)
	a.terminal.PrintError(errorMsg)
	a.session.AddAssistantMessage(errorMsg)
}

// ExtractToolCallsFromXML extracts tool calls from XML-formatted text
func (a *Agent) ExtractToolCallsFromXML(text string, knownTools []string) ([]session.ToolCall, error) {
	toolCalls, err := llm.ExtractToolCallsFromText(text, knownTools)
	if err != nil {
		return nil, err
	}

	// Convert to session tool calls
	sessionToolCalls := make([]session.ToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		argsStr := normalizeJSONArgs(tc.Function.Arguments)

		sessionToolCalls = append(sessionToolCalls, session.ToolCall{
			ID:   tc.ID,
			Type:  tc.Type,
			Function: session.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: argsStr,
			},
		})
	}

	return sessionToolCalls, nil
}

// RecoverMalformedJSON attempts to recover malformed JSON
func RecoverMalformedJSON(jsonStr string) (string, error) {
	// Remove trailing commas
	jsonStr = strings.TrimRight(jsonStr, ",")

	// Balance brackets
	openBraces := strings.Count(jsonStr, "{")
	closeBraces := strings.Count(jsonStr, "}")
	for i := 0; i < openBraces-closeBraces; i++ {
		jsonStr += "}"
	}

	openBrackets := strings.Count(jsonStr, "[")
	closeBrackets := strings.Count(jsonStr, "]")
	for i := 0; i < openBrackets-closeBrackets; i++ {
		jsonStr += "]"
	}

	return jsonStr, nil
}

// GetIterationCount returns the current iteration count
func (a *Agent) GetIterationCount() int {
	return 0 // This would be tracked in the Run method
}

// Stop stops the agent
func (a *Agent) Stop() {
	// Cancel any ongoing operations
}

// GetStatus returns agent status
func (a *Agent) GetStatus() string {
	return "running"
}

// CompactSession compacts the session if needed
func (a *Agent) CompactSession() *session.CompactionResult {
	return a.session.CompactIfNeeded()
}

// GetSession returns the current session
func (a *Agent) GetSession() *session.Session {
	return a.session
}

// Helper function to safely get string from map
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// convertParameterSchema converts tool.ParameterSchema to map[string]interface{}
func convertParameterSchema(schema *tool.ParameterSchema) map[string]interface{} {
	result := make(map[string]interface{})
	result["type"] = schema.Type

	if schema.Required != nil {
		result["required"] = schema.Required
	}

	if len(schema.Properties) > 0 {
		props := make(map[string]interface{})
		for key, prop := range schema.Properties {
			props[key] = convertPropertyDef(prop)
		}
		result["properties"] = props
	}

	return result
}

// convertPropertyDef converts tool.PropertyDef to map[string]interface{}
func convertPropertyDef(prop *tool.PropertyDef) map[string]interface{} {
	result := make(map[string]interface{})
	result["type"] = prop.Type

	if prop.Description != "" {
		result["description"] = prop.Description
	}

	if len(prop.Enum) > 0 {
		result["enum"] = prop.Enum
	}

	if prop.Default != nil {
		result["default"] = prop.Default
	}

	if len(prop.Properties) > 0 {
		props := make(map[string]interface{})
		for key, subProp := range prop.Properties {
			props[key] = convertPropertyDef(subProp)
		}
		result["properties"] = props
	}

	if len(prop.Required) > 0 {
		result["required"] = prop.Required
	}

	if prop.Items != nil {
		result["items"] = convertPropertyDef(prop.Items)
	}

	return result
}

// UpdateSystemPrompt updates the system prompt
func (a *Agent) UpdateSystemPrompt(prompt string) {
	a.session.SetSystemPrompt(prompt)
}

// Clear clears the session
func (a *Agent) Clear() {
	a.session.Clear()
	a.loopDetector.Reset()
}

// GetContextUsagePercent コンテキスト使用率を取得 (0-100)
func (a *Agent) GetContextUsagePercent() int {
	tokenCount := a.session.GetTokenCount()
	contextWindow := a.session.GetContextWindow()
	if contextWindow == 0 {
		return 0
	}
	pct := int(float64(tokenCount) / float64(contextWindow) * 100)
	if pct > 100 {
		pct = 100
	}
	return pct
}
