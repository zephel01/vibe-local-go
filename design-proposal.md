# vibe-local Go版 設計提案書

> 作成日: 2026-02-23 | 対象: vibe-local v0.9.4 → vibe-local-go v1.0

## 1. エグゼクティブサマリー

現行の vibe-local（Python 5,650行シングルファイル）を Go で書き直し、以下の3つの目標を達成する。

1. **パフォーマンス**: エージェントのオーバーヘッド削減、メモリ効率化（Python ランタイム不要）
2. **メモリ最適化**: 16GB/32GB/64GB の各ターゲットで最大限賢いモデルを動かす
3. **アーキテクチャ**: モジュール分割による保守性・拡張性の向上

---

## 2. なぜ Go か（Rust との比較）

### 2.1 判断根拠

| 評価軸 | Go | Rust | 判定 |
|--------|-----|------|------|
| シングルバイナリ配布 | `GOOS=windows go build` で完結 | ターゲットごとにツールチェーン必要 | **Go** |
| 並行処理 | goroutine + channel（ネイティブ） | async/await + tokio（ライブラリ依存） | **Go** |
| クロスコンパイル | 標準ツールチェーンのみ | cross, cargo-zigbuild 等が必要 | **Go** |
| 学習コスト | 低（1-2週間で生産的） | 高（所有権・ライフタイム習得に数ヶ月） | **Go** |
| コンパイル速度 | 数秒 | 数十秒〜数分 | **Go** |
| 実行速度 | 十分高速（Pythonの10-50倍） | 最速（Goの1.2-2倍） | Rust |
| メモリ使用量 | 低（ランタイム数MB） | 最低（ランタイムほぼゼロ） | Rust |
| エコシステム | HTTP/JSON/CLI が標準ライブラリ | serde/reqwest/clap 等が必要 | **Go** |

### 2.2 決定的な理由

vibe-local のボトルネックは**LLM推論（30秒〜46秒）**であり、エージェント側の処理時間（ミリ秒単位）ではない。Rust の実行速度優位性（Go の1.2-2倍）は、LLMの30秒待ちの前で完全に埋没する。

一方、Go の強みはこのプロジェクトの課題に直撃する:

- **goroutine**: ストリーミング受信、ツール並列実行、サブエージェントが自然に書ける
- **シングルバイナリ**: install.sh（49K）+ install.ps1（35K）が不要になる
- **Python不要**: ランタイムの200-300MB メモリ削減 → LLM に回せる
- **標準ライブラリの充実**: `net/http`, `encoding/json`, `os/exec` で外部依存ゼロを維持可能

---

## 3. メモリ別モデル戦略（最重要セクション）

### 3.1 原則

エージェントが使うメモリを最小化し、その分を LLM に回す。Go 化により Python ランタイム分（200-300MB）が丸々浮く。

### 3.2 メモリ別推奨構成

#### 16GB RAM（エントリーモデル）

```
OS + デスクトップ:  ~4GB
Go エージェント:    ~10MB（Python: ~300MB → 97%削減）
メインモデル:       qwen3:8b-q4_K_M（~5GB）
サイドカー:         qwen3:1.7b-q4_K_M（~1.2GB）
KVキャッシュ:       ~1.5GB（ctx=4096）
残り余裕:           ~4.3GB
```

**現行Python版との差**: Python ランタイム分（~300MB）がLLMのKVキャッシュに回せる。コンテキスト長を4096→6144に拡大可能。

**最適化オプション**:
- `--num-gpu 0`（CPU推論）で GPU VRAM 不要に
- Apple Silicon: Metal で統合メモリ活用（GPUレイヤー自動オフロード）
- コンテキスト長を4096に制限（KVキャッシュ削減）
- サイドカーを無効化して8b単体運用も可能

#### 32GB RAM（推奨モデル）

```
OS + デスクトップ:  ~4GB
Go エージェント:    ~10MB
メインモデル:       qwen3-coder:14b-q4_K_M（~9GB）
サイドカー:         qwen3:4b-q4_K_M（~2.5GB）
KVキャッシュ:       ~4GB（ctx=8192）
残り余裕:           ~12.5GB
```

**ポイント**: 14Bクラスは8Bからの品質ジャンプが大きく、コスパが最も良い。32GBなら余裕を持って動作する。

