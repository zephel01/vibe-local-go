# マルチプロバイダー設計書

## 概要

vibe-local-go を Ollama 専用から、複数の LLM バックエンドに対応させる。
ローカル優先の思想を維持しつつ、クラウド LLM もフォールバックや選択肢として使えるようにする。

## 対応プロバイダー一覧

### ローカルプロバイダー

| プロバイダー | API形式 | モデル管理 | 特徴 |
|---|---|---|---|
| **Ollama** | OpenAI互換 `/v1/chat/completions` + 独自API (`/api/tags`, `/api/pull`) | あり（pull/list） | 最も簡単。自動モデルDL対応 |
| **llama-server** (llama.cpp) | OpenAI互換 `/v1/chat/completions` | なし（起動時に指定済） | 軽量・高速。KV cache制御可能 |
| **llama.app** (macOS) | llama-server と同じ（内部でllama-server起動） | GUIで管理 | macOSネイティブ |
| **LM Studio** | OpenAI互換 `/v1/chat/completions` | GUIで管理 | Windows/Mac GUI。ポート1234 |
| **LocalAI** | OpenAI互換 | あり | Docker ベース |

### クラウドプロバイダー

| プロバイダー | API形式 | 特徴 |
|---|---|---|
| **OpenAI** | OpenAI API | GPT-4o, o1 等。Function calling 完全対応 |
| **Anthropic** | Anthropic Messages API | Claude。独自フォーマット（tool_use block） |
| **Google Gemini** | OpenAI互換 or Gemini API | Gemini 2.x。OpenAI互換エンドポイントあり |
| **GLM / CodeGeeX** (智谱AI) | OpenAI互換 | GLM-4, CodeGeeX。中国発。コーディング強い |
| **DeepSeek** | OpenAI互換 | DeepSeek Coder V2。コスパ極めて高い |
| **Groq** | OpenAI互換 | 超高速推論。Llama/Mixtral |
| **Together AI** | OpenAI互換 | OSS モデル多数。安い |
| **OpenRouter** | OpenAI互換 | 全プロバイダー統合ゲートウェイ |

### 共通点の発見

**ほぼ全員が OpenAI 互換 API を持っている。** Anthropic だけが独自フォーマット。

これは設計上の大きなメリット：
- OpenAI互換クライアント1つで、ローカル・クラウド問わず大半をカバーできる
- 特殊対応が必要なのは Anthropic のみ

---

## 現在のアーキテクチャ（v1.1.0 実装済み）

```
現在 (v1.1.0):
Agent → LLMProvider (インターフェース) → ProviderChain → 複数プロバイダー

フロー:
1. main.go: createProviderWithChain() でプロバイダー初期化
2. --provider 指定あり → createProvider() + buildChainWithFallbacks()
3. --provider 未指定  → AutoDetect() → 自動チェーン構築
4. Agent は LLMProvider インターフェースのみ知る（具象型に依存しない）
5. ProviderChain が FallbackCondition に基づいてフォールバック制御
```

### 解決済みの課題（旧アーキテクチャからの改善）

1. ✅ Agent が `LLMProvider` インターフェースに依存（具象型依存を排除）
2. ✅ Ollama 固有機能は `ModelManager` 拡張インターフェースに分離
3. ✅ Config が複数プロバイダー対応（`Provider`, `CloudAPIKeys` フィールド）
4. ✅ main.go がゼロコンフィグ対応（`createProviderWithChain` で自動検出）
5. ✅ エラーメッセージが `ErrorMessage()` で動的生成（プロバイダー名付き）

---

## 設計方針

### 原則

1. **OpenAI互換をベースレイヤーにする** — 大半のプロバイダーがこれで動く
2. **プロバイダー固有機能は拡張インターフェースで** — モデル管理等は Optional
3. **初心者向け自動検出** — 設定なしでも動く。ローカルで何が動いてるか自動検知
4. **フォールバックチェーン** — メイン失敗時に次のプロバイダーへ自動切り替え

### レイヤー構造

