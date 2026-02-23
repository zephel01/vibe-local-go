package llm

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	baseURL := "http://localhost:11434"
	client := NewClient(baseURL)

	if client == nil {
		t.Fatal("expected client to be non-nil")
	}

	if client.provider == nil {
		t.Error("expected provider to be non-nil")
	}

	// Verify the underlying provider has correct settings
	info := client.provider.Info()
	if info.BaseURL != baseURL {
		t.Errorf("expected baseURL %s, got %s", baseURL, info.BaseURL)
	}
}

func TestClient_SetTimeout(t *testing.T) {
	client := NewClient("http://localhost:11434")
	newTimeout := 60 * time.Second

	client.SetTimeout(newTimeout)

	// Verify via the underlying provider's httpClient
	if client.provider.httpClient.Timeout != newTimeout {
		t.Errorf("expected httpClient.Timeout %v, got %v", newTimeout, client.provider.httpClient.Timeout)
	}
}

func TestChatRequest_Marshal(t *testing.T) {
	req := ChatRequest{
		Model: "llama3",
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		Stream:      true,
		Temperature: 0.7,
		MaxTokens:   1000,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled ChatRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Model != req.Model {
		t.Errorf("expected model %s, got %s", req.Model, unmarshaled.Model)
	}

	if len(unmarshaled.Messages) != len(req.Messages) {
		t.Errorf("expected %d messages, got %d", len(req.Messages), len(unmarshaled.Messages))
	}

	if unmarshaled.Stream != req.Stream {
		t.Errorf("expected stream %v, got %v", req.Stream, unmarshaled.Stream)
	}
}

func TestMessage_Marshal(t *testing.T) {
	msg := Message{
		Role:    "user",
		Content: "Hello, World!",
		ToolCalls: []ToolCall{
			{
				ID:   "call_123",
				Type: "function",
				Function: FunctionCall{
					Name: "search",
					Arguments: json.RawMessage(`{"query":"test"}`),
				},
			},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled Message
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Role != msg.Role {
		t.Errorf("expected role %s, got %s", msg.Role, unmarshaled.Role)
	}

	if unmarshaled.Content != msg.Content {
		t.Errorf("expected content %s, got %s", msg.Content, unmarshaled.Content)
	}

	if len(unmarshaled.ToolCalls) != len(msg.ToolCalls) {
		t.Errorf("expected %d tool calls, got %d", len(msg.ToolCalls), len(unmarshaled.ToolCalls))
	}
}

func TestToolDef_Marshal(t *testing.T) {
	tool := ToolDef{
		Type: "function",
		Function: FunctionDef{
			Name:        "search",
			Description: "Search for information",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Search query",
					},
				},
			},
		},
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled ToolDef
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Type != tool.Type {
		t.Errorf("expected type %s, got %s", tool.Type, unmarshaled.Type)
	}

	if unmarshaled.Function.Name != tool.Function.Name {
		t.Errorf("expected name %s, got %s", tool.Function.Name, unmarshaled.Function.Name)
	}
}

func TestToolChoice_Marshal(t *testing.T) {
	tc := ToolChoice{
		Type: "function",
		Function: struct {
			Name string `json:"name"`
		}{
			Name: "search",
		},
	}

	data, err := json.Marshal(tc)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled ToolChoice
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Type != tc.Type {
		t.Errorf("expected type %s, got %s", tc.Type, unmarshaled.Type)
	}

	if unmarshaled.Function.Name != tc.Function.Name {
		t.Errorf("expected function name %s, got %s", tc.Function.Name, unmarshaled.Function.Name)
	}
}

func TestUsage_Marshal(t *testing.T) {
	usage := Usage{
		PromptTokens:     10,
		CompletionTokens: 20,
		TotalTokens:      30,
	}

	data, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled Usage
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.PromptTokens != usage.PromptTokens {
		t.Errorf("expected prompt tokens %d, got %d", usage.PromptTokens, unmarshaled.PromptTokens)
	}

	if unmarshaled.CompletionTokens != usage.CompletionTokens {
		t.Errorf("expected completion tokens %d, got %d", usage.CompletionTokens, unmarshaled.CompletionTokens)
	}

	if unmarshaled.TotalTokens != usage.TotalTokens {
		t.Errorf("expected total tokens %d, got %d", usage.TotalTokens, unmarshaled.TotalTokens)
	}
}

func TestErrorResponse_Marshal(t *testing.T) {
	errResp := ErrorResponse{
		Error: struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		}{
			Message: "Invalid request",
			Type:    "invalid_request_error",
			Code:    "400",
		},
	}

	data, err := json.Marshal(errResp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled ErrorResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Error.Message != errResp.Error.Message {
		t.Errorf("expected message %s, got %s", errResp.Error.Message, unmarshaled.Error.Message)
	}

	if unmarshaled.Error.Type != errResp.Error.Type {
		t.Errorf("expected type %s, got %s", errResp.Error.Type, unmarshaled.Error.Type)
	}
}

func TestClient_CheckConnection(t *testing.T) {
	// Test with context that will timeout quickly
	client := NewClient("http://localhost:11434")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := client.CheckConnection(ctx)
	// We expect an error since Ollama is not running
	if err == nil {
		t.Log("CheckConnection succeeded (Ollama is running)")
	} else {
		t.Logf("CheckConnection failed as expected: %v", err)
	}
}

func TestClient_CheckModel(t *testing.T) {
	client := NewClient("http://localhost:11434")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.CheckModel(ctx, "llama3")
	// We expect an error or false since Ollama is not running
	if err != nil {
		t.Logf("CheckModel failed as expected: %v", err)
	}
}

func TestChatRequest_WithTools(t *testing.T) {
	req := ChatRequest{
		Model: "llama3",
		Messages: []Message{
			{Role: "user", Content: "Search for information"},
		},
		Tools: []ToolDef{
			{
				Type: "function",
				Function: FunctionDef{
					Name: "search",
					Description: "Search the web",
				},
			},
		},
		ToolChoice: &ToolChoice{
			Type: "auto",
		},
		Stream: false,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled ChatRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(unmarshaled.Tools) != len(req.Tools) {
		t.Errorf("expected %d tools, got %d", len(req.Tools), len(unmarshaled.Tools))
	}

	if unmarshaled.ToolChoice == nil {
		t.Error("expected tool choice to be non-nil")
	}

	if unmarshaled.ToolChoice.Type != req.ToolChoice.Type {
		t.Errorf("expected tool choice type %s, got %s", req.ToolChoice.Type, unmarshaled.ToolChoice.Type)
	}
}

func TestMessage_WithToolID(t *testing.T) {
	msg := Message{
		Role:    "tool",
		Content: "Search result: ...",
		ToolID:  "call_123",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled Message
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.ToolID != msg.ToolID {
		t.Errorf("expected tool ID %s, got %s", msg.ToolID, unmarshaled.ToolID)
	}
}

func TestDelta_Marshal(t *testing.T) {
	delta := Delta{
		Role:    "assistant",
		Content: "Hello",
		ToolCalls: []ToolCall{
			{
				ID:   "call_123",
				Type: "function",
				Function: FunctionCall{
					Name: "search",
				},
			},
		},
	}

	data, err := json.Marshal(delta)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var unmarshaled Delta
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if unmarshaled.Role != delta.Role {
		t.Errorf("expected role %s, got %s", delta.Role, unmarshaled.Role)
	}

	if unmarshaled.Content != delta.Content {
		t.Errorf("expected content %s, got %s", delta.Content, unmarshaled.Content)
	}
}

func TestNewOllamaProvider(t *testing.T) {
	provider := NewOllamaProvider("http://localhost:11434", "qwen3:8b")

	if provider == nil {
		t.Fatal("expected provider to be non-nil")
	}

	info := provider.Info()
	if info.Name != "ollama" {
		t.Errorf("expected name 'ollama', got '%s'", info.Name)
	}
	if info.Type != ProviderTypeLocal {
		t.Errorf("expected type ProviderTypeLocal, got %v", info.Type)
	}
	if info.Model != "qwen3:8b" {
		t.Errorf("expected model 'qwen3:8b', got '%s'", info.Model)
	}
	if !info.Features.ModelManagement {
		t.Error("OllamaProvider should have ModelManagement feature")
	}
}

func TestClientGetProvider(t *testing.T) {
	client := NewClient("http://localhost:11434")

	provider := client.GetProvider()
	if provider == nil {
		t.Error("GetProvider() should return non-nil provider")
	}

	// Provider should implement LLMProvider
	var _ LLMProvider = provider
}