**代替モデル候補**:
- `deepseek-coder-v2:16b-q4_K_M` — コーディング特化
- `codellama:13b-q4_K_M` — Meta製、英語圏のコーディングに強い
- `qwen2.5-coder:14b-q4_K_M` — Qwen系コーディング特化

#### 64GB RAM（ハイエンド）

```
OS + デスクトップ:  ~4GB
Go エージェント:    ~10MB
メインモデル:       qwen3-coder:30b-q5_K_M（~22GB）
サイドカー:         qwen3:8b-q4_K_M（~5GB）
KVキャッシュ:       ~10GB（ctx=16384）
残り余裕:           ~23GB
```

**ポイント**: q4ではなくq5量子化が使える。q4→q5で体感できるレベルの品質改善がある。コンテキスト長も16384まで拡大可能。

**代替モデル候補**:
- `deepseek-coder-v2:33b-q4_K_M` — 30Bクラス最強のコーディングモデル
- `codestral:22b-q5_K_M` — Mistral製、コーディング特化
- `qwen2.5-coder:32b-q4_K_M` — 最新Qwen系

### 3.3 動的モデル選択ロジック（Go 実装）

```go
func selectModels(availableRAM uint64) (main, sidecar string) {
    switch {
    case availableRAM >= 56*GB:
        return "qwen3-coder:30b-q5_K_M", "qwen3:8b-q4_K_M"
    case availableRAM >= 40*GB:
        return "qwen3-coder:30b-q4_K_M", "qwen3:4b-q4_K_M"
    case availableRAM >= 24*GB:
        return "qwen3-coder:14b-q4_K_M", "qwen3:4b-q4_K_M"
    case availableRAM >= 12*GB:
        return "qwen3:8b-q4_K_M", "qwen3:1.7b-q4_K_M"
    default:
        return "qwen3:1.7b-q4_K_M", "" // サイドカー無効
    }
}
```

### 3.4 KVキャッシュ管理（新機能）

現行版にない重要な最適化。KVキャッシュはコンテキスト長に比例してメモリを消費する。

```go
func selectContextWindow(availableRAM uint64, modelSize uint64) int {
    freeForKV := availableRAM - modelSize - 4*GB // OS分を引く

    // KVキャッシュ概算: ctx_len * n_layers * 2 * d_model * 2bytes * 2(K+V)
    // 8Bモデル: ~0.5MB/1024tokens, 14B: ~0.8MB, 30B: ~1.5MB
    switch {
    case freeForKV >= 10*GB:
        return 16384
    case freeForKV >= 4*GB:
        return 8192
    case freeForKV >= 2*GB:
        return 4096
    default:
        return 2048
    }
}
```

### 3.5 モデルホットスワップ（新機能）

メインモデルとサイドカーを同時にVRAMに載せず、必要に応じて切り替える戦略。16GB環境で特に有効。

```
通常時:  メインモデル（8b）がVRAMに常駐
圧縮時:  メインモデルをアンロード → サイドカー（1.7b）ロード → 圧縮 → アンロード → メイン再ロード
```

Ollama の `keep_alive` パラメータで制御:
- メインモデル: `keep_alive: -1`（永続）
- サイドカー: `keep_alive: 30s`（30秒後に自動アンロード）

これにより16GB環境でも 8b + 1.7b の組み合わせが安定動作する。

---

## 4. アーキテクチャ設計

### 4.1 ディレクトリ構成

