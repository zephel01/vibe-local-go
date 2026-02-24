package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	execPackage "os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/zephel01/vibe-local-go/internal/agent"
	"github.com/zephel01/vibe-local-go/internal/config"
	"github.com/zephel01/vibe-local-go/internal/llm"
	"github.com/zephel01/vibe-local-go/internal/sandbox"
	"github.com/zephel01/vibe-local-go/internal/security"
	"github.com/zephel01/vibe-local-go/internal/session"
	"github.com/zephel01/vibe-local-go/internal/mcp"
	"github.com/zephel01/vibe-local-go/internal/skill"
	"github.com/zephel01/vibe-local-go/internal/tool"
	"github.com/zephel01/vibe-local-go/internal/ui"
	"github.com/zephel01/vibe-local-go/internal/watcher"
)

// Version はビルド時に ldflags で上書き可能:
//   go build -ldflags "-X main.Version=1.1.0"
var Version = "1.1.0"

// ShutdownManager handles graceful shutdown
type ShutdownManager struct {
	provider    llm.LLMProvider
	session     *session.Session
	persistence *session.PersistenceManager
	terminal    *ui.Terminal
	cancel      context.CancelFunc
	mcpMgr      *mcp.Manager
}

// NewShutdownManager creates a new shutdown manager
func NewShutdownManager(provider llm.LLMProvider, sess *session.Session, persistence *session.PersistenceManager, terminal *ui.Terminal, cancel context.CancelFunc) *ShutdownManager {
	return &ShutdownManager{
		provider:    provider,
		session:     sess,
		persistence: persistence,
		terminal:    terminal,
		cancel:      cancel,
	}
}

// Shutdown performs graceful shutdown
func (sm *ShutdownManager) Shutdown(reason string) {
	sm.terminal.Printf("\nシャットダウン中... (%s)\n", reason)

	// Cancel context to stop all goroutines
	sm.cancel()

	// Give goroutines time to cleanup
	time.Sleep(100 * time.Millisecond)

	// Stop MCP servers
	if sm.mcpMgr != nil {
		sm.mcpMgr.StopAll()
	}

	// Save session
	if sm.session.GetID() != "" {
		err := sm.persistence.SaveSession(sm.session)
		if err != nil {
			sm.terminal.PrintColored(ui.ColorRed, fmt.Sprintf("セッション保存エラー: %v\n", err))
		} else {
			sm.terminal.PrintColored(ui.ColorGreen, "✓ セッション保存完了\n")
		}
	}

	sm.terminal.Println("終了")
}

var (
	// CLI flags
	flagModel            string
	flagSidecar          string
	flagHost             string
	flagProvider         string
	flagAPIKey           string
	flagPrompt           string
	flagAutoConfirm      bool
	flagResume           string
	flagSessionID        string
	flagListSessions     bool
	flagMaxTokens        int
	flagTemperature      float64
	flagContextWindow    int
	flagVersion          bool
	flagSandbox          bool
	flagAutoVenv         bool
	flagVenvDir          string
	flagPermissionCheck  bool
)

func init() {
	flag.StringVar(&flagModel, "model", "", "Main model name")
	flag.StringVar(&flagSidecar, "sidecar", "", "Sidecar model name")
	flag.StringVar(&flagHost, "host", "", "Ollama host URL")
	flag.StringVar(&flagProvider, "provider", "", "LLM provider (ollama, openrouter)")
	flag.StringVar(&flagAPIKey, "api-key", "", "API key for cloud providers (or use OPENROUTER_API_KEY env)")
	flag.StringVar(&flagPrompt, "p", "", "One-shot prompt")
	flag.BoolVar(&flagAutoConfirm, "y", false, "Auto-confirm all tool executions")
	flag.StringVar(&flagResume, "resume", "", "Resume session (last or session-id)")
	flag.StringVar(&flagSessionID, "session-id", "", "Specify session ID")
	flag.BoolVar(&flagListSessions, "list-sessions", false, "List all sessions")
	flag.IntVar(&flagMaxTokens, "max-tokens", 0, "Maximum tokens")
	flag.Float64Var(&flagTemperature, "temperature", 0, "Temperature (0.0-2.0)")
	flag.IntVar(&flagContextWindow, "context-window", 0, "Context window size")
	flag.BoolVar(&flagVersion, "version", false, "Show version")
	flag.BoolVar(&flagSandbox, "sandbox", false, "Enable sandbox mode (stage files before applying)")
	flag.BoolVar(&flagAutoVenv, "auto-venv", false, "Auto-create and activate .venv for Python commands")
	flag.StringVar(&flagVenvDir, "venv-dir", ".venv", "Virtual environment directory name")
	flag.BoolVar(&flagPermissionCheck, "permission-check", false, "Show permission check dialog at startup")
}

func main() {
	flag.Parse()

	// Show version
	if flagVersion {
		showVersion()
		return
	}

	// Load configuration
	cfg := loadConfig()

	// List sessions
	if flagListSessions {
		listSessions(cfg)
		return
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize components
	terminal := ui.NewTerminal()
	provider := createProvider(cfg)
	router := createModelRouter(provider, cfg)
	permissionMgr, validator := createSecurityComponents(cfg)

	// スキルマネージャー初期化
	skillMgr := skill.NewSkillManager()
	if err := skillMgr.LoadSkills(); err != nil {
		terminal.PrintColored(ui.ColorYellow, fmt.Sprintf("スキル読み込み警告: %v\n", err))
	}
	if skillMgr.Count() > 0 {
		terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ %d 件のスキルを読み込みました\n", skillMgr.Count()))
	}

	sess := createSession(cfg, skillMgr)

	// サンドボックスマネージャー（現在はファイルステージング未使用、将来拡張用）
	var sbMgr *sandbox.Manager

	// 自動venv有効時のメッセージ
	if cfg.AutoVenv {
		terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ 自動venvモード有効 (%s)\n", cfg.VenvDir))
	}

	registry := createToolRegistry(terminal, permissionMgr, validator, sbMgr, cfg)

	// MCP マネージャー初期化
	mcpMgr := mcp.NewManager()
	if err := mcpMgr.LoadConfig(); err != nil {
		terminal.PrintColored(ui.ColorYellow, fmt.Sprintf("MCP設定読み込み警告: %v\n", err))
	}
	if mcpMgr.ServerCount() > 0 {
		terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("MCP: %d 件のサーバーを起動中...\n", mcpMgr.ServerCount()))
		errs := mcpMgr.StartAll(ctx)
		for _, e := range errs {
			terminal.PrintColored(ui.ColorYellow, fmt.Sprintf("  ⚠ %v\n", e))
		}
		toolCount := mcp.RegisterMCPTools(registry, mcpMgr)
		if toolCount > 0 {
			terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ MCP: %d 件のツールを登録 (%d サーバー)\n", toolCount, mcpMgr.RunningCount()))
		}
	}

	persistenceMgr, err := session.NewPersistenceManager(getSessionDir())
	if err != nil {
		terminal.PrintColored(ui.ColorRed, fmt.Sprintf("パーシスタンスマネージャー作成エラー: %v\n", err))
		os.Exit(1)
	}

	// Setup signal handler with shutdown manager
	shutdownMgr := NewShutdownManager(provider, sess, persistenceMgr, terminal, cancel)
	shutdownMgr.mcpMgr = mcpMgr
	setupSignalHandler(shutdownMgr)

	// パーミッション確認ダイアログ（--permission-check フラグが指定された場合）
	if flagPermissionCheck && !cfg.AutoApprove {
		autoApprove, err := terminal.ShowPermissionCheck()
		if err != nil {
			terminal.PrintColored(ui.ColorRed, fmt.Sprintf("入力エラー: %v\n", err))
			os.Exit(1)
		}
		if autoApprove {
			cfg.AutoApprove = true
			permissionMgr.SetAutoApprove(true)
		}
	}

	// Check provider connection（接続失敗時は再設定可能）
	provider = checkProviderConnection(ctx, provider, cfg, terminal)
	router = createModelRouter(provider, cfg)
	shutdownMgr.provider = provider

	// Pull model if needed (ModelManager対応プロバイダーのみ)
	// クラウド切替が選択された場合はプロバイダーを再作成
	switchedToCloud := pullModelIfNeeded(ctx, provider, cfg, terminal)
	if switchedToCloud {
		provider = checkProviderConnection(ctx, createProvider(cfg), cfg, terminal)
		router = createModelRouter(provider, cfg)
		shutdownMgr.provider = provider
	}

	// Show banner
	showBanner(terminal, cfg, router, provider)

	// Resume session if requested
	if flagResume != "" {
		resumeSession(ctx, sess, persistenceMgr, flagResume, cfg)
	}

	// Initialize agent with LLMProvider
	agt := agent.NewAgent(provider, registry, permissionMgr, validator, sess, terminal, cfg)

	// Register parallel_agents tool (requires provider + registry)
	parallelOrch := agent.NewParallelOrchestrator(provider, registry)
	parallelBridge := agent.NewParallelBridge(parallelOrch)
	registry.Register(tool.NewParallelAgentsTool(parallelBridge))

	// Create command handler with provider access
	cmdHandler := createCommandHandler(terminal, provider, cfg, sbMgr, skillMgr, mcpMgr, agt)

	// Process initial slash command from command line args
	args := flag.Args()
	if len(args) > 0 && strings.HasPrefix(args[0], "/") {
		initialCmd := strings.Join(args, " ")
		isCmd, _ := cmdHandler.Execute(initialCmd)
		if isCmd && (initialCmd == "/exit" || initialCmd == "/quit") {
			shutdownMgr.Shutdown("initial command")
			return
		}
		// コマンド実行後、対話モードに継続
	}

	// Run agent
	runAgent(ctx, agt, cfg, terminal, shutdownMgr, cmdHandler)
}

func loadConfig() *config.Config {
	cfg := config.DefaultConfig()

	// 1. config.json から読み込み（最低優先度）
	cfg.ParseConfigFile()

	// 2. 環境変数で上書き
	cfg.ParseEnv()

	// 3. 環境変数: 各クラウドプロバイダーのAPIキー
	if cfg.CloudAPIKeys == nil {
		cfg.CloudAPIKeys = make(map[string]string)
	}
	for _, def := range llm.CloudProviders {
		if envKey := os.Getenv(def.EnvKey); envKey != "" {
			if cfg.CloudAPIKeys[def.Key] == "" {
				cfg.CloudAPIKeys[def.Key] = envKey
			}
		}
	}
	// provider未指定の場合、環境変数からプロバイダーを自動検出（優先順）
	if flagProvider == "" && cfg.Provider == "ollama" {
		detectOrder := []string{"openrouter", "openai", "anthropic", "google", "deepseek", "groq", "zai"}
		for _, key := range detectOrder {
			if cfg.CloudAPIKeys[key] != "" {
				cfg.Provider = key
				break
			}
		}
	}

	// 4. CLIフラグで上書き（最優先）
	if flagModel != "" {
		cfg.Model = flagModel
		cfg.AutoModel = false
	}
	if flagSidecar != "" {
		cfg.SidecarModel = flagSidecar
	}
	if flagHost != "" {
		cfg.OllamaHost = flagHost
	}
	if flagProvider != "" {
		cfg.Provider = flagProvider
	}
	if flagAPIKey != "" {
		// --api-key は現在のプロバイダーに設定
		setAPIKeyForProvider(cfg, cfg.Provider, flagAPIKey)
	}
	if flagMaxTokens > 0 {
		cfg.MaxTokens = flagMaxTokens
	}
	if flagTemperature > 0 {
		cfg.Temperature = flagTemperature
	}
	if flagContextWindow > 0 {
		cfg.ContextWindow = flagContextWindow
	}
	if flagAutoConfirm {
		cfg.AutoApprove = true
	}
	if flagSandbox {
		cfg.SandboxMode = true
	}
	if flagAutoVenv {
		cfg.AutoVenv = true
	}
	if flagVenvDir != ".venv" {
		cfg.VenvDir = flagVenvDir
	}

	// 5. モデル自動選択（明示指定がない場合のみ）
	memoryGB := getMemoryGB()
	if cfg.AutoModel && cfg.Provider == "ollama" {
		cfg.Model = config.RecommendModel(memoryGB)
	}
	// クラウドプロバイダーのデフォルトモデル
	if cfg.Provider != "ollama" && cfg.Model == "" {
		def := llm.GetCloudProviderDef(cfg.Provider)
		if def != nil {
			cfg.Model = def.DefaultModel
		}
	}

	// Generate OS hints
	cfg.GenerateOSHints()

	return cfg
}

