# vibe-local-go 実装タスク一覧

> 作成日: 2026-02-23
> Python版: vibe-coder.py v0.9.4（5,650行）→ Go版 v1.0
> 見積もり: 5-8週間（1人開発）

## 凡例

- `[ ]` 未着手
- `[x]` 完了
- `[~]` 進行中
- 各タスクに **Python版の対応行番号** を記載（移植の参照元）
- **依存関係** を明記（先にやるべきタスクの番号）
- **推定行数** は Go 版の予想

---

## Phase 0: プロジェクト初期化

### T-000: リポジトリ初期化
- [x] `go mod init github.com/user/vibe-local-go`
- [x] ディレクトリ構成作成（cmd/, internal/, Makefile）
- [x] .gitignore（dist/, *.exe, .DS_Store）
- [x] LICENSE（MIT）
- 依存: なし
- 推定: 設定ファイルのみ

---

## Phase 1: コア基盤（Week 1-2）

### T-100: config パッケージ
**Python版: class Config（160-579行, 420行）+ _get_ram_gb()（580-615行）**

- [x] T-101: 基本構造体と定数定義
  - Config struct, デフォルト値
  - DEFAULT_OLLAMA_HOST, DEFAULT_MAX_TOKENS(8192), DEFAULT_TEMPERATURE(0.7), DEFAULT_CONTEXT_WINDOW(32768)
  - 推定: ~80行 | 依存: T-000

- [x] T-102: CLI 引数パーサー（flag パッケージ）
  - `--model`, `--sidecar`, `--host`, `-p`, `-y`, `--resume`, `--session-id`
  - `--max-tokens`, `--temperature`, `--context-window`
  - `--list-sessions`, `--version`
  - 推定: ~60行 | 依存: T-101

- [x] T-103: 設定ファイル読み込み
  - ~/.config/vibe-local/config.json
  - 環境変数（VIBE_LOCAL_MODEL, VIBE_LOCAL_HOST, etc.）
  - 優先順位: CLI > ENV > config.json > defaults
  - 推定: ~80行 | 依存: T-101

- [x] T-104: メモリ検出 + モデル自動選択（internal/config/memory.go）
  - macOS: sysctl hw.memsize
  - Linux: /proc/meminfo
  - Windows: wmic OS get TotalVisibleMemorySize
  - モデルティア定義（S/A/B/C/D/E）とRAM要件マッピング
  - KVキャッシュサイズの動的計算
  - 推定: ~150行 | 依存: T-101

- [x] T-105: OS固有ヒント生成（internal/config/platform.go）
  - macOS: brew, /Users/, system_profiler ヒント
  - Linux: apt/dnf, /home/ ヒント
  - Windows: winget, %USERPROFILE%, PowerShell ヒント
  - 推定: ~80行 | 依存: T-101

---

### T-200: LLM クライアント
**Python版: class OllamaClient（792-1081行, 290行）**

- [x] T-201: HTTP クライアント基盤（internal/llm/client.go）
  - Client struct（baseURL, httpClient, timeout=300s）
  - check_connection()（リトライ3回）
  - check_model()（モデル存在確認）
  - pull_model()（プログレスバー付きダウンロード）
  - 推定: ~150行 | 依存: T-101

- [x] T-202: 同期チャット（ツールコール用）
  - ChatSync() — POST /v1/chat/completions, stream=false
  - ChatRequest/ChatResponse 構造体
  - ツール使用時 temperature=0.3 に自動切替
  - エラーハンドリング（404→モデル未発見、400→コンテキスト超過）
  - <think> ブロック除去
  - 推定: ~120行 | 依存: T-201

- [x] T-203: ストリーミングチャット（internal/llm/streaming.go）
  - ChatStream() → <-chan StreamEvent
  - SSE パーサー（goroutine + channel）
  - バッファ制限: 1MB
  - 接続断・タイムアウト処理
  - context.Context によるキャンセル
  - 推定: ~130行 | 依存: T-201

- [x] T-204: XML フォールバック（internal/llm/xml_fallback.go）
  - Python版: _extract_tool_calls_from_text()（3317-3451行, 135行）
  - 3パターン対応: invoke, function=, シンプルタグ
  - コードブロック除去（インジェクション防止）
  - 既知ツール名フィルタ
  - JSON値自動パース
  - 重複排除
  - 推定: ~150行 | 依存: T-201

