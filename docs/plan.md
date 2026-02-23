# Agent Skills + MCP + Python版v1.0-v1.3機能 移植プラン

Python版 vibe-local v1.0〜v1.3 で追加された機能のうち、Go版に未実装のものを移植する。

---

## 機能一覧と実装優先度

| # | 機能 | Python版 | Go版 | 優先度 | 難易度 |
|---|------|---------|------|--------|--------|
| 1 | **Agent Skills** | v1.0 | ✅ | 高 | 中 |
| 2 | **MCP Client** | v1.0 | ✅ | 高 | 高 |
| 3 | **Plan/Act Mode** | v1.0 | ✅ | 中 | 中 |
| 4 | **Git Checkpoint** | v1.0 | ✅ | 中 | 低 |
| 5 | **Auto Test Loop** | v1.0 | ✅ | 中 | 中 |
| 6 | **File Watcher** (`/watch`) | v1.1 | ❌ | 低 | 低 |
| 7 | **Parallel Agents** | v1.1 | ❌ | 低 | 高 |
| 8 | **ESC 割り込み** | v1.3 | ✅ | 中 | 低 |
| 9 | **ステータス行** (Thinking... 8s) | v1.3 | ✅ | 中 | 低 |
| 10 | **Type-ahead 入力** | v1.3 | ❌ | 低 | 中 |

---

## Part 1: Agent Skills (Step 1-5)

### スキルの配置場所（Claude Code準拠）
```
~/.config/vibe-local-go/skills/   ← グローバルスキル
.vibe-local/skills/               ← プロジェクトスキル（cwdベース）
```

### SKILL.md フォーマット
```markdown
---
name: pdf-processing
description: PDFの読み取り、テキスト抽出、結合を行う。
---
# PDF Processing
## 手順
...
```

### 段階的開示
| レベル | タイミング | 内容 |
|--------|-----------|------|
| L1 | 起動時 | YAML frontmatter → システムプロンプトに注入 |
| L2 | トリガー時 | SKILL.md 本文 → read_file で読込 |
| L3 | 必要時 | 追加ファイル → read_file/bash で読込・実行 |

### Step 1: `internal/skill/skill.go` 新規作成
- `SkillMeta` 構造体（Name, Description, Dir, SkillFile, Source）
- `SkillManager` — LoadSkills, GetSkillMetadata, GetSkillByName
- YAML frontmatter パーサー（`strings` ベース、外部ライブラリ不要）

### Step 2: `internal/config/prompt.go` 拡張
- `BuildSystemPrompt(cfg, skills)` にスキルメタデータ一覧を注入
- LLMへの指示：「関連リクエスト時は read_file で SKILL.md を読んでから作業」

### Step 3: `cmd/vibe/main.go` 起動フロー
- SkillManager 初期化 → LoadSkills → BuildSystemPrompt に渡す

### Step 4: `/skills` スラッシュコマンド

### Step 5: `/help` 更新

---

## Part 2: MCP Client (Step 6-9)

### 設定ファイル（Claude Code互換）
```json
// ~/.config/vibe-local-go/mcp.json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
      "env": {}
    }
  }
}
```

### Step 6: `internal/mcp/` パッケージ新規作成

**`internal/mcp/client.go`** — stdio JSON-RPC 2.0 クライアント
- MCPClient: プロセス起動、stdin/stdout経由のJSON-RPC通信
- Initialize() → tools/list → tools/call

**`internal/mcp/manager.go`** — 複数サーバー管理
- LoadConfig, StartAll, StopAll, GetTools, CallTool

**`internal/mcp/config.go`** — mcp.json パーサー

**`internal/mcp/tool_adapter.go`** — MCPツール → tool.Tool アダプター
- ツール名: `mcp_{server}_{tool}` 形式
- 既存の tool.Registry にそのまま登録

### Step 7: ツールレジストリへの統合
- MCPツールを tool.Registry に Register

### Step 8: 起動フローへの組み込み + シャットダウン時の子プロセス終了

### Step 9: `/mcp` スラッシュコマンド

---

## Part 3: Plan/Act Mode (Step 10-11)

### 概要
2段階モード: `/plan` で読み取り専用計画モード、`/execute` で実行モード。
計画モードではファイル変更ツール（write_file, edit_file, bash）を制限。