// createProvider creates the LLM provider based on config
func createProvider(cfg *config.Config) llm.LLMProvider {
	switch cfg.Provider {
	case "openrouter", "openai", "anthropic", "google",
		"deepseek", "mistral", "groq", "together", "fireworks",
		"perplexity", "cohere", "zai", "zai-coding", "zhipu", "moonshot":
		apiKey := getAPIKeyForProvider(cfg)
		if apiKey == "" {
			def := llm.GetCloudProviderDef(cfg.Provider)
			envName := "API key"
			if def != nil {
				envName = def.EnvKey
			}
			fmt.Printf("エラー: %s を使用するにはAPIキーが必要です\n", cfg.Provider)
			fmt.Printf("  --api-key <key> または %s 環境変数を設定してください\n", envName)
			os.Exit(1)
		}
		return llm.NewCloudProvider(cfg.Provider, apiKey, cfg.Model)
	case "ollama", "lm-studio", "llama-server":
		// ローカルプロバイダー
		host := cfg.OllamaHost
		if def := llm.GetLocalProviderDef(cfg.Provider); def != nil {
			// プロファイルからホストを取得
			profiles := cfg.GetProviderProfiles()
			if profiles != nil {
				if p, ok := profiles[cfg.Provider]; ok && p.Host != "" {
					host = p.Host
				}
			}
			// なければデフォルトホスト
			if host == "" {
				host = def.DefaultHost
			}
		}

		if cfg.Provider == "ollama" {
			return llm.NewOllamaProvider(host, cfg.Model)
		}
		if cfg.Provider == "lm-studio" {
			return llm.NewLMStudioProvider(host, cfg.Model)
		}
		// llama-server はOpenAI互換API（/v1 を付与）
		normalizedHost := llm.NormalizeBaseURL(host)
		info := llm.ProviderInfo{
			Name:    cfg.Provider,
			Type:    llm.ProviderTypeLocal,
			BaseURL: normalizedHost,
			Model:   cfg.Model,
			Features: llm.Features{
				NativeFunctionCalling: true,
				Streaming:             true,
			},
		}
		return llm.NewOpenAICompatProvider(normalizedHost+"/v1", "", cfg.Model, info)
	default:
		// デフォルト: Ollama
		return llm.NewOllamaProvider(cfg.OllamaHost, cfg.Model)
	}
}

// getAPIKeyForProvider プロバイダーに対応するAPIキーを取得
func getAPIKeyForProvider(cfg *config.Config) string {
	if cfg.CloudAPIKeys == nil {
		return ""
	}
	return cfg.CloudAPIKeys[cfg.Provider]
}

// setAPIKeyForProvider プロバイダーに対応するAPIキーをcfgに設定
func setAPIKeyForProvider(cfg *config.Config, provider, apiKey string) {
	if cfg.CloudAPIKeys == nil {
		cfg.CloudAPIKeys = make(map[string]string)
	}
	cfg.CloudAPIKeys[provider] = apiKey
}

func createModelRouter(provider llm.LLMProvider, cfg *config.Config) *llm.ModelRouter {
	var sidecarProvider llm.LLMProvider
	if cfg.SidecarModel != "" {
		// サイドカーも同じホストで別モデル
		// ローカルプロバイダーの場合はホストを取得
		host := cfg.OllamaHost
		if cfg.Provider == "ollama" || cfg.Provider == "lm-studio" || cfg.Provider == "llama-server" {
			if def := llm.GetLocalProviderDef(cfg.Provider); def != nil {
				profiles := cfg.GetProviderProfiles()
				if profiles != nil {
					if p, ok := profiles[cfg.Provider]; ok && p.Host != "" {
						host = p.Host
					}
				}
				if host == "" {
					host = def.DefaultHost
				}
			}
			sidecarProvider = llm.NewOllamaProvider(host, cfg.SidecarModel)
		} else {
			// クラウドプロバイダーの場合
			sidecarProvider = llm.NewOllamaProvider(cfg.OllamaHost, cfg.SidecarModel)
		}
	}

	return llm.NewModelRouter(provider, sidecarProvider, cfg.Model, cfg.SidecarModel)
}

func createSecurityComponents(cfg *config.Config) (*security.PermissionManager, *security.PathValidator) {
	permMgr, err := security.NewPermissionManager(cfg.AutoApprove)
	if err != nil {
		fmt.Printf("パーミッションマネージャー作成エラー: %v\n", err)
		os.Exit(1)
	}

	wd, err := os.Getwd()
	if err != nil {
		fmt.Printf("作業ディレクトリ取得エラー: %v\n", err)
		os.Exit(1)
	}

	validator := security.NewPathValidator(wd)
	return permMgr, validator
}

func createSession(cfg *config.Config, skillMgr *skill.SkillManager) *session.Session {
	// Generate session ID if not specified
	sessionID := cfg.SessionID
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	// Build system prompt (with skill metadata if available)
	var systemPrompt string
	if skillMgr != nil && skillMgr.Count() > 0 {
		systemPrompt = config.BuildSystemPrompt(cfg, skillMgr.GetSkillMetadata())
	} else {
		systemPrompt = config.BuildSystemPrompt(cfg)
	}

	sess := session.NewSession(sessionID, systemPrompt)
	return sess
}

func createCommandHandler(terminal *ui.Terminal, provider llm.LLMProvider, cfg *config.Config, sbMgr *sandbox.Manager, skillMgr *skill.SkillManager, mcpMgr *mcp.Manager, agt *agent.Agent) *ui.CommandHandler {
	cmdHandler := ui.NewCommandHandler(terminal)

	cmdHandler.Register(&ui.SlashCommand{
		Name:        "models",
		Description: "利用可能なモデル一覧を表示・切替",
		Handler: func(args string) error {
			// ModelManagerインターフェースを持つプロバイダーのみモデル一覧が取得可能
			mm, ok := provider.(llm.ModelManager)
			if !ok {
				terminal.PrintColored(ui.ColorYellow, "このプロバイダーはモデル一覧をサポートしていません\n")
				return nil
			}

			terminal.PrintColored(ui.ColorCyan, "利用可能なモデルを取得中...\n")
			models, err := mm.ListModels(context.Background())
			if err != nil {
				terminal.PrintColored(ui.ColorRed, fmt.Sprintf("モデル一覧取得エラー: %v\n", err))
				return nil
			}

			if len(models) == 0 {
				terminal.Println("利用可能なモデルがありません")
				terminal.Println("コマンドでモデルをインストール: ollama pull <model-name>")
				return nil
			}

			terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("%d 件のモデルが見つかりました:\n", len(models)))
			currentModel := cfg.Model
			for i, model := range models {
				marker := ""
				if model == currentModel {
					marker = " [現在]"
				}
				terminal.Printf("  %2d. %s%s\n", i+1, model, marker)
			}

			// モデル切り替え選択
			terminal.Print("\n")
			terminal.Println("番号を入力してモデルを切り替え (Enterでキャンセル):")
			choice, err := terminal.ReadLine("選択> ")
			if err != nil || strings.TrimSpace(choice) == "" {
				return nil
			}

			var choiceNum int
			if _, err := fmt.Sscanf(strings.TrimSpace(choice), "%d", &choiceNum); err != nil || choiceNum < 1 || choiceNum > len(models) {
				terminal.PrintColored(ui.ColorYellow, "無効な選択です\n")
				return nil
			}

			selectedModel := models[choiceNum-1]
			if selectedModel == currentModel {
				terminal.PrintColored(ui.ColorYellow, fmt.Sprintf("既に %s を使用中です\n", selectedModel))
				return nil
			}

			// プロバイダーのモデルを切り替え
			if ms, ok := provider.(llm.ModelSwitcher); ok {
				ms.SetModel(selectedModel)
			}
			cfg.Model = selectedModel

			// プロバイダープロファイルも更新・保存
			if profiles := cfg.GetProviderProfiles(); profiles != nil {
				if profile, exists := profiles[cfg.Provider]; exists {
					profile.Model = selectedModel
					cfg.SaveProviderProfile(cfg.Provider, profile)
				}
			}

			terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ モデルを %s に切り替えました\n", selectedModel))
			return nil
		},
	})

	// /model コマンドを登録（モデル表示/直接切替）
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "model",
		Description: "現在のモデル表示 / モデル名指定で切替",
		Handler: func(args string) error {
			currentModel := cfg.Model
			if args == "" {
				// 引数なし: 現在のモデルを表示
				terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("現在のモデル: %s\n", currentModel))
				terminal.Println("切り替え: /model <モデル名>  または  /models で一覧から選択")
				return nil
			}

			newModel := strings.TrimSpace(args)
			if newModel == currentModel {
				terminal.PrintColored(ui.ColorYellow, fmt.Sprintf("既に %s を使用中です\n", newModel))
				return nil
			}

			// ModelManagerがあればモデル存在チェック
			if mm, ok := provider.(llm.ModelManager); ok {
				exists, err := mm.CheckModel(context.Background(), newModel)
				if err != nil {
					terminal.PrintColored(ui.ColorYellow, fmt.Sprintf("モデル確認中にエラー: %v\n", err))
					// エラーでも切り替えは許可
				} else if !exists {
					terminal.PrintColored(ui.ColorYellow, fmt.Sprintf("モデル '%s' が見つかりません\n", newModel))
					terminal.Println("利用可能なモデルは /models で確認できます")
					return nil
				}
			}

			// プロバイダーのモデルを切り替え
			if ms, ok := provider.(llm.ModelSwitcher); ok {
				ms.SetModel(newModel)
			}
			cfg.Model = newModel

			// プロバイダープロファイルも更新・保存
			if profiles := cfg.GetProviderProfiles(); profiles != nil {
				if profile, exists := profiles[cfg.Provider]; exists {
					profile.Model = newModel
					cfg.SaveProviderProfile(cfg.Provider, profile)
				}
			}

			terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ モデルを %s に切り替えました\n", newModel))
			return nil
		},
	})

	// /init コマンドを登録（CLAUDE.mdテンプレート作成）
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "init",
		Description: "CLAUDE.md テンプレートを作成",
		Handler: func(args string) error {
			cwd, err := os.Getwd()
			if err != nil {
				terminal.PrintColored(ui.ColorRed, fmt.Sprintf("ディレクトリ取得エラー: %v\n", err))
				return nil
			}
			claudePath := filepath.Join(cwd, "CLAUDE.md")

			// 既存チェック
			if _, err := os.Stat(claudePath); err == nil {
				terminal.PrintColored(ui.ColorYellow, fmt.Sprintf("CLAUDE.md は既に存在します: %s\n", claudePath))
				return nil
			}

			template := `# Project Instructions

## Overview
<!-- プロジェクトの概要を記述 -->

## Tech Stack
<!-- 使用技術・フレームワーク -->

## Code Style
<!-- コーディング規約・スタイル -->

## Important Rules
<!-- エージェントが守るべきルール -->
- テストを壊さないこと
- 既存のコードスタイルに従うこと

## Project Structure
<!-- ディレクトリ構成の説明 -->
`
			if err := os.WriteFile(claudePath, []byte(template), 0644); err != nil {
				terminal.PrintColored(ui.ColorRed, fmt.Sprintf("ファイル作成エラー: %v\n", err))
				return nil
			}
			terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ CLAUDE.md を作成しました: %s\n", claudePath))
			terminal.Println("  プロジェクト固有の指示を記述してください")
			return nil
		},
	})

	// /yes, /no コマンドを登録（自動承認切替）
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "yes",
		Description: "自動承認モード ON",
		Handler: func(args string) error {
			cfg.AutoApprove = true
			terminal.PrintColored(ui.ColorGreen, "✓ 自動承認モード ON — ツール実行を自動許可します\n")
			return nil
		},
	})
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "no",
		Description: "自動承認モード OFF",
		Handler: func(args string) error {
			cfg.AutoApprove = false
			terminal.PrintColored(ui.ColorYellow, "✓ 自動承認モード OFF — ツール実行前に確認します\n")
			return nil
		},
	})

	// /switch コマンドを登録（プロバイダー切替用ショートカット）
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "switch",
		Description: "プロバイダーを切替",
		Handler: func(args string) error {
			profiles := cfg.GetProviderProfiles()
			if profiles == nil || len(profiles) == 0 {
				terminal.PrintColored(ui.ColorYellow, "切替可能なプロバイダーが登録されていません\n")
				terminal.Println("先に /provider add でプロバイダーを追加してください")
				return nil
			}
			return providerSwitchInteractive(cfg, terminal, profiles)
		},
	})

	// /config コマンドを登録
	registerConfigCommands(cmdHandler, terminal, cfg)

	// /provider コマンドを登録
	registerProviderCommands(cmdHandler, terminal, cfg)

	// サンドボックスコマンドを登録
	registerSandboxCommands(cmdHandler, terminal, sbMgr)

	// スキルコマンドを登録
	registerSkillCommands(cmdHandler, terminal, skillMgr)

	// MCPコマンドを登録
	registerMCPCommands(cmdHandler, terminal, mcpMgr)

	// AutoTestコマンドを登録
	registerAutoTestCommands(cmdHandler, terminal, agt)

	// Planコマンドを登録
	registerPlanCommands(cmdHandler, terminal, agt)

	// /providers ステータスコマンドを登録
	registerProvidersStatusCommand(cmdHandler, terminal, provider)

	// Watchコマンドを登録
	registerWatchCommands(cmdHandler, terminal, agt)

	// タブ補完候補をLineEditorに設定
	terminal.GetLineEditor().SetCompletions(cmdHandler.CommandNames())

	return cmdHandler
}

