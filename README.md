# vibe-local-go

> Go言語で実装されたローカルAIコーディングエージェント

**バージョン**: 1.1.1

## 概要

vibe-local-goはGo言語で書かれた、オフラインで動作するAIコーディングエージェントです。ローカルLLM（Ollama）と直接通信し、コード生成、ファイル操作、コマンド実行などのタスクを支援します。

### 特徴

- **マルチプロバイダー対応**: Ollama（ローカル）+ 14個のクラウドLLM（OpenAI, Anthropic, Google, DeepSeek, Mistral, Groq, OpenRouter等）

- **ワンバイナリ**: Goコンパイルによる静的バイナリ、依存関係ゼロ

- **高速起動**: Go言語による最適化、~50msで起動

- **10の内蔵ツール**: Bash, Read, Write, Edit, Glob, Grep, WebFetch, WebSearch, NotebookEdit, ParallelAgents

- **プロバイダー管理**: 登録済みプロバイダーの切替・追加・編集・削除

- **モデル自動管理**: セットアップ時にモデル存在チェック＋自動ダウンロード提案（プログレスバー付き）

- **セッション管理**: JSONLによる永続化、セッション復旧機能

- **自動モデル選択**: RAM容量に応じて最適なモデルを推奨

- **柔軟な認証**: APIキー + 環境変数対応、再設定フロー搭載

- **セキュリティ**: パーミッション管理、パス検証、環境変数サニタイズ

## インストール

### 前提条件

