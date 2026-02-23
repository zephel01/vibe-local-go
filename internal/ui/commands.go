package ui

import (
	"fmt"
	"strings"
)

// SlashCommand スラッシュコマンド
type SlashCommand struct {
	Name        string
	Description string
	Handler     func(args string) error
}

// CommandHandler スラッシュコマンドハンドラ
type CommandHandler struct {
	terminal   *Terminal
	commands   map[string]*SlashCommand
	aliases    map[string]string // エイリアス: "exit" -> "quit"
}

// NewCommandHandler 新しいコマンドハンドラを作成
func NewCommandHandler(terminal *Terminal) *CommandHandler {
	ch := &CommandHandler{
		terminal: terminal,
		commands: make(map[string]*SlashCommand),
		aliases:  make(map[string]string),
	}
	ch.registerDefaultCommands()
	return ch
}

// registerDefaultCommands デフォルトコマンドを登録
func (ch *CommandHandler) registerDefaultCommands() {
	// 基本コマンド
	ch.Register(&SlashCommand{
		Name:        "help",
		Description: "ヘルプを表示",
		Handler:     ch.cmdHelp,
	})
	ch.Register(&SlashCommand{
		Name:        "exit",
		Description: "終了",
		Handler:     ch.cmdExit,
	})
	ch.Register(&SlashCommand{
		Name:        "quit",
		Description: "終了",
		Handler:     ch.cmdExit,
	})
	ch.Register(&SlashCommand{
		Name:        "q",
		Description: "終了",
		Handler:     ch.cmdExit,
	})
	ch.SetAlias("exit", "quit")

	// セッション管理
	ch.Register(&SlashCommand{
		Name:        "clear",
		Description: "セッションをクリア",
		Handler:     ch.cmdClear,
	})
	ch.Register(&SlashCommand{
		Name:        "save",
		Description: "セッションを保存",
		Handler:     ch.cmdSave,
	})

	// モデル関連
	ch.Register(&SlashCommand{
		Name:        "model",
		Description: "モデルを表示/変更",
		Handler:     ch.cmdModel,
	})
	ch.Register(&SlashCommand{
		Name:        "models",
		Description: "利用可能なモデル一覧",
		Handler:     ch.cmdModels,
	})

	// ステータス/情報
	ch.Register(&SlashCommand{
		Name:        "status",
		Description: "ステータスを表示",
		Handler:     ch.cmdStatus,
	})
	ch.Register(&SlashCommand{
		Name:        "tokens",
		Description: "トークン使用量を表示",
		Handler:     ch.cmdTokens,
	})

	// デバッグ
	ch.Register(&SlashCommand{
		Name:        "debug",
		Description: "デバッグモード切替",
		Handler:     ch.cmdDebug,
	})

	// コンフィグ
	ch.Register(&SlashCommand{
		Name:        "config",
		Description: "設定を表示",
		Handler:     ch.cmdConfig,
	})
}

// Register コマンドを登録
func (ch *CommandHandler) Register(cmd *SlashCommand) {
	ch.commands[cmd.Name] = cmd
}

// SetAlias エイリアスを設定
func (ch *CommandHandler) SetAlias(alias, target string) {
	ch.aliases[alias] = target
}

// CommandNames 登録済みコマンド名の一覧を返す（"/" 付き）
func (ch *CommandHandler) CommandNames() []string {
	names := make([]string, 0, len(ch.commands))
	for name := range ch.commands {
		names = append(names, "/"+name)
	}
	return names
}

// Execute コマンドを実行
func (ch *CommandHandler) Execute(input string) (bool, error) {
	// スラッシュで始まるか確認
	if !strings.HasPrefix(input, "/") {
		return false, nil
	}

	// コマンド名と引数を分離
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false, nil
	}

	cmdName := strings.TrimPrefix(parts[0], "/")
	args := ""
	if len(parts) > 1 {
		args = strings.Join(parts[1:], " ")
	}

	// エイリアスを解決
	if realName, ok := ch.aliases[cmdName]; ok {
		cmdName = realName
	}

	// コマンドを検索
	cmd, ok := ch.commands[cmdName]
	if !ok {
		ch.terminal.Printf("不明なコマンド: %s\n", cmdName)
		return true, nil
	}

	// コマンドを実行
	err := cmd.Handler(args)
	return true, err
}