---

### T-300: ツール基盤
**Python版: Tool ABC（1097-1118行）+ ToolRegistry（3156-3186行）+ ToolResult（1087-1095行）**

- [x] T-301: Tool インターフェース + Result 構造体（internal/tool/registry.go）
  - Tool interface: Name(), Execute(ctx, params), Schema()
  - Result struct: Output, IsError
  - FunctionSchema struct（OpenAI function calling 形式）
  - 推定: ~40行 | 依存: T-000

- [x] T-302: ツールレジストリ
  - Registry struct: tools map[string]Tool, schemaCache
  - Register(), Get(), Names(), GetSchemas()
  - スレッドセーフ（sync.RWMutex）
  - 推定: ~60行 | 依存: T-301

- [x] T-303: スキーマ生成ヘルパー（internal/tool/schema.go）
  - OpenAI function calling JSON スキーマの構築ヘルパー
  - パラメータ定義の DSL 的な構造体
  - 推定: ~80行 | 依存: T-301

---

### T-400: TUI 基盤
**Python版: class TUI（3941-4596行, 656行）**

- [x] T-401: ターミナル基本入出力（internal/ui/terminal.go）
  - Terminal struct
  - Print(), PrintColored(), PrintError()
  - ANSI カラー定数（C クラス相当）
  - ターミナル幅検出
  - CJK 文字幅計算
  - 推定: ~100行 | 依存: T-000

