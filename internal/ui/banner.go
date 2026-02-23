package ui

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/zephel01/vibe-local-go/internal/config"
)

// BannerOptions バナー表示オプション
type BannerOptions struct {
	Version       string
	ModelName     string
	ModelTier     string
	ContextWindow int
	MaxTokens     int
	MemoryGB      float64
	AutoApprove   bool
	OllamaHost    string
	CWD           string
}

// ShowBanner 起動時バナーを表示（Python版準拠）
func (t *Terminal) ShowBanner(opts BannerOptions) {
	// ASCII art ロゴ
	t.PrintColored(ColorCyan, `  ██╗   ██╗██╗██████╗ ███████╗     ██╗      ██████╗  ██████╗ █████╗ ██╗
  ██║   ██║██║██╔══██╗██╔════╝     ██║     ██╔═══██╗██╔════╝██╔══██╗██║
  ██║   ██║██║██████╔╝█████╗  ████ ██║     ██║   ██║██║     ███████║██║
  ╚██╗ ██╔╝██║██╔══██╗██╔══╝       ██║     ██║   ██║██║     ██╔══██╗██║
   ╚████╔╝ ██║██████╔╝███████╗     ███████╗╚██████╔╝╚██████╗██║  ██║███████╗
    ╚═══╝  ╚═╝╚═════╝ ╚══════╝     ╚══════╝ ╚═════╝  ╚═════╝╚═╝  ╚═╝╚══════╝
`)
	t.PrintColored(ColorGreen, "  🌴 O F F L I N E  A I  C O D I N G  A G E N T 🌴\n")
	t.PrintColored(ColorGray, fmt.Sprintf("  v%s  // No login • No cloud • Fully OSS • Powered by Ollama\n", opts.Version))

	// ステータス区切り線
	t.PrintColored(ColorGray, "  "+strings.Repeat("─", 48)+"\n")

	// モデル情報
	tierStr := ""
	if opts.ModelTier != "" && opts.ModelTier != "Unknown" {
		tierStr = fmt.Sprintf(" [Tier %s]", opts.ModelTier)
	}
	t.PrintColored(ColorCyan, "  🧠 Model  ")
	t.Printf("%s%s\n", opts.ModelName, tierStr)

	// モード
	modeStr := "✗ CONFIRM"
	if opts.AutoApprove {
		modeStr = "✓ AUTO-APPROVE"
	}
	t.PrintColored(ColorCyan, "  🔒 Mode   ")
	t.Printf("%s\n", modeStr)

	// エンジン
	ollamaHost := opts.OllamaHost
	if ollamaHost == "" {
		ollamaHost = "http://localhost:11434"
	}
	t.PrintColored(ColorCyan, "  🦙 Engine ")
	t.Printf("Ollama (%s)\n", ollamaHost)

	// RAM
	ctxTokens := opts.ContextWindow
	if ctxTokens == 0 {
		ctxTokens = 8192
	}
	t.PrintColored(ColorCyan, "  💾 RAM    ")
	t.Printf("%.0fGB (ctx: %d tokens)\n", opts.MemoryGB, ctxTokens)

	// CWD
	cwd := opts.CWD
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	t.PrintColored(ColorCyan, "  📁 CWD    ")
	t.Printf("%s\n", cwd)

	// 区切り線
	t.PrintColored(ColorGray, "  "+strings.Repeat("─", 48)+"\n")
}

