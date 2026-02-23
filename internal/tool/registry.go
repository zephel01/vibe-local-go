package tool

import (
	"context"
	"encoding/json"
	"sync"
)

// Tool represents an executable tool
type Tool interface {
	// Name returns the tool name
	Name() string

	// Execute executes the tool with the given parameters
	Execute(ctx context.Context, params json.RawMessage) (*Result, error)

	// Schema returns the OpenAI function calling schema
	Schema() *FunctionSchema
}

// Registry manages available tools
type Registry struct {
	tools      map[string]Tool
	schemaCache []*FunctionSchema
	mu         sync.RWMutex
}

// NewRegistry creates a new tool registry
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register registers a tool
func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools[tool.Name()] = tool
	r.schemaCache = nil // Invalidate cache
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	return tool, ok
}

// Names returns all tool names
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// GetSchemas returns all tool schemas (OpenAI function calling format)
func (r *Registry) GetSchemas() []*FunctionSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return cached schemas if available
	if r.schemaCache != nil {
		return r.schemaCache
	}

	// Build schema cache
	schemas := make([]*FunctionSchema, 0, len(r.tools))
	for _, tool := range r.tools {
		schemas = append(schemas, tool.Schema())
	}

	// Cache the result
	r.schemaCache = schemas
	return schemas
}

// Count returns the number of registered tools
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.tools)
}

// Result represents the result of a tool execution
type Result struct {
	// Output is the text output of the tool
	Output string `json:"output"`

	// IsError indicates if the result is an error
	IsError bool `json:"is_error"`

	// ToolCallID tracks which tool call this result corresponds to
	ToolCallID string `json:"tool_call_id,omitempty"`

	// Error contains the error message if IsError is true
	Error string `json:"error,omitempty"`
}

// FunctionSchema represents an OpenAI function calling schema
type FunctionSchema struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  *ParameterSchema       `json:"parameters,omitempty"`
}

// ParameterSchema represents function parameters
type ParameterSchema struct {
	Type       string                    `json:"type"`
	Properties map[string]*PropertyDef  `json:"properties,omitempty"`
	Required   []string                  `json:"required,omitempty"`
}

// PropertyDef represents a property definition
type PropertyDef struct {
	Type        string            `json:"type"`
	Description string            `json:"description,omitempty"`
	Enum        []string          `json:"enum,omitempty"`
	Default     interface{}       `json:"default,omitempty"`
	Properties  map[string]*PropertyDef `json:"properties,omitempty"`
	Required    []string          `json:"required,omitempty"`
	Items       *PropertyDef      `json:"items,omitempty"`
}

// NewResult creates a new tool result
func NewResult(output string) *Result {
	return &Result{
		Output:  output,
		IsError: false,
	}
}

// NewErrorResult creates a new error result
func NewErrorResult(err error) *Result {
	return &Result{
		Output:  "",
		IsError: true,
		Error:   err.Error(),
	}
}

// NewResultWithID creates a new result with a tool call ID
func NewResultWithID(toolCallID string, output string) *Result {
	return &Result{
		Output:     output,
		IsError:    false,
		ToolCallID: toolCallID,
	}
}

// NewErrorResultWithID creates a new error result with a tool call ID
func NewErrorResultWithID(toolCallID string, err error) *Result {
	return &Result{
		Output:     "",
		IsError:    true,
		Error:      err.Error(),
		ToolCallID: toolCallID,
	}
}