```
┌──────────────────────────────────────────────────────────────────┐
│                        Agent Layer                               │
│  agent.go: LLMProvider インターフェースのみ知る                    │
└───────────────────────────┬──────────────────────────────────────┘
                            │
┌───────────────────────────▼──────────────────────────────────────┐
│                    Provider Router                                │
│  フォールバック制御 / ロードバランス / リトライ                     │
│  Chain: [Local Main] → [Local Sub] → [Cloud Fallback]            │
└───────────────────────────┬──────────────────────────────────────┘
                            │
         ┌──────────────────┼──────────────────┐
         ▼                  ▼                  ▼
┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐
│ OpenAI Compatible│ │ OpenAI Compatible│ │   Anthropic     │
│   Provider      │ │   Provider      │ │   Provider      │
│                 │ │                 │ │                 │
│ - Ollama        │ │ - llama-server  │ │ - Claude API    │
│ - LM Studio    │ │ - OpenAI        │ │                 │
│ - GLM/CodeGeeX │ │ - DeepSeek      │ │ (独自実装)       │
│ - Groq         │ │ - Together      │ │                 │
│ - OpenRouter   │ │ - Gemini        │ │                 │
│ - LocalAI      │ │                 │ │                 │
└────────┬────────┘ └────────┬────────┘ └────────┬────────┘
         │                   │                    │
  ┌──────▼──────┐     ┌─────▼──────┐      ┌─────▼──────┐
  │ Model Mgmt  │     │  (なし)    │      │  (なし)    │
  │ Extension   │     │            │      │            │
  │ - Pull      │     │            │      │            │
  │ - List      │     │            │      │            │
  │ - Search    │     │            │      │            │
  └─────────────┘     └────────────┘      └────────────┘
```

---

## インターフェース設計

### コアインターフェース（実装済み: `internal/llm/provider.go`）

```go
package llm

// LLMProvider - 全プロバイダーが実装する最小インターフェース
type LLMProvider interface {
    Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
    ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error)
    CheckHealth(ctx context.Context) error
    Info() ProviderInfo
}

// ProviderInfo プロバイダーのメタ情報
type ProviderInfo struct {
    Name     string       // "ollama", "llama-server", "openai", "anthropic", etc.
    Type     ProviderType // Local or Cloud
    BaseURL  string       // 接続先
    Model    string       // 使用中のモデル名
    Features Features     // 対応機能フラグ
}

type ProviderType string

const (
    ProviderTypeLocal ProviderType = "local"
    ProviderTypeCloud ProviderType = "cloud"
)

// Features プロバイダーの対応機能
type Features struct {
    NativeFunctionCalling bool // true: OpenAI式tool_calls対応
    ModelManagement       bool // true: モデルDL/一覧が可能
    Streaming             bool // true: SSEストリーミング対応
}
```

### 拡張インターフェース（実装済み: `internal/llm/provider.go`）

```go
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
```

> **設計メモ**: 当初予定していた `ContextReporter` インターフェースは未実装。
> `Vision`, `ContextWindowReport` フィールドも Features から除外。
> 必要に応じて将来追加可能。

---

## プロバイダー実装

### 1. OpenAI互換プロバイダー（ベース）

大半のプロバイダーはこれを共有する。差分だけオーバーライド。

```go
// OpenAICompatProvider OpenAI互換APIのベース実装
type OpenAICompatProvider struct {
    baseURL    string
    apiKey     string     // クラウド用。ローカルは空
    model      string
    httpClient *http.Client
    info       ProviderInfo
}

// これ1つで対応:
// - Ollama (/v1/chat/completions)
// - llama-server
// - llama.app
// - LM Studio
// - OpenAI
// - GLM / CodeGeeX
// - DeepSeek
// - Groq
// - Together AI
// - OpenRouter
// - Google Gemini (OpenAI互換モード)
// - LocalAI
```

### 2. Ollama プロバイダー（OpenAI互換 + ModelManager拡張）

