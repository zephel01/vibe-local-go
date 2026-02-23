package llm

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// ChainRole プロバイダーチェーンでの役割
type ChainRole string

const (
	// RoleMain メインプロバイダー
	RoleMain ChainRole = "main"
	// RoleSub サブプロバイダー（ローカル別モデル）
	RoleSub ChainRole = "sub"
	// RoleFallback フォールバック（クラウド等）
	RoleFallback ChainRole = "fallback"
)

// ChainEntry チェーンエントリ
type ChainEntry struct {
	Provider LLMProvider
	Role     ChainRole
	Priority int // 低い値が優先
}

// ProviderChain フォールバック付きプロバイダーチェーン
// Phase 4: フォールバック機能対応
type ProviderChain struct {
	entries      []ChainEntry
	current      int
	lastError    error                   // 最後のエラー
	failureCount map[int]int             // プロバイダーごとの失敗カウント
	failureTime  map[int]time.Time        // プロバイダーごとの最後の失敗時刻
	fallbackOn   bool                     // フォールバック有効化フラグ
	maxRetries   int                      // 最大リトライ数
	mu           sync.RWMutex
}

// NewProviderChain 新しいプロバイダーチェーンを作成
func NewProviderChain(providers ...LLMProvider) *ProviderChain {
	entries := make([]ChainEntry, len(providers))
	for i, p := range providers {
		role := RoleFallback
		if i == 0 {
			role = RoleMain
		} else if i == 1 {
			role = RoleSub
		}
		entries[i] = ChainEntry{
			Provider: p,
			Role:     role,
			Priority: i,
		}
	}
	return &ProviderChain{
		entries:      entries,
		current:      0,
		failureCount: make(map[int]int),
		failureTime:  make(map[int]time.Time),
		fallbackOn:   len(providers) > 1, // 複数プロバイダーの場合のみ有効化
		maxRetries:   3,
	}
}

// EnableFallback フォールバック機能を有効化
func (c *ProviderChain) EnableFallback(enabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fallbackOn = enabled
}

// SetMaxRetries 最大リトライ回数を設定
func (c *ProviderChain) SetMaxRetries(max int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.maxRetries = max
}

// Chat 現在のプロバイダーでチャット（フォールバック対応）
func (c *ProviderChain) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	c.mu.RLock()
	if len(c.entries) == 0 {
		c.mu.RUnlock()
		return nil, fmt.Errorf("no providers in chain")
	}

	// fallbackOn フラグがない場合は単一プロバイダーで返す
	if !c.fallbackOn {
		provider := c.entries[c.current].Provider
		c.mu.RUnlock()
		return provider.Chat(ctx, req)
	}
	c.mu.RUnlock()

	// フォールバック有効: 複数プロバイダーで試行
	return c.chatWithFallback(ctx, req)
}

// chatWithFallback プロバイダーチェーンでフォールバック付きチャット
func (c *ProviderChain) chatWithFallback(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	c.mu.RLock()
	initialIdx := c.current
	c.mu.RUnlock()

	// 現在のプロバイダーから開始してリトライ
	for attempt := 0; attempt < len(c.entries); attempt++ {
		c.mu.RLock()
		if c.current >= len(c.entries) {
			c.mu.RUnlock()
			break
		}
		provider := c.entries[c.current].Provider
		providerInfo := provider.Info()
		c.mu.RUnlock()

		// チャット実行
		resp, err := provider.Chat(ctx, req)

		// 成功 → 失敗カウントをリセット
		if err == nil {
			c.mu.Lock()
			c.failureCount[c.current] = 0
			c.lastError = nil
			c.mu.Unlock()
			return resp, nil
		}

		// エラー発生 → Fallback 判定
		if !c.shouldFallback(err) {
			c.mu.Lock()
			c.lastError = err
			c.mu.Unlock()
			return nil, err
		}

		// Fallback 発動 → 次のプロバイダーへ
		c.mu.Lock()
		c.failureCount[c.current]++
		c.failureTime[c.current] = time.Now()
		c.lastError = err
		c.mu.Unlock()

		// 次のプロバイダーに切り替え
		if !c.switchToNext() {
			return nil, fmt.Errorf("all providers failed, last error: %w", err)
		}

		// UI通知
		c.mu.RLock()
		nextProvider := c.entries[c.current].Provider.Info()
		c.mu.RUnlock()
		// 通知はログで行われる（Terminal への直接出力は不要）
		_ = providerInfo
		_ = nextProvider
	}

	return nil, fmt.Errorf("all providers exhausted")
}

// ChatStream ストリーミングチャット（フォールバック対応）
func (c *ProviderChain) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	c.mu.RLock()
	if len(c.entries) == 0 {
		c.mu.RUnlock()
		return nil, fmt.Errorf("no providers in chain")
	}

	// fallbackOn フラグがない場合は単一プロバイダーで返す
	if !c.fallbackOn {
		provider := c.entries[c.current].Provider
		c.mu.RUnlock()
		return provider.ChatStream(ctx, req)
	}
	c.mu.RUnlock()

	// フォールバック有効: 複数プロバイダーで試行
	return c.chatStreamWithFallback(ctx, req)
}

