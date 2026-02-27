package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultNumCtxStages num_ctx 自動エスカレーションのデフォルト段階
var DefaultNumCtxStages = []int{8192, 16384, 32768, 65536}

// OllamaProvider Ollama固有機能付きプロバイダー
// OpenAI互換APIでチャット + Ollama APIでモデル管理
type OllamaProvider struct {
	*OpenAICompatProvider
	ollamaURL     string // Ollama API用（/api/...）
	numCtxStages  []int  // num_ctx エスカレーション段階
	autoEscalate  bool   // 自動エスカレーション有効/無効
	currentNumCtx int    // 現在使用中の num_ctx（0=Ollama任せ）
}

// normalizeBaseURL ベースURLの末尾 /v1 や / を除去してホストのみにする
// 例: "http://localhost:1234/v1" → "http://localhost:1234"
func normalizeBaseURL(rawURL string) string {
	u := strings.TrimSuffix(rawURL, "/")
	u = strings.TrimSuffix(u, "/v1")
	return strings.TrimSuffix(u, "/")
}

// NormalizeBaseURL は normalizeBaseURL の公開版（パッケージ外から利用可能）
func NormalizeBaseURL(rawURL string) string {
	return normalizeBaseURL(rawURL)
}

// NewOllamaProvider 新しいOllamaプロバイダーを作成
func NewOllamaProvider(host, model string) *OllamaProvider {
	host = normalizeBaseURL(host)
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
		ollamaURL:            host,
		numCtxStages:         DefaultNumCtxStages,
		autoEscalate:         true,
	}
}

// SetNumCtx 固定の num_ctx を設定（自動エスカレーションはその値以上から開始）
func (o *OllamaProvider) SetNumCtx(numCtx int) {
	o.currentNumCtx = numCtx
}

// SetAutoEscalate 自動エスカレーションの有効/無効を設定
func (o *OllamaProvider) SetAutoEscalate(enabled bool) {
	o.autoEscalate = enabled
}

// SetNumCtxStages エスカレーション段階をカスタマイズ
func (o *OllamaProvider) SetNumCtxStages(stages []int) {
	o.numCtxStages = stages
}

// GetCurrentNumCtx 現在の num_ctx を返す
func (o *OllamaProvider) GetCurrentNumCtx() int {
	return o.currentNumCtx
}

// Chat Ollama固有の Chat 実装（num_ctx 自動エスカレーション付き）
//
// 動作モード:
//   - --num-ctx 指定時: 指定値で開始 → context exceeded なら段階的に引き上げ
//   - --num-ctx 未指定時: num_ctx を送らない（Ollamaデフォルト） → context exceeded なら段階的に引き上げ
func (o *OllamaProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// 自動エスカレーション無効 or 段階未設定なら親のChatをそのまま呼ぶ
	if !o.autoEscalate || len(o.numCtxStages) == 0 {
		return o.chatWithNumCtx(ctx, req, o.currentNumCtx)
	}

	// まず現在の num_ctx（0ならOllamaデフォルト）で試行
	resp, err := o.chatWithNumCtx(ctx, req, o.currentNumCtx)
	if err == nil {
		return resp, nil
	}

	// コンテキスト超過以外のエラーはそのまま返す
	if ClassifyError(err) != ErrorClassContextWindow {
		return nil, err
	}

	// コンテキスト超過 → エスカレーション段階で順にリトライ
	lastErr := err
	for _, numCtx := range o.buildEscalationStages() {
		resp, err := o.chatWithNumCtx(ctx, req, numCtx)
		if err == nil {
			o.currentNumCtx = numCtx
			return resp, nil
		}

		if ClassifyError(err) != ErrorClassContextWindow {
			return nil, err
		}
		lastErr = err
	}

	return nil, fmt.Errorf("context exceeded all escalation stages: %w", lastErr)
}

// ChatStream Ollama固有の ChatStream 実装（num_ctx 自動エスカレーション付き）
func (o *OllamaProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	// 自動エスカレーション無効なら親のChatStreamをそのまま呼ぶ
	if !o.autoEscalate || len(o.numCtxStages) == 0 {
		o.applyNumCtx(req, o.currentNumCtx)
		return o.OpenAICompatProvider.ChatStream(ctx, req)
	}

	// まず現在の num_ctx で試行
	o.applyNumCtx(req, o.currentNumCtx)
	ch, err := o.OpenAICompatProvider.ChatStream(ctx, req)
	if err == nil {
		return ch, nil
	}

	if ClassifyError(err) != ErrorClassContextWindow {
		return nil, err
	}

	// コンテキスト超過 → エスカレーション
	lastErr := err
	for _, numCtx := range o.buildEscalationStages() {
		o.applyNumCtx(req, numCtx)
		ch, err := o.OpenAICompatProvider.ChatStream(ctx, req)
		if err == nil {
			o.currentNumCtx = numCtx
			return ch, nil
		}

		if ClassifyError(err) != ErrorClassContextWindow {
			return nil, err
		}
		lastErr = err
	}

	return nil, fmt.Errorf("context exceeded all escalation stages: %w", lastErr)
}

// chatWithNumCtx 指定 num_ctx で Chat を実行
func (o *OllamaProvider) chatWithNumCtx(ctx context.Context, req *ChatRequest, numCtx int) (*ChatResponse, error) {
	o.applyNumCtx(req, numCtx)
	return o.OpenAICompatProvider.Chat(ctx, req)
}

// applyNumCtx リクエストに num_ctx を設定
func (o *OllamaProvider) applyNumCtx(req *ChatRequest, numCtx int) {
	if numCtx <= 0 {
		return
	}
	if req.Options == nil {
		req.Options = make(map[string]interface{})
	}
	req.Options["num_ctx"] = numCtx
}

// buildEscalationStages 現在の num_ctx より大きいエスカレーション段階を構築
// Chat() で currentNumCtx が既に失敗した後に呼ばれるため、それより大きい値のみ返す
func (o *OllamaProvider) buildEscalationStages() []int {
	startFrom := o.currentNumCtx

	// currentNumCtx 未設定（Ollamaデフォルトで失敗した場合）→ 全段階を試す
	if startFrom <= 0 {
		return o.numCtxStages
	}

	// startFrom より大きい段階のみ返す（startFrom は既に失敗済み）
	stages := make([]int, 0)
	for _, stage := range o.numCtxStages {
		if stage > startFrom {
			stages = append(stages, stage)
		}
	}

	return stages
}

// FetchLocalProviderModels ローカルプロバイダーからモデルリストを取得
func FetchLocalProviderModels(host, providerKey string) ([]string, error) {
	host = normalizeBaseURL(host)
	var url string
	switch providerKey {
	case "ollama":
		url = host + "/api/tags"
	case "lm-studio":
		url = host + "/api/v1/models" // LM Studio Native REST API
	case "llama-server":
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

	switch providerKey {
	case "ollama":
		var result struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}
		models := make([]string, len(result.Models))
		for i, m := range result.Models {
			models[i] = m.Name
		}
		return models, nil
	case "lm-studio":
		// LM Studio Native REST API 形式: {"models": [{"key": "...", "type": "llm"|"embedding", ...}]}
		var result struct {
			Models []struct {
				Key  string `json:"key"`
				Type string `json:"type"`
			} `json:"models"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}
		models := make([]string, 0, len(result.Models))
		for _, m := range result.Models {
			if m.Type != "embedding" {
				models = append(models, m.Key)
			}
		}
		return models, nil
	default:
		// OpenAI互換形式（llama-server など）
		var result struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}
		models := make([]string, len(result.Data))
		for i, m := range result.Data {
			models[i] = m.ID
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
