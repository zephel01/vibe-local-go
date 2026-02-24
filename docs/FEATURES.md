# vibe-local-go v1.1.0 新機能ガイド

Phase 14 で追加された機能の利用方法をまとめたドキュメント。

---

## 1. ゼロコンフィグ自動初期化

### 概要

プロバイダーを指定せずに `vibe` を起動すると、ローカルで動作中の LLM サーバーを自動検出し、最適なプロバイダーチェーンを構築する。

### 検出対象

| プロバイダー | ポート | エンドポイント |
|---|---|---|
| Ollama | 11434 | `/api/tags` |
| llama-server | 8080 | `/v1/models` |
| LM Studio | 1234 | `/api/v1/models` |
| LiteLLM | 4000 | `/v1/models` |
| カスタム (`VIBE_LLM_URL`) | 任意 | `/v1/models` |

### 動作フロー

```
vibe 起動
  │
  ├─ --provider 指定あり → 従来通り単一プロバイダーで起動
  │                        （クラウドフォールバック自動追加）
  │
  └─ --provider 指定なし → ゼロコンフィグモード
       │
       ├─ ローカルサーバー並行検出（タイムアウト2秒）
       │   ├─ 検出成功 → 最優先プロバイダーをメインに設定
       │   │              他のローカルをサブに追加
       │   │              クラウドAPIキーがあればフォールバックに追加
       │   │
       │   └─ 検出失敗 → 環境変数からクラウドAPIキーを探索
       │                  ├─ 見つかった → クラウドプロバイダーで起動
       │                  └─ 見つからない → デフォルト(Ollama)で接続試行
       │
       └─ ProviderChain を Agent に渡す
```

### 使い方

```bash
# ゼロコンフィグ: 何も指定しない
vibe

# 出力例:
# 🔍 LLMプロバイダーを自動検出中...
# ✓ ollama 検出 (http://localhost:11434, モデル: 3件)
#   + lm-studio (http://localhost:1234) をサブプロバイダーに追加
#   + openai をフォールバックに追加
```

### 環境変数によるクラウドフォールバック

以下の環境変数が設定されていると、ローカル障害時のフォールバック先として自動追加される（優先順）:

```bash
export OPENAI_API_KEY="sk-..."       # 優先度1
export ANTHROPIC_API_KEY="sk-ant-..."  # 優先度2
export GOOGLE_API_KEY="..."            # 優先度3
export DEEPSEEK_API_KEY="..."          # 優先度4
```

---

## 2. ProviderChain フォールバック

### 概要

複数の LLM プロバイダーをチェーン構成し、メインプロバイダーが障害を起こした際に自動的に次のプロバイダーへ切り替える。

### フォールバック条件

| エラー種別 | デフォルト | 例 |
|---|---|---|
| ネットワークエラー | ✅ 有効 | 接続拒否、ホスト不在 |
| タイムアウト | ✅ 有効 | context deadline exceeded |
| サーバーエラー (5xx) | ✅ 有効 | HTTP 500, 502, 503 |
| コンテキスト超過 | ✅ 有効 | context length exceeds |
| レート制限 | ❌ 無効 | rate limit, too many requests |
| クライアントエラー (4xx) | ❌ 無効 | HTTP 401, 404 |

レート制限はリトライ（指数バックオフ）で対応するため、デフォルトではフォールバック対象外。

### チェーン構成例

```
[Main: Ollama qwen3:8b (ローカル)]
    │ 成功 → 応答返却
    │ 失敗 ↓
    ▼
[Sub: LM Studio codestral:22b (ローカル別モデル)]
    │ 成功 → 応答返却 + "⚠ サブモデルで応答" 表示
    │ 失敗 ↓
    ▼
[Fallback: OpenAI gpt-4o (クラウド)]
    │ 成功 → 応答返却 + "☁ クラウドフォールバック" 表示
    │ 失敗 ↓
    ▼
エラー表示（全プロバイダー失敗）
```

### /chain コマンド

```bash
# チェーン状態を表示
/chain

# 出力例:
# ━━━ プロバイダーチェーン ━━━
#   ▶ 🦙 ollama [main] model=qwen3:8b
#     🤖 openai [fallback] model=gpt-4o
#
#   フォールバック: 有効

# 手動でプロバイダーを切り替え（インデックス指定）
/chain 1
# ✓ openai に切り替えました
```

### フォールバック発生時の通知

フォールバックが発生するとターミナルに通知が表示される:

```
⚠ ollama に接続できません → openai にフォールバック
⏱ ollama がタイムアウト → openai にフォールバック
🔴 ollama がエラー状態 → openai にフォールバック
📚 ollama のコンテキストが不足 → openai にフォールバック
```

---

## 3. NotebookEdit ツール

### 概要

Jupyter Notebook (`.ipynb`) のセルを編集するツール。エージェントが LLM のツール呼び出しとして使用する。

### パラメータ

| パラメータ | 型 | 必須 | 説明 |
|---|---|---|---|
| `notebook_path` | string | ✅ | .ipynb ファイルのパス |
| `cell_number` | int | ✅ | 対象セル番号（0始まり） |
| `edit_mode` | string | - | `replace`(デフォルト), `insert`, `delete` |
| `new_source` | string | - | セルの新しい内容（delete時は不要） |
| `cell_type` | string | - | `code` または `markdown`（insert時は必須） |

### 操作モード