```go
// OllamaProvider = OpenAICompatProvider + Ollama固有機能
type OllamaProvider struct {
    OpenAICompatProvider          // 埋め込み
    ollamaURL string              // Ollama API用URL（/api/...）
}

// ModelManager インターフェースを追加実装
func (o *OllamaProvider) ListModels(ctx context.Context) ([]ModelInfo, error) { ... }
func (o *OllamaProvider) PullModel(ctx context.Context, name string) error { ... }
func (o *OllamaProvider) PullModelWithProgress(ctx context.Context, name string, progressFn PullProgressCallback) error { ... }
func (o *OllamaProvider) CheckModel(ctx context.Context, name string) (bool, error) { ... }
func (o *OllamaProvider) SearchModels(ctx context.Context, query string) ([]ModelInfo, error) { ... }
```

#### PullModelWithProgress（進捗表示付きモデルダウンロード）

`/api/pull` を `"stream": true` で呼び出し、JSON Lines形式の進捗情報をリアルタイムで受信する。
コールバック関数 `PullProgressCallback func(status string, completed, total int64)` を通じて
呼び出し元にステータス・ダウンロード済みバイト数・総バイト数を通知する。

```
Ollama /api/pull ストリーミングレスポンス:
  {"status":"pulling manifest"}
  {"status":"pulling 9eba2761cf0b","digest":"sha256:...","total":4567890,"completed":1234567}
  {"status":"verifying sha256 digest"}
  {"status":"writing manifest"}
  {"status":"success"}
```

呼び出し元（main.go）では `total > 0` の場合にプログレスバーを表示:
```
  ██████████████░░░░░░░░░░░░░░░░  47.2% [1.9/4.0 GB]
```

#### モデル存在チェック＋自動pullフロー

セットアップ時（`addLocalProvider`）と編集時（`providerEdit`）に、手動入力されたモデル名が
Ollamaにダウンロード済みか確認し、未ダウンロードの場合は以下の選択肢を提示:

1. 今すぐダウンロード（プログレスバー付き `ollama pull`）
2. そのまま設定を保存（後で手動ダウンロード）
3. 既存のモデルから選び直す

起動時にも `pullModelIfNeeded()` で二重チェックを行い、モデル不在時はダウンロードまたは
既存モデルへの切り替えを案内する。

### 3. Anthropic プロバイダー（独自実装）

```go
// AnthropicProvider Claude API用
// リクエスト/レスポンスの形式変換が必要
type AnthropicProvider struct {
    apiKey     string
    model      string     // "claude-sonnet-4-20250514" etc.
    httpClient *http.Client
}

// ChatRequest → Anthropic Messages API 形式に変換
// - messages[].content → content blocks
// - tools → tool definitions (Anthropic形式)
// - tool_calls → tool_use blocks
// - tool results → tool_result blocks
```

### 4. llama-server プロバイダー

```go
// LlamaServerProvider = OpenAICompatProvider（そのまま）
// モデル管理なし。起動時にモデルは指定済み。
// 自動検出: localhost:8080 のデフォルトポートをチェック
type LlamaServerProvider struct {
    OpenAICompatProvider
}

// llama.app も同じ（内部でllama-server起動）
// ポートが異なる可能性あり → 自動検出でカバー
```

---

## プロバイダーチェーン（フォールバック）— 実装済み

### 設計（実装: `internal/llm/chain.go`）

```go
// ChainRole プロバイダーチェーンでの役割
type ChainRole string

const (
    RoleMain     ChainRole = "main"      // メインプロバイダー
    RoleSub      ChainRole = "sub"       // サブ（ローカル別モデル）
    RoleFallback ChainRole = "fallback"  // フォールバック（クラウド等）
)

// ChainEntry チェーンエントリ
type ChainEntry struct {
    Provider LLMProvider
    Role     ChainRole
    Priority int // 低い値が優先
}

// FallbackCallback フォールバック発生時のコールバック
type FallbackCallback func(fromProvider, toProvider string, classification ErrorClassification)

// ProviderChain フォールバック付きプロバイダーチェーン
type ProviderChain struct {
    entries      []ChainEntry
    current      int
    lastError    error
    failureCount map[int]int        // プロバイダーごとの失敗カウント
    failureTime  map[int]time.Time  // プロバイダーごとの最後の失敗時刻
    fallbackOn   bool               // フォールバック有効化フラグ
    maxRetries   int
    condition    FallbackCondition   // フォールバック条件（fallback_condition.go）
    onFallback   FallbackCallback    // UI通知コールバック
    mu           sync.RWMutex
}
```