```
vibe-local-go/
├── cmd/
│   └── vibe/
│       └── main.go              # エントリーポイント（CLI引数解析）
├── internal/
│   ├── agent/
│   │   ├── agent.go             # メインエージェントループ（ReActパターン）
│   │   ├── subagent.go          # サブエージェント（goroutine）
│   │   └── loop_detector.go     # 無限ループ検出
│   ├── llm/
│   │   ├── client.go            # Ollama HTTP クライアント
│   │   ├── streaming.go         # SSE ストリーミング（goroutine + channel）
│   │   ├── routing.go           # デュアルモデルルーティング
│   │   └── xml_fallback.go      # XML ツールコール解析
│   ├── tool/
│   │   ├── registry.go          # ツールレジストリ（interface + map）
│   │   ├── schema.go            # OpenAI function calling スキーマ生成
│   │   ├── bash.go              # Bash 実行（os/exec + context）
│   │   ├── file_read.go         # Read（画像base64、PDF対応）
│   │   ├── file_write.go        # Write（アトミック書き込み）
│   │   ├── file_edit.go         # Edit（文字列置換）
│   │   ├── glob.go              # Glob（filepath.WalkDir）
│   │   ├── grep.go              # Grep（regexp + walker）
│   │   ├── web_fetch.go         # WebFetch（SSRF防御付き）
│   │   ├── web_search.go        # WebSearch（DuckDuckGo）
│   │   ├── notebook.go          # NotebookEdit（JSON操作）
│   │   ├── task.go              # TaskCreate/List/Get/Update
│   │   └── ask.go               # AskUserQuestion
│   ├── security/
│   │   ├── permission.go        # パーミッションマネージャ
│   │   ├── sanitize.go          # 環境変数サニタイズ
│   │   └── validator.go         # パス検証、symlink保護、SSRF防御
│   ├── session/
│   │   ├── session.go           # メッセージ管理
│   │   ├── persistence.go       # JSONL 読み書き（アトミック）
│   │   ├── compaction.go        # コンテキスト圧縮
│   │   └── token.go             # トークン推定（CJK対応）
│   ├── ui/
│   │   ├── terminal.go          # TUI（readline、ANSI）
│   │   ├── markdown.go          # マークダウンレンダリング
│   │   ├── spinner.go           # ツール実行中スピナー
│   │   └── i18n.go              # 多言語（ja/en/zh）
│   └── config/
│       ├── config.go            # 設定管理（ファイル/ENV/CLI）
│       ├── memory.go            # メモリ検出 + モデル自動選択
│       └── platform.go          # OS固有ヒント生成
├── go.mod
├── go.sum
├── Makefile                     # ビルド + クロスコンパイル
└── README.md
```

### 4.2 コアインターフェース

```go
// tool/registry.go
type Tool interface {
    Name() string
    Execute(ctx context.Context, params json.RawMessage) (*Result, error)
    Schema() *FunctionSchema
}

type Result struct {
    Output  string
    IsError bool
}

type Registry struct {
    tools map[string]Tool
    mu    sync.RWMutex
}
```

```go
// llm/client.go
type Client struct {
    baseURL    string
    httpClient *http.Client
}

type ChatRequest struct {
    Model       string      `json:"model"`
    Messages    []Message   `json:"messages"`
    Tools       []ToolDef   `json:"tools,omitempty"`
    Stream      bool        `json:"stream"`
    Temperature float64     `json:"temperature"`
    MaxTokens   int         `json:"max_tokens"`
    KeepAlive   interface{} `json:"keep_alive,omitempty"`
    Options     *Options    `json:"options,omitempty"`
}

// ストリーミングは channel で返す
func (c *Client) ChatStream(ctx context.Context, req *ChatRequest) (<-chan StreamEvent, error)

// 同期（ツールコール用）
func (c *Client) ChatSync(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
```

```go
// agent/agent.go
type Agent struct {
    client     *llm.Client
    registry   *tool.Registry
    permission *security.PermissionMgr
    session    *session.Session
    tui        *ui.Terminal
    config     *config.Config
}

func (a *Agent) Run(ctx context.Context, userInput string) error {
    // メインエージェントループ（最大50イテレーション）
    for i := 0; i < maxIterations; i++ {
        // 1. LLM 呼び出し
        // 2. ツールコール解析（JSON + XML フォールバック）
        // 3. 権限チェック
        // 4. ツール実行（読み取り専用は goroutine で並列）
        // 5. 結果を履歴に追加
        // 6. コンテキスト圧縮チェック
    }
}
```

### 4.3 並行処理設計

```
メインgoroutine
├── Agent.Run()
│   ├── LLM呼び出し goroutine（ストリーミング）
│   │   └── SSE パーサー → channel → TUI表示
│   ├── ツール並列実行 goroutine群
│   │   ├── Read tool goroutine
│   │   ├── Glob tool goroutine
│   │   └── Grep tool goroutine
│   └── サブエージェント goroutine
│       └── 独立した Agent.Run() ループ
├── Bash バックグラウンド goroutine群
│   └── os/exec.CommandContext（タイムアウト付き）
└── シグナルハンドラ goroutine
    └── os.Signal channel → graceful shutdown
```

