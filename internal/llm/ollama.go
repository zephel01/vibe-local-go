package llm

import (
	"bufio"
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

// PullProgressCallback ダウンロード進捗コールバック関数の型
// status: 現在のステータス ("pulling manifest", "downloading ...", "verifying sha256 digest", "writing manifest", "success" 等)
// completed: ダウンロード済みバイト数
// total: 総バイト数（0の場合はサイズ不明）
type PullProgressCallback func(status string, completed, total int64)

// PullModel モデルをダウンロード（進捗表示なし・後方互換）
func (o *OllamaProvider) PullModel(ctx context.Context, name string) error {
	return o.PullModelWithProgress(ctx, name, nil)
}

// PullModelWithProgress モデルをダウンロード（進捗コールバック付き）
func (o *OllamaProvider) PullModelWithProgress(ctx context.Context, name string, progressFn PullProgressCallback) error {
	url := o.ollamaURL + "/api/pull"
	stream := progressFn != nil
	payload := map[string]interface{}{
		"name":   name,
		"stream": stream,
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

	if !stream {
		// 非ストリーミング: レスポンスを読み捨てて完了
		io.Copy(io.Discard, resp.Body)
		return nil
	}

	// ストリーミング: JSON Lines を1行ずつ読んで進捗コールバックを呼ぶ
	// Ollama /api/pull のレスポンス形式:
	// {"status":"pulling manifest"}
	// {"status":"downloading abc123","digest":"sha256:...","total":4567890,"completed":1234567}
	// {"status":"verifying sha256 digest"}
	// {"status":"writing manifest"}
	// {"status":"success"}
	scanner := bufio.NewScanner(resp.Body)
	// 大きいレスポンス行に対応（デフォルト64KBでは不足する場合がある）
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

	var pullResp struct {
		Status    string `json:"status"`
		Digest    string `json:"digest"`
		Total     int64  `json:"total"`
		Completed int64  `json:"completed"`
		Error     string `json:"error"`
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		if err := json.Unmarshal(line, &pullResp); err != nil {
			continue // パースできない行はスキップ
		}

		if pullResp.Error != "" {
			return fmt.Errorf("pull error: %s", pullResp.Error)
		}

		progressFn(pullResp.Status, pullResp.Completed, pullResp.Total)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading pull response: %w", err)
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
