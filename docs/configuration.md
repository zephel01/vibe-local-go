# 設定ガイド / Configuration Guide

vibe-local-go の設定方法を解説します。設定は以下の3つの方法で行えます（優先順位順）：

1. **CLIオプション** — 起動時に指定（最優先）
2. **環境変数** — シェル環境で設定
3. **config.json** — ファイルで永続化（最低優先）

---

## config.json

### ファイルの場所

config.json は以下のパスを上から順に探索します。最初に見つかったファイルが使用されます。

| 優先順位 | パス |
|---------|------|
| 1 | `~/.config/vibe-local-go/config.json` |
| 2 | `~/.config/vibe-local/config.json` |
| 3 | `~/.config/vibe-coder/config.json` |
| 4 | `~/.vibe-local.json` |
| 5 | `~/.vibe-coder.json` |

**保存先**（`/config save` 時）: `~/.config/vibe-local-go/config.json`

### 基本例

```json
{
    "PROVIDER": "ollama",
    "MODEL": "qwen3:8b",
    "MAX_TOKENS": 8192,
    "TEMPERATURE": 0.7,
    "CONTEXT_WINDOW": 32768
}
```

### フル設定例

```json
{
    "PROVIDER": "zai",
    "MODEL": "glm-4.7",
    "SIDECAR_MODEL": "qwen3:1.7b",
    "OLLAMA_HOST": "http://localhost:11434",
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
            "api_key": "your-zai-api-key",
            "model": "glm-4.7"
        },
        "openai": {
            "type": "openai",
            "api_key": "sk-...",
            "model": "gpt-4.1",
            "max_tokens": 16384,
            "temperature": 0.5
        },
        "anthropic": {
            "type": "anthropic",
            "api_key": "sk-ant-...",
            "model": "claude-sonnet-4"
        },
        "google": {
            "type": "google",
            "api_key": "AIzaSy...",
            "model": "gemini-2.5-flash"
        }
    }
}
```

### グローバルフィールド一覧

| キー | 型 | デフォルト | 説明 |
|------|------|-----------|------|
| `PROVIDER` | string | `"ollama"` | アクティブプロバイダー名 |
| `MODEL` | string | (自動選択) | 使用するモデル名 |
| `SIDECAR_MODEL` | string | | サイドカーモデル（軽量タスク用） |
| `OLLAMA_HOST` | string | `"http://localhost:11434"` | Ollama APIエンドポイント |
| `MAX_TOKENS` | int | `8192` | LLMの最大出力トークン数 |
| `TEMPERATURE` | float | `0.7` | サンプリング温度 (0.0〜2.0) |
| `CONTEXT_WINDOW` | int | `32768` | コンテキストウィンドウサイズ（トークン数） |
| `OLLAMA_NUM_CTX` | int | `0` | Ollama KVキャッシュサイズ（後述） |
| `OLLAMA_NUM_GPU` | int | | Ollama GPUオフロードレイヤー数 |
| `PROVIDERS` | object | | プロバイダー別プロファイル（後述） |

### PROVIDERS プロファイル

`PROVIDERS` オブジェクト内の各キーはプロバイダー識別子で、値は以下の構造です：

```json
{
    "type": "openai",
    "host": "",
    "api_key": "sk-...",
    "model": "gpt-4.1",
    "max_tokens": 16384,
    "temperature": 0.5
}
```

| フィールド | 型 | 説明 |
|-----------|------|------|
| `type` | string | プロバイダー種別（`ollama`, `openai`, `anthropic`, `google`, `deepseek` 等） |
| `host` | string | ベースURL（ローカルプロバイダー用、例: `http://localhost:11434`） |
| `api_key` | string | APIキー（クラウドプロバイダー用） |
| `model` | string | このプロバイダーで使用するモデル名 |
| `max_tokens` | int | プロバイダー固有の最大トークン数（グローバル設定より優先） |
| `temperature` | float | プロバイダー固有の温度設定（グローバル設定より優先） |

**動作**: `PROVIDER` で指定されたアクティブプロバイダーに対応するプロファイルが自動適用されます。

### config.json の作成・更新方法