// registerConfigCommands は設定関連のスラッシュコマンドを登録する
func registerConfigCommands(cmdHandler *ui.CommandHandler, terminal *ui.Terminal, cfg *config.Config) {
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "config",
		Description: "設定の表示・保存・プロバイダー切替",
		Handler: func(args string) error {
			args = strings.TrimSpace(args)

			switch {
			case args == "save":
				// /config save — 現在の設定を config.json に保存
				if err := cfg.SaveConfigFile(); err != nil {
					terminal.PrintColored(ui.ColorRed, fmt.Sprintf("設定保存エラー: %v\n", err))
					return nil
				}
				terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ 設定を保存しました: %s\n", config.GetConfigFilePath()))

			case strings.HasPrefix(args, "provider "):
				// /config provider <name> — プロバイダー表示
				name := strings.TrimPrefix(args, "provider ")
				name = strings.TrimSpace(name)
				profiles := cfg.GetProviderProfiles()
				if profiles == nil {
					terminal.PrintColored(ui.ColorYellow, "config.json にプロバイダー設定がありません。\n")
					terminal.Println("先に /config save で保存するか、config.json を直接編集してください。")
					return nil
				}
				profile, ok := profiles[name]
				if !ok {
					terminal.PrintColored(ui.ColorRed, fmt.Sprintf("プロバイダー '%s' が見つかりません。\n", name))
					terminal.Println("設定済みプロバイダー:")
					for pName := range profiles {
						marker := ""
						if pName == cfg.Provider {
							marker = " [現在]"
						}
						terminal.Printf("  - %s%s\n", pName, marker)
					}
					return nil
				}
				terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("━━━ プロバイダー: %s ━━━\n", name))
				terminal.Printf("  タイプ: %s\n", profile.Type)
				if profile.Host != "" {
					terminal.Printf("  ホスト: %s\n", profile.Host)
				}
				if profile.APIKey != "" {
					// APIキーはマスク表示
					masked := profile.APIKey
					if len(masked) > 8 {
						masked = masked[:4] + "..." + masked[len(masked)-4:]
					}
					terminal.Printf("  APIキー: %s\n", masked)
				}
				if profile.Model != "" {
					terminal.Printf("  モデル: %s\n", profile.Model)
				}

			default:
				// /config — 現在の設定を表示
				terminal.PrintColored(ui.ColorCyan, "━━━ 現在の設定 ━━━\n")
				terminal.Printf("  プロバイダー: %s\n", cfg.Provider)
				terminal.Printf("  モデル:       %s\n", cfg.Model)
				if cfg.SidecarModel != "" {
					terminal.Printf("  サイドカー:   %s\n", cfg.SidecarModel)
				}
				terminal.Printf("  MaxTokens:    %d\n", cfg.MaxTokens)
				terminal.Printf("  Temperature:  %.1f\n", cfg.Temperature)
				terminal.Printf("  ContextWindow: %d\n", cfg.ContextWindow)

				if cfg.Provider == "ollama" {
					terminal.Printf("  OllamaHost:   %s\n", cfg.OllamaHost)
				} else {
					apiKey := getAPIKeyForProvider(cfg)
					if apiKey != "" {
						masked := apiKey
						if len(masked) > 8 {
							masked = masked[:4] + "..." + masked[len(masked)-4:]
						}
						terminal.Printf("  APIキー:      %s\n", masked)
					}
					if def := llm.GetCloudProviderDef(cfg.Provider); def != nil {
						terminal.Printf("  環境変数:     %s\n", def.EnvKey)
					}
				}

				terminal.Printf("  設定ファイル: %s\n", config.GetConfigFilePath())
				terminal.Print("\n")
				terminal.Println("使い方:")
				terminal.Println("  /config save              — 現在の設定をconfig.jsonに保存")
				terminal.Println("  /config provider <name>   — プロバイダー詳細を表示")
				terminal.Println("  /provider                 — プロバイダー管理（追加・切替・削除）")
			}
			return nil
		},
	})
}

// registerProviderCommands はプロバイダー管理のスラッシュコマンドを登録する
func registerProviderCommands(cmdHandler *ui.CommandHandler, terminal *ui.Terminal, cfg *config.Config) {
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "provider",
		Description: "プロバイダーの一覧・切替・追加・編集・削除",
		Handler: func(args string) error {
			args = strings.TrimSpace(args)

			switch {
			case args == "add":
				return providerAdd(cfg, terminal)
			case strings.HasPrefix(args, "edit "):
				name := strings.TrimSpace(strings.TrimPrefix(args, "edit "))
				return providerEdit(cfg, terminal, name)
			case args == "edit":
				return providerEditInteractive(cfg, terminal)
			case strings.HasPrefix(args, "delete "):
				name := strings.TrimSpace(strings.TrimPrefix(args, "delete "))
				return providerDelete(cfg, terminal, name)
			case args == "delete":
				return providerDeleteInteractive(cfg, terminal)
			case args != "":
				// /provider <name> — 直接切替
				return providerSwitch(cfg, terminal, args)
			default:
				// /provider — メインメニュー
				return providerMenu(cfg, terminal)
			}
		},
	})
}

// providerMenu プロバイダー管理メインメニュー
func providerMenu(cfg *config.Config, terminal *ui.Terminal) error {
	for {
		terminal.PrintColored(ui.ColorCyan, "━━━ プロバイダー管理 ━━━\n\n")

		// 登録済みプロバイダー一覧
		profiles := cfg.GetProviderProfiles()
		registered := make([]string, 0)
		if profiles != nil {
			for key := range profiles {
				registered = append(registered, key)
			}
		}

		// 現在のプロバイダー
		terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("  現在: %s (%s)\n\n", cfg.Provider, cfg.Model))

		// 登録済み一覧
		if len(registered) > 0 {
			terminal.Println("  登録済みプロバイダー:")
			idx := 1
			indexMap := make(map[int]string)
			for _, key := range registered {
				p := profiles[key]
				marker := ""
				if key == cfg.Provider {
					marker = " [現在]"
				}
				model := p.Model
				if model == "" {
					if def := llm.GetCloudProviderDef(key); def != nil {
						model = def.DefaultModel
					} else if def := llm.GetLocalProviderDef(key); def != nil {
						model = def.DefaultModel
					}
				}
				displayName := key
				if def := llm.GetCloudProviderDef(key); def != nil {
					displayName = def.Name
				} else if def := llm.GetLocalProviderDef(key); def != nil {
					displayName = def.Name
				}
				terminal.Printf("  %2d. %s (%s)%s\n", idx, displayName, model, marker)
				indexMap[idx] = key
				idx++
			}
			terminal.Print("\n")
		} else {
			terminal.PrintColored(ui.ColorYellow, "  登録済みプロバイダーなし\n\n")
		}

		// 操作メニュー
		terminal.Println("  操作:")
		terminal.Println("  A. プロバイダーを追加")
		if len(registered) > 1 {
			terminal.Println("  S. プロバイダーを切替")
		}
		if len(registered) > 0 {
			terminal.Println("  E. プロバイダーを編集")
			terminal.Println("  D. プロバイダーを削除")
		}
		terminal.Println("  Q. 戻る")
		terminal.Print("\n")

		choice, err := terminal.ReadLine("選択: ")
		if err != nil {
			return nil
		}
		choice = strings.TrimSpace(strings.ToLower(choice))

		switch choice {
		case "a":
			if err := providerAdd(cfg, terminal); err != nil {
				return err
			}
		case "s":
			if len(registered) > 1 {
				if err := providerSwitchInteractive(cfg, terminal, profiles); err != nil {
					return err
				}
			} else {
				terminal.PrintColored(ui.ColorYellow, "切替可能なプロバイダーが登録されていません\n")
			}
		case "e":
			if len(registered) > 0 {
				if err := providerEditInteractive(cfg, terminal); err != nil {
					return err
				}
			}
		case "d":
			if err := providerDeleteInteractive(cfg, terminal); err != nil {
				return err
			}
		case "q", "":
			// 戻る
			return nil
		default:
			// 番号で直接切替（プロバイダーが登録されている場合のみ）
			var num int
			if _, err := fmt.Sscanf(choice, "%d", &num); err == nil && profiles != nil && len(registered) > 0 {
				idx := 1
				for _, key := range registered {
					if idx == num {
						if err := providerSwitch(cfg, terminal, key); err != nil {
							return err
						}
						break
					}
					idx++
				}
			} else {
				// 不正な入力
				terminal.PrintColored(ui.ColorYellow, "無効な選択です\n")
			}
		}

		// メニューを再表示（ループ続行）
		terminal.Print("\n")
	}
}

// providerAdd 新しいプロバイダーを追加
func providerAdd(cfg *config.Config, terminal *ui.Terminal) error {
	terminal.PrintColored(ui.ColorCyan, "\n━━━ プロバイダーの種類を選択 ━━━\n")
	terminal.Println("  1. クラウドプロバイダー")
	terminal.Println("  2. ローカルプロバイダー")
	terminal.Println("  3. 戻る")

	choice, err := terminal.ReadLine("選択 [1-3]: ")
	if err != nil {
		return nil
	}

	switch choice {
	case "1":
		if switchToCloudProvider(cfg, terminal) {
			terminal.PrintColored(ui.ColorGreen, "✓ プロバイダーが追加されました\n")
			terminal.PrintColored(ui.ColorYellow, "注意: 新しいプロバイダーで接続するには再起動が必要です\n")
		}
	case "2":
		if addLocalProvider(cfg, terminal) {
			terminal.PrintColored(ui.ColorGreen, "✓ プロバイダーが追加されました\n")
			terminal.PrintColored(ui.ColorYellow, "注意: 新しいプロバイダーで接続するには再起動が必要です\n")
		}
	case "3", "":
		// 戻る
	default:
		terminal.PrintColored(ui.ColorYellow, "無効な選択です\n")
	}
	return nil
}

