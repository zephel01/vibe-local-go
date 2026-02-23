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

## 現状のアーキテクチャと問題点

```
現在:
Agent → *llm.Client (Ollama専用・具象型) → Ollama HTTP API

問題:
1. Agent が *llm.Client に直接依存（インターフェースなし）
2. Ollama 固有の API (CheckModel, PullModel) がクライアントに混在
3. Config が OllamaHost しか持たない
4. main.go が Ollama 前提のフロー (checkOllamaConnection, pullModelIfNeeded)
5. エラーメッセージが "Ollama error" ハードコード
```

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

### コアインターフェース

```go
package llm

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

// ProviderInfo プロバイダーのメタ情報
type ProviderInfo struct {
    Name        string       // "ollama", "llama-server", "openai", "anthropic", etc.
    Type        ProviderType // Local or Cloud
    BaseURL     string       // 接続先
    Model       string       // 使用中のモデル名
    Features    Features     // 対応機能フラグ
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
    Vision                bool // true: 画像入力対応
    ContextWindowReport   bool // true: モデルのctx sizeを返せる
}
```

### 拡張インターフェース（Optional）

```go
// ModelManager モデル管理ができるプロバイダー用（Ollama等）
type ModelManager interface {
    ListModels(ctx context.Context) ([]ModelInfo, error)
    PullModel(ctx context.Context, name string) error
    SearchModels(ctx context.Context, query string) ([]ModelInfo, error)
    DeleteModel(ctx context.Context, name string) error
}

// ModelInfo モデル情報
type ModelInfo struct {
    Name         string
    Size         int64   // バイト
    ContextSize  int     // コンテキストウィンドウ
    Family       string  // "qwen3", "llama3", etc.
    ParameterSize string // "8b", "32b", etc.
    Quantization string  // "Q4_K_M", "Q8_0", etc.
}

// ContextReporter コンテキストウィンドウ情報を返せるプロバイダー用
type ContextReporter interface {
    GetContextWindow(ctx context.Context, model string) (int, error)
    GetTokenUsage(ctx context.Context) (*Usage, error)
}
```

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

## プロバイダーチェーン（フォールバック）

### 設計

```go
// ProviderChain フォールバック付きプロバイダーチェーン
type ProviderChain struct {
    providers []ChainEntry
    current   int
    mu        sync.RWMutex
}

type ChainEntry struct {
    Provider  LLMProvider
    Role      ChainRole    // Main, Sub, Fallback
    Priority  int          // 低い値が優先
    Condition *ChainCondition // 切り替え条件
}

type ChainRole string

const (
    RoleMain     ChainRole = "main"      // メインプロバイダー
    RoleSub      ChainRole = "sub"       // サブ（ローカル別モデル）
    RoleFallback ChainRole = "fallback"  // フォールバック（クラウド等）
)

type ChainCondition struct {
    OnError     bool   // エラー時に切り替え
    OnTimeout   bool   // タイムアウト時に切り替え
    OnOverload  bool   // コンテキスト超過時に切り替え（大きいctxのモデルへ）
    TaskType    string // 特定タスク時のみ切り替え（code_review→大モデル等）
}
```

### フォールバック動作

```
ユーザーリクエスト
    │
    ▼
[Main: Ollama qwen3:8b (ローカル)]
    │ 成功 → 応答返却
    │ 失敗 ↓
    ▼
[Sub: llama-server codestral:22b (ローカル別モデル)]
    │ 成功 → 応答返却 + "⚠ サブモデルで応答" 表示
    │ 失敗 ↓
    ▼
[Fallback: OpenAI gpt-4o (クラウド)]
    │ 成功 → 応答返却 + "☁ クラウドフォールバック" 表示
    │ 失敗 ↓
    ▼
エラー表示（全プロバイダー失敗）
```

---

## 自動検出（ゼロコンフィグ対応）

初心者向け：何も設定しなくても、ローカルで動いているLLMサーバーを自動検出。

### 検出順序

```go
// AutoDetect ローカルで起動中のLLMサーバーを自動検出
func AutoDetect(ctx context.Context) []DetectedProvider {
    var detected []DetectedProvider

    // 並行チェック（タイムアウト2秒）
    checks := []struct {
        name    string
        url     string
        check   func(ctx context.Context, url string) bool
    }{
        // 1. Ollama (デフォルトポート 11434)
        {"ollama", "http://localhost:11434", checkOllama},

        // 2. llama-server / llama.app (デフォルトポート 8080)
        {"llama-server", "http://localhost:8080", checkOpenAICompat},

        // 3. LM Studio (デフォルトポート 1234)
        {"lm-studio", "http://localhost:1234", checkOpenAICompat},

        // 4. LocalAI (デフォルトポート 8080)
        // llama-serverと競合するが、/models レスポンスで判別
        {"localai", "http://localhost:8080", checkLocalAI},

        // 5. カスタムポート (環境変数 VIBE_LLM_URL)
        {"custom", os.Getenv("VIBE_LLM_URL"), checkOpenAICompat},
    }

    // 全ポート並行チェック
    for _, c := range checks {
        if c.url != "" && c.check(ctx, c.url) {
            detected = append(detected, DetectedProvider{
                Name: c.name,
                URL:  c.url,
            })
        }
    }

    return detected
}

func checkOllama(ctx context.Context, url string) bool {
    // GET /api/tags が 200 返すか
    resp, err := http.Get(url + "/api/tags")
    return err == nil && resp.StatusCode == 200
}

func checkOpenAICompat(ctx context.Context, url string) bool {
    // GET /v1/models が 200 返すか
    resp, err := http.Get(url + "/v1/models")
    return err == nil && resp.StatusCode == 200
}
```