### Step 10: Agent に PlanMode フラグ追加
- `agent.planMode bool` フラグ
- planMode=true 時は `write_file`, `edit_file`, bash の書込み系を拒否
- read_file, glob, grep は許可

### Step 11: `/plan`, `/execute` コマンド登録

---

## Part 4: Git Checkpoint (Step 12)

### 概要
エージェントがファイルを変更する前に `git stash` でチェックポイントを作成。
`/undo` で最後の変更をロールバック。

### Step 12: `internal/git/checkpoint.go` 新規作成
- `CreateCheckpoint()` — `git stash push -m "vibe-checkpoint-{timestamp}"`
- `Rollback()` — `git stash pop`
- `/undo` コマンドから呼び出し
- git リポジトリ外では無効化

---

## Part 5: Auto Test Loop (Step 13)

### 概要
ファイル編集後に自動でテスト/リント実行。失敗時はエラーをLLMにフィードバック。

### Step 13: `internal/agent/autotest.go` 新規作成
- テストフレームワーク自動検出:
  - `pytest` (pytest.ini, setup.py)
  - `npm test` (package.json)
  - `go test` (go.mod)
  - `cargo test` (Cargo.toml)
- write_file/edit_file 後にテスト自動実行
- 失敗時: エラー出力をセッションに追加してLLMに修正させる
- `/autotest [on|off]` コマンドで有効/無効切替

---

## Part 6: ESC 割り込み + ステータス行 (Step 14-15)

### Step 14: ESC 割り込み
- エージェント実行中に ESC キーで中断
- raw mode でキー入力を監視（Unix限定、`x/term`利用）

### Step 15: ステータス行
- LLM応答待ち中に `Thinking... (8s · ↓ 1.2k tokens)` を表示
- 経過時間とトークン数をリアルタイム更新
- `\r` + ANSI escape で同一行を上書き

---

## ファイル変更一覧

| ファイル | 変更内容 |
|---------|---------|
| **Skills** | |
| `internal/skill/skill.go` | **新規** — SkillMeta, SkillManager |
| `internal/config/prompt.go` | スキルメタデータ注入 |
| `cmd/vibe/main.go` | SkillManager初期化 + `/skills` |
| **MCP** | |
| `internal/mcp/client.go` | **新規** — MCPClient (JSON-RPC 2.0) |
| `internal/mcp/manager.go` | **新規** — MCPManager |
| `internal/mcp/config.go` | **新規** — mcp.json パーサー |
| `internal/mcp/tool_adapter.go` | **新規** — tool.Tool アダプター |
| `cmd/vibe/main.go` | MCP初期化 + `/mcp` |
| **Plan/Act** | |
| `internal/agent/agent.go` | planMode フラグ + ツール制限 |
| `cmd/vibe/main.go` | `/plan`, `/execute` コマンド |
| **Git Checkpoint** | |
| `internal/git/checkpoint.go` | **新規** — stash ベースのチェックポイント |
| `cmd/vibe/main.go` | `/undo` コマンド実装 |
| **Auto Test** | |
| `internal/agent/autotest.go` | **新規** — テスト自動検出・実行 |
| `cmd/vibe/main.go` | `/autotest` コマンド |
| **UX** | |
| `internal/ui/terminal.go` | ESC割り込み + ステータス行 |
| `internal/ui/commands.go` | `/help` 全面更新 |

## 実装順序

```
Phase 1: Skills (Step 1-5) ← まず最初に
Phase 2: MCP (Step 6-9) ← Skills の次
Phase 3: Plan/Act + Git Checkpoint (Step 10-12)
Phase 4: Auto Test + UX (Step 13-15)
```

## 設計のポイント

1. **Skills**: 新ツール不要。既存 read_file でSKILL.md読込（Claude Code方式）
2. **MCP**: 外部ライブラリ不要。encoding/json + os/exec + bufio で実装可能
3. **MCP**: tool.Tool アダプターパターンで既存レジストリにそのまま統合
4. **Plan/Act**: Agent の executeSingleTool にガードを追加するだけ
5. **Git**: os/exec で git コマンドを直接実行。gitが無い環境では自動スキップ
6. **Auto Test**: write_file/edit_file のフック。失敗をセッションに戻してLLMに修正させる