// addLocalProvider ローカルプロバイダーを追加
func addLocalProvider(cfg *config.Config, terminal *ui.Terminal) bool {
	terminal.Print("\n")
	terminal.PrintColored(ui.ColorCyan, "━━━ ローカルプロバイダー セットアップ ━━━\n")

	// ローカルプロバイダー一覧を表示
	providers := llm.GetLocalProviders()
	if len(providers) == 0 {
		terminal.PrintColored(ui.ColorRed, "利用可能なローカルプロバイダーがありません\n")
		return false
	}

	terminal.Println("\nローカルプロバイダー:")
	for i, p := range providers {
		terminal.Printf("  %d. %s\n", i+1, p.Name)
	}
	terminal.Printf("  %d. 戻る\n", len(providers)+1)

	choiceStr, err := terminal.ReadLine(fmt.Sprintf("選択 [1-%d]: ", len(providers)+1))
	if err != nil {
		terminal.PrintColored(ui.ColorRed, fmt.Sprintf("入力エラー: %v\n", err))
		return false
	}

	var choiceNum int
	_, err = fmt.Sscanf(choiceStr, "%d", &choiceNum)
	if err != nil || choiceNum < 1 || choiceNum > len(providers)+1 {
		terminal.PrintColored(ui.ColorRed, "無効な選択です\n")
		return false
	}

	if choiceNum == len(providers)+1 {
		return false // 戻る
	}

	selectedDef := providers[choiceNum-1]
	terminal.Print("\n")
	terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("━━━ %s セットアップ ━━━\n", selectedDef.Name))

	// ホスト設定
	terminal.Printf("デフォルトホスト: %s\n", selectedDef.DefaultHost)
	host, err := terminal.ReadLine(fmt.Sprintf("ホストURL [デフォルト: %s]: ", selectedDef.DefaultHost))
	if err != nil {
		return false
	}
	if host == "" {
		host = selectedDef.DefaultHost
	}
	host = strings.TrimSpace(host)

	// モデル設定
	terminal.Print("\n")
	terminal.Printf("デフォルトモデル: %s\n", selectedDef.DefaultModel)
	model, err := terminal.ReadLine(fmt.Sprintf("モデル名 [デフォルト: %s, Lで一覧から選択]: ", selectedDef.DefaultModel))
	if err != nil {
		return false
	}
	model = strings.TrimSpace(model)

	// L または l でモデルリストから選択
	if model == "L" || model == "l" {
		terminal.Print("\n")
		terminal.PrintColored(ui.ColorCyan, "モデルリストを取得中...\n")

		models, err := llm.FetchLocalProviderModels(host, selectedDef.Key)
		if err != nil {
			terminal.PrintColored(ui.ColorRed, fmt.Sprintf("モデルリスト取得エラー: %v\n", err))
			terminal.Print("手動入力に切り替えます\n")
			model, err = terminal.ReadLine(fmt.Sprintf("モデル名 [デフォルト: %s]: ", selectedDef.DefaultModel))
			if err != nil {
				return false
			}
			if model == "" {
				model = selectedDef.DefaultModel
			}
		} else if len(models) == 0 {
			terminal.PrintColored(ui.ColorYellow, "利用可能なモデルが見つかりませんでした\n")
			terminal.Print("手動入力に切り替えます\n")
			model, err = terminal.ReadLine(fmt.Sprintf("モデル名 [デフォルト: %s]: ", selectedDef.DefaultModel))
			if err != nil {
				return false
			}
			if model == "" {
				model = selectedDef.DefaultModel
			}
		} else {
			terminal.Printf("\n利用可能なモデル (%d件):\n", len(models))
			for i, m := range models {
				terminal.Printf("  %2d. %s\n", i+1, m)
			}
			terminal.Printf("  %2d. 手動入力\n", len(models)+1)

			choiceStr, err := terminal.ReadLine(fmt.Sprintf("選択 [1-%d]: ", len(models)+1))
			if err != nil {
				return false
			}

			var choiceNum int
			_, err = fmt.Sscanf(choiceStr, "%d", &choiceNum)
			if err != nil || choiceNum < 1 || choiceNum > len(models)+1 {
				terminal.PrintColored(ui.ColorYellow, "無効な選択です。デフォルトモデルを使用します\n")
				model = selectedDef.DefaultModel
			} else if choiceNum == len(models)+1 {
				// 手動入力
				model, err = terminal.ReadLine("モデル名: ")
				if err != nil {
					return false
				}
				if model == "" {
					model = selectedDef.DefaultModel
				}
			} else {
				model = models[choiceNum-1]
			}
		}
	} else if model == "" {
		model = selectedDef.DefaultModel
	}
	model = strings.TrimSpace(model)

	// Ollama の場合、モデルが存在するか確認し、なければ pull を提案
	if selectedDef.Key == "ollama" {
		model = checkAndPullOllamaModel(host, model, terminal)
	}

	// cfg を更新
	cfg.Provider = selectedDef.Key
	if selectedDef.Key == "ollama" {
		cfg.OllamaHost = host
	}
	cfg.Model = model
	cfg.AutoModel = false

	terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ %s に切替: %s\n", selectedDef.Name, model))
	terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("  ホスト: %s\n", host))

	// 設定を保存するか確認
	save, _ := terminal.ReadLine("この設定を config.json に保存しますか？ [Y/n]: ")
	if save != "n" && save != "N" {
		// プロファイルとして保存
		profile := config.ProviderProfile{
			Model: model,
			Host:  host,
		}
		if err := cfg.SaveProviderProfile(selectedDef.Key, profile); err != nil {
			terminal.PrintColored(ui.ColorYellow, fmt.Sprintf("設定保存スキップ: %v\n", err))
		} else {
			terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ 設定を保存: %s\n", config.GetConfigFilePath()))
		}
	}

	return true
}

// providerSwitch 登録済みプロバイダーに切替
func providerSwitch(cfg *config.Config, terminal *ui.Terminal, key string) error {
	profiles := cfg.GetProviderProfiles()
	if profiles == nil {
		terminal.PrintColored(ui.ColorRed, "登録済みプロバイダーがありません。先に /provider add で追加してください。\n")
		return nil
	}

	profile, ok := profiles[key]
	if !ok {
		terminal.PrintColored(ui.ColorRed, fmt.Sprintf("プロバイダー '%s' が見つかりません\n", key))
		return nil
	}

	displayName := key
	if def := llm.GetCloudProviderDef(key); def != nil {
		displayName = def.Name
	} else if def := llm.GetLocalProviderDef(key); def != nil {
		displayName = def.Name
	}

	modelName := profile.Model
	if modelName == "" {
		if def := llm.GetCloudProviderDef(key); def != nil {
			modelName = def.DefaultModel
		} else if def := llm.GetLocalProviderDef(key); def != nil {
			modelName = def.DefaultModel
		}
	}

	// 確認プロンプト
	confirm, _ := terminal.ReadLine(fmt.Sprintf("%s (%s) に切替えますか？ [y/N]: ", displayName, modelName))
	if confirm != "y" && confirm != "Y" {
		terminal.Println("キャンセルしました")
		return nil
	}

	// cfg を更新
	cfg.Provider = key
	if profile.Model != "" {
		cfg.Model = profile.Model
	} else if def := llm.GetCloudProviderDef(key); def != nil {
		cfg.Model = def.DefaultModel
	} else if def := llm.GetLocalProviderDef(key); def != nil {
		cfg.Model = def.DefaultModel
	}
	if key == "ollama" && profile.Host != "" {
		cfg.OllamaHost = profile.Host
	} else if def := llm.GetLocalProviderDef(key); def != nil && profile.Host != "" {
		// lm-studio, llama-server の場合
		cfg.OllamaHost = profile.Host
	}
	if profile.APIKey != "" {
		if cfg.CloudAPIKeys == nil {
			cfg.CloudAPIKeys = make(map[string]string)
		}
		cfg.CloudAPIKeys[key] = profile.APIKey
	}

	// アクティブプロバイダーをconfig.jsonに保存
	if err := cfg.SaveConfigFile(); err != nil {
		terminal.PrintColored(ui.ColorYellow, fmt.Sprintf("設定保存スキップ: %v\n", err))
	}

	terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ %s (%s) に切替しました\n", displayName, cfg.Model))
	terminal.PrintColored(ui.ColorYellow, "注意: 切替を反映するには再起動が必要です\n")
	return nil
}

// providerSwitchInteractive 登録済みプロバイダーから選択して切替
func providerSwitchInteractive(cfg *config.Config, terminal *ui.Terminal, profiles map[string]config.ProviderProfile) error {
	terminal.PrintColored(ui.ColorCyan, "\n━━━ プロバイダー切替 ━━━\n")

	keys := make([]string, 0)
	idx := 1
	for key := range profiles {
		marker := ""
		if key == cfg.Provider {
			marker = " [現在]"
		}
		displayName := key
		if def := llm.GetCloudProviderDef(key); def != nil {
			displayName = def.Name
		} else if def := llm.GetLocalProviderDef(key); def != nil {
			displayName = def.Name
		}
		p := profiles[key]
		model := p.Model
		if model == "" {
			if def := llm.GetCloudProviderDef(key); def != nil {
				model = def.DefaultModel
			} else if def := llm.GetLocalProviderDef(key); def != nil {
				model = def.DefaultModel
			}
		}
		terminal.Printf("  %d. %s (%s)%s\n", idx, displayName, model, marker)
		keys = append(keys, key)
		idx++
	}
	terminal.Printf("  %d. 戻る\n", idx)

	choice, err := terminal.ReadLine(fmt.Sprintf("選択 [1-%d]: ", idx))
	if err != nil {
		return nil
	}
	var num int
	if _, err := fmt.Sscanf(choice, "%d", &num); err != nil || num < 1 || num > idx {
		return nil
	}
	if num == idx {
		return nil
	}

	return providerSwitch(cfg, terminal, keys[num-1])
}

// providerEdit 登録済みプロバイダーの設定を編集
func providerEdit(cfg *config.Config, terminal *ui.Terminal, key string) error {
	profiles := cfg.GetProviderProfiles()
	if profiles == nil {
		terminal.PrintColored(ui.ColorRed, "登録済みプロバイダーがありません\n")
		return nil
	}

	profile, ok := profiles[key]
	if !ok {
		terminal.PrintColored(ui.ColorRed, fmt.Sprintf("プロバイダー '%s' が見つかりません\n", key))
		return nil
	}

	displayName := key
	if def := llm.GetCloudProviderDef(key); def != nil {
		displayName = def.Name
	} else if def := llm.GetLocalProviderDef(key); def != nil {
		displayName = def.Name
	}

	terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("\n━━━ %s を編集 ━━━\n", displayName))

	// --- APIキー編集（クラウドプロバイダーのみ）---
	if llm.GetCloudProviderDef(key) != nil {
		currentKey := profile.APIKey
		masked := "(未設定)"
		if currentKey != "" && len(currentKey) > 8 {
			masked = currentKey[:4] + "..." + currentKey[len(currentKey)-4:]
		} else if currentKey != "" {
			masked = "****"
		}
		terminal.Printf("  現在のAPIキー: %s\n", masked)
		newKey, _ := terminal.ReadLine("  新しいAPIキー (変更しない場合は空Enter): ")
		newKey = strings.TrimSpace(newKey)
		if newKey != "" {
			profile.APIKey = newKey
			// ランタイム cfg にも反映
			if cfg.CloudAPIKeys == nil {
				cfg.CloudAPIKeys = make(map[string]string)
			}
			cfg.CloudAPIKeys[key] = newKey
		}
	}

	// --- ホスト編集（ローカルプロバイダーのみ）---
	if llm.GetLocalProviderDef(key) != nil {
		currentHost := profile.Host
		if currentHost == "" {
			if key == "ollama" {
				currentHost = cfg.OllamaHost
			} else {
				currentHost = llm.GetLocalProviderDef(key).DefaultHost
			}
		}
		terminal.Printf("  現在のホスト: %s\n", currentHost)
		newHost, _ := terminal.ReadLine("  新しいホスト (変更しない場合は空Enter): ")
		newHost = strings.TrimSpace(newHost)
		if newHost != "" {
			profile.Host = newHost
			if key == "ollama" {
				cfg.OllamaHost = newHost
			}
		}
	}

	// --- モデル編集 ---
	currentModel := profile.Model
	if currentModel == "" {
		if def := llm.GetCloudProviderDef(key); def != nil {
			currentModel = def.DefaultModel
		} else if def := llm.GetLocalProviderDef(key); def != nil {
			currentModel = def.DefaultModel
		}
	}
	terminal.Printf("  現在のモデル: %s\n", currentModel)

	// クラウドプロバイダーの場合：推奨モデルの一覧を表示
	if def := llm.GetCloudProviderDef(key); def != nil && len(def.Models) > 0 {
		terminal.Println("  推奨モデル:")
		for i, m := range def.Models {
			mark := ""
			if m == currentModel {
				mark = " [現在]"
			}
			terminal.Printf("    %d. %s%s\n", i+1, m, mark)
		}
		customIdx := len(def.Models) + 1
		terminal.Printf("    %d. カスタムモデル名を入力\n", customIdx)
		terminal.Printf("    0. 変更しない\n")

		modelChoice, _ := terminal.ReadLine(fmt.Sprintf("  選択 [0-%d]: ", customIdx))
		var modelNum int
		if _, err := fmt.Sscanf(modelChoice, "%d", &modelNum); err == nil {
			if modelNum >= 1 && modelNum <= len(def.Models) {
				profile.Model = def.Models[modelNum-1]
			} else if modelNum == customIdx {
				custom, _ := terminal.ReadLine("  モデル名: ")
				custom = strings.TrimSpace(custom)
				if custom != "" {
					profile.Model = custom
				}
			}
			// 0 の場合は変更なし
		}
	} else if def := llm.GetLocalProviderDef(key); def != nil {
		// ローカルプロバイダーの場合：モデルリストを取得
		host := profile.Host
		if host == "" {
			if key == "ollama" {
				host = cfg.OllamaHost
			} else {
				host = def.DefaultHost
			}
		}
		terminal.Println("  選択肢:")
		terminal.Println("    1. 利用可能なモデル一覧から選択")
		terminal.Println("    2. カスタムモデル名を入力")
		terminal.Println("    0. 変更しない")

		choice, _ := terminal.ReadLine("  選択 [0-2]: ")
		switch strings.TrimSpace(choice) {
		case "1":
			terminal.Print("\n")
			terminal.PrintColored(ui.ColorCyan, "モデルリストを取得中...\n")
			models, err := llm.FetchLocalProviderModels(host, key)
			if err != nil {
				terminal.PrintColored(ui.ColorRed, fmt.Sprintf("モデルリスト取得エラー: %v\n", err))
				terminal.Print("手動入力に切り替えます\n")
				custom, _ := terminal.ReadLine("  モデル名: ")
				custom = strings.TrimSpace(custom)
				if custom != "" {
					profile.Model = custom
				}
			} else if len(models) == 0 {
				terminal.PrintColored(ui.ColorYellow, "利用可能なモデルが見つかりませんでした\n")
				terminal.Print("手動入力に切り替えます\n")
				custom, _ := terminal.ReadLine("  モデル名: ")
				custom = strings.TrimSpace(custom)
				if custom != "" {
					profile.Model = custom
				}
			} else {
				terminal.Printf("\n利用可能なモデル (%d件):\n", len(models))
				for i, m := range models {
					mark := ""
					if m == currentModel {
						mark = " [現在]"
					}
					terminal.Printf("  %2d. %s%s\n", i+1, m, mark)
				}
				terminal.Printf("  %2d. 手動入力\n", len(models)+1)
				terminal.Printf("  %2d. 変更しない\n", len(models)+2)

				choiceStr, err := terminal.ReadLine(fmt.Sprintf("  選択 [1-%d]: ", len(models)+2))
				if err == nil {
					var choiceNum int
					if _, err := fmt.Sscanf(choiceStr, "%d", &choiceNum); err == nil {
						if choiceNum >= 1 && choiceNum <= len(models) {
							profile.Model = models[choiceNum-1]
						} else if choiceNum == len(models)+1 {
							custom, _ := terminal.ReadLine("  モデル名: ")
							custom = strings.TrimSpace(custom)
							if custom != "" {
								profile.Model = custom
							}
						}
						// len(models)+2 または 0 の場合は変更なし
					}
				}
			}
		case "2":
			custom, _ := terminal.ReadLine("  モデル名: ")
			custom = strings.TrimSpace(custom)
			if custom != "" {
				profile.Model = custom
			}
		}
		// 0 の場合は変更なし
	} else {
		newModel, _ := terminal.ReadLine("  新しいモデル名 (変更しない場合は空Enter): ")
		newModel = strings.TrimSpace(newModel)
		if newModel != "" {
			profile.Model = newModel
		}
	}

	// Ollama の場合、モデルが変更されたらダウンロード状態を確認
	if key == "ollama" && profile.Model != "" && profile.Model != currentModel {
		host := profile.Host
		if host == "" {
			host = cfg.OllamaHost
		}
		if host == "" {
			host = llm.GetLocalProviderDef("ollama").DefaultHost
		}
		profile.Model = checkAndPullOllamaModel(host, profile.Model, terminal)
	}

	// ランタイム cfg に反映（現在のプロバイダーの場合）
	if key == cfg.Provider && profile.Model != "" {
		cfg.Model = profile.Model
	}

	// config.json に保存
	if err := cfg.SaveProviderProfile(key, profile); err != nil {
		terminal.PrintColored(ui.ColorRed, fmt.Sprintf("保存エラー: %v\n", err))
		return nil
	}

	terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ %s の設定を更新しました\n", displayName))
	if key == cfg.Provider {
		terminal.PrintColored(ui.ColorYellow, "注意: 変更を反映するには再起動が必要です\n")
	}
	return nil
}