```bash
# 対話モードで現在の設定を保存
/config save

# 対話モードで現在の設定を確認
/config
```

手動でファイルを編集することもできます。

---

## CLIオプション

起動時に指定するオプションです。config.json や環境変数よりも優先されます。

### 接続・プロバイダー

| オプション | 短縮 | 説明 | 例 |
|-----------|------|------|-----|
| `--provider <name>` | | LLMプロバイダー名 | `--provider openai` |
| `--api-key <key>` | | APIキー | `--api-key sk-...` |
| `--model <name>` | `-m` | モデル名 | `-m qwen3:8b` |
| `--host <url>` | | ローカルプロバイダーURL | `--host http://localhost:1234` |

### LLMパラメータ

| オプション | デフォルト | 説明 |
|-----------|-----------|------|
| `--max-tokens <n>` | `8192` | 最大出力トークン数 |
| `--temperature <f>` | `0.7` | サンプリング温度 |
| `--context-window <n>` | `32768` | コンテキストウィンドウサイズ |

### Ollama固有オプション

| オプション | デフォルト | 説明 |
|-----------|-----------|------|
| `--num-ctx <n>` | `0` (Ollamaデフォルト) | KVキャッシュサイズ（メモリ節約用） |
| `--num-gpu <n>` | `-1` (未指定) | GPUオフロードレイヤー数 |

### 動作モード

| オプション | 説明 |
|-----------|------|
| `-p <prompt>` | ワンショットモード |
| `-y` | 全ツール自動許可モード |
| `--resume <id>` | セッション復旧（`last` またはID） |
| `--session-id <id>` | セッションIDを指定 |
| `--list-sessions` | セッション一覧表示 |
| `--version` | バージョン表示 |

### 使用例

```bash
# 基本的な使い方
vibe

# モデル指定
vibe --model qwen3-coder:30b

# メモリ節約（Ollamaで大型モデル使用時）
vibe --model qwen3-coder:30b --num-ctx 8192

# クラウドプロバイダー
vibe --provider openai --api-key sk-... --model gpt-4.1

# ワンショット + 自動許可
vibe -y -p "hello world in Python"

# ベンチマーク向け
vibe --provider ollama --model qwen3:8b --num-ctx 8192 -y -p "solve the exercise"
```

---

## 環境変数

シェル環境で設定します。config.json よりも優先されますが、CLIオプションには劣ります。

### LLM設定

| 変数 | 説明 | 例 |
|------|------|-----|
| `OLLAMA_HOST` | Ollama APIエンドポイント | `http://localhost:11434` |
| `OLLAMA_NUM_CTX` | Ollama KVキャッシュサイズ | `8192` |
| `OLLAMA_NUM_GPU` | Ollama GPUレイヤー数 | `99` |

### APIキー

| 変数 | プロバイダー |
|------|------------|
| `OPENAI_API_KEY` | OpenAI |
| `ANTHROPIC_API_KEY` | Anthropic |
| `GEMINI_API_KEY` | Google Gemini |
| `DEEPSEEK_API_KEY` | DeepSeek |
| `MISTRAL_API_KEY` | Mistral |
| `GROQ_API_KEY` | Groq |
| `OPENROUTER_API_KEY` | OpenRouter |
| `TOGETHER_API_KEY` | Together AI |
| `FIREWORKS_API_KEY` | Fireworks AI |
| `PERPLEXITY_API_KEY` | Perplexity |
| `COHERE_API_KEY` | Cohere |
| `ZAI_API_KEY` | Z.AI / Z.AI Coding Plan |
| `ZHIPU_API_KEY` | 智谱AI（中国版） |
| `MOONSHOT_API_KEY` | Moonshot (Kimi) |

### デバッグ

| 変数 | 説明 |
|------|------|
| `VIBE_LOCAL_DEBUG` | `1` でデバッグログ有効化 |

### 設定例（.bashrc / .zshrc）

```bash
# Ollama のメモリ節約設定
export OLLAMA_NUM_CTX=16384

# クラウドLLM のAPIキー
export OPENAI_API_KEY="sk-..."
export GEMINI_API_KEY="AIzaSy..."
```