### 検出結果の表示（初心者向け）

```
🔍 LLMサーバーを検出中...
  ✓ Ollama (localhost:11434) - qwen3:8b, qwen3:32b が利用可能
  ✓ llama-server (localhost:8080) - モデルロード済み
  ✗ LM Studio (localhost:1234) - 未検出

→ Ollama (qwen3:8b) をメインで使用します
→ llama-server をサブで使用します
```

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

## Anthropic プロバイダーの変換仕様

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

## 実装フェーズ

### Phase 1: インターフェース抽出（破壊的変更なし）

**目標**: 既存動作を壊さずにインターフェースを導入

1. `LLMProvider` インターフェース定義
2. `OpenAICompatProvider` 実装（既存 `Client` コードを移動）
3. `OllamaProvider` 実装（`OpenAICompatProvider` 埋め込み + モデル管理）
4. `Agent.client *llm.Client` → `Agent.provider llm.LLMProvider` に変更
5. `main.go` で `OllamaProvider` を生成して渡す
6. 既存テスト全パス確認

ファイル構成:
```
internal/llm/
  provider.go           # LLMProvider インターフェース定義
  openai_compat.go      # OpenAI互換ベース実装（旧 client.go + sync.go + streaming.go）
  ollama.go             # Ollama固有機能（旧 client.go のモデル管理部分）
  chain.go              # ProviderChain
  xml_fallback.go       # (既存)
```

### Phase 2: llama-server / llama.app 対応

1. `LlamaServerProvider` 実装（= `OpenAICompatProvider` そのまま）
2. 自動検出ロジック (`autodetect.go`)
3. `--provider` / `--url` CLIフラグ追加
4. `config.json` のプロバイダー設定対応

### Phase 3: クラウドプロバイダー対応

1. APIキー管理（環境変数 + config.json、`${ENV_VAR}` 展開）
2. OpenAI / DeepSeek / GLM / Groq / Together / OpenRouter
   → 全て `OpenAICompatProvider` の URL + APIKey 差し替えで対応
3. `AnthropicProvider` 実装（独自変換）

### Phase 4: フォールバックチェーン

1. `ProviderChain` 実装
2. エラー時の自動フォールバック
3. コンテキスト超過時の大モデルへの切り替え
4. チェーン状態のUI表示（右パネル / ステータスバー）

### Phase 5: 自動検出＋ゼロコンフィグ

1. ローカルサーバー並行検出
2. 環境変数からクラウドプロバイダー自動設定
3. 検出結果からの自動チェーン構築
4. 初心者向けセットアップウィザード

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

## Agent 側の変更点

### Before

```go
type Agent struct {
    client *llm.Client    // Ollama直接依存
    ...
}

func NewAgent(client *llm.Client, ...) *Agent {
    return &Agent{client: client, ...}
}
```

### After

```go
type Agent struct {
    provider llm.LLMProvider  // インターフェース依存
    ...
}

func NewAgent(provider llm.LLMProvider, ...) *Agent {
    return &Agent{provider: provider, ...}
}

// callLLM は変更なし（ChatRequest/ChatResponse は共通型）
func (a *Agent) callLLM(ctx context.Context, messages []llm.Message, tools []llm.ToolDef) (*llm.ChatResponse, error) {
    req := &llm.ChatRequest{
        Model:       a.config.Model,
        Messages:    messages,
        Tools:       tools,
        Temperature: a.config.Temperature,
        MaxTokens:   a.config.MaxTokens,
    }
    return a.provider.Chat(ctx, req)  // client.ChatSync → provider.Chat
}
```

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

## コスト・レート制御（クラウド用）

```go
// CloudLimiter クラウドプロバイダーのコスト制御
type CloudLimiter struct {
    MaxRequestsPerMinute int     // レートリミット
    MaxTokensPerDay      int     // 1日あたり最大トークン
    MaxCostPerDay        float64 // 1日あたり最大コスト (USD)
    WarnThreshold        float64 // 警告しきい値 (0.0-1.0)
}

// デフォルト: フォールバックでの想定外課金を防ぐ
var DefaultCloudLimiter = CloudLimiter{
    MaxRequestsPerMinute: 10,
    MaxTokensPerDay:      100000,
    MaxCostPerDay:        5.0,     // $5/day
    WarnThreshold:        0.8,     // 80%で警告
}
```

初心者が知らずにクラウドで大量課金されるのを防ぐ：
- フォールバック発動時に「☁ クラウドモデルで応答します（APIコスト発生）」と表示
- 日次上限到達で自動停止 + メッセージ表示

---

## まとめ

### 設計のポイント

1. **OpenAI互換がベース**: 1つの実装で12+ プロバイダーをカバー
2. **Anthropicだけ特別対応**: 変換レイヤーで吸収
3. **段階的実装**: Phase 1 で既存コードを壊さずインターフェース導入
4. **自動検出優先**: 初心者は何も設定しなくてよい
5. **フォールバックチェーン**: ローカル失敗 → 別ローカル → クラウド
6. **コスト保護**: クラウドフォールバックの意図しない課金を防止

### 工数見積り

| Phase | 内容 | 目安 |
|---|---|---|
| Phase 1 | インターフェース抽出 | 2-3日 |
| Phase 2 | llama-server + 自動検出 | 1-2日 |
| Phase 3 | クラウドプロバイダー | 2-3日 |
| Phase 4 | フォールバックチェーン | 2日 |
| Phase 5 | ゼロコンフィグ + ウィザード | 1-2日 |
| **合計** | | **8-12日** |