Python の ThreadPoolExecutor（4ワーカー固定）と異なり、goroutine は軽量（スタック数KB）なので、ツールの数に応じて動的に起動できる。

### 4.4 エラー処理戦略

現行版のマルフォームJSON復旧ロジックを Go でも維持:

```go
func salvageJSON(raw string) (json.RawMessage, error) {
    // 1. 標準パース試行
    if json.Valid([]byte(raw)) {
        return json.RawMessage(raw), nil
    }

    // 2. トレーリングカンマ除去
    cleaned := trailingCommaRegex.ReplaceAllString(raw, "$1")
    if json.Valid([]byte(cleaned)) {
        return json.RawMessage(cleaned), nil
    }

    // 3. 不完全な JSON の閉じ括弧補完
    balanced := balanceBrackets(cleaned)
    if json.Valid([]byte(balanced)) {
        return json.RawMessage(balanced), nil
    }

    return nil, fmt.Errorf("JSON recovery failed")
}
```

---

## 5. 現行版の課題への対応

### 5.1 ROADMAP P0 課題への Go 版対応

| 課題 | 現行版の問題 | Go 版の対策 |
|------|-------------|------------|
| P0-1 冗長な応答 | プロンプトだけでは制御困難 | システムプロンプト強化 + 応答後処理（フォローアップ質問検出→自動削除） |
| P0-2 毎回の質問 | モデルの癖 | 応答末尾の疑問文を検出し、ストリーミング表示から除外するフィルター |
| P0-3 コマンド実行拒否 | モデルの安全バイアス | Bash ツールの使用を明示的に促すシステムプロンプト |
| P0-4 URL クォート | zsh 固有 | Bash ツール実行時に自動クォート処理 |

### 5.2 レスポンス速度の改善

LLM の推論速度自体は変えられないが、体感速度は改善できる:

1. **サイドカーのプリロード**: エージェント起動時にバックグラウンドでサイドカーモデルをウォームアップ
2. **ストリーミングの最適化**: Go の goroutine + channel で SSE パースとTUI表示を完全非同期化
3. **ツール並列実行**: Python の GIL なし → 真の並列実行（CPU バウンドなツールも並列可）
4. **プロンプトキャッシュ**: Ollama の `keep_alive: -1` でモデルを常駐させ、初回ロード待ちを排除

### 5.3 シングルファイル問題の解消

5,650行 → 約20ファイルに分割。各ファイルは200-400行に収まる。Go のパッケージシステムにより、依存関係が明確になる。

---

## 6. 外部依存ゼロの維持

現行版の「Python標準ライブラリのみ」という哲学を Go でも維持する。

| 機能 | Python 標準ライブラリ | Go 標準ライブラリ |
|------|---------------------|------------------|
| HTTP クライアント | urllib.request | net/http |
| JSON | json | encoding/json |
| 正規表現 | re | regexp |
| プロセス実行 | subprocess | os/exec |
| ファイル操作 | os, pathlib | os, filepath |
| テンプレート | — | text/template |
| Base64 | base64 | encoding/base64 |
| ハッシュ | hashlib | crypto/sha256 |
| Unicode | unicodedata | unicode |

唯一、TUI の readline 相当（行編集、ヒストリ）に外部パッケージが必要になる可能性がある。候補:

- `golang.org/x/term` — Go 準公式パッケージ（実質標準）
- `github.com/peterh/liner` — readline 互換（軽量）
- 自前実装 — raw モード + ANSI エスケープシーケンス解析

**推奨**: `golang.org/x/term` を使用。Go チームが管理しており、事実上の標準。

---

## 7. ビルド・配布戦略

### 7.1 クロスコンパイル