---

## Ollama num_ctx の詳細

### num_ctx とは

Ollamaの `num_ctx` はリクエストごとのKVキャッシュサイズ（トークン数）を制御します。大きいほど長い会話に対応できますが、VRAM/RAMを多く消費します。

### メモリ使用量の目安

| モデル | num_ctx | メモリ使用量（概算） |
|--------|---------|-------------------|
| qwen3-coder:30b | 262144 (デフォルト) | ~45 GB |
| qwen3-coder:30b | 32768 | ~25 GB |
| qwen3-coder:30b | 8192 | ~20 GB |
| qwen3:8b | 32768 (デフォルト) | ~6 GB |
| qwen3:8b | 8192 | ~5 GB |

### 動作の仕組み

- `--num-ctx` **未指定** (デフォルト): num_ctx を送信しない → Ollamaがモデルの Modelfile 定義値を使用
- `--num-ctx 8192` 指定時: 8192 で開始 → コンテキスト超過エラー発生時に自動エスカレーション

### 自動エスカレーション

コンテキスト超過エラーが発生した場合、vibe は自動的により大きな num_ctx で再試行します。

```
指定値 → (超過) → 8192 → 16384 → 32768 → 65536
```

例: `--num-ctx 8192` で開始した場合:
1. まず 8192 で送信
2. コンテキスト超過なら 16384 で再試行
3. まだ超過なら 32768 で再試行
4. まだ超過なら 65536 で再試行
5. すべて失敗したらエラー

`--num-ctx` 未指定の場合は Ollama デフォルト（モデル Modelfile の値）で送信し、超過した場合のみエスカレーション段階を順に試します。

### 推奨設定

| マシンスペック | モデル | 推奨 num_ctx |
|-------------|--------|-------------|
| 64GB RAM + Apple Silicon | qwen3-coder:30b | `8192`〜`16384` |
| 64GB RAM + Apple Silicon | qwen3:8b | (指定不要、デフォルトで十分) |
| 32GB RAM | qwen3:8b | `8192`〜`16384` |
| 16GB RAM | qwen3:4b | `8192` |

---

## 設定の優先順位

同じ設定項目が複数の場所で指定された場合、以下の順序で優先されます：

```
CLI オプション  >  環境変数  >  config.json  >  デフォルト値
```

さらに、config.json 内では：

```
PROVIDERS[activeProvider] のプロファイル値  >  グローバルフィールド値
```

例: `PROVIDER` が `"openai"` の場合、`PROVIDERS.openai.model` が `MODEL` より優先されます。

---

## 対応プロバイダー一覧

### ローカルLLM

| プロバイダー | デフォルトホスト | 備考 |
|------------|-----------------|------|
| `ollama` | `http://localhost:11434` | モデル管理対応（pull/list） |
| `lm-studio` | `http://localhost:1234/v1` | GUI管理 |
| `llama-server` | `http://localhost:8080/v1` | llama.cpp |

### クラウドLLM

| プロバイダー | type 値 | 主要モデル |
|-----------|---------|---------|
| OpenRouter | `openrouter` | gemini-2.5-flash, claude-sonnet-4 |
| OpenAI | `openai` | gpt-4.1, gpt-4.1-mini, o3 |
| Anthropic | `anthropic` | claude-sonnet-4, claude-opus-4 |
| Google Gemini | `google` | gemini-2.5-flash, gemini-2.5-pro |
| DeepSeek | `deepseek` | deepseek-chat, deepseek-reasoner |
| Mistral | `mistral` | mistral-large-latest |
| Groq | `groq` | llama-3.3-70b-versatile |
| Together AI | `together` | Llama-3.3-70B |
| Fireworks AI | `fireworks` | llama-v3p3-70b-instruct |
| Perplexity | `perplexity` | sonar-pro |
| Cohere | `cohere` | command-a-03-2025 |
| Z.AI | `zai` | glm-4.7 |
| Z.AI Coding | `zai-coding` | glm-4.7 |
| 智谱AI | `zhipu` | glm-4.7 |
| Moonshot | `moonshot` | kimi-k2-instruct |