#### 主要メソッド

| メソッド | 説明 |
|---|---|
| `NewProviderChain(providers...)` | チェーン作成。最初=Main、2番目=Sub、以降=Fallback |
| `AddProvider(provider, role)` | プロバイダーを動的追加 |
| `SwitchTo(index)` | 指定インデックスに手動切り替え（`/chain` コマンドで使用） |
| `SetFallbackCondition(cond)` | フォールバック条件を設定 |
| `SetFallbackCallback(cb)` | フォールバック発生時のUI通知コールバックを設定 |
| `EnableFallback(bool)` | フォールバック機能の有効/無効切り替え |
| `GetEntries()` | チェーン内全エントリ一覧 |
| `GetFailureCount(index)` | プロバイダーごとの失敗回数取得 |

### エラー分類（実装: `internal/llm/fallback_condition.go`）

```go
// FallbackCondition フォールバック条件
type FallbackCondition struct {
    OnNetworkError   bool          // 接続不可エラー時にフォールバック
    OnTimeout        bool          // タイムアウト時にフォールバック
    OnServerError    bool          // 5xx エラー時にフォールバック
    OnContextWindow  bool          // コンテキスト超過時にフォールバック
    OnRateLimit      bool          // レート制限時にフォールバック
    MaxRetries       int
    RetryDelay       time.Duration
}

// ErrorClassification: network, timeout, server_error, client_error,
//                      context_window, rate_limit, unknown

// ClassifyError(err) → ErrorClassification
// EvaluateFallback(err, condition) → bool
// GetRetryDelay(classification, attempt) → time.Duration  (指数バックオフ対応)
// ErrorMessage(classification, from, to) → string         (UI表示用メッセージ生成)
```

デフォルト条件: ネットワーク/タイムアウト/サーバーエラー/コンテキスト超過でフォールバック。
レート制限はリトライで対応（フォールバックしない）。4xx クライアントエラーもフォールバックしない。

### フォールバック動作フロー

```
ユーザーリクエスト
    │
    ▼
[Main: Ollama qwen3:8b (ローカル)]
    │ 成功 → 応答返却
    │ 失敗 → ClassifyError() で分類
    │        → EvaluateFallback() で判定
    │        → GetRetryDelay() で待機
    │        → FallbackCallback で UI 通知
    ▼
[Sub: llama-server (ローカル別モデル)]
    │ 成功 → 応答返却
    │ 失敗 ↓
    ▼
[Fallback: OpenAI gpt-4o (クラウド)]
    │ 成功 → 応答返却
    │ 失敗 ↓
    ▼
エラー表示「all providers failed, last error: ...」
```

### /chain コマンド（実装: `cmd/vibe/main.go` `registerChainCommands()`）

```
/chain              チェーン状態を表示（各プロバイダーの名前・ロール・失敗数）
/chain <番号>       指定番号のプロバイダーに手動切り替え
```

---

## 自動検出（ゼロコンフィグ対応）— 実装済み

`internal/llm/autodetect.go` に実装。`--provider` 未指定時に `createProviderWithChain()` から呼ばれる。

### 検出対象と優先順位

| 優先度 | プロバイダー | ポート | エンドポイント | モデルパーサー |
|---|---|---|---|---|
| 0 | Ollama | 11434 | `/api/tags` | `parseOllamaModels` |
| 1 | llama-server | 8080 | `/v1/models` | `parseLlamaServerModels` |
| 2 | LM Studio | 1234 | `/api/v1/models` (Native REST API 0.4.0+) | `parseLMStudioNativeModels` |
| 3 | LiteLLM | 4000 | `/v1/models` | `parseLlamaServerModels` |
| 4 | カスタム | (任意) | `/v1/models` | `parseLlamaServerModels` |