- **ローカルLLM** (以下いずれか):

  - **Ollama**: [Ollama公式サイト](https://ollama.com/) からインストール

  - **LM Studio**: [LM Studio](https://lmstudio.ai/) （デフォルト: http://localhost:1234/v1）

  - **Llama.app**: [Llama](https://github.com/janhq/jan) / Llama-server （デフォルト: http://localhost:8080/v1）

- または

- **クラウドLLMのAPIキー**:

  - OpenAI, Anthropic, Google Gemini, DeepSeek, Mistral, Groq, OpenRouter, Z.AI, など計14社対応

### 方法 1: ワンコマンドインストール（推奨）

```bash
curl -fsSL https://raw.githubusercontent.com/zephel01/vibe-local-go/main/scripts/install-go.sh | bash
```

**特徴**:
- 自動OS/CPU検出
- GitHub Release から最新バイナリをダウンロード
- PATH 自動設定
- **所要時間**: 5-10秒

### 方法 2: バイナリの手動ダウンロード

[GitHub Releases](https://github.com/zephel01/vibe-local-go/releases) から対応プラットフォーム用バイナリをダウンロード:

```bash
# ダウンロード（例: macOS Apple Silicon）
curl -fsSL https://github.com/zephel01/vibe-local-go/releases/download/v1.1.1/vibe-darwin-arm64.tar.gz -o vibe.tar.gz

# 解凍
tar xzf vibe.tar.gz

# PATH に追加
mv vibe ~/.local/bin/
chmod +x ~/.local/bin/vibe
```

**対応プラットフォーム**:
| OS | Architecture | ファイル |
|----|--------------|---------|
| **macOS** | Apple Silicon (arm64) | vibe-darwin-arm64.tar.gz |
| **macOS** | Intel (amd64) | vibe-darwin-amd64.tar.gz |
| **Linux** | x86_64 (amd64) | vibe-linux-amd64.tar.gz |
| **Linux** | ARM64 (aarch64) | vibe-linux-arm64.tar.gz |
| **Linux** | RISC-V (riscv64) | vibe-linux-riscv64.tar.gz |
| **Windows** | x86_64 (amd64) | vibe-windows-amd64.zip |

### 方法 3: ソースからビルド

**要件**: Go 1.26+

```bash
git clone https://github.com/zephel01/vibe-local-go.git
cd vibe-local-go

# ローカル環境用ビルド
make build
./dist/vibe --version

# または全プラットフォーム用ビルド
make build-all
make release

# インストール
make install
```

**Makefile コマンド**:
```bash
make build         # ローカル環境用ビルド
make build-all     # 6プラットフォーム対応
make release       # tar.gz/zip 圧縮
make test          # ユニットテスト実行
make lint          # コード検査
make clean         # ビルド成果物削除
make help          # ヘルプ表示
```

## クイックスタート

### オプション A: ローカルLLM (Ollama)

#### 1. Ollamaを起動

```bash
# macOS
open -a Ollama

# Linux / Windows
ollama serve
```

#### 2. モデルをダウンロード

```bash
# 推奨モデル（16GB RAMの場合）
ollama pull qwen3:8b

# 32GB以上RAMの場合
ollama pull qwen3-coder:30b
```

#### 3. vibeを起動

```bash
vibe
```

### オプション B: クラウドLLM (OpenAI, Google Gemini, など)

#### 1. APIキーを環境変数に設定

```bash
# OpenAI
export OPENAI_API_KEY="sk-..."

# Google Gemini
export GEMINI_API_KEY="AIzaSy..."

# Z.AI / Zhipu
export ZAI_API_KEY="your-key"

# その他対応プロバイダー
# ANTHROPIC_API_KEY, DEEPSEEK_API_KEY, MISTRAL_API_KEY, GROQ_API_KEY, OPENROUTER_API_KEY等
```

#### 2. vibeを起動

```bash
vibe
```

自動検出されるか、プロバイダー管理メニューで選択してください。

### オプション C: 複数プロバイダーを登録

```bash
vibe
```

起動後、対話モードで以下を実行:

```
/provider
```

メニューから「A. プロバイダーを追加」を選択してクラウドLLMを追加できます。

### 対話モードで使う

```
> PythonでHello Worldを書いて
```

## 使い方

### 対話モード

デフォルトの対話モードでは、AIとチャットしながらコードを生成・実行できます。

```bash
vibe
```

### ワンショットモード

1回だけ質問して終了するモードです。

```bash
vibe -p "Pythonでじゃんけんゲームを作って"
```

### セッション復旧

前回のセッションを再開できます。

```bash
# 直近のセッションを復旧
vibe --resume last

# 特定のセッションを復旧
vibe --resume sess_1234567890

# セッション一覧を表示
vibe --list-sessions
```

## コマンドラインオプション

| オプション | 短縮 | 説明 |
|-----------|--------|------|
| `--provider <name>` | | LLMプロバイダー名（ollama, openai, anthropic, google, zai, zhipu 等） |
| `--api-key <key>` | | クラウドプロバイダーのAPIキー |
| `--model <name>` | `-m` | 使用するLLMモデル名 |
| `--host <url>` | | ローカルプロバイダーのAPIエンドポイントURL（デフォルト: http://localhost:11434） |
| `-p <prompt>` | | ワンショットモード（プロンプトを指定して実行） |
| `-y` | | 全ツール実行を自動許可（上級者向け、自己責任） |
| `--resume <id>` | | セッションを復旧（`last` またはセッションID） |
| `--session-id <id>` | | 特定のセッションIDを指定して開始 |
| `--list-sessions` | | 保存済みセッション一覧を表示 |
| `--max-tokens <n>` | | 最大出力トークン数（デフォルト: 8192） |
| `--temperature <f>` | | サンプリング温度（デフォルト: 0.7） |
| `--context-window <n>` | | コンテキストウィンドウサイズ（デフォルト: 32768） |
| `--num-ctx <n>` | | Ollama num_ctx (KVキャッシュサイズ、メモリ節約用) |
| `--num-gpu <n>` | | Ollama num_gpu (GPUレイヤー数) |
| `--version` | | バージョンを表示 |

### 例

```bash
# ローカルLLM（Ollama）
vibe --provider ollama --model qwen3:8b

# クラウドLLM（OpenAI）
vibe --provider openai --api-key sk-... --model gpt-4.1

# クラウドLLM（Google Gemini）
vibe --provider google --api-key AIzaSy... --model gemini-2.5-flash

# クラウドLLM（Z.AI 国際版）
vibe --provider zai --api-key your-key --model glm-4.7

# ワンショット
vibe -p "現在のディレクトリのファイルを一覧にして"

# 自動許可モード
vibe -y

# 直近セッションを復旧
vibe --resume last

# トークン数を調整
vibe --max-tokens 4096 --temperature 0.5
```

## 対話コマンド

対話モード中に使用できるスラッシュコマンド：

| コマンド | 説明 |
|----------|------|
| `/help` | ヘルプを表示 |
| `/exit`, `/quit`, `/q` | 終了（セッションは自動保存） |
| `/clear` | 会話履歴をクリア |
| `/status` | セッション情報（トークン数、モデル、CWD）を表示 |
| `/save` | 現在のセッションを保存 |
| `/tokens` | 詳細なトークン使用量を表示 |
| `/config` | 現在の設定を表示 |
| `/config save` | 現在の設定をconfig.jsonに保存 |
| `/provider` | **プロバイダー管理メニュー**（一覧・切替・追加・編集・削除） |
| `/provider add` | 新しいプロバイダーを追加 |
| `/provider <name>` | 指定プロバイダーに切替（例: `/provider openai`) |
| `/provider edit` | 登録済みプロバイダーを編集（APIキー・モデル等） |
| `/provider delete` | 登録済みプロバイダーを削除 |
| `/models` | 利用可能なモデル一覧を表示（ローカルプロバイダーのみ） |
| `/sandbox [on\|off]` | サンドボックスモードの切替 |
| `/watch start [pattern]` | ファイル監視を開始（例: `*.go`, `src/**/*.ts`） |
| `/watch stop` | ファイル監視を停止 |
| `/watch status` | 監視状態と検知ファイル数を表示 |
| `/chain` | プロバイダーチェーンの状態表示 |
| `/chain <番号>` | 指定プロバイダーに手動切替 |

## サポートプロバイダー一覧

### ローカルLLM

| プロバイダー | デフォルトホスト | 対応モデル形式 |
|------------|-----------------|-------------|
| **Ollama** | http://localhost:11434 | `model:size` (e.g., qwen3:8b) |
| **LM Studio** | http://localhost:1234/v1 | OpenAI互換モデル |
| **Llama.app** / **Llama-server** | http://localhost:8080/v1 | OpenAI互換モデル |

### クラウドLLM（APIキー必須）

| プロバイダー | 環境変数 | 主要モデル |
|-----------|---------|---------|
| **OpenRouter** | `OPENROUTER_API_KEY` | gemini-2.5-flash, claude-sonnet-4 |
| **OpenAI** | `OPENAI_API_KEY` | gpt-4.1, gpt-4.1-mini, o3 |
| **Anthropic** | `ANTHROPIC_API_KEY` | claude-sonnet-4, claude-opus-4 |
| **Google Gemini** | `GEMINI_API_KEY` | gemini-2.5-flash, gemini-2.5-pro |
| **DeepSeek** | `DEEPSEEK_API_KEY` | deepseek-chat, deepseek-reasoner |
| **Mistral** | `MISTRAL_API_KEY` | mistral-large-latest, codestral-latest |
| **Groq** | `GROQ_API_KEY` | llama-3.3-70b-versatile |
| **Together AI** | `TOGETHER_API_KEY` | Llama-3.3-70B, Qwen2.5-72B |
| **Fireworks AI** | `FIREWORKS_API_KEY` | llama-v3p3-70b-instruct |
| **Perplexity** | `PERPLEXITY_API_KEY` | sonar-pro (検索特化) |
| **Cohere** | `COHERE_API_KEY` | command-a-03-2025 (RAG特化) |
| **Z.AI（国際版）** | `ZAI_API_KEY` | glm-4.7 (Zhipu AIの国際サービス) |
| **Z.AI Coding Plan** | `ZAI_API_KEY` | glm-4.7 (Coding専用エンドポイント) |
| **智谱AI（中国版）** | `ZHIPU_API_KEY` | glm-4.7 (Zhipu AI本体) |
| **Moonshot（Kimi）** | `MOONSHOT_API_KEY` | kimi-k2-instruct, kimi-k2.5 |

## ローカルLLMの推奨モデル

システムRAM容量に基づいて、以下のモデルが推奨されます：

| RAM容量 | 推奨モデル | 備考 |
|---------|-----------|------|
| 256GB+ | qwen3:72b | 最高品質、処理は遅め |
| 96GB+ | qwen3:32b | 高品質、実用的な速度 |
| 32GB+ | qwen3-coder:30b | 高品質と速度のバランス |
| 16GB+ | qwen3:8b | 十分な品質、高速 |
| 8GB+ | qwen3:4b | 軽量、非常に高速 |
| 4GB+ | qwen3:1.7b | 最小限、瞬時に実行 |

**注**: `--model` オプションで任意のモデルを指定できます。利用可能なモデルは `/models` コマンドまたは `ollama search <名前>` で検索できます。

## 内蔵ツール

現在、以下の10のツールが実装されています：

| ツール | 説明 | パーミッション |
|--------|------|-------------|
| **bash** | シェルコマンド実行（バックグラウンド対応、エラーヒント付き） | 要確認 |
| **read_file** | ファイル読み込み（テキスト、画像、Jupyter、PDF対応） | 安全 |
| **write_file** | ファイル書き込み（アトミック） | 要確認 |
| **edit_file** | ファイル編集（文字列置換、diff生成） | 要確認 |
| **glob** | ファイルパターン検索（ファイル推定ヒント付き） | 安全 |
| **grep** | テキストパターン検索（正規表現） | 安全 |
| **web_fetch** | Webページ取得（HTML→テキスト変換） | 安全 |
| **web_search** | DuckDuckGo検索 | 安全 |
| **notebook_edit** | Jupyter Notebookセル編集（replace/insert/delete） | 要確認 |
| **parallel_agents** | 並列サブエージェント実行（最大4並列） | 安全 |

### パーミッションについて

- **安全ツール**: 確認なしで実行

- **要確認ツール**: 実行前に `y/n` で確認

## アーキテクチャ

```
┌──────────────────────────────────────────────┐
│  CLI Entry Point (cmd/vibe/main.go)       │
└──────────────────┬─────────────────────────┘
                   │
    ┌──────────────┼──────────────┬──────────────┐
    │              │              │              │
    ▼              ▼              ▼              ▼
┌────────┐ ┌────────────┐ ┌──────────┐ ┌──────────┐
│ Agent  │ │   Config   │ │ Security │ │ Session  │
└───┬────┘ └──────┬─────┘ └──────────┘ └────┬─────┘
    │             │                          │
    │      ┌──────────────────────────────────┘
    │      │
    ▼      ▼
┌──────────────────────────┐
│  Provider Abstraction    │
├──────────────────────────┤
│ ├─ LLMProvider (interface)
│ ├─ CloudProvider (OpenAI-compat)
│ ├─ OllamaProvider
│ ├─ OpenRouterProvider
│ └─ LocalProviders (LM Studio, Llama.app)
└───┬──────────────────────┘
    │
    ├────────────┬────────────┬─────────────┐
    │            │            │             │
    ▼            ▼            ▼             ▼
┌──────┐ ┌──────────┐ ┌──────────┐ ┌─────────────┐
│Local │ │ Cloud 14 │ │   Tool    │ │  Command    │
│ LLMs │ │ Providers│ │(10 tools) │ │  Handler    │
└──────┘ └──────────┘ └───────────┘ └─────────────┘
                            │
                     ┌──────┴──────┐
                     │   Watcher   │
                     │(File Watch) │
                     └─────────────┘
```

**ポイント**:

- **LLMProvider**: すべてのLLMバックエンドの共通インターフェース

- **OpenAICompatProvider**: OpenAI形式のAPIを実装するプロバイダーの基盤

- **CloudProvider Factory**: APIキー + モデル名からプロバイダーを動的作成

- **LocalProvider Manager**: Ollama/LM Studio/Llama.appの複数ローカルプロバイダーに対応

- **Config System**: マルチプロバイダーをJSONで永続化

### ディレクトリ構造

```
vibe-local-go/
├── cmd/
│   └── vibe/           # エントリーポイント (main.go)
└── internal/
    ├── agent/          # エージェントループ、ディスパッチャー
    ├── config/         # 設定管理、モデル推奨
    ├── llm/            # LLMクライアント、ストリーミング
    ├── security/        # パーミッション管理、パス検証
    ├── session/         # セッション管理、永続化
    ├── tool/           # 内蔵ツール (10種)
    ├── ui/             # TUI、コマンドハンドラー
    └── watcher/        # ファイル監視、変更通知インジェクター
```

## 設定

### 設定ファイル

```bash
~/.config/vibe-local-go/config.json
```

### 設定形式（JSON）

```json
{
    "PROVIDER": "zai",
    "MODEL": "glm-4.7",
    "MAX_TOKENS": 8192,
    "TEMPERATURE": 0.7,
    "CONTEXT_WINDOW": 32768,
    "OLLAMA_NUM_CTX": 8192,
    "OLLAMA_NUM_GPU": 99,
    "PROVIDERS": {
        "ollama": {
            "type": "ollama",
            "host": "http://localhost:11434",
            "model": "qwen3:8b"
        },
        "zai": {
            "type": "zai",
            "api_key": "your-key",
            "model": "glm-4.7"
        },
        "openai": {
            "type": "openai",
            "api_key": "sk-...",
            "model": "gpt-4.1"
        }
    }
}
```

| キー | 型 | 説明 |
|------|------|------|
| `PROVIDER` | string | アクティブプロバイダー |
| `MODEL` | string | モデル名 |
| `MAX_TOKENS` | int | 最大出力トークン数 |
| `TEMPERATURE` | float | サンプリング温度 (0.0-2.0) |
| `CONTEXT_WINDOW` | int | コンテキストウィンドウサイズ |
| `OLLAMA_NUM_CTX` | int | Ollama num_ctx (KVキャッシュサイズ、0=Ollamaデフォルト) |
| `OLLAMA_NUM_GPU` | int | Ollama num_gpu (GPUレイヤー数) |
| `PROVIDERS` | object | プロバイダー別プロファイル |

### 環境変数（プロバイダーのAPIキー）

| 変数 | 説明 |
|------|------|
| `OPENROUTER_API_KEY` | OpenRouter APIキー |
| `OPENAI_API_KEY` | OpenAI APIキー |
| `ANTHROPIC_API_KEY` | Anthropic APIキー |
| `GEMINI_API_KEY` | Google Gemini APIキー |
| `DEEPSEEK_API_KEY` | DeepSeek APIキー |
| `MISTRAL_API_KEY` | Mistral APIキー |
| `GROQ_API_KEY` | Groq APIキー |
| `TOGETHER_API_KEY` | Together AI APIキー |
| `FIREWORKS_API_KEY` | Fireworks AI APIキー |
| `PERPLEXITY_API_KEY` | Perplexity APIキー |
| `COHERE_API_KEY` | Cohere APIキー |
| `ZAI_API_KEY` | Z.AI / Z.AI Coding Plan APIキー |
| `ZHIPU_API_KEY` | 智谱AI (中国版) APIキー |
| `MOONSHOT_API_KEY` | Moonshot (Kimi) APIキー |
| `OLLAMA_HOST` | Ollama APIエンドポイントURL |
| `OLLAMA_NUM_CTX` | Ollama num_ctx (KVキャッシュサイズ、メモリ節約用) |
| `OLLAMA_NUM_GPU` | Ollama num_gpu (GPUレイヤー数) |
| `VIBE_LOCAL_DEBUG` | `1` でデバッグログ有効化 |

## セキュリティ

### ⚠️ 重要

`vibe-local-go` はAIがコマンドを実行するため、危険な操作のリスクがあります。

### 安全な使用方法

1. **初回は必ず `n`（確認モード）を選択**

   - 各ツール実行前に確認が表示されます
   - 内容を理解できない場合は `n` で拒否

2. **危険なキーワードに注意**

   - `sudo` で始まるコマンド（システム全体への影響）
   - `rm -rf` / `dd` / `mkfs`（破壊的操作）
   - `chmod` / `chown`（権限変更）
   - `>` で設定ファイル上書き

3. **重要なフォルダでは使わない**

   - 新しい空フォルダで練習

4. **`Ctrl+C` でいつでも停止**

```bash
# 推奨（安全）
vibe

# 上級者のみ（自己責任）
vibe -y
```

### 組み込みセキュリティ

- **パーミッション管理**: 安全ツール/要確認ツールの分類

- **パス検証**: シンボリックリンク保護、パストラバーサル防止

- **環境変数サニタイズ**: トークン/パスワードの除外

- **最大反復制限**: 50回で自動停止

- **Graceful Shutdown**: Ctrl+C で安全に終了

## 開発

### テスト

```bash
# 全テスト実行
go test ./... -v

# 特定パッケージのテスト
go test ./internal/agent -v
```

### ビルド

```bash
# ローカルビルド
go build -o vibe-local-go ./cmd/vibe

# クロスコンパイル
GOOS=darwin GOARCH=arm64 go build -o vibe-local-go-darwin-arm64 ./cmd/vibe
GOOS=linux GOARCH=amd64 go build -o vibe-local-go-linux-amd64 ./cmd/vibe
```

### コードスタイル

```bash
# フォーマット
gofmt -w .

# 静的解析
go vet ./...
```

## トラブルシューティング

### "接続エラー（ローカルLLM）"

**症状**: `接続エラー: health check failed with status...`

**対策**:

1. **Ollamaが起動しているか確認**
   ```bash
   ollama ps
   ```

2. **Ollamaを起動**
   ```bash
   # macOS
   open -a Ollama

   # Linux / Windows
   ollama serve
   ```

3. **リトライ/再設定メニュー**
   - エラー表示後、`1. リトライ` を選択
   - `2. プロバイダーを再設定` でクラウド/ローカルを選んで別のプロバイダーに切替
   - `3. 終了` で終了

### "接続エラー（クラウドLLM）"

**症状**: `接続エラー: health check failed with status 401`

**対策**:

1. **APIキーが正しいか確認**
   ```bash
   # 環境変数に設定済みか確認
   echo $OPENAI_API_KEY
   ```

2. **APIキーを更新**
   ```bash
   /provider edit
   ```
   メニューから編集対象を選択してAPIキーを変更

3. **新規プロバイダーを追加**
   ```bash
   /provider add
   ```

### "モデルが見つかりません"

**症状**: `モデル 'xxx' が見つかりません`

**対策**:

vibeの起動時に自動でモデル存在チェックが行われ、未ダウンロードのモデルは以下の選択肢が表示されます：

1. **利用可能なモデルから選択** — ダウンロード済みモデルの一覧から選ぶ
2. **指定したモデルをダウンロード** — プログレスバー付きでダウンロード
3. **クラウドプロバイダーに切替** — クラウドLLMを使用

また、プロバイダー追加・編集時に手動入力したモデル名も自動チェックされ、
未ダウンロードの場合はその場でダウンロードを提案します。

手動でダウンロードする場合：
```bash
ollama pull qwen3:8b # モデルをダウンロード
ollama search qwen3  # モデルを検索
/models              # 利用可能なモデル一覧
```

### "コマンドが見つかりません"

```bash
# バイナリがPATHにあるか確認
which vibe

# インストールスクリプトで再インストール（推奨）
curl -fsSL https://raw.githubusercontent.com/zephel01/vibe-local-go/main/scripts/install-go.sh | bash

# 手動再インストール（macOS Apple Siliconの場合）
curl -fsSL https://github.com/zephel01/vibe-local-go/releases/download/v1.1.0/vibe-darwin-arm64.tar.gz | tar xz
mv vibe ~/.local/bin/
chmod +x ~/.local/bin/vibe
```

### "APIキーが保存されない"

**原因**: config.jsonが未作成の状態

**対策**:

```bash
# 現在の設定をconfig.jsonに保存
/config save
```

これで `~/.config/vibe-local-go/config.json` に設定が永続化されます。

## 機能一覧

### 実装済み

- ✅ マルチプロバイダー対応（ローカル + クラウド14社）
- ✅ プロバイダー管理（追加・切替・編集・削除）
- ✅ 10の内蔵ツール（Bash, Read, Write, Edit, Glob, Grep, WebFetch, WebSearch, NotebookEdit, ParallelAgents）
- ✅ セッション管理と永続化
- ✅ 環境変数ベースのAPIキー設定
- ✅ config.json による設定永続化
- ✅ ローカルモデルの自動検出（Ollama, LM Studio, Llama.app）
- ✅ クラウドプロバイダーの自動検出
- ✅ 接続エラー時のリトライ/再設定フロー（クラウド/ローカル両方対応）
- ✅ マルチホスト対応（Ollama, LM Studio, Llama.app）
- ✅ プロバイダー編集（APIキー・モデル変更）
- ✅ モデル存在チェック＋自動ダウンロード提案（セットアップ・編集・起動時）
- ✅ ダウンロード進捗表示（プログレスバー付き ollama pull）
- ✅ Agent Skills（グローバル/プロジェクトスキル管理、`/skills` コマンド）
- ✅ MCP Client（Model Context Protocol、外部ツールサーバー連携、`/mcp` コマンド）
- ✅ Plan/Act モード（`/plan [on|off]`、書き込み禁止による安全な計画フェーズ）
- ✅ Git Checkpoint（`/checkpoint`、git stash ベースの作業復元）
- ✅ Auto Test（ファイル変更後の自動テスト実行、`/autotest [on|off]`）
- ✅ ESC 割り込み（エージェント実行の中断）
- ✅ ステータス行（経過時間・トークン数のリアルタイム表示）
- ✅ クロスプラットフォームビルド（Makefile + GitHub Actions、6プラットフォーム対応）
- ✅ ワンコマンドインストール（`install-go.sh`）
- ✅ Jupyter Notebook 編集ツール（NotebookEdit: replace/insert/delete）
- ✅ PDF テキスト抽出（file_read ツールで .pdf 自動対応、Pure Go実装）
- ✅ ファイル監視（`/watch` コマンド、ポーリングベース、外部依存なし）
- ✅ 並列サブエージェント（最大4並列、書き込み競合検知、進捗表示）

- ✅ ProviderChain フォールバック（プロバイダー障害時の自動切り替え、`/chain` コマンド）
- ✅ ゼロコンフィグ自動初期化（ローカルサーバー自動検出 + クラウドフォールバック構築）

### 開発中

- 🔄 Anthropic Messages API 専用実装（native messages API 変換）

### 未実装

- ❌ タスク管理ツール（TodoWrite / TodoRead）
- ❌ ユーザー質問ツール（AskUserQuestion）
- ❌ 多言語対応 UI（ja / en / zh の自動切り替え）
- ❌ コスト・レート制限（クラウドAPI使用量管理）

## 依存関係

- **Go 1.26+**: ビルドに必要

- **LLM プロバイダー** (以下いずれか):
  - Ollama / LM Studio / Llama-server（ローカル）
  - クラウドLLM APIキー（OpenAI, Anthropic, Google など）

- **外部ライブラリ**: なし（Go標準ライブラリのみ）

## ライセンス

MIT License

## 貢献

問題報告やプルリクエストを歓迎します。

## 関連プロジェクト

- [Ollama](https://ollama.com/) - ローカルLLMランタイム

- [vibe-local (Python版)](https://github.com/ochyai/vibe-local) - 元のPython実装