```makefile
# Makefile
VERSION := 1.0.0
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build-all
build-all:
	GOOS=darwin  GOARCH=arm64 go build $(LDFLAGS) -o dist/vibe-darwin-arm64   ./cmd/vibe/
	GOOS=darwin  GOARCH=amd64 go build $(LDFLAGS) -o dist/vibe-darwin-amd64   ./cmd/vibe/
	GOOS=linux   GOARCH=amd64 go build $(LDFLAGS) -o dist/vibe-linux-amd64    ./cmd/vibe/
	GOOS=linux   GOARCH=arm64 go build $(LDFLAGS) -o dist/vibe-linux-arm64    ./cmd/vibe/
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/vibe-windows.exe    ./cmd/vibe/
```

### 7.2 インストール方法（新）

```bash
# macOS / Linux: バイナリダウンロード一発
curl -fsSL https://github.com/user/vibe-local-go/releases/latest/download/install.sh | sh

# Windows: PowerShell 一発
irm https://github.com/user/vibe-local-go/releases/latest/download/install.ps1 | iex

# Go ユーザー向け
go install github.com/user/vibe-local-go/cmd/vibe@latest
```

現行の install.sh（49K）/ install.ps1（35K）は、バイナリダウンロード + PATH 設定のみの軽量スクリプト（各1KB程度）に置き換わる。

### 7.3 バイナリサイズ

```
Python版: vibe-coder.py (263KB) + Python ランタイム (~100MB)
Go版:     vibe バイナリ (~8-12MB, upx圧縮で ~4MB)
```

---

## 8. 実装ロードマップ

### Phase 1: コア基盤（1-2週間）

- [ ] プロジェクト初期化（go mod init）
- [ ] config パッケージ（CLI引数、設定ファイル、メモリ検出）
- [ ] llm/client.go（Ollama API クライアント、接続確認）
- [ ] llm/streaming.go（SSE ストリーミング、goroutine + channel）
- [ ] tool/registry.go（ツールインターフェース、レジストリ）
- [ ] ui/terminal.go（基本入出力、ANSI カラー）

### Phase 2: ツール実装（1-2週間）

- [ ] tool/bash.go（os/exec、タイムアウト、バックグラウンド実行）
- [ ] tool/file_read.go（テキスト、画像base64、PDF）
- [ ] tool/file_write.go（アトミック書き込み、サイズ制限）
- [ ] tool/file_edit.go（文字列置換、symlink保護）
- [ ] tool/glob.go（filepath.WalkDir + パターンマッチ）
- [ ] tool/grep.go（regexp + walker、ReDoS防御）

### Phase 3: エージェントループ（1週間）

- [ ] agent/agent.go（メイン ReAct ループ）
- [ ] session/session.go（メッセージ管理、JSONL永続化）
- [ ] security/permission.go（safe/ask/deny、危険パターン検出）
- [ ] llm/xml_fallback.go（XML ツールコール解析）

### Phase 4: 高度な機能（1-2週間）

- [ ] session/compaction.go（コンテキスト圧縮、サイドカー呼び出し）
- [ ] llm/routing.go（デュアルモデルルーティング、ホットスワップ）
- [ ] agent/subagent.go（サブエージェント、goroutine）
- [ ] tool/web_fetch.go + tool/web_search.go（SSRF防御付き）
- [ ] tool/notebook.go（Jupyter JSON操作）
- [ ] tool/task.go（タスク管理4ツール）
- [ ] ui/markdown.go（マークダウンレンダリング）

### Phase 5: 仕上げ（1週間）

- [ ] ui/i18n.go（日本語/英語/中国語）
- [ ] config/platform.go（OS固有ヒント）
- [ ] テスト（現行版の500+テストケースの移植）
- [ ] Makefile + CI/CD（GitHub Actions）
- [ ] クロスコンパイル + リリース

**合計見積もり**: 5-8週間（1人開発の場合）

---

## 9. テスト戦略

### 9.1 現行版からの移植

現行版の test_vibe_coder.py（6,919行、500+テスト）のテストケース仕様を Go のテストに移植する。Go は `testing` パッケージが標準で付属しており、外部フレームワーク不要。

```go
// tool/bash_test.go
func TestBashDangerousCommand(t *testing.T) {
    tests := []struct {
        name    string
        command string
        want    bool // should be blocked
    }{
        {"rm -rf /", "rm -rf /", true},
        {"sudo apt", "sudo apt install vim", true},
        {"safe ls", "ls -la", false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := isDangerous(tt.command)
            if got != tt.want {
                t.Errorf("isDangerous(%q) = %v, want %v", tt.command, got, tt.want)
            }
        })
    }
}
```

