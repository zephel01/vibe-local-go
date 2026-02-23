# CLAUDE.md Format Specification

`CLAUDE.md` は vibe-local-go にプロジェクト固有の指示を与えるためのファイルです。プロジェクトルートに配置することで、AIエージェントがプロジェクトの文脈を理解し、適切なコードを生成するのを支援します。

## 基本形式

`CLAUDE.md` は標準的な Markdown 形式で記述します。

```markdown
# Project Name

## Overview
Brief description of the project.

## Tech Stack
List of technologies and frameworks used.

## Code Style
Coding conventions and guidelines.

## Important Notes
Any critical information the AI should know.
```

## 推奨セクション

以下のセクションを含めることを推奨します：

### 1. Overview / 概要
プロジェクトの目的とスコープを簡潔に説明します。

```markdown
## Overview
This is a CLI tool for managing Docker containers.
It supports create, start, stop, and delete operations.
```

### 2. Tech Stack / 技術スタック
使用している技術、ライブラリ、バージョンを列挙します。

```markdown
## Tech Stack
- Go 1.26
- Ollama (local LLM runtime)
- No external dependencies
```

### 3. Code Style / コードスタイル
コーディング規約、命名規則、フォーマットルールを指定します。

```markdown
## Code Style
- Use table-driven tests for test functions
- Keep functions under 50 lines
- Follow Go conventions (gofmt, go vet)
- Use descriptive variable names
```

### 4. Architecture / アーキテクチャ
プロジェクトの構造、重要なパッケージ、データフローを説明します。

```markdown
## Architecture
```
cmd/vibe/       # Entry point
internal/
  ├── agent/     # Agent loop and tool execution
  ├── llm/       # LLM client and streaming
  ├── tool/      # Built-in tools
  └── ui/        # Terminal UI
```
```

### 5. Important Notes / 重要事項
AIが知っておくべき重要な情報、注意点、制約事項を記述します。

```markdown
## Important Notes
- Always write tests for new functions
- Do not add external dependencies without approval
- Performance is critical: avoid unnecessary allocations
```

### 6. Examples / 使用例
典型的な使用パターンやコード例を示します。

```markdown
## Examples
```go
func ExampleFunction() {
    client := NewClient("http://localhost:11434")
    resp, err := client.Chat(context.Background(), &ChatRequest{...})
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(resp.Message)
}
```
```

### 7. Development Workflow / 開発ワークフロー
開発手順、ビルド方法、テスト手順を説明します。

```markdown
## Development Workflow
1. Make code changes
2. Run `go test ./...` to verify
3. Run `gofmt` to format code
4. Run `go build -o vibe-local-go ./cmd/vibe` to build
```

## 完全な例

以下は完全な `CLAUDE.md` の例です：

```markdown
# vibe-local-go

## Overview
A Go-based AI coding agent that runs locally with Ollama.
It provides 15 built-in tools for file operations, command execution, and code editing.

## Tech Stack
- Go 1.26
- Ollama (local LLM runtime)
- Standard library only (no external dependencies)

## Architecture
```
cmd/vibe/           # Entry point (main.go)
internal/
  ├── agent/         # Agent loop, dispatcher, loop detector
  ├── config/        # Configuration and model selection
  ├── llm/           # LLM client, streaming, XML fallback
  ├── tool/          # Built-in tools (bash, file, glob, grep, etc.)
  ├── security/      # Permission manager, path validation
  ├── session/       # Session management and persistence
  └── ui/            # Terminal UI, markdown rendering
```

## Code Style
- Use table-driven tests
- Keep functions focused and under 50 lines
- Follow Go conventions (gofmt, go vet)
- Use descriptive variable and function names
- Add comments for exported functions
- Handle errors explicitly (don't ignore them)

## Important Notes
- Always write tests for new functions
- Zero dependencies: use only Go standard library
- Performance is critical for streaming and tool execution
- Security: validate all user inputs and file paths
- Tests cover all major functionality (338 tests)

## Development Workflow
1. Make code changes
2. Run tests: `go test ./... -v`
3. Format code: `gofmt -w .`
4. Check for issues: `go vet ./...`
5. Build binary: `go build -o vibe-local-go ./cmd/vibe`
6. Test manually: `./vibe-local-go`

## Testing
- Unit tests for each package
- Table-driven tests for complex logic
- Mock dependencies where appropriate
- Test coverage goal: 80%+

## Model Compatibility
- Works with OpenAI-compatible APIs (Ollama)
- Supports streaming responses
- XML fallback for models that don't support tool calls
- CJK character support for token counting
```

## ベストプラクティス

### 簡潔に保つ
- 重要な情報のみを含める
- 冗長な説明は避ける
- 箇条書きを使用する

### 具体的にする
- 抽象的な指示ではなく具体的な例を示す
- コードスニペットを含める
- バージョン番号や具体的な設定を指定する

### 更新する
- プロジェクトが進化したら `CLAUDE.md` を更新する
- 新しい規約や技術が追加されたら反映する
- 古い情報を削除する

### AIに優しい
- 簡潔な段落で構成する
- セクションを明確に分ける
- 重要なキーワードを強調する

## 命名規則

以下の命名規則を `CLAUDE.md` 内で使用すると一貫性が保たれます：

- `##` レベルの見出しは主要セクション
- `###` レベルの見出しはサブセクション
- コードブロックには言語を指定する（```go, ```bash など）
- 重要なポイントは箇条書きで列挙する

## トラブルシューティング

AIが `CLAUDE.md` の指示に従わない場合：

1. **指示が曖昧** → 具体的な例を追加する
2. **情報が多すぎる** → 簡潔にする、重要な部分に焦点を当てる
3. **矛盾がある** → 指示の整合性を確認する
4. **古い情報** → プロジェクトの現状に合わせて更新する

## バージョン管理

`CLAUDE.md` はプロジェクトの一部として Git で管理することを推奨します。

- チーム開発ではプルリクエストでレビューする
- プロジェクトの進化に合わせて定期的に更新する
- 変更履歴をコミットメッセージで追跡する

## 関連リソース

- **README.md**: インストールと使用方法
- **task.md**: 開発タスクリスト
- **LICENSE**: プロジェクトライセンス

---

## 日本語での記述例

```markdown
# My Project

## 概要
これは Go で書かれた CLI ツールです。

## 技術スタック
- Go 1.26
- Ollama

## コードスタイル
- 表駆動テストを使用する
- 関数は 50 行以内に収める
- Go の規約に従う（gofmt, go vet）

## 重要事項
- 常にテストを書く
- パフォーマンスを重視する
- エラーを適切に処理する
```
