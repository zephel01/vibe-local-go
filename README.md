<div align="center">

# vibe-local-go

**Go 製ローカル AI コーディングエージェント**

ローカル LLM とクラウド LLM を自在に切り替え、ターミナルからコード生成・ファイル操作・コマンド実行を対話的に行えるオールインワン CLI ツール。

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-blue?style=flat-square)](LICENSE)
[![Release](https://img.shields.io/github/v/release/zephel01/vibe-local-go?style=flat-square&color=green)](https://github.com/zephel01/vibe-local-go/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/zephel01/vibe-local-go/ci.yml?branch=main&style=flat-square&label=CI)](https://github.com/zephel01/vibe-local-go/actions)

[インストール](#インストール) · [クイックスタート](#クイックスタート) · [対応プロバイダー](#サポートプロバイダー一覧) · [ドキュメント](#使い方)

</div>

---

## Why vibe-local-go?

| | |
|---|---|
| **ワンバイナリ** | Go 静的コンパイル。依存ゼロ、~50ms 起動 |
| **マルチプロバイダー** | Ollama / LM Studio + OpenAI / Anthropic / Google / DeepSeek 等 14 社のクラウド LLM |
| **10 の内蔵ツール** | Bash, Read, Write, Edit, Glob, Grep, WebFetch, WebSearch, NotebookEdit, ParallelAgents |
| **セッション管理** | JSONL 永続化、復旧・再開対応 |
| **セキュリティ** | パーミッション管理、パス検証、環境変数サニタイズ |
| **ゼロコンフィグ** | ローカル LLM 自動検出 + クラウドフォールバック |

---

## インストール

### ワンコマンド（推奨）

```bash
curl -fsSL https://raw.githubusercontent.com/zephel01/vibe-local-go/main/scripts/install-go.sh | bash
```

自動で OS / CPU を検出し、最新バイナリをダウンロード＆PATH 設定まで完了します（5-10 秒）。

### バイナリ手動ダウンロード

[GitHub Releases](https://github.com/zephel01/vibe-local-go/releases) から取得:

```bash
# 例: macOS Apple Silicon
curl -fsSL https://github.com/zephel01/vibe-local-go/releases/latest/download/vibe-darwin-arm64.tar.gz | tar xz
mv vibe ~/.local/bin/ && chmod +x ~/.local/bin/vibe
```

<details>
<summary>対応プラットフォーム</summary>

| OS | Arch | ファイル |
|---|---|---|
| macOS | Apple Silicon (arm64) | `vibe-darwin-arm64.tar.gz` |
| macOS | Intel (amd64) | `vibe-darwin-amd64.tar.gz` |
| Linux | x86_64 | `vibe-linux-amd64.tar.gz` |
| Linux | ARM64 | `vibe-linux-arm64.tar.gz` |
| Linux | RISC-V | `vibe-linux-riscv64.tar.gz` |
| Windows | x86_64 | `vibe-windows-amd64.zip` |

</details>

### ソースからビルド

```bash
git clone https://github.com/zephel01/vibe-local-go.git && cd vibe-local-go
make build        # ローカルビルド
make install      # ~/.local/bin にインストール
```

---

## クイックスタート

### ローカル LLM（Ollama）

```bash
# 1. Ollama を起動
ollama serve          # Linux / Windows
open -a Ollama        # macOS

# 2. モデルをダウンロード
ollama pull qwen3:8b

# 3. vibe を起動
vibe
```

### クラウド LLM

```bash
# API キーを設定（例: OpenAI）
export OPENAI_API_KEY="sk-..."

# 起動するだけで自動検出
vibe
```

### ワンショットモード

```bash
vibe -p "PythonでHello Worldを書いて"
```

---

## 使い方

### コマンドラインオプション

```
vibe [options]

--provider <name>    LLM プロバイダー名
--model, -m <name>   使用モデル
--api-key <key>      API キー
--host <url>         ローカル API エンドポイント
-p <prompt>          ワンショットモード
-y                   全ツール自動許可（上級者向け）
--resume <id|last>   セッション復旧
--list-sessions      セッション一覧
--max-tokens <n>     最大出力トークン数（デフォルト: 8192）
--temperature <f>    温度（デフォルト: 0.7）
--context-window <n> コンテキスト長（デフォルト: 32768）
--version            バージョン表示
```

### 対話コマンド

| コマンド | 説明 |
|---|---|
| `/help` | ヘルプ表示 |
| `/exit` `/quit` `/q` | 終了（自動保存） |
| `/clear` | 会話クリア |
| `/status` | セッション情報 |
| `/provider` | プロバイダー管理（一覧・切替・追加・編集・削除） |
| `/models` | モデル一覧 |
| `/sandbox [on\|off]` | サンドボックス切替 |
| `/watch start [pattern]` | ファイル監視開始 |
| `/chain` | プロバイダーチェーン状態 |
| `/plan [on\|off]` | Plan/Act モード |
| `/checkpoint` | Git stash ベースの作業復元 |
| `/autotest [on\|off]` | 変更後の自動テスト |
| `/skills` | スキル一覧 |
| `/mcp` | MCP サーバー管理 |

---

## サポートプロバイダー一覧

### ローカル LLM

| プロバイダー | デフォルトホスト |
|---|---|
| **Ollama** | `http://localhost:11434` |
| **LM Studio** | `http://localhost:1234/v1` |
| **Llama.app / Llama-server** | `http://localhost:8080/v1` |

### クラウド LLM

| プロバイダー | 環境変数 | 主要モデル |
|---|---|---|
| **OpenRouter** | `OPENROUTER_API_KEY` | gemini-2.5-flash, claude-sonnet-4 |
| **OpenAI** | `OPENAI_API_KEY` | gpt-4.1, o3 |
| **Anthropic** | `ANTHROPIC_API_KEY` | claude-sonnet-4, claude-opus-4 |
| **Google Gemini** | `GEMINI_API_KEY` | gemini-2.5-flash, gemini-2.5-pro |
| **DeepSeek** | `DEEPSEEK_API_KEY` | deepseek-chat, deepseek-reasoner |
| **Mistral** | `MISTRAL_API_KEY` | mistral-large-latest |
| **Groq** | `GROQ_API_KEY` | llama-3.3-70b-versatile |
| **Together AI** | `TOGETHER_API_KEY` | Llama-3.3-70B |
| **Fireworks AI** | `FIREWORKS_API_KEY` | llama-v3p3-70b-instruct |
| **Perplexity** | `PERPLEXITY_API_KEY` | sonar-pro |
| **Cohere** | `COHERE_API_KEY` | command-a-03-2025 |
| **Z.AI** | `ZAI_API_KEY` | glm-4.7 |
| **智谱AI** | `ZHIPU_API_KEY` | glm-4.7 |
| **Moonshot (Kimi)** | `MOONSHOT_API_KEY` | kimi-k2-instruct |

---

## 内蔵ツール

| ツール | 説明 | 安全性 |
|---|---|---|
| `bash` | シェルコマンド実行（バックグラウンド対応） | 要確認 |
| `read_file` | ファイル読み込み（テキスト / 画像 / PDF / Jupyter） | 安全 |
| `write_file` | アトミックファイル書き込み | 要確認 |
| `edit_file` | ファイル編集（文字列置換 + diff 生成） | 要確認 |
| `glob` | ファイルパターン検索 | 安全 |
| `grep` | テキスト検索（正規表現） | 安全 |
| `web_fetch` | Web ページ取得（HTML → テキスト変換） | 安全 |
| `web_search` | DuckDuckGo 検索 | 安全 |
| `notebook_edit` | Jupyter Notebook セル編集 | 要確認 |
| `parallel_agents` | 並列サブエージェント実行（最大 4 並列） | 安全 |

---

## 推奨モデル（Ollama）

| RAM | 推奨モデル | 備考 |
|---|---|---|
| 256GB+ | `qwen3:72b` | 最高品質 |
| 96GB+ | `qwen3:32b` | 高品質・実用的 |
| 32GB+ | `qwen3-coder:30b` | バランス型 |
| 16GB+ | `qwen3:8b` | 十分な品質・高速 |
| 8GB+ | `qwen3:4b` | 軽量・高速 |
| 4GB+ | `qwen3:1.7b` | 最小構成 |

---

## アーキテクチャ

```
cmd/vibe/main.go          # CLI エントリーポイント
internal/
  agent/                   # エージェントループ、ディスパッチャー
  config/                  # 設定管理、モデル推奨
  llm/                     # LLM クライアント、ストリーミング
  security/                # パーミッション、パス検証
  session/                 # セッション永続化
  tool/                    # 内蔵ツール (10 種)
  skill/                   # スキル管理
  mcp/                     # Model Context Protocol クライアント
  git/                     # Git checkpoint
  sandbox/                 # サンドボックス管理
  ui/                      # TUI、コマンドハンドラー
  watcher/                 # ファイル監視
```

---

## 設定

設定ファイル: `~/.config/vibe-local-go/config.json`

```json
{
  "PROVIDER": "ollama",
  "MODEL": "qwen3:8b",
  "MAX_TOKENS": 8192,
  "TEMPERATURE": 0.7,
  "CONTEXT_WINDOW": 32768,
  "PROVIDERS": {
    "ollama": {
      "type": "ollama",
      "host": "http://localhost:11434",
      "model": "qwen3:8b"
    }
  }
}
```

対話モードで `/config save` を実行すると現在の設定が永続化されます。

---

## セキュリティ

> **注意**: AI がシェルコマンドを実行するため、危険な操作のリスクがあります。

- **安全ツール**: 確認なしで実行（read_file, glob, grep 等）
- **要確認ツール**: 実行前に `y/n` で確認（bash, write_file, edit_file 等）
- 組み込みのパーミッション管理、パストラバーサル防止、環境変数サニタイズ
- 最大 50 反復で自動停止、`Ctrl+C` でいつでも安全終了

```bash
vibe          # 推奨（確認モード）
vibe -y       # 上級者のみ（全自動許可）
```

---

## 開発

```bash
make test          # テスト実行
make coverage      # カバレッジレポート
make lint          # go vet + golangci-lint
make build         # ビルド
make build-all     # 全プラットフォームビルド
make help          # 全コマンド一覧
```

---

## ライセンス

[MIT License](LICENSE)

## 貢献

Issue や Pull Request を歓迎します。

## 関連プロジェクト

- [Ollama](https://ollama.com/) - ローカル LLM ランタイム
- [vibe-local (Python 版)](https://github.com/ochyai/vibe-local) - オリジナルの Python 実装
