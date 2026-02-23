package ui

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/zephel01/vibe-local-go/internal/config"
)

// BannerOptions バナー表示オプション
type BannerOptions struct {
	Version      string
	ModelName    string
	ContextWindow int
	MaxTokens    int
	MemoryGB     float64
}

// ShowBanner 起動時バナーを表示
func (t *Terminal) ShowBanner(opts BannerOptions) {
	// バナー区切り線
	separator := strings.Repeat("─", 60)

	t.Println(separator)

	// タイトル
	titleColor := ColorCyan
	t.PrintColored(titleColor, "  vibe-local-go")
	t.Printf(" - Go版 AIコーディングアシスタント v%s\n", opts.Version)

	// モデル情報
	if opts.ModelName != "" {
		t.PrintColored(ColorGreen, "  モデル: ")
		t.Printf("%s", opts.ModelName)
		if opts.ContextWindow > 0 {
			t.Printf(" (コンテキスト: %d トークン)", opts.ContextWindow)
		}
		t.Print("\n")
	}

	// メモリ情報
	if opts.MemoryGB > 0 {
		t.PrintColored(ColorGreen, "  メモリ: ")
		t.Printf("%.1f GB", opts.MemoryGB)
		t.Printf(" (OS: %s/%s)", runtime.GOOS, runtime.GOARCH)
		t.Print("\n")
	}

	// 推奨モデル
	if opts.MemoryGB > 0 {
		recommended := config.RecommendModel(opts.MemoryGB)
		t.PrintColored(ColorYellow, "  推奨: ")
		t.Printf("%s\n", recommended)
	}

	t.Println(separator)
	t.Print("\n")
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

// ShowWelcome ウェルカムメッセージを表示
func (t *Terminal) ShowWelcome(version string) {
	t.PrintColored(ColorCyan, "╔════════════════════════════════════════════════════════════╗\n")
	t.PrintColored(ColorCyan, "║                                                                ║\n")
	t.PrintColored(ColorCyan, "║  ")
	t.PrintColored(ColorGreen, "★")
	t.PrintColored(ColorCyan, "  vibe-local-go ")
	t.PrintColored(ColorYellow, fmt.Sprintf("v%s", version))
	t.PrintColored(ColorCyan, "  ")
	t.PrintColored(ColorGreen, "★")
	t.PrintColored(ColorCyan, "                              ║\n")
	t.PrintColored(ColorCyan, "║                                                                ║\n")
	t.PrintColored(ColorCyan, "║  Go版 AIコーディングアシスタント                           ║\n")
	t.PrintColored(ColorCyan, "║                                                                ║\n")
	t.PrintColored(ColorCyan, "║  ヘルプ: ")
	t.PrintColored(ColorYellow, "/help")
	t.PrintColored(ColorCyan, "  終了: ")
	t.PrintColored(ColorYellow, "/exit")
	t.PrintColored(ColorCyan, "                                ║\n")
	t.PrintColored(ColorCyan, "║                                                                ║\n")
	t.PrintColored(ColorCyan, "╚════════════════════════════════════════════════════════════╝\n")
	t.Print("\n")
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