**replace** — 既存セルの内容を置換:
```json
{
  "notebook_path": "analysis.ipynb",
  "cell_number": 2,
  "new_source": "import pandas as pd\ndf = pd.read_csv('data.csv')\ndf.head()"
}
```

**insert** — 指定位置に新しいセルを挿入:
```json
{
  "notebook_path": "analysis.ipynb",
  "cell_number": 3,
  "edit_mode": "insert",
  "cell_type": "code",
  "new_source": "# データの可視化\ndf.plot(kind='bar')"
}
```

**delete** — 指定セルを削除:
```json
{
  "notebook_path": "analysis.ipynb",
  "cell_number": 5,
  "edit_mode": "delete"
}
```

### 注意事項

- ノートブックのメタデータ（カーネル情報、nbformat等）は保持される
- セル出力（outputs）は replace/insert 時にクリアされる
- セル番号が範囲外の場合はエラーを返す

---

## 4. PDF テキスト抽出

### 概要

`file_read` ツールで `.pdf` ファイルを読むと、自動的にテキスト抽出が行われる。外部依存なし（Pure Go 実装）。

### 使い方

エージェントは通常の `file_read` ツールとして使用する:

```json
{
  "file_path": "report.pdf"
}
```

### 対応形式

- FlateDecode（zlib圧縮）ストリームの解凍
- Tj / TJ オペレータによるテキスト抽出
- UTF-16BE エンコーディングのデコード
- 最大50ページまで抽出

### 制限事項

- 画像ベースの PDF（スキャン文書等）は OCR 非対応
- 暗号化された PDF は非対応
- 複雑なフォントマッピング（CMap）は部分対応
- 結合結果にレイアウト情報は含まれない（プレーンテキスト）

---

## 5. ファイル監視（/watch）

### 概要

指定パターンのファイルを監視し、変更を検知してエージェントのコンテキストに自動注入する。ポーリングベースで外部依存なし。

### 使い方

```bash
# パターン指定で監視開始
/watch *.go

# 複数パターン
/watch *.go *.ts src/**/*.jsx

# 監視状態の確認
/watch

# 監視停止
/watch off
```

### 動作仕様

- ポーリング間隔: 1秒
- デバウンス: 500ms（短時間の連続変更をまとめる）
- 検知イベント: 作成(created), 変更(modified), 削除(deleted)

### 自動除外パターン

以下のディレクトリは自動的に監視対象外:

```
.git/  node_modules/  __pycache__/  .venv/  vendor/
dist/  build/  .next/  target/  *.pyc  *.o  *.exe
```

### 変更通知の例

ファイル変更が検知されると以下のように表示される:

```
[Watch] 2 ファイルが変更されました
  modified: src/main.go
  created: src/utils.go
```

変更内容はエージェントのセッションに自動挿入され、次の応答で参照可能になる。

---

## 6. 並列サブエージェント

### 概要

複数の独立タスクを並列実行するサブエージェント機構。最大4エージェントが同時に動作し、結果を統合して返す。

### パラメータ（parallel_agents ツール）

| パラメータ | 型 | 必須 | 説明 |
|---|---|---|---|
| `tasks` | array | ✅ | タスク配列（最大4件） |
| `tasks[].description` | string | ✅ | タスクの説明 |
| `tasks[].prompt` | string | ✅ | サブエージェントへの指示 |

### 使い方の例

エージェントが内部的に呼び出す:

```json
{
  "tasks": [
    {
      "description": "テストファイルの調査",
      "prompt": "test/ ディレクトリのテストファイルを読んで構成を報告して"
    },
    {
      "description": "依存関係の確認",
      "prompt": "go.mod を読んで外部依存関係を一覧にして"
    },
    {
      "description": "TODO コメントの収集",
      "prompt": "ソースコード内の TODO/FIXME コメントを検索して一覧にして"
    }
  ]
}
```

### 安全機構

- **書き込み競合検知**: 複数エージェントが同じファイルに書き込もうとした場合、実行前に警告
- **読み取り専用モード**: サブエージェントはデフォルトで読み取り専用（write_file, edit_file は使用不可）
- **ターン制限**: 各サブエージェントは最大20ターンで強制終了
- **独立セッション**: 各サブエージェントは独立したセッションとループ検出器を持つ

### 進捗表示

実行中は各エージェントの状態がリアルタイムで表示される:

```
🚀 [1/3] テストファイルの調査
🔄 [2/3] 依存関係の確認 — glob 実行中
✅ [3/3] TODO コメントの収集 — 完了（3ターン）
```

---

## コマンド一覧（v1.1.0 追加分）

| コマンド | 説明 |
|---|---|
| `/watch <pattern>` | ファイル監視を開始 |
| `/watch` | 監視状態を表示 |
| `/watch off` | 監視を停止 |
| `/chain` | プロバイダーチェーンの状態表示 |
| `/chain <番号>` | プロバイダーを手動切替 |

## ツール一覧（v1.1.0 追加分）

| ツール名 | 権限 | 説明 |
|---|---|---|
| `notebook_edit` | 要確認 | Jupyter Notebook セル編集 |
| `parallel_agents` | 安全 | 並列サブエージェント実行 |

`file_read` ツールは既存だが、v1.1.0 で `.pdf` 拡張子の自動テキスト抽出に対応。

---

**更新日**: 2026-02-25
**バージョン**: v1.1.0
