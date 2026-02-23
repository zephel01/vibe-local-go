package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// OpenRouterBaseURL OpenRouterのAPI基盤URL（/v1含む）
	OpenRouterBaseURL = "https://openrouter.ai/api/v1"
	// OpenRouterDefaultModel デフォルトモデル
	OpenRouterDefaultModel = "google/gemini-2.5-flash"
)

// OpenRouterProvider OpenRouter固有プロバイダー
// OpenAI互換API + OpenRouter固有ヘッダー（HTTP-Referer, X-Title）
type OpenRouterProvider struct {
	*OpenAICompatProvider
	referer string // HTTP-Referer ヘッダー（アプリ識別用）
	title   string // X-Title ヘッダー（アプリ名称）
}

// NewOpenRouterProvider 新しいOpenRouterプロバイダーを作成
func NewOpenRouterProvider(apiKey, model string) *OpenRouterProvider {
	if model == "" {
		model = OpenRouterDefaultModel
	}

	info := ProviderInfo{
		Name:    "openrouter",
		Type:    ProviderTypeCloud,
		BaseURL: OpenRouterBaseURL,
		Model:   model,
		Features: Features{
			NativeFunctionCalling: true,
			ModelManagement:       false,
			Streaming:             true,
		},
	}

	return &OpenRouterProvider{
		OpenAICompatProvider: NewOpenAICompatProvider(OpenRouterBaseURL, apiKey, model, info),
		referer:              "https://github.com/zephel01/vibe-local-go",
		title:                "vibe-local",
	}
}

// Chat OpenRouter固有ヘッダーを追加して同期チャット
func (o *OpenRouterProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// ツール使用時は temperature を低く
	if req.ToolChoice != nil {
		req.Temperature = 0.3
	}
	req.Stream = false

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := o.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}
	o.setHeaders(httpReq)

	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("OpenRouter error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var response ChatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		salvaged, salvageErr := salvageJSON(body)
		if salvageErr != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		if err := json.Unmarshal(salvaged, &response); err != nil {
			return nil, fmt.Errorf("failed to parse salvaged response: %w", err)
		}
	}

	// XMLフォールバック: ネイティブtool_callsがない場合
	if len(response.Choices) > 0 && len(response.Choices[0].Message.ToolCalls) == 0 {
		if response.Choices[0].Message.Content != "" && req.Tools != nil && len(req.Tools) > 0 {
			knownTools := extractToolNames(req.Tools)
			calls, err := ExtractToolCallsFromText(response.Choices[0].Message.Content, knownTools)
			if err == nil && len(calls) > 0 {
				response.Choices[0].Message.ToolCalls = calls
				response.Choices[0].FinishReason = "tool_calls"
			}
		}
	}

	return &response, nil
}

// ChatStream OpenRouter固有ヘッダーを追加してストリーミング
func (o *OpenRouterProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	req.Stream = true

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := o.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}
	o.setHeaders(httpReq)

	resp, err := o.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	eventChan := make(chan StreamEvent, 10)
	go o.parseSSE(ctx, resp.Body, eventChan)
	return eventChan, nil
}

// CheckHealth OpenRouterの生存確認
func (o *OpenRouterProvider) CheckHealth(ctx context.Context) error {
	url := o.baseURL + "/models"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	o.setHeaders(req)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to OpenRouter: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OpenRouter health check failed with status %d", resp.StatusCode)
	}

	return nil
}

// Info プロバイダー情報
func (o *OpenRouterProvider) Info() ProviderInfo {
	return o.OpenAICompatProvider.Info()
}

// SetReferer HTTP-Refererヘッダーを設定
func (o *OpenRouterProvider) SetReferer(referer string) {
	o.referer = referer
}

// SetTitle X-Titleヘッダーを設定
func (o *OpenRouterProvider) SetTitle(title string) {
	o.title = title
}

// setHeaders OpenRouter固有のHTTPヘッダーを設定
func (o *OpenRouterProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if o.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+o.apiKey)
	}
	if o.referer != "" {
		req.Header.Set("HTTP-Referer", o.referer)
	}
	if o.title != "" {
		req.Header.Set("X-Title", o.title)
	}
}

// ListAvailableModels OpenRouterで利用可能なモデル一覧を取得
func (o *OpenRouterProvider) ListAvailableModels(ctx context.Context) ([]OpenRouterModel, error) {
	url := o.baseURL + "/models"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	o.setHeaders(req)

	// タイムアウトを短く
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list models: status %d", resp.StatusCode)
	}

	var result struct {
		Data []OpenRouterModel `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Data, nil
}

// OpenRouterModel OpenRouterのモデル情報
type OpenRouterModel struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	ContextLength int     `json:"context_length"`
	Pricing       Pricing `json:"pricing"`
}

// Pricing モデル料金情報
type Pricing struct {
	Prompt     string `json:"prompt"`     // $/token
	Completion string `json:"completion"` // $/token
}

// String モデル情報の表示用文字列
func (m OpenRouterModel) String() string {
	return fmt.Sprintf("%s (ctx: %d)", m.ID, m.ContextLength)
}

// IsFreeTier 無料ティアかどうか
func (m OpenRouterModel) IsFreeTier() bool {
	return m.Pricing.Prompt == "0" && m.Pricing.Completion == "0"
}

// FilterModelsByPrefix プレフィックスでモデルをフィルタ
func FilterModelsByPrefix(models []OpenRouterModel, prefix string) []OpenRouterModel {
	if prefix == "" {
		return models
	}
	prefix = strings.ToLower(prefix)
	filtered := make([]OpenRouterModel, 0)
	for _, m := range models {
		if strings.HasPrefix(strings.ToLower(m.ID), prefix) {
			filtered = append(filtered, m)
		}
	}
	return filtered
}
