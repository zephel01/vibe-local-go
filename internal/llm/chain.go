package llm

import (
	"context"
	"fmt"
	"sync"
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
// Phase 1: 単一プロバイダーのみ。Phase 4でフォールバックを追加
type ProviderChain struct {
	entries []ChainEntry
	current int
	mu      sync.RWMutex
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
		entries: entries,
		current: 0,
	}
}

// Chat 現在のプロバイダーでチャット（Phase 1: フォールバックなし）
func (c *ProviderChain) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.entries) == 0 {
		return nil, fmt.Errorf("no providers in chain")
	}

	// Phase 1: 現在のプロバイダーのみ使用
	provider := c.entries[c.current].Provider
	return provider.Chat(ctx, req)
}

// ChatStream ストリーミングチャット
func (c *ProviderChain) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.entries) == 0 {
		return nil, fmt.Errorf("no providers in chain")
	}

	provider := c.entries[c.current].Provider
	return provider.ChatStream(ctx, req)
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