- 全プロバイダーを **goroutine で並行チェック**（タイムアウト 2秒）
- カスタムプロバイダーは環境変数 `VIBE_LLM_URL` で指定
- 検出結果は優先度順にソートして返却
- 各プロバイダーの `DetectedProvider` にモデル一覧も含む

### DetectedProvider 構造体

```go
type DetectedProvider struct {
    Name     string     // "ollama", "llama-server", "lm-studio", "litellm", "custom"
    URL      string     // "http://localhost:11434"
    Models   []string   // ["qwen3:8b", "mistral:7b"]
    Health   bool       // 検出成功フラグ
    Features Features   // 対応機能フラグ
    BasePort int        // 検出ポート
}
```

### ゼロコンフィグの全体フロー（`createProviderWithChain`）

```
vibe 起動（--provider 未指定）
    │
    ▼
AutoDetect() → goroutine で並行チェック (2秒タイムアウト)
    │
    ├─ 検出あり → best = detected[0] をメインに設定
    │             他の detected をサブとして AddProvider
    │             環境変数からクラウドフォールバック追加
    │             → ProviderChain 返却
    │
    └─ 検出なし → detectCloudFromEnv() でクラウドAPIキーをチェック
                  ├─ APIキーあり → クラウドプロバイダー返却
                  └─ APIキーなし → デフォルト Ollama で試行
```

### 補助関数

- `DetectProvidersByPort(ctx, ports)` — カスタムポートでの検出（設定変更時用）
- `(*DetectedProvider).IsReachable(ctx)` — 再到達チェック（1秒タイムアウト）
- `(*DetectedProvider).ToProviderInfo(model)` — `ProviderInfo` への変換

---

## 設定ファイル

### 新しい Config 構造

```go
type Config struct {
    // === プロバイダー設定 ===
    Providers []ProviderConfig  // 優先順位順

    // === 旧 Ollama 設定（後方互換） ===
    OllamaHost string          // 互換用。Providers未設定時に使用

    // === 共通設定 ===
    Model         string
    SidecarModel  string
    AutoModel     bool
    MaxTokens     int
    Temperature   float64
    ContextWindow int

    // ... (既存フィールド)
}

type ProviderConfig struct {
    Name     string            `json:"name"`      // "ollama", "llama-server", "openai", etc.
    Type     string            `json:"type"`      // "local" or "cloud"
    URL      string            `json:"url"`       // エンドポイント
    APIKey   string            `json:"api_key"`   // クラウド用（環境変数参照可）
    Model    string            `json:"model"`     // このプロバイダーで使うモデル
    Role     string            `json:"role"`      // "main", "sub", "fallback"
    Priority int               `json:"priority"`  // 低いほど優先
    Options  map[string]string `json:"options"`   // プロバイダー固有オプション
}
```

### 設定ファイル例: `~/.config/vibe-local/config.json`

```json
{
  "providers": [
    {
      "name": "ollama",
      "type": "local",
      "url": "http://localhost:11434",
      "model": "qwen3:8b",
      "role": "main",
      "priority": 1
    },
    {
      "name": "llama-server",
      "type": "local",
      "url": "http://localhost:8080",
      "model": "",
      "role": "sub",
      "priority": 2,
      "options": {
        "note": "モデルはサーバー起動時に指定済み"
      }
    },
    {
      "name": "openai",
      "type": "cloud",
      "url": "https://api.openai.com",
      "api_key": "${OPENAI_API_KEY}",
      "model": "gpt-4o",
      "role": "fallback",
      "priority": 10
    }
  ]
}
```

### CLIフラグ（後方互換 + 新規）

```
# 後方互換（既存）
--host <url>          Ollama URL（= --provider ollama --url <url> と同じ）
--model <name>        メインモデル

# 新規
--provider <name>     プロバイダー指定 (ollama, llama-server, openai, anthropic, ...)
--url <url>           プロバイダーURL
--api-key <key>       APIキー（クラウド用）
--fallback <name>     フォールバックプロバイダー
--auto-detect         自動検出（デフォルト: true）
--list-providers      検出済みプロバイダー一覧
```

