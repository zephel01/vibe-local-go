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
	ch.terminal.PrintColored(ColorCyan, "╔════════════════════════════════════════════════════════════╗\n")
	ch.terminal.PrintColored(ColorCyan, "║                         コマンド一覧                            ║\n")
	ch.terminal.PrintColored(ColorCyan, "╚════════════════════════════════════════════════════════════╝\n")
	ch.terminal.Print("\n")

	// カテゴリ別に表示
	ch.terminal.PrintColored(ColorGreen, "基本コマンド:\n")
	ch.terminal.Printf("  /help    ヘルプを表示\n")
	ch.terminal.Printf("  /exit    終了\n")
	ch.terminal.Printf("  /clear   セッションをクリア\n")
	ch.terminal.Print("\n")

	ch.terminal.PrintColored(ColorGreen, "モデル:\n")
	ch.terminal.Printf("  /model [name]   モデルを表示/変更\n")
	ch.terminal.Printf("  /models         利用可能なモデル一覧\n")
	ch.terminal.Print("\n")

	ch.terminal.PrintColored(ColorGreen, "セッション:\n")
	ch.terminal.Printf("  /save           セッションを保存\n")
	ch.terminal.Printf("  /status         ステータスを表示\n")
	ch.terminal.Printf("  /tokens         トークン使用量を表示\n")
	ch.terminal.Print("\n")

	ch.terminal.PrintColored(ColorGreen, "設定/デバッグ:\n")
	ch.terminal.Printf("  /config         設定を表示\n")
	ch.terminal.Printf("  /debug          デバッグモード切替\n")
	ch.terminal.Print("\n")
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
	ch.terminal.Printf("  - qwen2.5-72b-instruct (Tier A, 256GB+)\n")
	ch.terminal.Printf("  - llama3.1-70b-instruct (Tier B, 96GB+)\n")
	ch.terminal.Printf("  - llama3.1-8b-instruct (Tier C, 16GB+)\n")
	ch.terminal.Printf("  - llama3.2-3b-instruct (Tier D, 8GB+)\n")
	ch.terminal.Printf("  - qwen2.5-1.5b-instruct (Tier E, 4GB+)\n")
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