- [x] T-402: 行入力（readline 互換）
  - golang.org/x/term による raw モード入力
  - ヒストリ（↑↓キー）
  - 複数行入力（"""）
  - Ctrl+C / Ctrl+D 処理
  - 推定: ~120行 | 依存: T-401

- [x] T-403: ストリーミング表示
  - stream_response() — channel からトークンを受信して逐次表示
  - <think> ブロックのフィルタリング
  - スピナー表示（ツール実行中）
  - 推定: ~80行 | 依存: T-401, T-203

- [x] T-404: ツール呼び出し表示 + 結果表示
  - show_tool_call()（ツール名・パラメータ表示）
  - show_tool_result()（結果のフォーマット表示、長文の切り詰め）
  - 推定: ~60行 | 依存: T-401

- [x] T-405: パーミッション確認 UI
  - ask_permission()（y/n/always/deny プロンプト）
  - 推定: ~40行 | 依存: T-401

---

## Phase 2: ツール実装（Week 2-3）

### T-500: Bash ツール
**Python版: class BashTool（1120-1382行, 263行）**

- [x] T-501: 基本コマンド実行（internal/tool/bash.go）
  - os/exec.CommandContext
  - タイムアウト（デフォルト120s、最大600s）
  - 出力キャプチャ（stdout + stderr 結合）
  - 出力切り詰め（30,000文字 → 15k+15k）
  - 推定: ~80行 | 依存: T-301

- [x] T-502: バックグラウンド実行
  - run_in_background パラメータ
  - goroutine + 結果保存（sync.Map）
  - タスクID管理（MAX_BG_TASKS=50）
  - 古いタスク(>1h)の自動クリーンアップ
  - 推定: ~60行 | 依存: T-501

- [x] T-503: セキュリティ（危険コマンド検出）
  - 危険パターン: rm -rf /, sudo, mkfs, dd of=/dev/
  - 環境変数サニタイズ（GITHUB_TOKEN等の除外）
  - パイプ攻撃検出（curl | sh）
  - 推定: ~80行 | 依存: T-501

---

### T-600: ファイル操作ツール

- [x] T-601: ReadTool（internal/tool/file_read.go）
  - Python版: 1401-1633行, 233行
  - テキスト読み込み（行番号付き、offset/limit）
  - 画像 base64 エンコード（PNG/JPG/GIF/WebP/SVG/ICO/TIFF/BMP）
  - PDF テキスト抽出（FlateDecode/zlib、ページ指定）
  - Jupyter .ipynb 読み込み
  - バイナリ検出（null byte チェック）
  - サイズ制限: テキスト100MB、画像10MB
  - symlink 解決
  - 推定: ~250行 | 依存: T-301

- [x] T-602: WriteTool（internal/tool/file_write.go）
  - Python版: 1654-1743行, 90行
  - アトミック書き込み（tempfile + os.Rename）
  - 親ディレクトリ自動作成
  - サイズ制限: 10MB
  - symlink 保護
  - 保護ファイルへの書き込みブロック
  - Undo スタック（最大20エントリ）
  - 推定: ~100行 | 依存: T-301

- [x] T-603: EditTool（internal/tool/file_edit.go）
  - Python版: 1745-1903行, 159行
  - old_string → new_string 置換
  - replace_all パラメータ
  - Unicode NFC 正規化（macOS互換）
  - 一意性チェック（複数マッチ時エラー）
  - Unified diff 生成（最大40行）
  - サイズ制限: 50MB
  - Undo スタック連携
  - 推定: ~140行 | 依存: T-301

---

### T-700: 検索ツール

- [x] T-701: GlobTool（internal/tool/glob.go）
  - Python版: 1906-2014行, 109行
  - filepath.WalkDir + パターンマッチ
  - ** サポート（filepath.Match + 再帰）
  - 結果制限: MAX_RESULTS=200
  - mtime 降順ソート
  - SKIP_DIRS: .git, node_modules, __pycache__, etc.
  - 推定: ~100行 | 依存: T-301

- [x] T-702: GrepTool（internal/tool/grep.go）
  - Python版: 2016-2211行, 196行
  - regexp パッケージ（Go は RE2 = ReDoS 安全）
  - 3モード: content / files_with_matches / count
  - コンテキスト行（-A, -B, -C）
  - 大規模ファイルスキップ（50MB）
  - 行番号表示、head_limit
  - ファイルタイプフィルタ
  - 推定: ~170行 | 依存: T-301

---

### T-800: Web ツール

- [ ] T-801: WebFetchTool（internal/tool/web_fetch.go）
  - Python版: 2213-2346行, 134行
  - HTTP GET + HTML→テキスト変換
  - SSRF 防御（プライベートIP ブロック: 127.0.0.0/8, 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fd00::/8）
  - リダイレクト先の IP 検証
  - http/https スキーム限定
  - タイムアウト管理
  - 推定: ~130行 | 依存: T-301

- [ ] T-802: WebSearchTool（internal/tool/web_search.go）
  - Python版: 2348-2480行, 133行
  - DuckDuckGo HTML エンドポイント検索
  - レート制限: 2秒間隔、セッション50回上限
  - HTML パース → 結果リスト抽出
  - 推定: ~120行 | 依存: T-301

---

### T-900: その他のツール

- [ ] T-901: NotebookEditTool（internal/tool/notebook.go）
  - Python版: 2482-2620行, 139行
  - Jupyter .ipynb JSON 操作
  - replace / insert / delete モード
  - セルタイプ: code, markdown, raw
  - サイズ制限: 50MB
  - 推定: ~120行 | 依存: T-301

- [ ] T-902: TaskCreate/List/Get/Update（internal/tool/task.go）
  - Python版: 2625-2841行, 217行
  - インメモリタスクストア（sync.Map）
  - MAX_TASKS=200
  - 依存関係管理（blocks/blockedBy）
  - サイクル検出（DFS）
  - 4ツールを1ファイルに
  - 推定: ~180行 | 依存: T-301

- [ ] T-903: AskUserQuestionTool（internal/tool/ask.go）
  - Python版: 2847-2905行, 59行
  - 選択肢表示（番号入力）
  - 自由回答
  - TUI 連携
  - 推定: ~50行 | 依存: T-301, T-401

---

## Phase 3: エージェントループ（Week 3-4）

### T-1000: セキュリティ
**Python版: class PermissionMgr（3192-3299行, 108行）**

- [x] T-1001: パーミッションマネージャ（internal/security/permission.go）
  - SAFE_TOOLS / ASK_TOOLS / NETWORK_TOOLS 分類
  - セッションレベルの allow/deny 記憶
  - 永続ルール（~/.config/vibe-local/permissions.json）
  - -y モードでも危険パターンは常に確認
  - 推定: ~100行 | 依存: T-101, T-405

- [x] T-1002: 環境変数サニタイズ（internal/security/sanitize.go）
  - 除外リスト: *_TOKEN, *_KEY, *_SECRET, *_PASSWORD
  - subprocess 実行時にフィルタリング
  - 推定: ~40行 | 依存: T-000

- [x] T-1003: パス検証（internal/security/validator.go）
  - symlink 解決・保護
  - パストラバーサル防止
  - 保護ファイルリスト
  - 推定: ~60行 | 依存: T-000

---

### T-1100: セッション管理
**Python版: class Session（3458-3935行, 478行）**

- [x] T-1101: メッセージ管理（internal/session/session.go）
  - Session struct: messages, systemPrompt, tokenEstimate
  - AddUserMessage(), AddAssistantMessage(), AddToolResults()
  - GetMessages()
  - MAX_MESSAGES=500 の上限管理
  - 推定: ~100行 | 依存: T-000

- [x] T-1102: JSONL 永続化（internal/session/persistence.go）
  - SaveSession() / LoadSession()（アトミック書き込み）
  - セッション一覧（ListSessions, GetLastSession）
  - プロジェクトインデックス（cwd_hash → session_id）
  - サイズ制限: 50MB
  - 推定: ~120行 | 依存: T-1101

- [x] T-1103: トークン推定（internal/session/token.go）
  - CJK対応トークンカウント
  - CJK文字 = 1トークン、非CJK = 4文字/トークン
  - 画像 = ~800トークン
  - 推定: ~40行 | 依存: T-000

- [x] T-1104: コンテキスト圧縮（internal/session/compaction.go）
  - compact_if_needed()（70%閾値 or 300メッセージ超）
  - サイドカーモデルによる要約（3-5箇条書き）
  - フォールバック: 要約失敗時は古いメッセージを単純削除
  - 直近30メッセージは常に保持
  - 推定: ~100行 | 依存: T-1101, T-202

---

### T-1200: エージェントループ
**Python版: class Agent（4602-4961行, 360行）**

- [x] T-1201: メインエージェントループ（internal/agent/agent.go）
  - Agent struct: client, registry, permission, session, tui, config
  - Run(ctx, userInput) — ReAct ループ
  - MAX_ITERATIONS=50、MAX_RETRIES=2
  - ツールコール解析（JSON + XML フォールバック）
  - マルフォーム JSON 復旧（トレーリングカンマ除去、括弧補完）
  - 推定: ~200行 | 依存: T-201, T-204, T-302, T-1001, T-1101, T-401

- [x] T-1202: ツール実行ディスパッチ
  - 権限チェック → TUI表示 → 実行
  - 並列実行判定（Read/Glob/Grep のみ）
  - goroutine + WaitGroup で並列実行
  - 順次実行（書き込みツール）
  - 推定: ~100行 | 依存: T-1201

- [x] T-1203: ループ検出（internal/agent/loop_detector.go）
  - MAX_SAME_TOOL_REPEAT=3
  - 直近3回の tool_call を比較
  - 同一パターン検出時に停止
  - 推定: ~40行 | 依存: T-000

---

### T-1300: サブエージェント
**Python版: class SubAgentTool（2911-3150行, 240行）**

- [ ] T-1301: サブエージェント（internal/agent/subagent.go）
  - goroutine で独立実行
  - 独立メッセージ履歴
  - HARD_MAX_TURNS=20
  - ツールセット制限（デフォルト: 読み取り専用）
  - allow_writes=true で書き込みツール追加
  - 専用システムプロンプト
  - 推定: ~150行 | 依存: T-1201

---

## Phase 4: 仕上げ機能（Week 4-5）

### T-1400: システムプロンプト
**Python版: _build_system_prompt()（621-786行, 166行）**

- [x] T-1401: システムプロンプト構築（internal/config/prompt.go ※またはagent内）
  - コアルール（15ルール）
  - ツール使用ガイド + 例
  - OS固有ヒント注入
  - プロジェクト固有指示（CLAUDE.md / .vibe-coder.json）読み込み
  - プロンプトインジェクション防止（ファイル内容のサニタイズ）
  - 最大 4000 バイト
  - 推定: ~120行 | 依存: T-105, T-1003（セキュリティバリデータ）

- [x] T-1402: 改善版プロンプト（ROADMAP P0 対策）
  - ZERO preamble ルール強化
  - フォローアップ質問禁止
  - コマンド自己実行強制
  - 応答長制限（1文以内）
  - 推定: プロンプトテキストのみ | 依存: T-1401

---

### T-1500: TUI 拡張

- [x] T-1501: マークダウンレンダリング（internal/ui/markdown.go）
  - コードブロック表示（シンタックスハイライト風）
  - テーブル表示
  - インライン装飾（`太字`, `コード`）
  - 推定: ~120行 | 依存: T-401

- [x] T-1502: スラッシュコマンド
  - /help, /exit, /quit, /clear, /model, /models
  - /status, /save, /compact, /tokens, /debug
  - /yes, /no, /commit, /diff, /git, /undo, /init
  - /config
  - 推定: ~100行 | 依存: T-402

- [ ] T-1503: 多言語対応（internal/ui/i18n.go）
  - 日本語 / 英語 / 中国語
  - 環境変数 LANG からの自動検出
  - メッセージカタログ（map[string]map[string]string）
  - 推定: ~80行 | 依存: T-401

- [x] T-1504: バナー表示 + モデル情報
  - 起動時バナー
  - モデル名、メモリ使用量、コンテキスト長の表示
  - バージョン情報
  - 推定: ~40行 | 依存: T-401

---

### T-1600: デュアルモデルルーティング
**Python版: Config 内のモデル選択ロジック**

- [x] T-1601: モデルルーティング（internal/llm/routing.go）
  - メインモデル / サイドカーの使い分け
  - モデルホットスワップ（keep_alive 制御）
  - メモリ量に応じたモデル自動選択
  - サイドカーのプリロード（バックグラウンド goroutine）
  - 推定: ~80行 | 依存: T-201, T-104

---

### T-1700: 応答フィルター（新機能）

- [x] T-1701: フォローアップ質問除去フィルター
  - ストリーミング後処理
  - パターンマッチ: "何かお手伝い", "他に何か", "Is there anything", etc.
  - パターン以降を切り捨て
  - 推定: ~40行 | 依存: T-403

---

## Phase 5: main() + ビルド（Week 5-6）

### T-1800: エントリーポイント
**Python版: main()（4980-5650行, 671行）**

- [x] T-1801: main.go（cmd/vibe/main.go）
  - Config ロード
  - --list-sessions 処理
  - バナー表示
  - Ollama 接続確認（macOS/Linux: 自動起動）
  - モデル確認（未インストール時 pull）
  - セッション復旧（--resume）
  - TUI + Registry + Permission 初期化
  - エージェントループ（対話 or ワンショット -p）
  - セッション保存（終了時）
  - シグナルハンドラ（Ctrl+C → graceful shutdown）
  - 推定: ~200行 | 依存: 全 Phase 1-4

- [x] T-1802: Graceful Shutdown + リソースクリーンアップ
  - os.Signal channel で SIGINT/SIGTERM を捕捉
  - context.Cancel で全 goroutine を停止
  - バックグラウンド Bash タスクの終了待ち
  - セッション自動保存
  - readline ヒストリ保存
  - Ollama 接続クローズ
  - 推定: ~50行 | 依存: T-1801

---

### T-1900: ビルド + 配布

- [ ] T-1901: Makefile
  - build（ローカルビルド）
  - build-all（クロスコンパイル 5プラットフォーム）
  - test（go test ./...）
  - lint（go vet, staticcheck）
  - clean（dist/ 削除）
  - 推定: ~40行 | 依存: T-000

- [ ] T-1902: GitHub Actions CI/CD
  - テスト（ubuntu/macos/windows matrix）
  - ビルド + リリース（tag push → バイナリ添付）
  - 推定: ~80行（YAML） | 依存: T-1901

- [ ] T-1903: 軽量インストーラ
  - install.sh（バイナリDL + PATH設定、~30行）
  - install.ps1（バイナリDL + PATH設定、~30行）
  - 現行版の49K+35Kから大幅削減
  - 推定: ~60行 | 依存: T-1902

---

## Phase 6: テスト（Week 6-7）

### T-2000: テスト
**Python版: test_vibe_coder.py（6,919行, 500+テスト）**

- [x] T-2001: security パッケージテスト
  - 危険コマンド検出（rm -rf, sudo, mkfs, dd）
  - SSRF防御（プライベートIP全パターン）
  - symlink保護
  - 環境変数サニタイズ
  - カバレッジ目標: 95%
  - 推定: ~300行 | 依存: T-1001, T-1002, T-1003

- [x] T-2002: tool パッケージテスト
  - BashTool: 正常実行、タイムアウト、危険コマンド、バックグラウンド
  - ReadTool: テキスト、画像、PDF、Jupyter、バイナリ検出
  - WriteTool: アトミック書き込み、サイズ制限、symlink保護
  - EditTool: 置換、replace_all、NFC正規化、diff生成
  - GlobTool: パターンマッチ、SKIP_DIRS、結果制限
  - GrepTool: regex、モード切替、コンテキスト行
  - WebFetchTool: SSRF防御
  - WebSearchTool: レート制限
  - カバレッジ目標: 90%
  - 推定: ~800行 | 依存: T-500〜T-900

- [x] T-2003: llm パッケージテスト
  - JSON パース（正常、マルフォーム復旧）
  - XML フォールバック（3パターン）
  - ストリーミング SSE パース
  - カバレッジ目標: 85%
  - 推定: ~300行 | 依存: T-200

- [x] T-2004: session パッケージテスト
  - JSONL 読み書き（アトミック性）
  - トークン推定（CJK）
  - コンテキスト圧縮
  - メッセージ上限
  - カバレッジ目標: 90%
  - 推定: ~200行 | 依存: T-1100

- [x] T-2005: agent パッケージテスト
  - ループ検出
  - ツール実行フロー（モック）
  - サブエージェント
  - カバレッジ目標: 80%
  - 推定: ~200行 | 依存: T-1200, T-1300

- [ ] T-2006: 統合テスト
  - Ollama モック → エージェント完全ループ
  - ワンショットモード
  - セッション resume
  - 推定: ~200行 | 依存: 全パッケージ

---

## Phase 7: ドキュメント + リリース（Week 7-8）

### T-2100: ドキュメント

- [x] T-2101: README.md
  - インストール方法（3パターン）
  - 使い方（対話、ワンショット、resume）
  - メモリ別推奨モデル表
  - 推定: ~200行

- [x] T-2102: CLAUDE.md / プロジェクト設定
  - プロジェクト固有指示のフォーマット仕様
  - 推定: ~50行

---

## 依存関係グラフ（実装順序）

```
Phase 0: T-000
    ↓
Phase 1: T-101 → T-102, T-103, T-104, T-105
         T-201 → T-202, T-203, T-204
         T-301 → T-302, T-303
         T-401 → T-402, T-403, T-404, T-405
    ↓
Phase 2: T-501 → T-502, T-503
         T-601, T-602, T-603
         T-701, T-702
         T-801, T-802
         T-901, T-902, T-903
    ↓
Phase 3: T-1001, T-1002, T-1003
         T-1101 → T-1102, T-1103, T-1104
         T-1201 → T-1202, T-1203
         T-1301
    ↓
Phase 4: T-1401 → T-1402
         T-1501, T-1502, T-1503, T-1504
         T-1601
         T-1701
    ↓
Phase 5: T-1801（全体統合）
         T-1901, T-1902, T-1903
    ↓
Phase 6: T-2001〜T-2006
    ↓
Phase 7: T-2101, T-2102
```

---

## 推定コード量

| パッケージ | 推定行数 | Python版参考行数 |
|-----------|---------|----------------|
| cmd/vibe/ | ~200行 | main() 671行 |
| internal/config/ | ~450行 | Config 420行 + misc |
| internal/llm/ | ~550行 | OllamaClient 290行 + XML 135行 |
| internal/tool/ | ~1,540行 | 各ツール計 ~2,100行 |
| internal/security/ | ~200行 | PermissionMgr 108行 + misc |
| internal/session/ | ~360行 | Session 478行 |
| internal/ui/ | ~580行 | TUI 656行 |
| internal/agent/ | ~490行 | Agent 360行 + SubAgent 240行 |
| **合計** | **~4,370行** | **5,650行** |
| テスト | ~2,000行 | 6,919行 |

Go 版は Python 版の約77%の行数で同等の機能を実現できる見込み。テストは Go の table-driven テストにより大幅に圧縮される。

---

## クリティカルパス

最短で動作確認できるルート:

```
T-000 → T-101 → T-201 → T-203 → T-301 → T-401 → T-501 → T-1101 → T-1201 → T-1801
```

この順で実装すれば、**Phase 1-3 の途中（約3週間）で Ollama と対話 + Bash実行ができるミニマムバージョン**が動作する。残りのツールは順次追加していく形になる。
