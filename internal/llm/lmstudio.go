package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// LMStudioProvider LM Studio専用プロバイダー
// OpenAI互換API(/v1/chat/completions)でチャット
// Native REST API(/api/v1/models)でモデル管理
type LMStudioProvider struct {
	*OpenAICompatProvider
	baseHost string // http://localhost:1234 形式（/v1 なし）
}

// NewLMStudioProvider 新しいLM Studioプロバイダーを作成
// host は http://localhost:1234 または http://localhost:1234/v1 どちらでも可
func NewLMStudioProvider(host, model string) *LMStudioProvider {
	// ベースホストに正規化（/v1 を除去）
	baseHost := normalizeBaseURL(host)

	info := ProviderInfo{
		Name:    "lm-studio",
		Type:    ProviderTypeLocal,
		BaseURL: baseHost,
		Model:   model,
		Features: Features{
			NativeFunctionCalling: true,
			Streaming:             true,
		},
	}

	// OpenAI互換APIのbaseURLは baseHost + "/v1"
	return &LMStudioProvider{
		OpenAICompatProvider: NewOpenAICompatProvider(baseHost+"/v1", "", model, info),
		baseHost:            baseHost,
	}
}

// Info プロバイダー情報を返す
func (p *LMStudioProvider) Info() ProviderInfo {
	info := p.OpenAICompatProvider.Info()
	info.Name = "lm-studio"
	info.BaseURL = p.baseHost
	return info
}

// ListModels LM Studio Native API /api/v1/models でモデル一覧を取得
// フォールバック: OpenAI互換 /v1/models
func (p *LMStudioProvider) ListModels(ctx context.Context) ([]string, error) {
	// まず Native API を試みる
	models, err := p.listModelsNative(ctx)
	if err == nil {
		return models, nil
	}
	// フォールバック: OpenAI互換 /v1/models
	return p.listModelsOpenAICompat(ctx)
}

// listModelsNative LM Studio Native REST API でモデル一覧を取得
// GET /api/v1/models → { "data": [{ "id": "...", "state": "loaded" }] }
func (p *LMStudioProvider) listModelsNative(ctx context.Context) ([]string, error) {
	url := p.baseHost + "/api/v1/models"

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "vibe-local-go/lmstudio")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LM Studio /api/v1/models returned %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID    string `json:"id"`
			State string `json:"state"` // "loaded" | "not-loaded"
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode LM Studio models: %w", err)
	}

	models := make([]string, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	return models, nil
}

// listModelsOpenAICompat OpenAI互換 /v1/models でモデル一覧を取得（フォールバック）
func (p *LMStudioProvider) listModelsOpenAICompat(ctx context.Context) ([]string, error) {
	url := p.baseHost + "/v1/models"

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "vibe-local-go/lmstudio")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LM Studio /v1/models returned %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode LM Studio models (compat): %w", err)
	}

	models := make([]string, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, m.ID)
	}
	return models, nil
}

// IsReachable LM Studio が起動中かどうかを確認
func (p *LMStudioProvider) IsReachable(ctx context.Context) bool {
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// Native API を優先してチェック
	url := p.baseHost + "/api/v1/models"
	req, err := http.NewRequestWithContext(checkCtx, "GET", url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "vibe-local-go/healthcheck")

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// フォールバック: /v1/models で確認
		url2 := p.baseHost + "/v1/models"
		req2, err2 := http.NewRequestWithContext(checkCtx, "GET", url2, nil)
		if err2 != nil {
			return false
		}
		resp2, err2 := client.Do(req2)
		if err2 != nil {
			return false
		}
		defer resp2.Body.Close()
		return resp2.StatusCode == http.StatusOK
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