// ShowHelp ヘルプを表示
func (ch *CommandHandler) ShowHelp() {
	ch.terminal.PrintColored(ColorCyan, "  ━━ Commands ━━━━━━━━━━━━━━━━━━━━━━━━\n")
	ch.terminal.Printf("  /help              ヘルプを表示\n")
	ch.terminal.Printf("  /exit, /quit, /q   終了\n")
	ch.terminal.Printf("  /clear             会話をクリア\n")
	ch.terminal.Printf("  /model <name>      モデルを切替\n")
	ch.terminal.Printf("  /models            モデル一覧・選択切替\n")
	ch.terminal.Printf("  /status            セッション情報\n")
	ch.terminal.Printf("  /save              セッションを保存\n")
	ch.terminal.Printf("  /tokens            トークン使用量を表示\n")
	ch.terminal.Printf("  /init              CLAUDE.md テンプレート作成\n")
	ch.terminal.Printf("  /yes               自動承認 ON\n")
	ch.terminal.Printf("  /no                自動承認 OFF\n")
	ch.terminal.Printf("  /config            設定を表示\n")
	ch.terminal.Printf("  /debug             デバッグモード切替\n")
	ch.terminal.Printf("  /provider          プロバイダー管理\n")
	ch.terminal.Printf("  /switch            プロバイダー切替\n")
	ch.terminal.Printf("  \"\"\"                複数行入力\n")
	ch.terminal.PrintColored(ColorCyan, "  ━━ Skills ━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	ch.terminal.Printf("  /skills            スキル一覧を表示\n")
	ch.terminal.PrintColored(ColorCyan, "  ━━ MCP ━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	ch.terminal.Printf("  /mcp               MCPサーバー状況・ツール一覧\n")
	ch.terminal.PrintColored(ColorCyan, "  ━━ Auto Test ━━━━━━━━━━━━━━━━━━━━━━\n")
	ch.terminal.Printf("  /autotest [on|off] ファイル編集後の自動テスト\n")
	ch.terminal.PrintColored(ColorCyan, "  ━━ Sandbox ━━━━━━━━━━━━━━━━━━━━━━━━\n")
	ch.terminal.Printf("  /sandbox [on|off]  サンドボックス切替\n")
	ch.terminal.Printf("  /commit [file]     ステージを本番に反映\n")
	ch.terminal.Printf("  /discard [file]    ステージを破棄\n")
	ch.terminal.Printf("  /diff [file]       ステージの差分を表示\n")
	ch.terminal.Printf("  /staged            ステージ一覧\n")
	ch.terminal.PrintColored(ColorCyan, "  ━━ Keyboard ━━━━━━━━━━━━━━━━━━━━━━━\n")
	ch.terminal.Printf("  Ctrl+C             現在のタスクを停止\n")
	ch.terminal.Printf("  Ctrl+C x2          終了 (1.5秒以内)\n")
	ch.terminal.Printf("  Ctrl+D             終了\n")
	ch.terminal.Printf("  ↑/↓               入力履歴\n")
	ch.terminal.Printf("  Tab                コマンド補完\n")
	ch.terminal.PrintColored(ColorCyan, "  ━━ Startup Flags ━━━━━━━━━━━━━━━━━━\n")
	ch.terminal.Printf("  -y                 全ツール自動承認\n")
	ch.terminal.Printf("  --debug            デバッグ出力\n")
	ch.terminal.Printf("  --resume last      直近セッション復旧\n")
	ch.terminal.Printf("  --resume <id>      セッション指定復旧\n")
	ch.terminal.Printf("  --model NAME       モデル指定\n")
	ch.terminal.Printf("  --list-sessions    セッション一覧\n")
	ch.terminal.Printf("  -p \"prompt\"        ワンショットモード\n")
	ch.terminal.PrintColored(ColorCyan, "  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
}

// コマンドハンドラ

func (ch *CommandHandler) cmdHelp(args string) error {
	ch.ShowHelp()
	return nil
}

func (ch *CommandHandler) cmdExit(args string) error {
	ch.terminal.Println("終了します...")
	return fmt.Errorf("exit")
}

func (ch *CommandHandler) cmdClear(args string) error {
	ch.terminal.Println("セッションをクリアしました")
	return nil
}

func (ch *CommandHandler) cmdSave(args string) error {
	ch.terminal.Println("セッションを保存しました")
	return nil
}

func (ch *CommandHandler) cmdModel(args string) error {
	if args == "" {
		ch.terminal.Println("現在のモデル: (デフォルト)")
	} else {
		ch.terminal.Printf("モデルを変更: %s\n", args)
	}
	return nil
}

func (ch *CommandHandler) cmdModels(args string) error {
	ch.terminal.Println("利用可能なモデル:")
	ch.terminal.Printf("  - qwen3:72b (Tier A, 256GB+)\n")
	ch.terminal.Printf("  - qwen3:32b (Tier B, 96GB+)\n")
	ch.terminal.Printf("  - qwen3-coder:30b (Tier C, 32GB+)\n")
	ch.terminal.Printf("  - qwen3:8b (Tier C, 16GB+)\n")
	ch.terminal.Printf("  - qwen3:4b (Tier D, 8GB+)\n")
	ch.terminal.Printf("  - qwen3:1.7b (Tier E, 4GB+)\n")
	return nil
}

func (ch *CommandHandler) cmdStatus(args string) error {
	ch.terminal.Println("ステータス:")
	ch.terminal.Printf("  状態: 実行中\n")
	ch.terminal.Printf("  メッセージ数: 0\n")
	ch.terminal.Printf("  トークン使用量: 0\n")
	return nil
}

func (ch *CommandHandler) cmdTokens(args string) error {
	ch.terminal.Println("トークン使用量: 0 / 32768")
	return nil
}

func (ch *CommandHandler) cmdDebug(args string) error {
	ch.terminal.Println("デバッグモードを切替えました")
	return nil
}

func (ch *CommandHandler) cmdConfig(args string) error {
	ch.terminal.Println("設定:")
	ch.terminal.Printf("  Ollamaホスト: http://localhost:11434\n")
	ch.terminal.Printf("  最大トークン: 8192\n")
	ch.terminal.Printf("  温度: 0.7\n")
	return nil
}
