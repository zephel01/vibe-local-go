package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

// DetectedProvider represents a detected LLM provider
type DetectedProvider struct {
	Name     string     `json:"name"`      // "ollama", "llama-server", "lm-studio"
	URL      string     `json:"url"`       // "http://localhost:11434"
	Models   []string   `json:"models"`    // ["qwen3:8b", "mistral:7b"]
	Health   bool       `json:"health"`    // Detection successful
	Features Features   `json:"features"`  // Supported features
	BasePort int        `json:"-"`         // Port for detection (not serialized)
}

// AutoDetect detects available LLM providers on localhost
// Returns a list of detected providers (empty if none found)
func AutoDetect(ctx context.Context) []DetectedProvider {
	// Create context with 2-second timeout
	detectCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	resultChan := make(chan DetectedProvider)

	// Providers to check with their ports and endpoints
	providers := []struct {
		name     string
		port     int
		endpoint string
		parser   func([]byte) ([]string, error)
	}{
		{
			name:     "ollama",
			port:     11434,
			endpoint: "/api/tags",
			parser:   parseOllamaModels,
		},
		{
			name:     "llama-server",
			port:     8080,
			endpoint: "/v1/models",
			parser:   parseLlamaServerModels,
		},
		{
			name:     "lm-studio",
			port:     1234,
			endpoint: "/api/v1/models", // LM Studio Native REST API (0.4.0+)
			parser:   parseLlamaServerModels,
		},
		{
			name:     "litellm",
			port:     4000,
			endpoint: "/v1/models",
			parser:   parseLlamaServerModels,
		},
	}

	// Check each provider in parallel
	for _, p := range providers {
		wg.Add(1)
		go func(name string, port int, endpoint string, parser func([]byte) ([]string, error)) {
			defer wg.Done()

			url := fmt.Sprintf("http://localhost:%d%s", port, endpoint)
			models, err := checkProvider(detectCtx, url, parser)
			if err == nil {
				resultChan <- DetectedProvider{
					Name:     name,
					URL:      fmt.Sprintf("http://localhost:%d", port),
					Models:   models,
					Health:   true,
					Features: getDefaultFeatures(name),
					BasePort: port,
				}
			}
		}(p.name, p.port, p.endpoint, p.parser)
	}

	// Check custom provider from environment variable
	customURL := os.Getenv("VIBE_LLM_URL")
	if customURL != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Try to detect custom provider (assume OpenAI-compatible API)
			// normalizeBaseURL でベースURLから /v1 などを除去してから付け直す
			baseURL := normalizeBaseURL(customURL)
			url := baseURL + "/v1/models"
			models, err := checkProvider(detectCtx, url, parseLlamaServerModels)
			if err == nil {
				resultChan <- DetectedProvider{
					Name:     "custom",
					URL:      baseURL,
					Models:   models,
					Health:   true,
					Features: getDefaultFeatures("custom"),
					BasePort: 0,
				}
			}
		}()
	}

	// Wait for all checks to complete in a goroutine
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	var results []DetectedProvider
	for result := range resultChan {
		results = append(results, result)
	}

	// Sort by provider priority (Ollama → llama-server → lm-studio → custom)
	sort.Slice(results, func(i, j int) bool {
		priority := map[string]int{
			"ollama":        0,
			"llama-server":  1,
			"lm-studio":     2,
			"custom":        3,
		}
		return priority[results[i].Name] < priority[results[j].Name]
	})

	return results
}

// checkProvider performs a health check on a provider endpoint
func checkProvider(ctx context.Context, url string, modelParser func([]byte) ([]string, error)) ([]string, error) {
	// Create HTTP client with short timeout
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "vibe-local-go/autodetect")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Read response body (limit to 5MB)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024))
	if err != nil {
		return nil, err
	}

	// Parse models from response
	models, err := modelParser(body)
	if err != nil {
		return nil, err
	}

	return models, nil
}

// parseOllamaModels extracts model names from Ollama /api/tags response
// Response format: {"models": [{"name": "qwen3:8b", ...}, ...]}
func parseOllamaModels(data []byte) ([]string, error) {
	var response struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}

	if len(response.Models) == 0 {
		return nil, fmt.Errorf("no models found")
	}

	models := make([]string, len(response.Models))
	for i, m := range response.Models {
		models[i] = m.Name
	}

	return models, nil
}

// parseLlamaServerModels extracts model names from llama-server /v1/models response
// Response format: {"object": "list", "data": [{"id": "model-name", ...}, ...]}
func parseLlamaServerModels(data []byte) ([]string, error) {
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(data, &response); err != nil {
		return nil, err
	}

	if len(response.Data) == 0 {
		return nil, fmt.Errorf("no models found")
	}

	models := make([]string, len(response.Data))
	for i, d := range response.Data {
		models[i] = d.ID
	}

	return models, nil
}