### 環境変数

```bash
# プロバイダー固有
OLLAMA_HOST=http://localhost:11434
LLAMA_SERVER_URL=http://localhost:8080
LM_STUDIO_URL=http://localhost:1234

# クラウド API キー
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
DEEPSEEK_API_KEY=sk-...
GLM_API_KEY=...
GROQ_API_KEY=gsk_...
TOGETHER_API_KEY=...
OPENROUTER_API_KEY=sk-or-...

# 汎用
VIBE_LLM_URL=http://localhost:8080    # カスタムOpenAI互換サーバー
VIBE_LLM_API_KEY=...                  # 汎用APIキー
VIBE_PROVIDER=ollama                  # デフォルトプロバイダー
```

---

## Anthropic プロバイダーの変換仕様（未実装 — 設計メモ）

> **注**: 現在 Anthropic は OpenRouter 等の OpenAI 互換ゲートウェイ経由で利用可能。
> ネイティブ Messages API の直接対応は将来の課題。

Anthropic だけ OpenAI 互換ではないため、リクエスト/レスポンスの変換が必要。

### リクエスト変換

```
OpenAI形式 (内部)              →  Anthropic Messages API
────────────────────────────────────────────────────────
messages[].role: "system"      →  system パラメータ (トップレベル)
messages[].role: "user"        →  messages[].role: "user"
messages[].role: "assistant"   →  messages[].role: "assistant"
messages[].role: "tool"        →  messages[].content: [{type: "tool_result", ...}]

tools[].function               →  tools[].name, tools[].input_schema
tool_choice: "auto"            →  tool_choice: {type: "auto"}
tool_choice: "required"        →  tool_choice: {type: "any"}

temperature, max_tokens        →  そのまま
```

### レスポンス変換

```
Anthropic Messages API          →  OpenAI形式 (内部)
────────────────────────────────────────────────────────
content[].type: "text"          →  choices[0].message.content
content[].type: "tool_use"      →  choices[0].message.tool_calls[]
stop_reason: "end_turn"         →  choices[0].finish_reason: "stop"
stop_reason: "tool_use"         →  choices[0].finish_reason: "tool_calls"
usage.input_tokens              →  usage.prompt_tokens
usage.output_tokens             →  usage.completion_tokens
```

---

## XML フォールバックとの統合

小さいローカルモデル（1.7b〜8b）は OpenAI 式 function calling に対応していない場合がある。
既存の `xml_fallback.go` をプロバイダーレベルで統合。

```go
// OpenAICompatProvider の Chat で自動判定
func (p *OpenAICompatProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
    resp, err := p.doRequest(ctx, req)
    if err != nil {
        return nil, err
    }

    // ネイティブ tool_calls が返ってきた → そのまま返す
    if len(resp.Choices) > 0 && len(resp.Choices[0].Message.ToolCalls) > 0 {
        return resp, nil
    }

    // テキスト応答のみ → XMLフォールバックでtool_calls抽出を試みる
    if len(resp.Choices) > 0 && resp.Choices[0].Message.Content != "" {
        if req.Tools != nil && len(req.Tools) > 0 {
            knownTools := extractToolNames(req.Tools)
            calls, err := ExtractToolCallsFromText(resp.Choices[0].Message.Content, knownTools)
            if err == nil && len(calls) > 0 {
                resp.Choices[0].Message.ToolCalls = calls
                resp.Choices[0].FinishReason = "tool_calls"
            }
        }
    }

    return resp, nil
}
```

---

## ModelRouter（メイン/サイドカーモデル切り替え）— 実装済み

`internal/llm/routing.go` に実装。同一プロバイダー内でメインモデルとサイドカーモデルを切り替える。

```go
type ModelRouter struct {
    mainProvider    LLMProvider
    sidecarProvider LLMProvider
    mainModel       string    // e.g. "qwen3:8b"
    sidecarModel    string    // e.g. "qwen3:32b"
    useSidecar      bool
}
```