// chatStreamWithFallback ストリーミングでフォールバック付きチャット
func (c *ProviderChain) chatStreamWithFallback(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	c.mu.RLock()
	initialIdx := c.current
	c.mu.RUnlock()

	// 現在のプロバイダーから開始してリトライ
	for attempt := 0; attempt < len(c.entries); attempt++ {
		c.mu.RLock()
		if c.current >= len(c.entries) {
			c.mu.RUnlock()
			break
		}
		provider := c.entries[c.current].Provider
		c.mu.RUnlock()

		// ストリーミング開始
		eventChan, err := provider.ChatStream(ctx, req)

		// 接続成功 → チャネルを通じてイベントを転送
		if err == nil {
			// 成功カウントをリセット
			c.mu.Lock()
			c.failureCount[c.current] = 0
			c.lastError = nil
			c.mu.Unlock()

			// イベントチャネルをラップして返す
			return c.wrapStreamWithFallback(ctx, eventChan), nil
		}

		// エラー発生 → Fallback 判定
		if !c.shouldFallback(err) {
			c.mu.Lock()
			c.lastError = err
			c.mu.Unlock()
			return nil, err
		}

		// Fallback 発動
		c.mu.Lock()
		c.failureCount[c.current]++
		c.failureTime[c.current] = time.Now()
		c.lastError = err
		c.mu.Unlock()

		if !c.switchToNext() {
			return nil, fmt.Errorf("all providers failed, last error: %w", err)
		}
	}

	return nil, fmt.Errorf("all providers exhausted")
}

// wrapStreamWithFallback ストリーミングをラップしてエラーをハンドル
func (c *ProviderChain) wrapStreamWithFallback(ctx context.Context, eventChan <-chan StreamEvent) <-chan StreamEvent {
	outChan := make(chan StreamEvent, 1)
	go func() {
		defer close(outChan)
		for event := range eventChan {
			select {
			case <-ctx.Done():
				outChan <- StreamEvent{Error: ctx.Err()}
				return
			case outChan <- event:
			}
		}
	}()
	return outChan
}

// CheckHealth 現在のプロバイダーのヘルスチェック
func (c *ProviderChain) CheckHealth(ctx context.Context) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.entries) == 0 {
		return fmt.Errorf("no providers in chain")
	}

	return c.entries[c.current].Provider.CheckHealth(ctx)
}

// Info 現在のプロバイダーの情報
func (c *ProviderChain) Info() ProviderInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.entries) == 0 {
		return ProviderInfo{Name: "none"}
	}

	return c.entries[c.current].Provider.Info()
}

// GetCurrentProvider 現在のプロバイダーを返す
func (c *ProviderChain) GetCurrentProvider() LLMProvider {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.entries) == 0 {
		return nil
	}

	return c.entries[c.current].Provider
}

// GetEntries チェーンエントリ一覧を返す
func (c *ProviderChain) GetEntries() []ChainEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]ChainEntry, len(c.entries))
	copy(result, c.entries)
	return result
}

// SwitchTo 指定インデックスのプロバイダーに切り替え
func (c *ProviderChain) SwitchTo(index int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if index < 0 || index >= len(c.entries) {
		return fmt.Errorf("invalid provider index: %d", index)
	}

	c.current = index
	return nil
}

// shouldFallback エラーからフォールバックすべきかを判定
func (c *ProviderChain) shouldFallback(err error) bool {
	if err == nil {
		return false
	}

	// ネットワークエラー → Fallback 対象
	if _, ok := err.(net.Error); ok {
		return true
	}

	// 特定のエラーメッセージパターンで判定
	errStr := err.Error()

	// タイムアウト → Fallback 対象
	if errStr == "context deadline exceeded" {
		return true
	}

	// 接続拒否 → Fallback 対象
	if errStr == "connection refused" {
		return true
	}

	// ホスト不在 → Fallback 対象
	if errStr == "no such host" {
		return true
	}

	// HTTP 5xx エラー → Fallback 対象
	if len(errStr) >= 4 && errStr[0] == '5' && errStr[1] == '0' && errStr[2] == '0' {
		return true
	}

	// "HTTP 5" から始まるエラー → Fallback 対象
	if len(errStr) > 7 && errStr[:5] == "HTTP " && errStr[5] >= '5' && errStr[5] <= '9' {
		return true
	}

	// その他のエラー（HTTP 4xx, モデルエラー等）→ Fallback しない
	return false
}

// switchToNext 次の利用可能なプロバイダーに切り替え
func (c *ProviderChain) switchToNext() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) <= 1 {
		return false
	}

	// 次のプロバイダーを探す（role優先度: main > sub > fallback）
	startIdx := c.current
	for {
		c.current = (c.current + 1) % len(c.entries)
		if c.current == startIdx {
			// 一周した = 全て試した
			return false
		}

		// 新しいインデックスに切り替えた
		return true
	}
}

// GetLastError 最後のエラーを返す
func (c *ProviderChain) GetLastError() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastError
}

// GetFailureCount プロバイダーの失敗カウントを返す
func (c *ProviderChain) GetFailureCount(index int) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.failureCount[index]
}

// GetFailureTime プロバイダーの最後の失敗時刻を返す
func (c *ProviderChain) GetFailureTime(index int) time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.failureTime[index]
}
