package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaProvider Ollama固有機能付きプロバイダー
// OpenAI互換APIでチャット + Ollama APIでモデル管理
type OllamaProvider struct {
	*OpenAICompatProvider
	ollamaURL string // Ollama API用（/api/...）
}

// NewOllamaProvider 新しいOllamaプロバイダーを作成
func NewOllamaProvider(host, model string) *OllamaProvider {
	info := ProviderInfo{
		Name:    "ollama",
		Type:    ProviderTypeLocal,
		BaseURL: host,
		Model:   model,
		Features: Features{
			NativeFunctionCalling: true,
			ModelManagement:       true,
			Streaming:             true,
		},
	}

	return &OllamaProvider{
		OpenAICompatProvider: NewOpenAICompatProvider(host+"/v1", "", model, info),
		ollamaURL:           host,
	}
}

// FetchLocalProviderModels ローカルプロバイダーからモデルリストを取得
func FetchLocalProviderModels(host, providerKey string) ([]string, error) {
	var url string
	switch providerKey {
	case "ollama":
		url = host + "/api/tags"
	case "lm-studio", "llama-server":
		url = host + "/v1/models"
	default:
		return nil, fmt.Errorf("unsupported provider: %s", providerKey)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("provider returned status %d", resp.StatusCode)
	}

	if providerKey == "ollama" {
		var result struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}
		models := make([]string, len(result.Models))
		for i, model := range result.Models {
			models[i] = model.Name
		}
		return models, nil
	} else {
		// OpenAI互換形式
		var result struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}
		models := make([]string, len(result.Data))
		for i, model := range result.Data {
			models[i] = model.ID
		}
		return models, nil
	}
}

// CheckHealth Ollama固有の生存確認（/api/tags を使用）
func (o *OllamaProvider) CheckHealth(ctx context.Context) error {
	url := o.ollamaURL + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Ollama: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Ollama returned status %d", resp.StatusCode)
	}

	return nil
}

// Info プロバイダー情報（ModelManagement = true）
func (o *OllamaProvider) Info() ProviderInfo {
	info := o.OpenAICompatProvider.Info()
	info.Features.ModelManagement = true
	return info
}

// --- ModelManager インターフェース実装 ---

// ListModels 利用可能なモデル一覧を返す
func (o *OllamaProvider) ListModels(ctx context.Context) ([]string, error) {
	url := o.ollamaURL + "/api/tags"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ollama returned status %d", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]string, len(result.Models))
	for i, model := range result.Models {
		models[i] = model.Name
	}

	return models, nil
}

// PullModel モデルをダウンロード
func (o *OllamaProvider) PullModel(ctx context.Context, name string) error {
	url := o.ollamaURL + "/api/pull"
	payload := map[string]interface{}{
		"name":   name,
		"stream": false,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// モデルDLは時間がかかるため専用タイムアウト
	pullClient := &http.Client{
		Timeout: 30 * time.Minute,
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonPayload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := pullClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to pull model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to pull model %s: %s", name, string(body))
	}

	return nil
}

// CheckModel 指定モデルが利用可能か確認
func (o *OllamaProvider) CheckModel(ctx context.Context, name string) (bool, error) {
	models, err := o.ListModels(ctx)
	if err != nil {
		return false, err
	}

	for _, model := range models {
		if model == name {
			return true, nil
		}
	}

	return false, nil
}