### 9.2 テストカバレッジ目標

| パッケージ | カバレッジ目標 | 重点テスト |
|-----------|---------------|-----------|
| security/ | 95%+ | 危険コマンド検出、SSRF防御、symlink保護 |
| tool/ | 90%+ | 各ツールの正常系・異常系 |
| llm/ | 85%+ | JSON パース、XML フォールバック、ストリーミング |
| session/ | 90%+ | JSONL 読み書き、圧縮、トークン推定 |
| agent/ | 80%+ | ループ検出、ツール実行フロー |

---

## 10. 品質改善: プロンプトエンジニアリング

Go 化と並行して、ローカル LLM の品質問題をプロンプトで改善する。

### 10.1 改善版システムプロンプト（抜粋）

```
## CRITICAL RULES (厳守事項)

1. ZERO preamble: ツール呼び出し前に説明テキストを一切書くな
2. ZERO follow-up: 応答の末尾に質問を絶対に書くな。ユーザーの次の指示を待て
3. ALWAYS execute: コマンドは必ず Bash ツールで実行せよ。ユーザーに実行を指示するな
4. MINIMAL output: ツール結果後の説明は1文以内
5. FIX errors: エラーが出たら自分で修正せよ。別の方法を提案するな

## BAD examples (禁止パターン)
❌ "回線速度を測定するには、専用のツールが必要です。まずは..."
✅ [即座に Bash ツールで brew install speedtest-cli を実行]

❌ "何か他にお手伝いできることはありますか？"
✅ [応答終了。質問しない]

❌ "以下のコマンドをターミナルで実行してください: python3 app.py"
✅ [Bash ツールで python3 app.py を実行]
```

### 10.2 応答フィルター（Go 実装）

```go
// ストリーミング後処理で冗長なフォローアップを検出・除外
var followUpPatterns = []string{
    "何かお手伝い",
    "他に何か",
    "ご質問があれば",
    "お気軽に",
    "Is there anything",
    "Let me know if",
    "Would you like",
}

func filterFollowUp(text string) string {
    for _, pattern := range followUpPatterns {
        if idx := strings.Index(text, pattern); idx > 0 {
            // パターン以降を切り捨て
            return strings.TrimRight(text[:idx], "\n ")
        }
    }
    return text
}
```

---

## 11. リスクと緩和策

| リスク | 影響 | 緩和策 |
|--------|------|--------|
| Go での readline 互換性 | Windows/macOS で入力体験が異なる | golang.org/x/term + 自前行編集 |
| PDF テキスト抽出 | Python 版は標準ライブラリで FlateDecode 対応 | Go で同等実装 or 外部コマンド呼び出し |
| CJK 文字幅計算 | unicode パッケージに East_Asian_Width がない | go-runewidth パッケージ使用（100行程度、自前実装も可） |
| テスト移植の工数 | 6,919行のテストコード | テストケース仕様のみ移植、Go idiom で書き直し |
| Ollama API 互換性 | Ollama のバージョンアップで API 変更 | OpenAI 互換 API（/v1/chat/completions）を使用、安定している |

---

## 12. まとめ

### 得られるもの

1. **メモリ効率**: Python ランタイム排除で 200-300MB 節約 → LLM に回せる
2. **配布の簡素化**: 85KB のインストーラスクリプト → 4MB のシングルバイナリ
3. **真の並行処理**: GIL なし → ストリーミング・ツール実行・サブエージェントが並列動作
4. **保守性**: 5,650行のモノリス → 20ファイルのモジュール構成
5. **体感速度向上**: エージェントのオーバーヘッド削減 + プリロード + ストリーミング最適化

### 失うもの

1. **Python のプロトタイピング速度**: ただし Go も十分高速
2. **既存テストコードの直接再利用**: テストケース仕様は活用可能
3. **教育的な「Python で読める」価値**: Go も読みやすい言語だが、Python ほどではない

### 最初の一歩

Phase 1 のコア基盤（config + llm/client + streaming + tool/registry + ui/terminal）から始めることを推奨。これだけで Ollama と対話できる最小限の CLI が動作し、早期にフィードバックが得られる。
