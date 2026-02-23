package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ModelRouter モデルルーター（LLMProviderベース）
type ModelRouter struct {
	mainProvider    LLMProvider
	sidecarProvider LLMProvider
	mainModel       string
	sidecarModel    string
	useSidecar      bool
	sidecarLoaded   bool
	mu              sync.RWMutex
}

// NewModelRouter 新しいモデルルーターを作成
func NewModelRouter(mainProvider, sidecarProvider LLMProvider, mainModel, sidecarModel string) *ModelRouter {
	return &ModelRouter{
		mainProvider:    mainProvider,
		sidecarProvider: sidecarProvider,
		mainModel:       mainModel,
		sidecarModel:    sidecarModel,
		useSidecar:      false,
		sidecarLoaded:   false,
	}
}

// PreloadSidecar サイドカーモデルをプリロード
func (mr *ModelRouter) PreloadSidecar(ctx context.Context) error {
	if mr.sidecarProvider == nil || mr.sidecarModel == "" {
		return nil
	}

	mr.mu.Lock()
	defer mr.mu.Unlock()

	if mr.sidecarLoaded {
		return nil
	}

	// ModelManagerインターフェースを持つプロバイダーのみプル可能
	if mm, ok := mr.sidecarProvider.(ModelManager); ok {
		if err := mm.PullModel(ctx, mr.sidecarModel); err != nil {
			return fmt.Errorf("サイドカーモデルのロードに失敗: %w", err)
		}
	}

	mr.sidecarLoaded = true
	return nil
}

// SwitchToMain メインモデルに切り替え
func (mr *ModelRouter) SwitchToMain() {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.useSidecar = false
}

// SwitchToSidecar サイドカーモデルに切り替え
func (mr *ModelRouter) SwitchToSidecar() {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.useSidecar = true
}

// GetActiveModel アクティブなモデル名を取得
func (mr *ModelRouter) GetActiveModel() string {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	if mr.useSidecar {
		return mr.sidecarModel
	}
	return mr.mainModel
}

// GetActiveProvider アクティブなプロバイダーを取得
func (mr *ModelRouter) GetActiveProvider() LLMProvider {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	if mr.useSidecar {
		return mr.sidecarProvider
	}
	return mr.mainProvider
}

// Chat メイン/サイドカーを自動選択してチャット
func (mr *ModelRouter) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	provider := mr.GetActiveProvider()
	req.Model = mr.GetActiveModel()
	return provider.Chat(ctx, req)
}

// ChatStream ストリーミングチャット
func (mr *ModelRouter) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	provider := mr.GetActiveProvider()
	req.Model = mr.GetActiveModel()
	return provider.ChatStream(ctx, req)
}

// CheckHealth アクティブプロバイダーのヘルスチェック
func (mr *ModelRouter) CheckHealth(ctx context.Context) error {
	return mr.GetActiveProvider().CheckHealth(ctx)
}

// Info アクティブプロバイダーの情報
func (mr *ModelRouter) Info() ProviderInfo {
	return mr.GetActiveProvider().Info()
}

// AutoSelectModel タスクに基づいてモデルを自動選択
func (mr *ModelRouter) AutoSelectModel(taskType string) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	switch taskType {
	case "code_generation":
		mr.useSidecar = false
	case "code_review":
		if mr.sidecarModel != "" {
			mr.useSidecar = true
		} else {
			mr.useSidecar = false
		}
	case "documentation":
		if mr.sidecarModel != "" {
			mr.useSidecar = true
		} else {
			mr.useSidecar = false
		}
	case "quick_edit":
		mr.useSidecar = false
	default:
		mr.useSidecar = false
	}
}

// SwapModelHot モデルをホットスワップ
func (mr *ModelRouter) SwapModelHot(ctx context.Context, toSidecar bool) error {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if mr.useSidecar == toSidecar {
		return nil
	}

	if toSidecar && !mr.sidecarLoaded && mr.sidecarProvider != nil {
		if mm, ok := mr.sidecarProvider.(ModelManager); ok {
			if err := mm.PullModel(ctx, mr.sidecarModel); err != nil {
				return fmt.Errorf("サイドカーモデルのロードに失敗: %w", err)
			}
		}
		mr.sidecarLoaded = true
	}

	mr.useSidecar = toSidecar
	return nil
}

// SelectModelByMemory メモリ量に基づいてモデルを選択
func (mr *ModelRouter) SelectModelByMemory(memoryGB float64) string {
	switch {
	case memoryGB >= 256:
		return "qwen3:72b"
	case memoryGB >= 96:
		return "qwen3:32b"
	case memoryGB >= 32:
		return "qwen3-coder:30b"
	case memoryGB >= 16:
		return "qwen3:8b"
	case memoryGB >= 8:
		return "qwen3:4b"
	default:
		return "qwen3:1.7b"
	}
}

// KeepAliveAlive モデルをアライブ状態に保つ
func (mr *ModelRouter) KeepAliveAlive(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if mr.mainProvider != nil {
				_ = mr.mainProvider.CheckHealth(ctx)
			}
			if mr.sidecarProvider != nil && mr.sidecarModel != "" {
				_ = mr.sidecarProvider.CheckHealth(ctx)
			}
		}
	}
}

// GetStatus ルーターのステータスを取得
func (mr *ModelRouter) GetStatus() RouterStatus {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	activeModel := mr.mainModel
	if mr.useSidecar {
		activeModel = mr.sidecarModel
	}

	return RouterStatus{
		MainModel:     mr.mainModel,
		SidecarModel:  mr.sidecarModel,
		ActiveModel:   activeModel,
		UsingSidecar:  mr.useSidecar,
		SidecarLoaded: mr.sidecarLoaded,
	}
}

// RouterStatus ルーターステータス
type RouterStatus struct {
	MainModel     string
	SidecarModel  string
	ActiveModel   string
	UsingSidecar  bool
	SidecarLoaded bool
}

// GetModelTier モデルのティアを取得
func (mr *ModelRouter) GetModelTier(model string) string {
	switch {
	case strings.Contains(model, "72b"):
		return "A"
	case strings.Contains(model, "70b"):
		return "B"
	case strings.Contains(model, "32b") || strings.Contains(model, "30b"):
		return "C"
	case strings.Contains(model, "8b"):
		return "D"
	case strings.Contains(model, "1.5b") || strings.Contains(model, "1.7b"):
		return "E"
	case strings.Contains(model, "7b"):
		return "D"
	case strings.Contains(model, "4b") || strings.Contains(model, "3b"):
		return "E"
	default:
		return "Unknown"
	}
}
