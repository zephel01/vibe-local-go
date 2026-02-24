package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
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
	// MaxValidationAttempts is the maximum number of script validation attempts
	MaxValidationAttempts = 3
	// ScriptValidationTimeout is the timeout for script validation
	ScriptValidationTimeout = 30 * time.Second
)

// Agent represents the main agent loop
type Agent struct {
	provider              llm.LLMProvider
	registry              *tool.Registry
	permissionMgr         *security.PermissionManager
	validator             *security.PathValidator
	session               *session.Session
	terminal              *ui.Terminal
	config                *config.Config
	loopDetector          *LoopDetector
	spinner               *ui.ToolSpinner
	statusLine            *ui.StatusLineUpdater
	scriptValidationCount int // Track number of script validation attempts
	autoTestEnabled       bool // Enable automatic test execution after file edits
	planMode              bool // When true, reject write_file/edit_file/bash
}

// NewAgent creates a new agent
func NewAgent(
	provider llm.LLMProvider,
	registry *tool.Registry,
	permissionMgr *security.PermissionManager,
	validator *security.PathValidator,
	sess *session.Session,
	term *ui.Terminal,
	cfg *config.Config,
) *Agent {
	return &Agent{
		provider:        provider,
		registry:        registry,
		permissionMgr:   permissionMgr,
		validator:       validator,
		session:         sess,
		terminal:        term,
		config:          cfg,
		loopDetector:    NewLoopDetector(),
		spinner:         ui.NewToolSpinner(term),
		statusLine:      ui.NewStatusLineUpdater(term),
		autoTestEnabled: false, // Disabled by default, enable with /autotest on
		planMode:        false, // Disabled by default, enable with /plan on
	}
}

// SetAutoTestEnabled sets whether auto test is enabled
func (a *Agent) SetAutoTestEnabled(enabled bool) {
	a.autoTestEnabled = enabled
}

// IsAutoTestEnabled returns whether auto test is enabled
func (a *Agent) IsAutoTestEnabled() bool {
	return a.autoTestEnabled
}

// SetPlanMode sets whether plan mode is enabled (write operations disabled)
func (a *Agent) SetPlanMode(enabled bool) {
	a.planMode = enabled
}

// IsPlanMode returns whether plan mode is enabled
func (a *Agent) IsPlanMode() bool {
	return a.planMode
}