- `LLMProvider` インターフェースを実装（Agent から透過的に使える）
- `AutoSelectModel(taskType)` — タスク種別に応じて自動選択（code_generation → main、code_review → sidecar）
- `SelectModelByMemory(memoryGB)` — メモリ量に応じた推奨モデル選択
- `SwapModelHot(ctx, toSidecar)` — 実行時ホットスワップ
- `KeepAliveAlive(ctx, interval)` — 定期的にヘルスチェックしてモデルをアライブ状態に維持
- `GetModelTier(model)` — モデルのパラメータ数に基づくティア判定（A〜E）

> **注**: ProviderChain はプロバイダー間のフォールバック、ModelRouter は同一プロバイダー内のモデル切り替え。
> 両者は独立した機能で、組み合わせて使用可能。

---

## 実装フェーズと進捗

### Phase 1: インターフェース抽出 ✅ 完了

- `LLMProvider` インターフェース定義 (`provider.go`)
- `OpenAICompatProvider` 実装 (`ollama.go` — Ollama = OpenAI互換ベース)
- `ModelManager`, `ModelSwitcher` 拡張インターフェース
- `Agent.provider llm.LLMProvider` に変更済み
- 既存テスト全パス

ファイル構成:
```
internal/llm/
  provider.go           # LLMProvider インターフェース + Features + ModelManager
  ollama.go             # Ollama プロバイダー（OpenAI互換 + モデル管理）
  chain.go              # ProviderChain（フォールバック付き）
  fallback_condition.go # エラー分類 + フォールバック条件
  autodetect.go         # ゼロコンフィグ自動検出
  routing.go            # ModelRouter（メイン/サイドカー切り替え）
  xml_fallback.go       # XMLフォールバック（小モデル用）
  sync.go               # 同期チャット
  streaming.go          # ストリーミングチャット
```

### Phase 2: llama-server / LM Studio 対応 ✅ 完了

- `--provider`, `--url` CLIフラグ対応
- `AutoDetect()` で llama-server (8080), LM Studio (1234), LiteLLM (4000) を自動検出
- LM Studio Native REST API (0.4.0+) の `/api/v1/models` 対応

### Phase 3: クラウドプロバイダー対応 ✅ 部分完了

- OpenAI互換クラウド対応（OpenAI, DeepSeek, OpenRouter 等）
- 環境変数からの APIキー自動検出 (`CloudAPIKeys` マップ)
- `NewCloudProvider()`, `GetCloudProviderDef()` でクラウドプロバイダー作成
- **未実装**: Anthropic ネイティブ API（独自変換レイヤー）

### Phase 4: フォールバックチェーン ✅ 完了

- `ProviderChain` 実装（`chain.go`）
- `FallbackCondition` によるエラー分類ベースの自動フォールバック
- `ClassifyError()` / `EvaluateFallback()` / `GetRetryDelay()` / `ErrorMessage()`
- `/chain` コマンドで状態表示・手動切り替え
- `FallbackCallback` による UI 通知

### Phase 5: ゼロコンフィグ ✅ 完了

- `AutoDetect()` による goroutine 並行検出
- 検出結果からの自動チェーン構築 (`createProviderWithChain`)
- 環境変数クラウドフォールバック自動追加 (`addCloudFallbackToChain`)
- **未実装**: セットアップウィザード（初心者向け対話形式設定）

---

## 後方互換性

### 既存ユーザーへの影響

```
変更前: vibe --model qwen3:8b --host http://localhost:11434
変更後: vibe --model qwen3:8b --host http://localhost:11434  ← 同じ

--host は内部で providers[0] = {name: "ollama", url: <host>} に変換。
config.json の "OllamaHost" も引き続き動作。
```

### マイグレーション

```json
// 旧 config.json
{
  "OllamaHost": "http://localhost:11434",
  "Model": "qwen3:8b"
}

// → 内部で自動変換 →

// 新 config.json 相当
{
  "providers": [
    {"name": "ollama", "url": "http://localhost:11434", "model": "qwen3:8b", "role": "main"}
  ]
}
```

---

## Agent 側の変更点（実装済み）