// providerEditInteractive 編集対象を選択
func providerEditInteractive(cfg *config.Config, terminal *ui.Terminal) error {
	profiles := cfg.GetProviderProfiles()
	if profiles == nil || len(profiles) == 0 {
		terminal.PrintColored(ui.ColorYellow, "編集可能なプロバイダーがありません\n")
		return nil
	}

	terminal.PrintColored(ui.ColorCyan, "\n━━━ プロバイダー編集 ━━━\n")

	keys := make([]string, 0)
	idx := 1
	for key := range profiles {
		displayName := key
		if def := llm.GetCloudProviderDef(key); def != nil {
			displayName = def.Name
		} else if def := llm.GetLocalProviderDef(key); def != nil {
			displayName = def.Name
		}
		marker := ""
		if key == cfg.Provider {
			marker = " [現在]"
		}
		terminal.Printf("  %d. %s%s\n", idx, displayName, marker)
		keys = append(keys, key)
		idx++
	}
	terminal.Printf("  %d. 戻る\n", idx)

	choice, err := terminal.ReadLine(fmt.Sprintf("選択 [1-%d]: ", idx))
	if err != nil {
		return nil
	}
	var num int
	if _, err := fmt.Sscanf(choice, "%d", &num); err != nil || num < 1 || num > idx {
		return nil
	}
	if num == idx {
		return nil
	}

	return providerEdit(cfg, terminal, keys[num-1])
}

// providerDelete 指定プロバイダーを削除
func providerDelete(cfg *config.Config, terminal *ui.Terminal, key string) error {
	if key == cfg.Provider {
		terminal.PrintColored(ui.ColorRed, "現在使用中のプロバイダーは削除できません。先に切替えてください。\n")
		return nil
	}

	displayName := key
	if def := llm.GetCloudProviderDef(key); def != nil {
		displayName = def.Name
	} else if def := llm.GetLocalProviderDef(key); def != nil {
		displayName = def.Name
	}

	confirm, _ := terminal.ReadLine(fmt.Sprintf("%s を削除しますか？ [y/N]: ", displayName))
	if confirm != "y" && confirm != "Y" {
		terminal.Println("キャンセルしました")
		return nil
	}

	if err := cfg.DeleteProviderProfile(key); err != nil {
		terminal.PrintColored(ui.ColorRed, fmt.Sprintf("削除エラー: %v\n", err))
		return nil
	}

	terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ %s を削除しました\n", displayName))
	return nil
}

// providerDeleteInteractive 削除対象を選択
func providerDeleteInteractive(cfg *config.Config, terminal *ui.Terminal) error {
	profiles := cfg.GetProviderProfiles()
	if profiles == nil || len(profiles) == 0 {
		terminal.PrintColored(ui.ColorYellow, "削除可能なプロバイダーがありません\n")
		return nil
	}

	terminal.PrintColored(ui.ColorCyan, "\n━━━ プロバイダー削除 ━━━\n")

	keys := make([]string, 0)
	idx := 1
	for key := range profiles {
		marker := ""
		if key == cfg.Provider {
			marker = " [現在 — 削除不可]"
		}
		displayName := key
		if def := llm.GetCloudProviderDef(key); def != nil {
			displayName = def.Name
		} else if def := llm.GetLocalProviderDef(key); def != nil {
			displayName = def.Name
		}
		terminal.Printf("  %d. %s%s\n", idx, displayName, marker)
		keys = append(keys, key)
		idx++
	}
	terminal.Printf("  %d. 戻る\n", idx)

	choice, err := terminal.ReadLine(fmt.Sprintf("削除する番号 [1-%d]: ", idx))
	if err != nil {
		return nil
	}
	var num int
	if _, err := fmt.Sscanf(choice, "%d", &num); err != nil || num < 1 || num > idx {
		return nil
	}
	if num == idx {
		return nil
	}

	return providerDelete(cfg, terminal, keys[num-1])
}

// registerSandboxCommands はサンドボックス関連のスラッシュコマンドを登録する
func registerSandboxCommands(cmdHandler *ui.CommandHandler, terminal *ui.Terminal, sbMgr *sandbox.Manager) {
	// /sandbox [on|off] — サンドボックスモード切替
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "sandbox",
		Description: "サンドボックスモード切替",
		Handler: func(args string) error {
			if sbMgr == nil {
				terminal.PrintColored(ui.ColorYellow, "サンドボックスが初期化されていません。--sandbox フラグで起動してください。\n")
				return nil
			}

			switch strings.TrimSpace(args) {
			case "on":
				if err := sbMgr.SetEnabled(true); err != nil {
					terminal.PrintColored(ui.ColorRed, fmt.Sprintf("サンドボックス有効化エラー: %v\n", err))
					return nil
				}
				terminal.PrintColored(ui.ColorGreen, "✓ サンドボックスモード: ON\n")
			case "off":
				count := sbMgr.StagedCount()
				if count > 0 {
					terminal.PrintColored(ui.ColorYellow, fmt.Sprintf("⚠ %d件のステージされたファイルがあります。先に /commit または /discard してください。\n", count))
					return nil
				}
				if err := sbMgr.SetEnabled(false); err != nil {
					terminal.PrintColored(ui.ColorRed, fmt.Sprintf("サンドボックス無効化エラー: %v\n", err))
					return nil
				}
				terminal.PrintColored(ui.ColorYellow, "✓ サンドボックスモード: OFF\n")
			default:
				status := "OFF"
				if sbMgr.IsEnabled() {
					status = "ON"
				}
				terminal.Printf("サンドボックスモード: %s\n", status)
				terminal.Printf("ステージされたファイル: %d件\n", sbMgr.StagedCount())
			}
			return nil
		},
	})

	// /commit [file] — ステージされたファイルを本番反映
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "commit",
		Description: "ステージされたファイルを本番に反映",
		Handler: func(args string) error {
			if sbMgr == nil {
				terminal.PrintColored(ui.ColorYellow, "サンドボックスが初期化されていません。\n")
				return nil
			}

			if sbMgr.StagedCount() == 0 {
				terminal.Println("コミットするファイルがありません。")
				return nil
			}

			args = strings.TrimSpace(args)
			if args != "" {
				if err := sbMgr.CommitFile(args); err != nil {
					terminal.PrintColored(ui.ColorRed, fmt.Sprintf("コミットエラー: %v\n", err))
					return nil
				}
				terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ コミット完了: %s\n", args))
			} else {
				committed, err := sbMgr.Commit()
				if err != nil {
					terminal.PrintColored(ui.ColorRed, fmt.Sprintf("コミットエラー: %v\n", err))
					return nil
				}
				terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ %d件のファイルをコミットしました:\n", len(committed)))
				for _, f := range committed {
					terminal.Printf("  📄 %s\n", f)
				}
			}
			return nil
		},
	})

	// /discard [file] — ステージされたファイルを破棄
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "discard",
		Description: "ステージされたファイルを破棄",
		Handler: func(args string) error {
			if sbMgr == nil {
				terminal.PrintColored(ui.ColorYellow, "サンドボックスが初期化されていません。\n")
				return nil
			}

			if sbMgr.StagedCount() == 0 {
				terminal.Println("破棄するファイルがありません。")
				return nil
			}

			args = strings.TrimSpace(args)
			if args != "" {
				if err := sbMgr.DiscardFile(args); err != nil {
					terminal.PrintColored(ui.ColorRed, fmt.Sprintf("破棄エラー: %v\n", err))
					return nil
				}
				terminal.PrintColored(ui.ColorYellow, fmt.Sprintf("✗ 破棄しました: %s\n", args))
			} else {
				if err := sbMgr.Discard(); err != nil {
					terminal.PrintColored(ui.ColorRed, fmt.Sprintf("破棄エラー: %v\n", err))
					return nil
				}
				terminal.PrintColored(ui.ColorYellow, "✗ 全てのステージされたファイルを破棄しました\n")
			}
			return nil
		},
	})

	// /diff [file] — 差分を表示
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "diff",
		Description: "ステージされたファイルの差分を表示",
		Handler: func(args string) error {
			if sbMgr == nil {
				terminal.PrintColored(ui.ColorYellow, "サンドボックスが初期化されていません。\n")
				return nil
			}

			if sbMgr.StagedCount() == 0 {
				terminal.Println("ステージされたファイルがありません。")
				return nil
			}

			args = strings.TrimSpace(args)
			if args != "" {
				diff, err := sbMgr.Diff(args)
				if err != nil {
					terminal.PrintColored(ui.ColorRed, fmt.Sprintf("差分エラー: %v\n", err))
					return nil
				}
				terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("━━━ %s ━━━\n", args))
				terminal.Print(diff)
			} else {
				diff, err := sbMgr.DiffAll()
				if err != nil {
					terminal.PrintColored(ui.ColorRed, fmt.Sprintf("差分エラー: %v\n", err))
					return nil
				}
				terminal.PrintColored(ui.ColorCyan, "━━━ Staged Changes ━━━\n")
				terminal.Print(diff)
			}
			return nil
		},
	})

	// /staged — ステージされたファイル一覧
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "staged",
		Description: "ステージされたファイル一覧を表示",
		Handler: func(args string) error {
			if sbMgr == nil {
				terminal.PrintColored(ui.ColorYellow, "サンドボックスが初期化されていません。\n")
				return nil
			}

			files := sbMgr.ListStaged()
			if len(files) == 0 {
				terminal.Println("ステージされたファイルがありません。")
				return nil
			}

			terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("━━━ ステージされたファイル (%d件) ━━━\n", len(files)))
			for _, f := range files {
				status := "M" // modified
				if f.IsNew {
					status = "A" // added
				}
				terminal.Printf("  [%s] %s\n", status, f.RelativePath)
			}
			return nil
		},
	})
}

func createToolRegistry(terminal *ui.Terminal, perm *security.PermissionManager, validator *security.PathValidator, sbMgr *sandbox.Manager, cfg *config.Config) *tool.Registry {
	registry := tool.NewRegistry()

	// Create tools
	bashTool := tool.NewBashTool()
	writeTool := tool.NewWriteTool()
	editTool := tool.NewEditTool()

	// NOTE: ファイルステージング（.vibe-sandbox/経由）は現在無効
	// サンドボックスの目的はPython仮想環境(.venv/)による隔離のみ
	// write_file/edit_fileは直接プロジェクトに書き込む
	_ = sbMgr // 将来の拡張用に引数は維持

	// 自動venvが有効な場合、BashToolに設定
	if cfg.AutoVenv {
		bashTool.SetAutoVenv(true, cfg.VenvDir)
	}

	// Register tools
	registry.Register(bashTool)
	registry.Register(tool.NewReadTool())
	registry.Register(writeTool)
	registry.Register(editTool)
	registry.Register(tool.NewGlobTool())
	registry.Register(tool.NewGrepTool())
	registry.Register(tool.NewWebFetchTool())
	registry.Register(tool.NewWebSearchTool())
	registry.Register(tool.NewNotebookEditTool())

	return registry
}