// Run executes the agent loop
func (a *Agent) Run(ctx context.Context, userInput string) error {
	// Reset loop detector and validation counter for each new user request
	// This ensures loop detection and validation tracking only apply within a single request
	a.loopDetector.Reset()
	a.scriptValidationCount = 0

	// Add user input to session
	a.session.AddUserMessage(userInput)

	// ReAct loop
	iteration := 0
	for iteration < MaxIterations {
		select {
		case <-ctx.Done():
			// Context cancelled (ESC/Ctrl+C)
			a.terminal.PrintWarning("Agent execution interrupted")
			return ctx.Err()
		default:
		}

		iteration++

		// Check for loop (only for actual tool execution, not validation)
		// Skip loop detection if we're in validation phase
		if a.scriptValidationCount == 0 && a.loopDetector.DetectLoop() {
			a.terminal.PrintWarning("Detected repeated tool calls, stopping to prevent infinite loop")
			break
		}

		// Prepare chat request
		messages := a.session.GetMessagesForLLM()
		tools := a.registry.GetSchemas()

		// Call LLM (ステータス行表示)
		a.statusLine.Start("💭 Thinking...")
		response, err := a.callLLM(ctx, messages, tools)
		a.statusLine.Stop()
		if err != nil {
			return fmt.Errorf("LLM call failed: %w", err)
		}

		// Update status line with token count
		if response.PromptTokens > 0 || response.CompletionTokens > 0 {
			a.statusLine.SetTokenCount(response.PromptTokens + response.CompletionTokens)
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

		// Script validation phase: Check if any write_file tool was called
		validationErr := a.validateGeneratedScripts(ctx, response.ToolCalls)
		if validationErr != nil {
			// Increment validation counter
			a.scriptValidationCount++

			// Check if we've exceeded max validation attempts
			if a.scriptValidationCount >= MaxValidationAttempts {
				// Exceeded max attempts - inform user but continue gracefully
				errorMsg := fmt.Sprintf(
					"Script validation failed after %d attempts. The script may need manual adjustment. Please review the error and either fix it yourself or ask for a different approach.",
					MaxValidationAttempts,
				)
				a.terminal.PrintError(errorMsg)
				a.session.AddAssistantMessage(errorMsg)
				// Reset counter and break to exit loop gracefully
				a.scriptValidationCount = 0
				// Reset loop detector to avoid false positives
				a.loopDetector.Reset()
				break
			}

			// Add validation error to session so LLM can see it and attempt to fix
			a.session.AddToolResults([]session.ToolResult{
				{
					Content:    validationErr.Error(),
					ToolCallID: "",
				},
			})
			a.terminal.PrintError(validationErr.Error())

			// Continue loop: LLM will see validation error and attempt fix
			continue
		}

		// Reset validation counter on success
		a.scriptValidationCount = 0

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

	// Call LLM via provider
	resp, err := a.provider.Chat(ctx, req)
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

	// Check plan mode first (before permission check)
	if a.planMode {
		writeTools := map[string]bool{
			"write_file": true,
			"edit_file":  true,
			"bash":       true,
		}
		if writeTools[toolName] {
			return ToolResult{
				ToolCallID: toolCall.ID,
				IsSuccess:   false,
				Error:       fmt.Sprintf("Cannot execute %s in plan mode. Use '/plan off' to allow modifications.", toolName),
			}
		}
	}

	// Get tool
	toolCfg, exists := a.registry.Get(toolName)
	if !exists {
		return ToolResult{
			ToolCallID: toolCall.ID,
			IsSuccess:   false,
			Error:       fmt.Sprintf("Tool not found: %s", toolName),
		}
	}
	toolInst := toolCfg.Tool

	// Check permission
	allowed, reason, err := a.permissionMgr.CheckPermission(toolName, nil)
	if err != nil {
		a.LogToolError(toolName, err, arguments, 0)
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
			a.LogToolError(toolName, err, arguments, 0)
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
		// Enhanced error logging
		a.LogToolError(toolName, err, arguments, 0)

		// Handle based on tool category
		if toolCfg.Metadata != nil {
			switch toolCfg.Metadata.Category {
			case tool.ToolCategoryOptional:
				a.terminal.PrintWarning(fmt.Sprintf("⚠️ Optional tool %s failed, continuing: %v", toolName, err))
				return ToolResult{
					ToolCallID: toolCall.ID,
					IsSuccess:   true,
					Content:     fmt.Sprintf("// Tool %s unavailable: %v", toolName, err),
					Error:       "",
				}
			case tool.ToolCategoryEnhancing:
				a.terminal.PrintWarning(fmt.Sprintf("⚠️ Enhancing tool %s failed, using fallback", toolName))
				return ToolResult{
					ToolCallID: toolCall.ID,
					IsSuccess:   true,
					Content:     a.getFallbackResult(toolName),
					Error:       fmt.Sprintf("Tool %s failed (using fallback)", toolName),
				}
			case tool.ToolCategoryEssential:
				return ToolResult{
					ToolCallID: toolCall.ID,
					IsSuccess:   false,
					Error:       err.Error(),
				}
			}
		}

		// Default behavior for tools without category
		return ToolResult{
			ToolCallID: toolCall.ID,
			IsSuccess:   false,
			Error:       err.Error(),
		}
	}

	// Show tool result
	a.terminal.ShowToolResult(toolResult)

	// Run auto test if enabled and this is a file write operation
	if a.autoTestEnabled && (toolName == "write_file" || toolName == "edit_file") && !toolResult.IsError {
		// Extract file path from arguments
		var args map[string]interface{}
		if err := json.Unmarshal(json.RawMessage(arguments), &args); err == nil {
			if filePath, ok := args["path"].(string); ok {
				a.terminal.Println("🔄 Running auto tests...")
				if !a.runAutoTestIfNeeded(filePath) {
					// Tests failed - the error has been added to session
					a.terminal.PrintWarning("⚠️  Auto tests failed - LLM will attempt to fix")
				}
			}
		}
	}

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

// LogToolError logs detailed error information
func (a *Agent) LogToolError(toolName string, err error, args string, attempt int) {
	errorMsg := fmt.Sprintf(`
⚠️ Tool Error Details
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Tool:        %s
Attempt:     %d
Error:       %s
Error Type:  %T
Args:        %s
Timestamp:   %s
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━`,
		toolName,
		attempt,
		err.Error(),
		err,
		args,
		time.Now().Format(time.RFC3339),
	)

	a.terminal.PrintError(errorMsg)
}

// getFallbackResult returns a fallback result for a failed tool
func (a *Agent) getFallbackResult(toolName string) string {
	switch toolName {
	case "web_search":
		return "[]\n// Note: web_search unavailable - search the web manually if needed"

	case "web_fetch":
		return "// Note: web_fetch unavailable - content could not be retrieved"

	case "glob":
		return "[]\n// Note: glob returned no results (fallback)"

	case "grep":
		return "// Note: grep returned no matches (fallback)"

	case "bash":
		return "// Command execution failed (fallback - may need manual execution)"

	default:
		return fmt.Sprintf("// Tool %s unavailable (fallback)", toolName)
	}
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

// validateGeneratedScripts validates any scripts generated by write_file tools
func (a *Agent) validateGeneratedScripts(ctx context.Context, toolCalls []session.ToolCall) error {
	// Only validate the most recent write_file call to avoid repeated validation
	var lastWriteFileCall *session.ToolCall
	for i := len(toolCalls) - 1; i >= 0; i-- {
		if toolCalls[i].Function.Name == "write_file" {
			lastWriteFileCall = &toolCalls[i]
			break
		}
	}

	// No write_file call found, nothing to validate
	if lastWriteFileCall == nil {
		return nil
	}

	// Extract file path from arguments
	var args struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(json.RawMessage(lastWriteFileCall.Function.Arguments), &args); err != nil {
		// Cannot extract file path, skip validation
		return nil
	}

	// Check if it's a script file
	ext := strings.ToLower(filepath.Ext(args.FilePath))
	if !isScriptExtension[ext] {
		// Not a script file, no validation needed
		return nil
	}

	// Generate test input
	testInput := GenerateTestInput(args.FilePath)

	// Validate the script
	result, err := ValidateGeneratedScript(ctx, args.FilePath, testInput, ScriptValidationTimeout)
	if err != nil {
		return fmt.Errorf("script validation error for %s: %v", args.FilePath, err)
	}

	if !result.IsSuccess {
		// Build detailed error message for LLM
		errorMsg := fmt.Sprintf(
			"Script validation failed for %s:\n"+
				"Error Type: %s\n"+
				"Error Message: %s\n"+
				"Stderr: %s\n"+
				"Suggestion: %s\n"+
				"Please fix the script and try again.",
			args.FilePath,
			result.ErrorType,
			result.ErrorMessage,
			result.StdErr,
			result.Suggestion,
		)

		// Display validation failure
		a.terminal.PrintError(fmt.Sprintf("✗ Script validation failed for %s", args.FilePath))
		a.terminal.Println(fmt.Sprintf("  Error: %s", result.ErrorType))
		a.terminal.Println(fmt.Sprintf("  Suggestion: %s", result.Suggestion))

		return fmt.Errorf("%s", errorMsg)
	}

	// Success
	a.terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ Script validation passed for %s\n", args.FilePath))
	return nil
}