Agent は `llm.LLMProvider` インターフェースのみに依存。
具象型（Ollama, ProviderChain 等）を知らない。

```go
type Agent struct {
    provider llm.LLMProvider  // インターフェース依存（具象型に依存しない）
    ...
}

func NewAgent(provider llm.LLMProvider, ...) *Agent {
    return &Agent{provider: provider, ...}
}
```

`main.go` 側で `createProviderWithChain()` が返す `LLMProvider`（単一プロバイダーまたは ProviderChain）を Agent に渡す。
Agent からは単一プロバイダーもチェーンも同じインターフェースで扱える。

---

## 設定例集

### 例1: Ollama のみ（初心者デフォルト）

```bash
# 何も設定しなくても自動検出で動く
vibe
```

### 例2: llama-server をメインで使う

```bash
# llama-server 起動済み (localhost:8080)
vibe --provider llama-server --url http://localhost:8080
```

### 例3: ローカルメイン + クラウドフォールバック

```json
{
  "providers": [
    {"name": "ollama", "url": "http://localhost:11434", "model": "qwen3:8b", "role": "main"},
    {"name": "deepseek", "url": "https://api.deepseek.com", "api_key": "${DEEPSEEK_API_KEY}", "model": "deepseek-coder", "role": "fallback"}
  ]
}
```

### 例4: GLM / CodeGeeX でコーディング

```bash
vibe --provider glm --api-key $GLM_API_KEY --model glm-4-plus
```

### 例5: OpenRouter 経由で好きなモデル

```bash
vibe --provider openrouter --api-key $OPENROUTER_API_KEY --model anthropic/claude-sonnet-4
```

### 例6: ローカル2台構成

```json
{
  "providers": [
    {"name": "ollama", "url": "http://localhost:11434", "model": "qwen3:8b", "role": "main", "priority": 1},
    {"name": "ollama", "url": "http://192.168.1.100:11434", "model": "qwen3:32b", "role": "sub", "priority": 2}
  ]
}
```

---

## コスト・レート制御（クラウド用）— 部分実装

### 実装済み

- **レート制限検出**: `ClassifyError()` が `ErrorClassRateLimit` を検出（`"rate limit"`, `"too many requests"`, `"quota"` を含むエラー）
- **指数バックオフ**: `GetRetryDelay(ErrorClassRateLimit, attempt)` で 1s → 2s → 4s... (最大30s) のバックオフ
- **フォールバック通知**: `FallbackCallback` + `ErrorMessage()` で UI にフォールバック理由を表示

### 未実装（将来の課題）

- `CloudLimiter`（1日あたりのトークン数・コスト上限管理）
- 日次コスト追跡と自動停止
- クラウドフォールバック時のコスト警告表示

---

## まとめ

### 設計のポイント

1. **OpenAI互換がベース**: 1つの実装で Ollama / llama-server / LM Studio / LiteLLM / クラウドをカバー
2. **LLMProvider インターフェース**: Agent が具象型に依存しない設計
3. **FallbackCondition**: エラー分類ベースの柔軟なフォールバック制御
4. **自動検出優先**: `--provider` 未指定で goroutine 並行検出 → チェーン自動構築
5. **フォールバックチェーン**: ローカル失敗 → 別ローカル → クラウド（自動 + 手動切り替え）

### 実装状況 (v1.1.0)

| Phase | 内容 | 状態 |
|---|---|---|
| Phase 1 | インターフェース抽出 | ✅ 完了 |
| Phase 2 | llama-server + 自動検出 | ✅ 完了 |
| Phase 3 | クラウドプロバイダー | ⚠️ 部分完了（Anthropic ネイティブ未実装） |
| Phase 4 | フォールバックチェーン | ✅ 完了 |
| Phase 5 | ゼロコンフィグ | ✅ 完了（ウィザード未実装） |

### 今後の課題

- Anthropic ネイティブ API 対応（Messages API 変換レイヤー）
- CloudLimiter（コスト制御・日次上限）
- セットアップウィザード（初心者向け対話形式設定）

---

**更新日**: 2026-02-25 (v1.1.0 実装反映)