// checkProviderConnection checks the LLM provider connection
// 接続失敗時はリトライ/再設定/終了を選択できる
// プロバイダーが再設定された場合は新しいプロバイダーを返す
func checkProviderConnection(ctx context.Context, provider llm.LLMProvider, cfg *config.Config, terminal *ui.Terminal) llm.LLMProvider {
	for {
		info := provider.Info()
		terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("%s (%s) 接続を確認中...\n", info.Name, info.BaseURL))

		err := provider.CheckHealth(ctx)
		if err == nil {
			terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ %s 接続確認\n", info.Name))
			return provider
		}

		terminal.PrintColored(ui.ColorRed, fmt.Sprintf("接続エラー: %v\n", err))
		terminal.Print("\n")
		terminal.Println("  1. リトライ")
		terminal.Println("  2. プロバイダーを再設定")
		terminal.Println("  3. 終了")

		choice, readErr := terminal.ReadLine("選択 [1-3]: ")
		if readErr != nil {
			os.Exit(1)
		}

		switch choice {
		case "1":
			// リトライ — ループ先頭に戻る
			continue
		case "2":
			// プロバイダー再設定（クラウド/ローカル選択）
			terminal.PrintColored(ui.ColorCyan, "\n━━━ プロバイダーの種類を選択 ━━━\n")
			terminal.Println("  1. クラウドプロバイダー")
			terminal.Println("  2. ローカルプロバイダー")
			terminal.Println("  3. 戻る")
			typeChoice, _ := terminal.ReadLine("選択 [1-3]: ")
			switched := false
			switch typeChoice {
			case "1":
				switched = switchToCloudProvider(cfg, terminal)
			case "2":
				switched = addLocalProvider(cfg, terminal)
			}
			if switched {
				provider = createProvider(cfg)
				continue
			}
			// 戻るが選択された場合はリトライ
			continue
		default:
			os.Exit(0)
		}
	}
}

// pullModelIfNeeded checks and pulls model if needed (ModelManager対応プロバイダーのみ)
// クラウドプロバイダーへの切替が選択された場合は true を返す
func pullModelIfNeeded(ctx context.Context, provider llm.LLMProvider, cfg *config.Config, terminal *ui.Terminal) bool {
	mm, ok := provider.(llm.ModelManager)
	if !ok {
		// ModelManager非対応の場合はスキップ
		return false
	}

	modelName := cfg.Model
	terminal.Printf("モデル '%s' を確認中...\n", modelName)
	exists, err := mm.CheckModel(ctx, modelName)
	if err != nil {
		terminal.PrintColored(ui.ColorRed, fmt.Sprintf("モデル確認エラー: %v\n", err))
		os.Exit(1)
	}

	if exists {
		terminal.PrintColored(ui.ColorGreen, "✓ モデル確認済み\n")
		return false
	}

	terminal.PrintColored(ui.ColorYellow, fmt.Sprintf("モデル '%s' が見つかりません\n", modelName))

	availableModels, err := mm.ListModels(ctx)
	if err != nil || len(availableModels) == 0 {
		terminal.PrintColored(ui.ColorYellow, "利用可能なモデルがありません。ダウンロードを試みます...\n")
		terminal.Printf("モデル '%s' をダウンロード中...\n", modelName)
		// OllamaProvider の場合はプログレス表示付きでpull
		if ollamaP, ok := provider.(*llm.OllamaProvider); ok {
			err = pullOllamaModelWithProgress(ctx, ollamaP, modelName, terminal)
		} else {
			err = mm.PullModel(ctx, modelName)
		}
		if err != nil {
			terminal.PrintColored(ui.ColorRed, fmt.Sprintf("\nモデルダウンロードエラー: %v\n", err))
			terminal.Println("以下の方法でモデルをインストールしてください：")
			terminal.Println("  1. 別のモデルを使用する: ./vibe-local-go -model <model-name>")
			terminal.Println("  2. モデルを手動でインストール: ollama pull <model-name>")
			os.Exit(1)
		}
		terminal.PrintColored(ui.ColorGreen, "\n✓ モデルダウンロード完了\n")
		return false
	}

	terminal.PrintColored(ui.ColorCyan, "利用可能なローカルモデル:\n")
	for i, model := range availableModels {
		terminal.Printf("  %2d. %s\n", i+1, model)
	}
	terminal.Print("\n")

	terminal.Println("選択肢:")
	terminal.Println("  1. 利用可能なモデルから選択")
	terminal.Println("  2. 指定したモデルをダウンロード")
	terminal.PrintColored(ui.ColorCyan, "  3. クラウドプロバイダーに切替\n")
	terminal.Println("  4. 終了")

	choice, err := terminal.ReadLine("選択してください [1-4]: ")
	if err != nil {
		terminal.PrintColored(ui.ColorRed, fmt.Sprintf("入力エラー: %v\n", err))
		os.Exit(1)
	}

	switch choice {
	case "1":
		idx, err := terminal.ReadLine("モデル番号を入力: ")
		if err != nil {
			terminal.PrintColored(ui.ColorRed, fmt.Sprintf("入力エラー: %v\n", err))
			os.Exit(1)
		}
		var num int
		_, err = fmt.Sscanf(idx, "%d", &num)
		if err != nil || num < 1 || num > len(availableModels) {
			terminal.PrintColored(ui.ColorRed, "無効な選択です\n")
			os.Exit(1)
		}
		selectedModel := availableModels[num-1]
		terminal.Printf("モデル '%s' を使用します\n", selectedModel)
		cfg.Model = selectedModel
		cfg.AutoModel = false
		return false

	case "2":
		// モデル名を入力（デフォルトは設定のモデル）
		input, err := terminal.ReadLine(fmt.Sprintf("ダウンロードするモデル名 [%s]: ", modelName))
		if err != nil {
			terminal.PrintColored(ui.ColorRed, fmt.Sprintf("入力エラー: %v\n", err))
			os.Exit(1)
		}
		if input != "" {
			modelName = input
		}
		terminal.Printf("モデル '%s' をダウンロード中...\n", modelName)
		// OllamaProvider の場合はプログレス表示付きでpull
		if ollamaP, ok := provider.(*llm.OllamaProvider); ok {
			err = pullOllamaModelWithProgress(ctx, ollamaP, modelName, terminal)
		} else {
			err = mm.PullModel(ctx, modelName)
		}
		if err != nil {
			terminal.PrintColored(ui.ColorRed, fmt.Sprintf("\nモデルダウンロードエラー: %v\n", err))
			os.Exit(1)
		}
		terminal.PrintColored(ui.ColorGreen, "\n✓ モデルダウンロード完了\n")
		cfg.Model = modelName
		cfg.AutoModel = false
		return false

	case "3":
		return switchToCloudProvider(cfg, terminal)

	default:
		os.Exit(0)
		return false
	}
}

// switchToCloudProvider クラウドプロバイダーへの切替処理
func switchToCloudProvider(cfg *config.Config, terminal *ui.Terminal) bool {
	terminal.Print("\n")
	terminal.PrintColored(ui.ColorCyan, "━━━ クラウドプロバイダー セットアップ ━━━\n")

	// カテゴリ別にプロバイダーを表示（番号は通し番号）
	// indexMap: 表示番号 → CloudProviderDef
	indexMap := make(map[int]llm.CloudProviderDef)
	num := 1
	for _, cat := range llm.CloudProviderCategories {
		providers := llm.GetProvidersByCategory(cat.Key)
		if len(providers) == 0 {
			continue
		}
		terminal.PrintColored(ui.ColorGray, fmt.Sprintf("\n  ── %s ──\n", cat.Label))
		for _, def := range providers {
			envStatus := ""
			if envKey := os.Getenv(def.EnvKey); envKey != "" {
				envStatus = " ✓"
			}
			terminal.Printf("  %2d. %s%s\n", num, def.Name, envStatus)
			indexMap[num] = def
			num++
		}
	}
	terminal.Print("\n")
	terminal.Printf("  %2d. 戻る\n", num)

	choiceStr, err := terminal.ReadLine(fmt.Sprintf("選択 [1-%d]: ", num))
	if err != nil {
		terminal.PrintColored(ui.ColorRed, fmt.Sprintf("入力エラー: %v\n", err))
		return false
	}
	var choiceNum int
	_, err = fmt.Sscanf(choiceStr, "%d", &choiceNum)
	if err != nil || choiceNum < 1 || choiceNum > num {
		terminal.PrintColored(ui.ColorRed, "無効な選択です\n")
		return false
	}
	if choiceNum == num {
		return false // 戻る
	}

	selectedDef := indexMap[choiceNum]
	terminal.Print("\n")
	terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("━━━ %s セットアップ ━━━\n", selectedDef.Name))

	// APIキー取得: 環境変数 → config.json保存済み → ユーザー入力
	apiKey := os.Getenv(selectedDef.EnvKey)

	// config.jsonの保存済みキーを確認
	if apiKey == "" {
		profiles := cfg.GetProviderProfiles()
		if profiles != nil {
			if p, ok := profiles[selectedDef.Key]; ok && p.APIKey != "" {
				apiKey = p.APIKey
			}
		}
	}

	if apiKey != "" {
		// 既存キーをマスク表示
		masked := apiKey
		if len(masked) > 8 {
			masked = masked[:4] + "..." + masked[len(masked)-4:]
		}
		terminal.Printf("検出済みAPIキー: %s\n", masked)
		use, _ := terminal.ReadLine("このキーを使用しますか？ [Y/n]: ")
		if use == "n" || use == "N" {
			apiKey = ""
		}
	}

	if apiKey == "" {
		terminal.Printf("APIキーを入力してください (%s)\n", selectedDef.EnvKey)
		key, err := terminal.ReadLine("APIキー: ")
		if err != nil || key == "" {
			terminal.PrintColored(ui.ColorRed, "APIキーが必要です。ローカルモードで続行します。\n")
			return false
		}
		apiKey = key
	}

	// モデル選択
	terminal.Print("\n")
	terminal.Println("モデルを選択してください:")
	for i, m := range selectedDef.Models {
		defaultMark := ""
		if m == selectedDef.DefaultModel {
			defaultMark = " (デフォルト)"
		}
		terminal.Printf("  %d. %s%s\n", i+1, m, defaultMark)
	}
	customIdx := len(selectedDef.Models) + 1
	terminal.Printf("  %d. カスタムモデル名を入力\n", customIdx)

	modelChoice, _ := terminal.ReadLine(fmt.Sprintf("選択 [1-%d]: ", customIdx))
	var modelNum int
	var model string
	_, err = fmt.Sscanf(modelChoice, "%d", &modelNum)
	if err == nil && modelNum >= 1 && modelNum <= len(selectedDef.Models) {
		model = selectedDef.Models[modelNum-1]
	} else if modelNum == customIdx {
		m, err := terminal.ReadLine("モデル名: ")
		if err != nil || m == "" {
			model = selectedDef.DefaultModel
			terminal.Printf("デフォルトモデルを使用: %s\n", model)
		} else {
			model = m
		}
	} else {
		model = selectedDef.DefaultModel
		terminal.Printf("デフォルトモデルを使用: %s\n", model)
	}

	// cfg を更新
	cfg.Provider = selectedDef.Key
	setAPIKeyForProvider(cfg, selectedDef.Key, apiKey)
	cfg.Model = model
	cfg.AutoModel = false

	terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ %s に切替: %s\n", selectedDef.Name, model))

	// 設定を保存するか確認
	save, _ := terminal.ReadLine("この設定を config.json に保存しますか？ [Y/n]: ")
	if save != "n" && save != "N" {
		if err := cfg.SaveConfigFile(); err != nil {
			terminal.PrintColored(ui.ColorYellow, fmt.Sprintf("設定保存スキップ: %v\n", err))
		} else {
			terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ 設定を保存: %s\n", config.GetConfigFilePath()))
		}
	}

	return true
}