// getDefaultFeatures returns default features for a provider type
func getDefaultFeatures(providerName string) Features {
	// All local providers support native function calling and streaming
	features := Features{
		NativeFunctionCalling: true,
		ModelManagement:       false,
		Streaming:             true,
	}

	// Only Ollama has model management
	if providerName == "ollama" {
		features.ModelManagement = true
	}

	// LiteLLM supports most features (same as other OpenAI-compatible servers)
	if providerName == "litellm" {
		features.NativeFunctionCalling = true
		features.Streaming = true
	}

	return features
}

// ProviderToInfo converts a DetectedProvider to ProviderInfo
func (dp *DetectedProvider) ToProviderInfo(model string) ProviderInfo {
	// Determine ProviderType
	var providerType ProviderType
	switch dp.Name {
	case "ollama":
		providerType = ProviderTypeLocal
	case "llama-server", "lm-studio":
		providerType = ProviderTypeLocal
	case "custom":
		providerType = ProviderTypeLocal
	default:
		providerType = ProviderTypeLocal
	}

	// Use provided model or first available
	selectedModel := model
	if selectedModel == "" && len(dp.Models) > 0 {
		selectedModel = dp.Models[0]
	}

	return ProviderInfo{
		Name:     dp.Name,
		Type:     providerType,
		BaseURL:  dp.URL,
		Model:    selectedModel,
		Features: dp.Features,
	}
}

// IsReachable checks if a detected provider is still reachable
func (dp *DetectedProvider) IsReachable(ctx context.Context) bool {
	// Create context with short timeout
	checkCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	// Create HTTP client
	client := &http.Client{
		Timeout: 1 * time.Second,
	}

	// Determine endpoint based on provider
	endpoint := "/v1/models"
	switch dp.Name {
	case "ollama":
		endpoint = "/api/tags"
	case "lm-studio":
		endpoint = "/api/v1/models" // LM Studio Native REST API
	}

	// normalizeBaseURL で /v1 が二重にならないよう正規化
	url := normalizeBaseURL(dp.URL) + endpoint

	// Create request
	req, err := http.NewRequestWithContext(checkCtx, "GET", url, nil)
	if err != nil {
		return false
	}

	req.Header.Set("User-Agent", "vibe-local-go/healthcheck")

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Check status code
	return resp.StatusCode == http.StatusOK
}

// DetectProvidersByPort checks specific ports for LLM servers
// Useful for configuration when default ports are changed
func DetectProvidersByPort(ctx context.Context, ports []int) []DetectedProvider {
	// Create context with timeout
	detectCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	resultChan := make(chan DetectedProvider)

	// Common endpoints to try
	endpoints := []struct {
		path   string
		parser func([]byte) ([]string, error)
	}{
		{"/api/tags", parseOllamaModels},           // Ollama
		{"/api/v1/models", parseLlamaServerModels}, // LM Studio Native REST API (0.4.0+)
		{"/v1/models", parseLlamaServerModels},     // llama-server (OpenAI-compat)
	}

	// Check each port with each endpoint
	for _, port := range ports {
		for _, ep := range endpoints {
			wg.Add(1)
			go func(p int, endpoint string, parser func([]byte) ([]string, error)) {
				defer wg.Done()

				url := fmt.Sprintf("http://localhost:%d%s", p, endpoint)
				models, err := checkProvider(detectCtx, url, parser)
				if err == nil {
					// Determine provider name
					var name string
					switch endpoint {
					case "/api/tags":
						name = "ollama"
					case "/api/v1/models":
						name = "lm-studio"
					case "/v1/models":
						if p == 1234 {
							name = "lm-studio"
						} else {
							name = "llama-server"
						}
					default:
						name = "unknown"
					}

					resultChan <- DetectedProvider{
						Name:     name,
						URL:      fmt.Sprintf("http://localhost:%d", p),
						Models:   models,
						Health:   true,
						Features: getDefaultFeatures(name),
						BasePort: p,
					}
				}
			}(port, ep.path, ep.parser)
		}
	}

	// Wait and collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var results []DetectedProvider
	seen := make(map[string]bool)
	for result := range resultChan {
		// Deduplicate by URL
		if !seen[result.URL] {
			results = append(results, result)
			seen[result.URL] = true
		}
	}

	// Sort by priority
	sort.Slice(results, func(i, j int) bool {
		priority := map[string]int{
			"ollama":        0,
			"llama-server":  1,
			"lm-studio":     2,
			"unknown":       3,
		}
		return priority[results[i].Name] < priority[results[j].Name]
	})

	return results
}
