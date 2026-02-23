package llm

import "context"

// LLMProvider - 全プロバイダーが実装する最小インターフェース
type LLMProvider interface {
	// Chat 同期チャットリクエスト（ツール使用時）
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// ChatStream ストリーミングチャット（対話時）
	ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error)

	// CheckHealth プロバイダーの生存確認
	CheckHealth(ctx context.Context) error

	// Info プロバイダー情報
	Info() ProviderInfo
}

// ProviderType プロバイダーの種別
type ProviderType string

const (
	// ProviderTypeLocal ローカルプロバイダー
	ProviderTypeLocal ProviderType = "local"
	// ProviderTypeCloud クラウドプロバイダー
	ProviderTypeCloud ProviderType = "cloud"
)

// ProviderInfo プロバイダーのメタ情報
type ProviderInfo struct {
	Name     string       // "ollama", "llama-server", "openai", "anthropic", etc.
	Type     ProviderType // Local or Cloud
	BaseURL  string       // 接続先
	Model    string       // 使用中のモデル名
	Features Features     // 対応機能フラグ
}

// Features プロバイダーの対応機能
type Features struct {
	NativeFunctionCalling bool // true: OpenAI式tool_calls対応
	ModelManagement       bool // true: モデルDL/一覧が可能
	Streaming             bool // true: SSEストリーミング対応
}

// ModelManager モデル管理ができるプロバイダー用（Ollama等）
type ModelManager interface {
	ListModels(ctx context.Context) ([]string, error)
	PullModel(ctx context.Context, name string) error
	CheckModel(ctx context.Context, name string) (bool, error)
}

// ModelSwitcher モデルを動的に切り替え可能なプロバイダー用
type ModelSwitcher interface {
	GetModel() string
	SetModel(model string)
}