func showBanner(terminal *ui.Terminal, cfg *config.Config, router *llm.ModelRouter, provider llm.LLMProvider) {
	tier := router.GetModelTier(cfg.Model)
	cwd, _ := os.Getwd()

	// プロバイダーに応じてホスト表示を変更
	hostDisplay := cfg.OllamaHost
	if def := llm.GetCloudProviderDef(cfg.Provider); def != nil {
		hostDisplay = def.BaseURL
	}

	// ProviderChain の場合はチェーン情報を構築
	chainInfo := ""
	if chain, ok := provider.(*llm.ProviderChain); ok {
		entries := chain.GetEntries()
		if len(entries) > 1 {
			parts := make([]string, 0, len(entries))
			for _, e := range entries {
				info := e.Provider.Info()
				icon := ui.ProviderIcon(info.Name)
				parts = append(parts, fmt.Sprintf("%s %s→%s", icon, info.Name, string(e.Role)))
			}
			chainInfo = strings.Join(parts, " / ")
		}
	}

	opts := ui.BannerOptions{
		Version:       Version,
		ModelName:     cfg.Model,
		ModelTier:     tier,
		ContextWindow: cfg.ContextWindow,
		MaxTokens:     cfg.MaxTokens,
		MemoryGB:      getMemoryGB(),
		AutoApprove:   cfg.AutoApprove,
		Provider:      cfg.Provider,
		EngineHost:    hostDisplay,
		CWD:           cwd,
		ChainInfo:     chainInfo,
	}
	terminal.ShowBanner(opts)
}

func resumeSession(ctx context.Context, sess *session.Session, persistenceMgr *session.PersistenceManager, resumeFlag string, cfg *config.Config) {
	terminal := ui.NewTerminal()

	var sessionID string
	if resumeFlag == "last" {
		lastID := getLastSessionID(persistenceMgr)
		if lastID == "" {
			terminal.PrintColored(ui.ColorYellow, "直近のセッションが見つかりません\n")
			return
		}
		sessionID = lastID
	} else if resumeFlag == "list" {
		// --resume list でセッション一覧を表示
		sessions, err := persistenceMgr.ListSessions()
		if err != nil {
			terminal.PrintColored(ui.ColorRed, fmt.Sprintf("セッション一覧エラー: %v\n", err))
			return
		}
		terminal.PrintColored(ui.ColorCyan, "═══ セッション一覧 ═══\n")
		for i, sessID := range sessions {
			terminal.Printf("%3d. %s\n", i+1, sessID)
		}
		if len(sessions) == 0 {
			terminal.Println("  セッションが見つかりません")
		}
		terminal.Println("\n使用例: ./vibe --resume <session-id>")
		return
	} else {
		sessionID = resumeFlag
	}

	loadedSess, err := persistenceMgr.LoadSession(sessionID)
	if err != nil {
		terminal.PrintColored(ui.ColorRed, fmt.Sprintf("セッション復旧エラー: %v\n", err))
		return
	}

	// Copy from loaded session
	sess.SetID(loadedSess.GetID())
	sess.SetSystemPrompt(loadedSess.SystemPrompt)
	for _, msg := range loadedSess.Messages {
		if msg.Role == session.RoleUser {
			sess.AddUserMessage(msg.Content)
		} else if msg.Role == session.RoleAssistant {
			if len(msg.ToolCalls) > 0 {
				sess.AddToolCall(msg.ToolCalls)
			} else {
				sess.AddAssistantMessage(msg.Content)
			}
		} else if msg.Role == session.RoleTool {
			result := session.ToolResult{
				Content:    msg.Content,
				ToolCallID: msg.ToolID,
			}
			sess.AddToolResults([]session.ToolResult{result})
		}
	}

	terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ セッション '%s' を復旧しました\n", sessionID))
}

func runAgent(ctx context.Context, agt *agent.Agent, cfg *config.Config, terminal *ui.Terminal, shutdownMgr *ShutdownManager, cmdHandler *ui.CommandHandler) {
	// One-shot mode
	if flagPrompt != "" {
		runOneShot(ctx, agt, flagPrompt, terminal)
		shutdownMgr.Shutdown("one-shot complete")
		return
	}

	// Interactive mode
	terminal.ShowWelcome(Version)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// コンテキスト使用率を計算してプロンプトに表示
			contextUsagePct := agt.GetContextUsagePercent()
			prompt := ui.FormatPrompt(contextUsagePct)

			input, err := terminal.ReadLine(prompt)
			if err != nil {
				if err == io.EOF {
					shutdownMgr.Shutdown("EOF")
					return
				}
				terminal.PrintColored(ui.ColorRed, fmt.Sprintf("入力エラー: %v\n", err))
				continue
			}

			if input == "" {
				continue
			}

			// 履歴に追加（メインループの入力のみ）
			terminal.GetLineEditor().AddHistory(input)

			// Check for slash commands
			if isCmd, _ := cmdHandler.Execute(input); isCmd {
				// Check if it was an exit command
				if input == "/exit" || input == "/quit" || input == "/q" {
					shutdownMgr.Shutdown("user request")
					return
				}
				continue
			}

			// Run agent
			err = agt.Run(ctx, input)
			if err != nil {
				terminal.PrintColored(ui.ColorRed, fmt.Sprintf("エージェントエラー: %v\n", err))
			}
		}
	}
}

func runOneShot(ctx context.Context, agt *agent.Agent, prompt string, terminal *ui.Terminal) {
	err := agt.Run(ctx, prompt)
	if err != nil {
		terminal.PrintColored(ui.ColorRed, fmt.Sprintf("エージェントエラー: %v\n", err))
		os.Exit(1)
	}
}

func setupSignalHandler(shutdownMgr *ShutdownManager) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		sigName := ""
		switch sig {
		case syscall.SIGINT:
			sigName = "SIGINT"
		case syscall.SIGTERM:
			sigName = "SIGTERM"
		}
		shutdownMgr.Shutdown(sigName)
	}()
}

func showVersion() {
	fmt.Printf("vibe-local-go v%s\n", Version)
	fmt.Printf("Go %s (%s/%s)\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

func listSessions(cfg *config.Config) {
	terminal := ui.NewTerminal()
	persistenceMgr, err := session.NewPersistenceManager(getSessionDir())
	if err != nil {
		terminal.PrintColored(ui.ColorRed, fmt.Sprintf("パーシスタンスマネージャー作成エラー: %v\n", err))
		os.Exit(1)
	}

	sessions, err := persistenceMgr.ListSessions()
	if err != nil {
		terminal.PrintColored(ui.ColorRed, fmt.Sprintf("セッション一覧エラー: %v\n", err))
		os.Exit(1)
	}

	terminal.PrintColored(ui.ColorCyan, "═══ セッション一覧 ═══\n")
	for i, sessID := range sessions {
		terminal.Printf("%3d. %s\n", i+1, sessID)
	}
	if len(sessions) == 0 {
		terminal.Println("  セッションが見つかりません")
	}
}

// Helper functions

func getSessionDir() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(homeDir, ".config", "vibe-local")
}

func generateSessionID() string {
	return fmt.Sprintf("sess_%d", time.Now().Unix())
}

func getLastSessionID(persistenceMgr *session.PersistenceManager) string {
	sessions, err := persistenceMgr.ListSessions()
	if err != nil || len(sessions) == 0 {
		return ""
	}
	return sessions[len(sessions)-1]
}

func getMemoryGB() float64 {
	switch runtime.GOOS {
	case "darwin":
		// macOS: sysctl hw.memsize
		out, err := execCommand("sysctl", "-n", "hw.memsize")
		if err == nil {
			var bytes uint64
			if _, err := fmt.Sscanf(strings.TrimSpace(out), "%d", &bytes); err == nil {
				return float64(bytes) / (1024 * 1024 * 1024)
			}
		}
		return 16.0
	case "linux":
		// Linux: /proc/meminfo
		data, err := os.ReadFile("/proc/meminfo")
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(line, "MemTotal:") {
					var kb uint64
					if _, err := fmt.Sscanf(line, "MemTotal: %d kB", &kb); err == nil {
						return float64(kb) / (1024 * 1024)
					}
				}
			}
		}
		return 16.0
	default:
		return 16.0
	}
}

// pullOllamaModelWithProgress プログレスバー付きでOllamaモデルをダウンロード
func pullOllamaModelWithProgress(ctx context.Context, provider *llm.OllamaProvider, modelName string, terminal *ui.Terminal) error {
	lastStatus := ""
	wasProgress := false // 前回がプログレスバー表示だったか
	return provider.PullModelWithProgress(ctx, modelName, func(status string, completed, total int64) {
		// total > 0 のレイヤーダウンロード中（"pulling <digest>"）はプログレスバー表示
		// "pulling manifest" は total=0 なのでここには入らない
		if total > 0 {
			pct := float64(completed) / float64(total) * 100
			if pct > 100 {
				pct = 100
			}
			barWidth := 30
			filled := int(pct / 100 * float64(barWidth))
			if filled > barWidth {
				filled = barWidth
			}
			bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

			// サイズ表示（MB/GB）
			completedMB := float64(completed) / (1024 * 1024)
			totalMB := float64(total) / (1024 * 1024)
			sizeUnit := "MB"
			if totalMB >= 1024 {
				completedMB /= 1024
				totalMB /= 1024
				sizeUnit = "GB"
			}
			fmt.Printf("\r  %s %5.1f%% [%.1f/%.1f %s]", bar, pct, completedMB, totalMB, sizeUnit)
			wasProgress = true
		} else if status != lastStatus {
			// ステータス変化時のみ表示（manifest取得、SHA検証、書き込み等）
			if wasProgress {
				fmt.Println() // プログレスバー行の後に改行
				wasProgress = false
			}
			terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("  %s\n", status))
		}
		lastStatus = status
	})
}

// execCommand はコマンドを実行して標準出力を返す
func execCommand(name string, args ...string) (string, error) {
	cmd := execPackage.Command(name, args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// checkAndPullOllamaModel Ollamaにモデルが存在するか確認し、なければ pull を提案する
// セットアップ・編集時の手動入力後に呼ばれる共通関数
// 戻り値: 最終的に使用するモデル名（pullしたモデル or 元のモデル）
func checkAndPullOllamaModel(host, model string, terminal *ui.Terminal) string {
	// モデルリストを取得して存在チェック
	models, err := llm.FetchLocalProviderModels(host, "ollama")
	if err != nil {
		// Ollama に接続できない場合はチェックをスキップ
		terminal.PrintColored(ui.ColorYellow, fmt.Sprintf("⚠ モデル存在チェックをスキップ（Ollama接続エラー: %v）\n", err))
		terminal.PrintColored(ui.ColorYellow, "  起動時に再度確認されます\n")
		return model
	}

	// モデルが既に存在するか確認
	for _, m := range models {
		if m == model {
			return model
		}
	}

	// モデルが見つからない場合
	terminal.Print("\n")
	terminal.PrintColored(ui.ColorYellow, fmt.Sprintf("⚠ モデル '%s' はOllamaにまだダウンロードされていません\n", model))
	terminal.Println("選択肢:")
	terminal.Println("  1. 今すぐダウンロード (ollama pull)")
	terminal.Println("  2. そのまま設定を保存（後で手動ダウンロード）")
	if len(models) > 0 {
		terminal.Println("  3. 既存のモデルから選び直す")
	}

	maxChoice := 2
	if len(models) > 0 {
		maxChoice = 3
	}
	choice, err := terminal.ReadLine(fmt.Sprintf("選択 [1-%d]: ", maxChoice))
	if err != nil {
		return model
	}

	switch strings.TrimSpace(choice) {
	case "1":
		terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("モデル '%s' をダウンロード中（サイズによって数分〜数十分かかります）...\n", model))
		tmpProvider := llm.NewOllamaProvider(host, model)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if pullErr := pullOllamaModelWithProgress(ctx, tmpProvider, model, terminal); pullErr != nil {
			terminal.PrintColored(ui.ColorRed, fmt.Sprintf("\nダウンロードエラー: %v\n", pullErr))
			terminal.PrintColored(ui.ColorYellow, "後で以下のコマンドで手動ダウンロードしてください:\n")
			terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("  ollama pull %s\n", model))
		} else {
			terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("\n✓ モデル '%s' のダウンロード完了\n", model))
		}
		return model

	case "3":
		if len(models) > 0 {
			terminal.Printf("\n利用可能なモデル (%d件):\n", len(models))
			for i, m := range models {
				terminal.Printf("  %2d. %s\n", i+1, m)
			}
			choiceStr, readErr := terminal.ReadLine(fmt.Sprintf("選択 [1-%d]: ", len(models)))
			if readErr == nil {
				var num int
				if _, scanErr := fmt.Sscanf(choiceStr, "%d", &num); scanErr == nil && num >= 1 && num <= len(models) {
					terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("✓ モデル '%s' を選択\n", models[num-1]))
					return models[num-1]
				}
			}
			terminal.PrintColored(ui.ColorYellow, "無効な選択です。元のモデル名を維持します\n")
			return model
		}
		// models が空の場合はフォールスルー
		fallthrough

	default:
		// そのまま保存
		terminal.PrintColored(ui.ColorYellow, "後で以下のコマンドで手動ダウンロードしてください:\n")
		terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("  ollama pull %s\n", model))
		return model
	}
}