// ShowPermissionCheck パーミッション確認ダイアログを表示
func (t *Terminal) ShowPermissionCheck() (bool, error) {
	t.Println("")
	t.PrintColored(ColorYellow, strings.Repeat("═", 44)+"\n")
	t.PrintColored(ColorYellow, " ⚠️  パーミッション確認 / Permission Check\n")
	t.PrintColored(ColorYellow, strings.Repeat("═", 44)+"\n")
	t.Println(" vibe-local はツール自動許可モード (-y) で起動できます。")
	t.Println(" This means the AI can execute commands, read/write")
	t.Println(" files, and modify your system WITHOUT asking.")
	t.Println(" ローカルLLMはクラウドAIより精度が低いため、")
	t.Println(" 意図しない操作が実行される可能性があります。")
	t.PrintColored(ColorGray, strings.Repeat("-", 44)+"\n")
	t.Println(" [y] 自動許可モード (Auto-approve all tools)")
	t.Println(" [N] 通常モード (Ask before each tool use)")
	t.PrintColored(ColorGray, strings.Repeat("-", 44)+"\n")

	input, err := t.ReadLine(" 続行しますか？ / Continue? [y/N]: ")
	if err != nil {
		return false, err
	}

	input = strings.TrimSpace(strings.ToLower(input))
	if input == "y" || input == "yes" {
		t.PrintColored(ColorGreen, " → 自動許可モードで起動します\n")
		return true, nil
	}

	t.PrintColored(ColorCyan, " → 通常モードで起動します\n")
	return false, nil
}

// ShowWelcome ウェルカムメッセージ＋ヘルプヒントを表示
func (t *Terminal) ShowWelcome(version string) {
	t.PrintColored(ColorGray, "  /help commands • Ctrl+C to interrupt (press twice to quit) • \"\"\" for multiline\n")
	t.PrintColored(ColorGreen, "  First time? Try typing: \"create a hello world in Python\"\n")
	t.PrintColored(ColorGray, "  Type /help for commands, or just ask anything in natural language.\n")
	t.Println("")
}

// ShowModelInfo モデル情報を表示
func (t *Terminal) ShowModelInfo(model string, contextWindow int) {
	t.PrintColored(ColorGreen, "使用中のモデル: ")
	t.Printf("%s\n", model)

	t.PrintColored(ColorGreen, "コンテキストウィンドウ: ")
	t.Printf("%d トークン\n", contextWindow)

	if contextWindow >= 32768 {
		t.PrintColored(ColorCyan, "  ✓ 大規模コンテキスト対応\n")
	} else if contextWindow >= 16384 {
		t.PrintColored(ColorCyan, "  ✓ 中規模コンテキスト対応\n")
	}

	t.Print("\n")
}

// ShowVersion バージョン情報のみを表示
func (t *Terminal) ShowVersion(version string) {
	fmt.Printf("vibe-local-go v%s\n", version)
	fmt.Printf("Go %s (%s/%s)\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

// ShowErrorSummary エラー概要を表示
func (t *Terminal) ShowErrorSummary(errorCount int, warningCount int) {
	if errorCount == 0 && warningCount == 0 {
		return
	}

	t.PrintColored(ColorRed, "═══ エラー概要 ═══\n")
	if errorCount > 0 {
		t.Printf("  エラー: ")
		t.PrintColored(ColorRed, fmt.Sprintf("%d\n", errorCount))
	}
	if warningCount > 0 {
		t.Printf("  警告: ")
		t.PrintColored(ColorYellow, fmt.Sprintf("%d\n", warningCount))
	}
	t.Println("═══════════════════\n")
}

// ShowTokenUsage トークン使用量を表示（Python版準拠）
// promptTokens: 入力トークン数, completionTokens: 出力トークン数, contextWindow: コンテキストウィンドウサイズ
func (t *Terminal) ShowTokenUsage(promptTokens, completionTokens, contextWindow int) {
	if contextWindow == 0 {
		contextWindow = 8192
	}

	totalTokens := promptTokens + completionTokens
	usagePct := float64(totalTokens) / float64(contextWindow) * 100

	t.PrintColored(ColorGray, fmt.Sprintf("  tokens: %d→%d (%d%% ctx)\n", promptTokens, completionTokens, int(usagePct)))
}

// FormatPrompt コンテキスト使用率付きのプロンプトを生成（Python版準拠）
func FormatPrompt(contextUsagePct int) string {
	return fmt.Sprintf("ctx:%d%% ❯ ", contextUsagePct)
}

// RecommendModel is a convenience wrapper
func RecommendModel(memoryGB float64) string {
	return config.RecommendModel(memoryGB)
}
