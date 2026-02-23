# vibe-local-go

> Go言語で実装されたローカルAIコーディングエージェント

**バージョン**: 1.0.0

## 概要

vibe-local-goはGo言語で書かれた、オフラインで動作するAIコーディングエージェントです。ローカルLLM（Ollama）と直接通信し、コード生成、ファイル操作、コマンド実行などのタスクを支援します。

### 特徴

- **完全ローカル動作**: ネットワーク不要、外部API不要
- **単一バイナリ**: Goコンパイルによる静的バイナリ、依存関係ゼロ
- **高速起動**: Go言語による最適化、~50msで起動
- **6つの内蔵ツール**: Bash, Read, Write, Edit, Glob, Grep
- **セッション管理**: JSONLによる永続化、セッション復旧機能
- **自動モデル選択**: RAM容量に応じて最適なモデルを推奨
- **セキュリティ**: パーミッション管理、パス検証、環境変数サニタイズ

## インストール

### 前提条件

- **Ollama**: ローカルLLMランタイム
  - [Ollama公式サイト](https://ollama.com/) からインストール
  - サポート: macOS, Linux, Windows (WSL2)

### バイナリをダウンロード

```bash
# macOS (Apple Silicon)
curl -L https://github.com/zephel01/vibe-local-go/releases/download/v1.0.0/vibe-local-go-darwin-arm64 -o /usr/local/bin/vibe-local-go
chmod +x /usr/local/bin/vibe-local-go

# macOS (Intel)
curl -L https://github.com/zephel01/vibe-local-go/releases/download/v1.0.0/vibe-local-go-darwin-amd64 -o /usr/local/bin/vibe-local-go
chmod +x /usr/local/bin/vibe-local-go

# Linux (amd64)
curl -L https://github.com/zephel01/vibe-local-go/releases/download/v1.0.0/vibe-local-go-linux-amd64 -o /usr/local/bin/vibe-local-go
chmod +x /usr/local/bin/vibe-local-go

# Linux (arm64)
curl -L https://github.com/zephel01/vibe-local-go/releases/download/v1.0.0/vibe-local-go-linux-arm64 -o /usr/local/bin/vibe-local-go
chmod +x /usr/local/bin/vibe-local-go
```

### ソースからビルド

```bash
git clone https://github.com/zephel01/vibe-local-go.git
cd vibe-local-go
go build -o vibe-local-go ./cmd/vibe
```

## クイックスタート

### 1. Ollamaを起動

```bash
# macOS
open -a Ollama

# Linux / Windows
ollama serve
```

### 2. モデルをダウンロード（初回のみ）

```bash
# 推奨モデル（16GB RAMの場合）
ollama pull qwen3:8b

# 32GB以上RAMの場合
ollama pull qwen3-coder:30b

# モデルが見つからない場合は検索してみてください
ollama search qwen3
```

> **注意**: Ollamaのモデル名は `モデル名:サイズ` の形式です（例: `qwen3:8b`）。
> Hugging Faceの名前（`qwen2.5-32b-instruct` 等）とは異なります。
> 利用可能なモデルは `ollama search <名前>` で検索できます。

### 3. vibe-local-goを起動

```bash
vibe-local-go
```

### 4. 対話モードで使う

```
> PythonでHello Worldを書いて
```

## 使い方

### 対話モード

デフォルトの対話モードでは、AIとチャットしながらコードを生成・実行できます。

```bash
vibe-local-go
```

### ワンショットモード

1回だけ質問して終了するモードです。

```bash
vibe-local-go -p "Pythonでじゃんけんゲームを作って"
```

### セッション復旧

前回のセッションを再開できます。

```bash
# 直近のセッションを復旧
vibe-local-go --resume last

# 特定のセッションを復旧
vibe-local-go --resume sess_1234567890

# セッション一覧を表示
vibe-local-go --list-sessions
```

## コマンドラインオプション

| オプション | 短縮 | 説明 |
|-----------|--------|------|
| `--model <name>` | `-m` | 使用するLLMモデル名（指定しない場合は自動選択） |
| `--host <url>` | | OllamaのAPIエンドポイントURL（デフォルト: http://localhost:11434） |
| `-p <prompt>` | | ワンショットモード（プロンプトを指定して実行） |
| `-y` | | 全ツール実行を自動許可（上級者向け、自己責任） |
| `--resume <id>` | | セッションを復旧（`last` またはセッションID） |
| `--session-id <id>` | | 特定のセッションIDを指定して開始 |
| `--list-sessions` | | 保存済みセッション一覧を表示 |
| `--max-tokens <n>` | | 最大出力トークン数（デフォルト: 8192） |
| `--temperature <f>` | | サンプリング温度（デフォルト: 0.7） |
| `--context-window <n>` | | コンテキストウィンドウサイズ（デフォルト: 32768） |
| `--version` | | バージョンを表示 |

### 例

```bash
# モデルを指定（Ollamaのモデル名を使用）
vibe-local-go --model qwen3:8b

# ワンショット
vibe-local-go -p "現在のディレクトリのファイルを一覧にして"

# 自動許可モード
vibe-local-go -y

# 直近セッションを復旧
vibe-local-go --resume last

# トークン数を調整
vibe-local-go --max-tokens 4096 --temperature 0.5
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

## 推奨モデル

システムRAM容量に基づいて、以下のモデルが推奨されます：

| RAM容量 | 推奨モデル | 備考 |
|---------|-----------|------|
| 256GB+ | qwen3:72b | 最高品質、処理は遅め |
| 96GB+ | qwen3:32b | 高品質、実用的な速度 |
| 32GB+ | qwen3-coder:30b | 高品質と速度のバランス |
| 16GB+ | qwen3:8b | 十分な品質、高速 |
| 8GB+ | qwen3:4b | 軽量、非常に高速 |
| 4GB+ | qwen3:1.7b | 最小限、瞬時に実行 |

**注**: `--model` オプションで任意のモデルを指定できます。利用可能なモデルは `ollama search <名前>` で検索できます。

## 内蔵ツール

現在、以下の6つのツールが実装されています：

| ツール | 説明 | パーミッション |
|--------|------|-------------|
| **bash** | シェルコマンド実行（バックグラウンド対応） | 要確認 |
| **read** | ファイル読み込み（テキスト、画像、Jupyter） | 安全 |
| **write** | ファイル書き込み（アトミック） | 要確認 |
| **edit** | ファイル編集（文字列置換、diff生成） | 要確認 |
| **glob** | ファイルパターン検索 | 安全 |
| **grep** | テキストパターン検索（正規表現） | 安全 |

### パーミッションについて

- **安全ツール**: 確認なしで実行
- **要確認ツール**: 実行前に `y/n` で確認

## アーキテクチャ

```
┌─────────────────────────────────────────────┐
│  CLI Entry Point (cmd/vibe/main.go)      │
└─────────────────┬───────────────────────┘
                  │
    ┌─────────────┼─────────────┬─────────────┐
    │             │             │             │
    ▼             ▼             ▼             ▼
┌─────────┐ ┌─────────┐ ┌──────────┐ ┌──────────┐
│  Agent  │ │  Config │ │ Security │ │ Session  │
└────┬────┘ └─────────┘ └──────────┘ └────┬─────┘
     │                                       │
     ├─────────────┬─────────────────────────┘
     │             │
     ▼             ▼
┌─────────┐ ┌─────────────┐
│   LLM   │ │    Tool     │
│ (Ollama)│ │  (6 tools)  │
└─────────┘ └─────────────┘
```

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
    ├── tool/           # 内蔵ツール
    └── ui/             # TUI、コマンドハンドラー
```

## 設定

### 設定ファイル

```bash
~/.config/vibe-local/config
```

### 設定項目

```bash
# モデル設定
MODEL=""                      # 空で自動選択
OLLAMA_HOST="http://localhost:11434"

# LLM設定
MAX_TOKENS=8192
TEMPERATURE=0.7
CONTEXT_WINDOW=32768
```

### 環境変数

| 変数 | 説明 |
|------|------|
| `OLLAMA_HOST` | Ollama APIエンドポイントURL |
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
vibe-local-go

# 上級者のみ（自己責任）
vibe-local-go -y
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

### "Ollama接続エラー"

```bash
# Ollamaが起動しているか確認
ollama ps

# 再起動
ollama serve
```

### "モデルが見つかりません"

```bash
# モデルを検索（正しい名前を確認）
ollama search qwen3

# モデルをダウンロード（Ollamaの名前形式: モデル名:サイズ）
ollama pull qwen3:8b

# モデル名がわからない場合
ollama search <キーワード>
```

### "コマンドが見つかりません"

```bash
# バイナリがPATHにあるか確認
which vibe-local-go

# 再インストール（macOS Apple Siliconの場合）
curl -L https://github.com/zephel01/vibe-local-go/releases/download/v1.0.0/vibe-local-go-darwin-arm64 -o /usr/local/bin/vibe-local-go
chmod +x /usr/local/bin/vibe-local-go
```

## 制限事項

- 現在実装されているのは6つのツールのみ
- 画像/PDF読み取りのサポートは開発中
- Web検索/WebFetchツールは未実装
- サブエージェント機能は未実装
- メモリ検出は簡易実装（デフォルト値を使用）

## 依存関係

- **Go 1.26+**: ビルドに必要
- **Ollama**: LLMランタイム（必須）
- **外部ライブラリ**: なし（Go標準ライブラリのみ）

## ライセンス

MIT License

## 貢献

問題報告やプルリクエストを歓迎します。

## 関連プロジェクト

- [Ollama](https://ollama.com/) - ローカルLLMランタイム
- [vibe-local (Python版)](https://github.com/ochyai/vibe-local) - 元のPython実装
