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

// OpenAICompatProvider OpenAI互換APIのベース実装
// Ollama, llama-server, LM Studio, OpenAI, DeepSeek, Groq 等で共通利用
type OpenAICompatProvider struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
	info       ProviderInfo
}

// NewOpenAICompatProvider OpenAI互換プロバイダーを作成
func NewOpenAICompatProvider(baseURL, apiKey, model string, info ProviderInfo) *OpenAICompatProvider {
	return &OpenAICompatProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: 300 * time.Second,
		},
		info: info,
	}
}

// Chat 同期チャットリクエスト（ツール使用対応）
func (p *OpenAICompatProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// ツール使用時は temperature を低く
	if req.ToolChoice != nil {
		req.Temperature = 0.3
	}
	req.Stream = false

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := p.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(httpReq)
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
			return nil, fmt.Errorf("LLM error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Empty or extremely short response body likely indicates context window overflow
	// (Ollama may return HTTP 200 with empty/truncated body when context is exceeded)
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, fmt.Errorf("empty response from LLM (possible context length exceeded)")
	}

	var response ChatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		salvaged, salvageErr := salvageJSON(body)
		if salvageErr != nil {
			return nil, fmt.Errorf("failed to parse response (possible context length exceeded, body=%d bytes): %w", len(body), err)
		}
		if err := json.Unmarshal(salvaged, &response); err != nil {
			return nil, fmt.Errorf("failed to parse salvaged response (possible context length exceeded): %w", err)
		}
	}

	// XMLフォールバック: ネイティブtool_callsがない場合、テキストからXML形式のtool呼び出しを抽出
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

// ChatStream ストリーミングチャットリクエスト
func (p *OpenAICompatProvider) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	req.Stream = true

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := p.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(body))
	}

	eventChan := make(chan StreamEvent, 10)
	go p.parseSSE(ctx, resp.Body, eventChan)
	return eventChan, nil
}

// CheckHealth プロバイダーの生存確認
func (p *OpenAICompatProvider) CheckHealth(ctx context.Context) error {
	url := p.baseURL + "/models"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	return nil
}

// Info プロバイダー情報を返す
func (p *OpenAICompatProvider) Info() ProviderInfo {
	return p.info
}

// SetTimeout タイムアウトを設定
func (p *OpenAICompatProvider) SetTimeout(timeout time.Duration) {
	p.httpClient.Timeout = timeout
}

// GetModel 使用中のモデル名を返す
func (p *OpenAICompatProvider) GetModel() string {
	return p.model
}

// SetModel モデルを変更
func (p *OpenAICompatProvider) SetModel(model string) {
	p.model = model
	p.info.Model = model
}

// parseSSE SSEストリームをパース
func (p *OpenAICompatProvider) parseSSE(ctx context.Context, body io.ReadCloser, eventChan chan<- StreamEvent) {
	defer close(eventChan)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			eventChan <- StreamEvent{Error: ctx.Err()}
			return
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			eventChan <- StreamEvent{Done: true}
			return
		}

		var chunk struct {
			Choices []Choice `json:"choices"`
		}
		if err := json.NewDecoder(strings.NewReader(data)).Decode(&chunk); err != nil {
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta
		finishReason := chunk.Choices[0].FinishReason

		if delta.Content == "" && len(delta.ToolCalls) == 0 {
			continue
		}

		eventChan <- StreamEvent{
			Delta: &Delta{
				Role:      delta.Role,
				Content:   delta.Content,
				ToolCalls: delta.ToolCalls,
			},
			Tokens: []Token{
				{
					Text:         delta.Content,
					FinishReason: finishReason,
				},
			},
		}

		if finishReason != "" {
			eventChan <- StreamEvent{Done: true}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		eventChan <- StreamEvent{Error: fmt.Errorf("SSE scanner error: %w", err)}
	}
}

// extractToolNames ツール定義からツール名一覧を取得
func extractToolNames(tools []ToolDef) []string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Function.Name)
	}
	return names
}
