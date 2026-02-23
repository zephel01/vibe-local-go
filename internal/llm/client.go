package llm

import (
	"context"
	"encoding/json"
	"time"
)

// Client represents an Ollama HTTP client (後方互換ラッパー)
// 内部では OllamaProvider に委譲する
type Client struct {
	provider *OllamaProvider
}

// ChatRequest represents a chat completion request (OpenAI互換)
type ChatRequest struct {
	Model       string                 `json:"model"`
	Messages    []Message              `json:"messages"`
	Tools       []ToolDef              `json:"tools,omitempty"`
	ToolChoice  *ToolChoice            `json:"tool_choice,omitempty"`
	Stream      bool                   `json:"stream"`
	Temperature float64                `json:"temperature,omitempty"`
	MaxTokens   int                    `json:"max_tokens,omitempty"`
	Options     map[string]interface{} `json:"options,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	ToolID    string     `json:"tool_id,omitempty"`
}

// ToolCall represents a tool call request
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall represents a function call within a tool call
type FunctionCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolDef represents a tool definition (OpenAI function calling format)
type ToolDef struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef represents a function definition
type FunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// ToolChoice represents tool choice preferences
type ToolChoice struct {
	Type     string `json:"type"`
	Function struct {
		Name string `json:"name"`
	} `json:"function,omitempty"`
}

// ChatResponse represents a chat completion response
type ChatResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
	Usage   Usage    `json:"usage"`
}

// Choice represents a completion choice
type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
	Delta        Delta   `json:"delta,omitempty"`
}

// Delta represents incremental updates in streaming responses
type Delta struct {
	Role      string     `json:"role,omitempty"`
	Content   string     `json:"content,omitempty"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// Usage represents token usage information
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ErrorResponse represents an error from the LLM API
type ErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// NewClient creates a new Ollama client (後方互換)
func NewClient(baseURL string) *Client {
	return &Client{
		provider: NewOllamaProvider(baseURL, ""),
	}
}

// SetTimeout sets the request timeout
func (c *Client) SetTimeout(timeout time.Duration) {
	c.provider.SetTimeout(timeout)
}

// CheckConnection checks if Ollama is running and accessible
func (c *Client) CheckConnection(ctx context.Context) error {
	return c.provider.CheckHealth(ctx)
}

// CheckModel checks if a specific model is available
func (c *Client) CheckModel(ctx context.Context, modelName string) (bool, error) {
	return c.provider.CheckModel(ctx, modelName)
}

// ListModels returns a list of all available models
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	return c.provider.ListModels(ctx)
}

// PullModel downloads a model from Ollama
func (c *Client) PullModel(ctx context.Context, modelName string) error {
	return c.provider.PullModel(ctx, modelName)
}

// GetProvider returns the underlying OllamaProvider (for LLMProvider interface access)
func (c *Client) GetProvider() *OllamaProvider {
	return c.provider
}
