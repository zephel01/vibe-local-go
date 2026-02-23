package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ModelRouter モデルルーター
type ModelRouter struct {
	mainClient     *Client
	sidecarClient  *Client
	mainModel      string
	sidecarModel   string
	useSidecar     bool
	sidecarLoaded  bool
	mu             sync.RWMutex
}

// NewModelRouter 新しいモデルルーターを作成
func NewModelRouter(mainClient, sidecarClient *Client, mainModel, sidecarModel string) *ModelRouter {
	return &ModelRouter{
		mainClient:    mainClient,
		sidecarClient: sidecarClient,
		mainModel:     mainModel,
		sidecarModel:  sidecarModel,
		useSidecar:    false,
		sidecarLoaded: false,
	}
}

// PreloadSidecar サイドカーモデルをプリロード
func (mr *ModelRouter) PreloadSidecar(ctx context.Context) error {
	if mr.sidecarClient == nil || mr.sidecarModel == "" {
		return nil
	}

	mr.mu.Lock()
	defer mr.mu.Unlock()

	// すでにロード済み
	if mr.sidecarLoaded {
		return nil
	}

	// サイドカーをロード
	err := mr.sidecarClient.PullModel(ctx, mr.sidecarModel)
	if err != nil {
		return fmt.Errorf("サイドカーモデルのロードに失敗: %w", err)
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

// GetActiveClient アクティブなクライアントを取得
func (mr *ModelRouter) GetActiveClient() *Client {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	if mr.useSidecar {
		return mr.sidecarClient
	}
	return mr.mainClient
}

// Chat メイン/サイドカーを自動選択してチャット
func (mr *ModelRouter) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	// 現在のアクティブなクライアントを使用
	client := mr.GetActiveClient()

	// モデル名を更新
	req.Model = mr.GetActiveModel()

	return client.ChatSync(ctx, req)
}

// ChatStream ストリーミングチャット
func (mr *ModelRouter) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error) {
	// 現在のアクティブなクライアントを使用
	client := mr.GetActiveClient()

	// モデル名を更新
	req.Model = mr.GetActiveModel()

	return client.ChatStream(ctx, req)
}

// AutoSelectModel タスクに基づいてモデルを自動選択
func (mr *ModelRouter) AutoSelectModel(taskType string) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	// タスクタイプに基づいて選択
	switch taskType {
	case "code_generation":
		// コード生成はメインモデル
		mr.useSidecar = false
	case "code_review":
		// コードレビューはサイドカー（より詳細な分析）
		if mr.sidecarModel != "" {
			mr.useSidecar = true
		} else {
			mr.useSidecar = false
		}
	case "documentation":
		// ドキュメンテーションはサイドカー
		if mr.sidecarModel != "" {
			mr.useSidecar = true
		} else {
			mr.useSidecar = false
		}
	case "quick_edit":
		// クイック編集はメインモデル（高速）
		mr.useSidecar = false
	default:
		// デフォルトはメインモデル
		mr.useSidecar = false
	}
}

// SwapModelHot モデルをホットスワップ
func (mr *ModelRouter) SwapModelHot(ctx context.Context, toSidecar bool) error {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	// 現在の状態と同じなら何もしない
	if mr.useSidecar == toSidecar {
		return nil
	}

	// 切り替え先のモデルをロード済みチェック
	if toSidecar && !mr.sidecarLoaded && mr.sidecarClient != nil {
		// サイドカーをロード
		err := mr.sidecarClient.PullModel(ctx, mr.sidecarModel)
		if err != nil {
			return fmt.Errorf("サイドカーモデルのロードに失敗: %w", err)
		}
		mr.sidecarLoaded = true
	}

	// 切り替え
	mr.useSidecar = toSidecar

	return nil
}

// SelectModelByMemory メモリ量に基づいてモデルを選択
func (mr *ModelRouter) SelectModelByMemory(memoryGB float64) string {
	// メモリティアに基づいて推奨モデルを返す
	switch {
	case memoryGB >= 256:
		// Tier A: qwen3:72b
		return "qwen3:72b"
	case memoryGB >= 96:
		// Tier B: qwen3:32b
		return "qwen3:32b"
	case memoryGB >= 32:
		// Tier C上位: qwen3-coder:30b
		return "qwen3-coder:30b"
	case memoryGB >= 16:
		// Tier C中位: qwen3:8b
		return "qwen3:8b"
	case memoryGB >= 8:
		// Tier D: qwen3:4b
		return "qwen3:4b"
	default:
		// Tier E: qwen3:1.7b
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
			// 軽いリクエストを送信してアライブを維持
			if mr.mainClient != nil {
				// メインモデルをチェック
				_ = mr.mainClient.CheckConnection(ctx)
			}
			if mr.sidecarClient != nil && mr.sidecarModel != "" {
				// サイドカーをチェック
				_ = mr.sidecarClient.CheckConnection(ctx)
			}
		}
	}
}

// GetStatus ルーターのステータスを取得
func (mr *ModelRouter) GetStatus() RouterStatus {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	return RouterStatus{
		MainModel:     mr.mainModel,
		SidecarModel:  mr.sidecarModel,
		ActiveModel:   mr.GetActiveModel(),
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
	// モデル名からティアを判定
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
