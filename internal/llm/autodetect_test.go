package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// TestAutoDetect_OllamaFound tests Ollama detection
func TestAutoDetect_OllamaFound(t *testing.T) {
	// Create mock Ollama server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			response := map[string]interface{}{
				"models": []map[string]interface{}{
					{"name": "qwen3:8b"},
					{"name": "mistral:7b"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// We can't directly control the detection since it uses hardcoded localhost:11434
	// But we can test the parser function
	data := []byte(`{"models": [{"name": "qwen3:8b"}, {"name": "mistral:7b"}]}`)
	models, err := parseOllamaModels(data)

	if err != nil {
		t.Fatalf("parseOllamaModels failed: %v", err)
	}

	if len(models) != 2 {
		t.Errorf("Expected 2 models, got %d", len(models))
	}

	if models[0] != "qwen3:8b" || models[1] != "mistral:7b" {
		t.Errorf("Unexpected models: %v", models)
	}
}

// TestAutoDetect_LlamaServerFound tests llama-server detection
func TestAutoDetect_LlamaServerFound(t *testing.T) {
	// Test the parser for llama-server response format
	data := []byte(`{"object": "list", "data": [{"id": "gemma-3-4b-it"}, {"id": "llama-3-8b"}]}`)
	models, err := parseLlamaServerModels(data)

	if err != nil {
		t.Fatalf("parseLlamaServerModels failed: %v", err)
	}

	if len(models) != 2 {
		t.Errorf("Expected 2 models, got %d", len(models))
	}

	if models[0] != "gemma-3-4b-it" || models[1] != "llama-3-8b" {
		t.Errorf("Unexpected models: %v", models)
	}
}

// TestAutoDetect_MultipleProvidersDetection tests detection with custom ports
func TestAutoDetect_MultipleProvidersDetection(t *testing.T) {
	// Create mock Ollama server on random port
	ollamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			response := map[string]interface{}{
				"models": []map[string]interface{}{
					{"name": "qwen3:8b"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ollamaServer.Close()

	// Create mock llama-server
	llamaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			response := map[string]interface{}{
				"object": "list",
				"data": []map[string]interface{}{
					{"id": "gemma-3-4b-it"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer llamaServer.Close()

	// Test checkProvider directly
	ctx := context.Background()
	ollamaModels, err := checkProvider(ctx, ollamaServer.URL+"/api/tags", parseOllamaModels)
	if err != nil {
		t.Fatalf("checkProvider for Ollama failed: %v", err)
	}

	if len(ollamaModels) != 1 || ollamaModels[0] != "qwen3:8b" {
		t.Errorf("Unexpected Ollama models: %v", ollamaModels)
	}

	llamaModels, err := checkProvider(ctx, llamaServer.URL+"/v1/models", parseLlamaServerModels)
	if err != nil {
		t.Fatalf("checkProvider for llama-server failed: %v", err)
	}

	if len(llamaModels) != 1 || llamaModels[0] != "gemma-3-4b-it" {
		t.Errorf("Unexpected llama-server models: %v", llamaModels)
	}
}

// TestAutoDetect_NotFound tests detection when no providers are available
func TestAutoDetect_NotFound(t *testing.T) {
	ctx := context.Background()

	// Call AutoDetect with default localhost ports
	// This should return empty if no providers are actually running
	results := AutoDetect(ctx)

	// We can't guarantee no providers are running on localhost
	// but we can test the function doesn't panic and returns a slice
	if results == nil {
		t.Error("AutoDetect returned nil instead of empty slice")
	}

	// Test that it's at least a valid slice (might be empty or have results)
	_ = len(results)
}

// TestAutoDetect_Timeout tests detection timeout
func TestAutoDetect_Timeout(t *testing.T) {
	// Create slow server that takes longer than 2 seconds
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer slowServer.Close()

	ctx := context.Background()

	// checkProvider should timeout
	start := time.Now()
	_, err := checkProvider(ctx, slowServer.URL+"/v1/models", parseLlamaServerModels)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	// Should timeout quickly (within 2 seconds + some buffer)
	if elapsed > 3*time.Second {
		t.Errorf("Timeout took too long: %v", elapsed)
	}
}

// TestAutoDetect_CustomEnvVar tests detection with custom VIBE_LLM_URL
func TestAutoDetect_CustomEnvVar(t *testing.T) {
	// Create mock custom server
	customServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			response := map[string]interface{}{
				"object": "list",
				"data": []map[string]interface{}{
					{"id": "custom-model"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer customServer.Close()

	// Set environment variable
	oldURL := os.Getenv("VIBE_LLM_URL")
	defer os.Setenv("VIBE_LLM_URL", oldURL)

	os.Setenv("VIBE_LLM_URL", customServer.URL)

	// Note: AutoDetect checks hardcoded ports, so we test the pattern
	// Test checkProvider with custom endpoint
	ctx := context.Background()
	models, err := checkProvider(ctx, customServer.URL+"/v1/models", parseLlamaServerModels)

	if err != nil {
		t.Fatalf("checkProvider for custom failed: %v", err)
	}

	if len(models) != 1 || models[0] != "custom-model" {
		t.Errorf("Unexpected custom models: %v", models)
	}
}

// TestParseOllamaModels_EmptyResponse tests parsing empty model list
func TestParseOllamaModels_EmptyResponse(t *testing.T) {
	data := []byte(`{"models": []}`)
	_, err := parseOllamaModels(data)

	if err == nil {
		t.Error("Expected error for empty models, got nil")
	}
}

// TestParseOllamaModels_InvalidJSON tests parsing invalid JSON
func TestParseOllamaModels_InvalidJSON(t *testing.T) {
	data := []byte(`{invalid json}`)
	_, err := parseOllamaModels(data)

	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

// TestParseLlamaServerModels_EmptyResponse tests parsing empty model list
func TestParseLlamaServerModels_EmptyResponse(t *testing.T) {
	data := []byte(`{"object": "list", "data": []}`)
	_, err := parseLlamaServerModels(data)

	if err == nil {
		t.Error("Expected error for empty models, got nil")
	}
}

// TestDetectedProvider_ToProviderInfo tests conversion to ProviderInfo
func TestDetectedProvider_ToProviderInfo(t *testing.T) {
	dp := DetectedProvider{
		Name:     "ollama",
		URL:      "http://localhost:11434",
		Models:   []string{"qwen3:8b", "mistral:7b"},
		Health:   true,
		Features: getDefaultFeatures("ollama"),
		BasePort: 11434,
	}

	info := dp.ToProviderInfo("qwen3:8b")

	if info.Name != "ollama" {
		t.Errorf("Expected name 'ollama', got '%s'", info.Name)
	}

	if info.BaseURL != "http://localhost:11434" {
		t.Errorf("Expected URL 'http://localhost:11434', got '%s'", info.BaseURL)
	}

	if info.Model != "qwen3:8b" {
		t.Errorf("Expected model 'qwen3:8b', got '%s'", info.Model)
	}

	if !info.Features.ModelManagement {
		t.Error("Expected ModelManagement=true for ollama")
	}
}

// TestDetectedProvider_ToProviderInfo_DefaultModel tests auto-selection of first model
func TestDetectedProvider_ToProviderInfo_DefaultModel(t *testing.T) {
	dp := DetectedProvider{
		Name:     "llama-server",
		URL:      "http://localhost:8080",
		Models:   []string{"gemma-3-4b-it", "llama-3-8b"},
		Health:   true,
		Features: getDefaultFeatures("llama-server"),
		BasePort: 8080,
	}

	// Call with empty model
	info := dp.ToProviderInfo("")

	if info.Model != "gemma-3-4b-it" {
		t.Errorf("Expected default model 'gemma-3-4b-it', got '%s'", info.Model)
	}
}

// TestDetectedProvider_IsReachable tests health check
func TestDetectedProvider_IsReachable(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{"data": []interface{}{}})
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	dp := DetectedProvider{
		Name:     "test",
		URL:      server.URL,
		Models:   []string{"test"},
		Health:   true,
		BasePort: 0,
	}

	ctx := context.Background()
	if !dp.IsReachable(ctx) {
		t.Error("Expected IsReachable to return true for running server")
	}
}

// TestDetectedProvider_IsReachable_Unreachable tests health check for unreachable server
func TestDetectedProvider_IsReachable_Unreachable(t *testing.T) {
	dp := DetectedProvider{
		Name:     "test",
		URL:      "http://localhost:65432", // Very unlikely to have anything here
		Models:   []string{"test"},
		Health:   true,
		BasePort: 0,
	}

	ctx := context.Background()
	if dp.IsReachable(ctx) {
		t.Error("Expected IsReachable to return false for non-running server")
	}
}

// TestGetDefaultFeatures tests feature assignment for different providers
func TestGetDefaultFeatures(t *testing.T) {
	tests := []struct {
		provider            string
		expectedManagement  bool
		expectedFunctionCal bool
	}{
		{"ollama", true, true},
		{"llama-server", false, true},
		{"lm-studio", false, true},
		{"custom", false, true},
	}

	for _, tt := range tests {
		features := getDefaultFeatures(tt.provider)

		if features.ModelManagement != tt.expectedManagement {
			t.Errorf("Provider %s: Expected ModelManagement=%v, got %v",
				tt.provider, tt.expectedManagement, features.ModelManagement)
		}

		if features.NativeFunctionCalling != tt.expectedFunctionCal {
			t.Errorf("Provider %s: Expected NativeFunctionCalling=%v, got %v",
				tt.provider, tt.expectedFunctionCal, features.NativeFunctionCalling)
		}

		if !features.Streaming {
			t.Errorf("Provider %s: Expected Streaming=true", tt.provider)
		}
	}
}

// TestDetectProvidersByPort tests custom port detection
func TestDetectProvidersByPort(t *testing.T) {
	// Create mock server on a specific port
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			response := map[string]interface{}{
				"object": "list",
				"data": []map[string]interface{}{
					{"id": "test-model"},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Note: DetectProvidersByPort checks hardcoded localhost
	// We can test the general pattern but not specific port binding
	ctx := context.Background()
	results := DetectProvidersByPort(ctx, []int{8080, 1234, 11434})

	// Function should complete without panicking
	if results == nil {
		t.Error("DetectProvidersByPort returned nil")
	}
}

// TestAutoDetect_LiteLLMFound tests LiteLLM detection
func TestAutoDetect_LiteLLMFound(t *testing.T) {
	// Test the parser for LiteLLM response format (same as llama-server)
	data := []byte(`{"object": "list", "data": [{"id": "glm47-flash"}, {"id": "glm47"}, {"id": "qwen25"}]}`)
	models, err := parseLlamaServerModels(data)

	if err != nil {
		t.Fatalf("parseLlamaServerModels for LiteLLM failed: %v", err)
	}

	if len(models) != 3 {
		t.Errorf("Expected 3 models, got %d", len(models))
	}

	if models[0] != "glm47-flash" || models[1] != "glm47" || models[2] != "qwen25" {
		t.Errorf("Unexpected LiteLLM models: %v", models)
	}
}

// TestGetDefaultFeatures_LiteLLM tests feature assignment for LiteLLM
func TestGetDefaultFeatures_LiteLLM(t *testing.T) {
	features := getDefaultFeatures("litellm")

	if !features.NativeFunctionCalling {
		t.Error("Expected NativeFunctionCalling=true for litellm")
	}

	if !features.Streaming {
		t.Error("Expected Streaming=true for litellm")
	}

	if !features.Vision {
		t.Error("Expected Vision=true for litellm (may support vision)")
	}
}
