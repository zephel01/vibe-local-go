package llm

import (
	"bytes"
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

	// LM Studio Native REST API 形式:
	// {"models": [{"key": "...", "type": "llm"|"embedding", "display_name": "...", ...}]}
	var result struct {
		Models []struct {
			Key  string `json:"key"`
			Type string `json:"type"` // "llm" | "embedding"
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode LM Studio models: %w", err)
	}

	models := make([]string, 0, len(result.Models))
	for _, m := range result.Models {
		// embeddingモデルを除外
		if m.Type != "embedding" {
			models = append(models, m.Key)
		}
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("no llm models found in /api/v1/models")
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

// Chat モデルをロードしてからチャット
func (p *LMStudioProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	if err := p.ensureModelLoaded(ctx, req.Model); err != nil {
		// ロード失敗はwarningとして扱い、そのままチャット試行
		_ = err
	}
	return p.OpenAICompatProvider.Chat(ctx, req)
}

// ChatStream モデルをロードしてからストリーミングチャット
func (p *LMStudioProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	if err := p.ensureModelLoaded(ctx, req.Model); err != nil {
		_ = err
	}
	return p.OpenAICompatProvider.ChatStream(ctx, req)
}

// ensureModelLoaded 指定モデルがロード済みでなければロードする
func (p *LMStudioProvider) ensureModelLoaded(ctx context.Context, model string) error {
	// 現在ロード済みのモデルを確認
	loadedKey, err := p.getLoadedModelKey(ctx)
	if err == nil && loadedKey == model {
		return nil // すでにロード済み
	}

	// モデルをロード（タイムアウトを長めに設定）
	loadCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	return p.loadModel(loadCtx, model)
}

// getLoadedModelKey 現在ロード済みのモデルキーを取得
// GET /api/v1/models/get
func (p *LMStudioProvider) getLoadedModelKey(ctx context.Context) (string, error) {
	url := p.baseHost + "/api/v1/models/get"
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "vibe-local-go/lmstudio")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET /api/v1/models/get returned %d", resp.StatusCode)
	}

	var result struct {
		Model *struct {
			Key string `json:"key"`
		} `json:"model"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.Model == nil || result.Model.Key == "" {
		return "", fmt.Errorf("no model currently loaded")
	}
	return result.Model.Key, nil
}

// loadModel POST /api/v1/models/load でモデルをロード
func (p *LMStudioProvider) loadModel(ctx context.Context, key string) error {
	url := p.baseHost + "/api/v1/models/load"

	body := map[string]interface{}{
		"identifier": key,
		"config":     map[string]interface{}{},
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "vibe-local-go/lmstudio")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to load model %q: %w", key, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST /api/v1/models/load returned %d", resp.StatusCode)
	}
	return nil
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