// registerSkillCommands スキル関連のスラッシュコマンドを登録
func registerSkillCommands(cmdHandler *ui.CommandHandler, terminal *ui.Terminal, skillMgr *skill.SkillManager) {
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "skills",
		Description: "利用可能なスキル一覧",
		Handler: func(args string) error {
			skills := skillMgr.GetSkills()

			if len(skills) == 0 {
				terminal.PrintColored(ui.ColorYellow, "スキルが見つかりません\n\n")
				terminal.Printf("スキルの配置場所:\n")
				terminal.Printf("  グローバル: %s\n", skillMgr.GlobalDir())
				terminal.Printf("  プロジェクト: %s\n\n", skillMgr.ProjectDir())
				terminal.Printf("スキルの作成方法:\n")
				terminal.Printf("  1. 上記ディレクトリにフォルダを作成\n")
				terminal.Printf("  2. フォルダ内に SKILL.md を配置\n")
				terminal.Printf("  3. YAML frontmatter で name と description を定義\n")
				return nil
			}

			terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("━━ Skills (%d件) ━━━━━━━━━━━━━━━━━━━━\n", len(skills)))

			for _, s := range skills {
				sourceLabel := "global"
				if s.Source == skill.SourceProject {
					sourceLabel = "project"
				}
				terminal.Printf("  %-20s [%s]\n", s.Name, sourceLabel)
				if s.Description != "" {
					terminal.PrintColored(ui.ColorGray, fmt.Sprintf("    %s\n", s.Description))
				}
				terminal.PrintColored(ui.ColorGray, fmt.Sprintf("    → %s\n", s.SkillFile))
			}

			terminal.PrintColored(ui.ColorCyan, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			return nil
		},
	})
}

// registerMCPCommands MCP関連のスラッシュコマンドを登録
func registerMCPCommands(cmdHandler *ui.CommandHandler, terminal *ui.Terminal, mcpMgr *mcp.Manager) {
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "mcp",
		Description: "MCPサーバー接続状況・ツール一覧",
		Handler: func(args string) error {
			serverNames := mcpMgr.GetServerNames()

			if len(serverNames) == 0 {
				terminal.PrintColored(ui.ColorYellow, "MCPサーバーが設定されていません\n\n")
				terminal.Printf("設定ファイルの配置場所:\n")
				homeDir, _ := os.UserHomeDir()
				terminal.Printf("  グローバル: %s/.config/vibe-local-go/mcp.json\n", homeDir)
				terminal.Printf("  プロジェクト: .vibe-local/mcp.json\n\n")
				terminal.Printf("設定例:\n")
				terminal.PrintColored(ui.ColorGray, "  {\n")
				terminal.PrintColored(ui.ColorGray, "    \"mcpServers\": {\n")
				terminal.PrintColored(ui.ColorGray, "      \"filesystem\": {\n")
				terminal.PrintColored(ui.ColorGray, "        \"command\": \"npx\",\n")
				terminal.PrintColored(ui.ColorGray, "        \"args\": [\"-y\", \"@modelcontextprotocol/server-filesystem\", \"/tmp\"]\n")
				terminal.PrintColored(ui.ColorGray, "      }\n")
				terminal.PrintColored(ui.ColorGray, "    }\n")
				terminal.PrintColored(ui.ColorGray, "  }\n")
				return nil
			}

			terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("━━ MCP Servers (%d/%d 稼働) ━━━━━━━━━━━━\n",
				mcpMgr.RunningCount(), len(serverNames)))

			allTools := mcpMgr.GetAllTools()
			for _, name := range serverNames {
				status := "✗ 停止"
				statusColor := ui.ColorRed
				if mcpMgr.IsRunning(name) {
					status = "✓ 稼働"
					statusColor = ui.ColorGreen
				}
				terminal.Printf("  ")
				terminal.PrintColored(statusColor, status)
				terminal.Printf(" %s\n", name)

				if tools, ok := allTools[name]; ok {
					for _, t := range tools {
						terminal.PrintColored(ui.ColorGray, fmt.Sprintf("    → mcp_%s_%s", name, t.Name))
						if t.Description != "" {
							terminal.PrintColored(ui.ColorGray, fmt.Sprintf(": %s", t.Description))
						}
						terminal.Println("")
					}
				}
			}

			terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("━━ 合計 %d ツール ━━━━━━━━━━━━━━━━━━━\n", mcpMgr.TotalToolCount()))
			return nil
		},
	})
}

// registerAutoTestCommands AutoTest関連のスラッシュコマンドを登録
func registerAutoTestCommands(cmdHandler *ui.CommandHandler, terminal *ui.Terminal, agt *agent.Agent) {
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "autotest",
		Description: "ファイル編集後の自動テスト実行 [on|off]",
		Handler: func(args string) error {
			args = strings.TrimSpace(args)

			if args == "" {
				// 現在の状態を表示
				status := "OFF"
				if agt.IsAutoTestEnabled() {
					status = "ON"
				}
				terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("Auto Test: %s\n", status))
				terminal.Println("  使用方法: /autotest [on|off]")
				return nil
			}

			switch strings.ToLower(args) {
			case "on":
				agt.SetAutoTestEnabled(true)
				terminal.PrintColored(ui.ColorGreen, "✓ Auto Test: ON (ファイル編集後に自動でテストを実行します)\n")
				return nil
			case "off":
				agt.SetAutoTestEnabled(false)
				terminal.PrintColored(ui.ColorYellow, "✗ Auto Test: OFF\n")
				return nil
			default:
				terminal.PrintError(fmt.Sprintf("不正な引数: %s\n  使用方法: /autotest [on|off]", args))
				return nil
			}
		},
	})
}

// registerPlanCommands Plan関連のスラッシュコマンドを登録
func registerPlanCommands(cmdHandler *ui.CommandHandler, terminal *ui.Terminal, agt *agent.Agent) {
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "plan",
		Description: "計画モード [on|off] - 計画立案時は書込み操作を禁止",
		Handler: func(args string) error {
			args = strings.TrimSpace(args)

			if args == "" {
				// 現在の状態を表示
				status := "OFF"
				if agt.IsPlanMode() {
					status = "ON"
					terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("Plan Mode: %s\n", status))
					terminal.Println("  ✓ read_file, glob, grep は許可")
					terminal.Println("  ✗ write_file, edit_file, bash は禁止")
					terminal.PrintInfo("計画を確認したら '/plan off' で実行モードに切り替えてください")
					return nil
				}
				terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("Plan Mode: %s\n", status))
				terminal.Println("  使用方法: /plan [on|off]")
				return nil
			}

			switch strings.ToLower(args) {
			case "on":
				agt.SetPlanMode(true)
				terminal.PrintColored(ui.ColorYellow, "🔒 Plan Mode: ON\n")
				terminal.PrintInfo("write_file, edit_file, bash は実行できません")
				terminal.PrintInfo("計画が完成したら '/plan off' で実行モードに切り替えてください")
				return nil
			case "off":
				agt.SetPlanMode(false)
				terminal.PrintColored(ui.ColorGreen, "✓ Plan Mode: OFF (実行モード)\n")
				terminal.PrintInfo("すべてのツールが実行可能です")
				return nil
			default:
				terminal.PrintError(fmt.Sprintf("不正な引数: %s\n  使用方法: /plan [on|off]", args))
				return nil
			}
		},
	})
}

// registerProvidersStatusCommand プロバイダー状態確認コマンドを登録（T-8503）
func registerProvidersStatusCommand(cmdHandler *ui.CommandHandler, terminal *ui.Terminal, provider llm.LLMProvider) {
	cmdHandler.Register(&ui.SlashCommand{
		Name:        "providers",
		Description: "登録済みプロバイダーの接続状況と一覧を表示",
		Handler: func(args string) error {
			terminal.PrintColored(ui.ColorCyan, "━━ Providers ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

			// ProviderChain の場合は全エントリを表示
			if chain, ok := provider.(*llm.ProviderChain); ok {
				entries := chain.GetEntries()
				currentProvider := chain.GetCurrentProvider()
				currentInfo := currentProvider.Info()

				for i, e := range entries {
					info := e.Provider.Info()
					icon := ui.ProviderIcon(info.Name)

					// アクティブなプロバイダーをハイライト
					isActive := info.Name == currentInfo.Name && info.BaseURL == currentInfo.BaseURL
					marker := "  "
					if isActive {
						marker = "▶ "
					}

					// 接続チェック
					ctx := context.Background()
					status := "✅"
					statusMsg := "接続OK"
					if err := e.Provider.CheckHealth(ctx); err != nil {
						status = "❌"
						statusMsg = "接続不可"
					}

					// 失敗回数
					failCount := chain.GetFailureCount(i)
					failInfo := ""
					if failCount > 0 {
						failInfo = fmt.Sprintf(" (失敗: %dx)", failCount)
					}

					// ロール表示
					roleStr := string(e.Role)
					typeStr := string(info.Type)

					terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("%s%s %s", marker, icon, info.Name))
					terminal.Printf(" [%s/%s] %s %s%s\n", roleStr, typeStr, status, statusMsg, failInfo)
					terminal.PrintColored(ui.ColorGray, fmt.Sprintf("     Model: %s\n", info.Model))
					terminal.PrintColored(ui.ColorGray, fmt.Sprintf("     URL:   %s\n", info.BaseURL))
				}

				// フォールバック状態
				terminal.PrintColored(ui.ColorGray, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
				terminal.Printf("  現在のプロバイダー: %s %s (%s)\n",
					ui.ProviderIcon(currentInfo.Name), currentInfo.Name, currentInfo.Model)

			} else {
				// 単一プロバイダーの場合
				info := provider.Info()
				icon := ui.ProviderIcon(info.Name)

				ctx := context.Background()
				status := "✅ 接続OK"
				if err := provider.CheckHealth(ctx); err != nil {
					status = fmt.Sprintf("❌ 接続不可: %v", err)
				}

				terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("▶ %s %s\n", icon, info.Name))
				terminal.Printf("  Model:  %s\n", info.Model)
				terminal.Printf("  URL:    %s\n", info.BaseURL)
				terminal.Printf("  Status: %s\n", status)
			}

			terminal.PrintColored(ui.ColorGray, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			terminal.PrintColored(ui.ColorGray, "  /provider でプロバイダーを管理できます\n")
			return nil
		},
	})
}

// registerWatchCommands はファイル監視関連のスラッシュコマンドを登録する（T-14203）
func registerWatchCommands(cmdHandler *ui.CommandHandler, terminal *ui.Terminal, agt *agent.Agent) {
	var fw *watcher.FileWatcher
	var injector *watcher.Injector

	cmdHandler.Register(&ui.SlashCommand{
		Name:        "watch",
		Description: "ファイル監視（/watch *.go で開始, /watch off で停止）",
		Handler: func(args string) error {
			args = strings.TrimSpace(args)

			// /watch — 状態表示
			if args == "" {
				if fw == nil || !fw.IsRunning() {
					terminal.PrintColored(ui.ColorYellow, "ファイル監視: OFF\n")
					terminal.Printf("  使い方: /watch *.go  — 監視開始\n")
				} else {
					terminal.PrintColored(ui.ColorGreen, "ファイル監視: ON\n")
					terminal.Printf("  パターン: %s\n", strings.Join(fw.Patterns(), ", "))
					terminal.Printf("  監視ファイル数: %d\n", fw.WatchedFileCount())
				}
				return nil
			}

			// /watch off — 停止
			if args == "off" || args == "stop" {
				if fw != nil && fw.IsRunning() {
					fw.Stop()
					terminal.PrintColored(ui.ColorYellow, "ファイル監視を停止しました\n")
				} else {
					terminal.PrintColored(ui.ColorYellow, "ファイル監視は動作していません\n")
				}
				return nil
			}

			// /watch <patterns> — 開始
			// 既存の watcher があれば停止
			if fw != nil && fw.IsRunning() {
				fw.Stop()
			}

			// 作業ディレクトリを取得
			cwd, err := os.Getwd()
			if err != nil {
				terminal.PrintColored(ui.ColorRed, fmt.Sprintf("エラー: %v\n", err))
				return nil
			}

			fw = watcher.NewFileWatcher(cwd)
			injector = watcher.NewInjector(agt.GetSession())

			patterns := strings.Fields(args)
			if err := fw.Start(patterns); err != nil {
				terminal.PrintColored(ui.ColorRed, fmt.Sprintf("監視開始エラー: %v\n", err))
				return nil
			}

			terminal.PrintColored(ui.ColorGreen, fmt.Sprintf("ファイル監視を開始しました: %s\n", strings.Join(patterns, ", ")))
			terminal.Printf("  監視ファイル数: %d\n", fw.WatchedFileCount())

			// イベントリスナー goroutine
			go func() {
				for events := range fw.Events() {
					if len(events) > 0 {
						terminal.PrintColored(ui.ColorCyan, fmt.Sprintf("\n[Watch] %d ファイルが変更されました\n", len(events)))
						for _, ev := range events {
							terminal.Printf("  %s: %s\n", ev.EventType, ev.Path)
						}
						injector.InjectChanges(events)
					}
				}
			}()

			return nil
		},
	})
}
