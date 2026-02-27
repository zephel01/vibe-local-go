package config

import "os"

// Default configuration values
const (
	DefaultOllamaHost    = "http://localhost:11434"
	DefaultMaxTokens     = 8192
	DefaultTemperature  = 0.2
	DefaultContextWindow = 32768
)

// Model tiers based on available RAM
const (
	TierS = "S" // 768GB+ - Frontier models (671b)
	TierA = "A" // 256GB+ - Expert models (235b)
	TierB = "B" // 96GB+  - Advanced models (70b)
	TierC = "C" // 16GB+  - Solid models (30b)
	TierD = "D" // 8GB+   - Light models (8b)
	TierE = "E" // 4GB+   - Minimal models (1.7b)
)

// Config holds all configuration for the agent
type Config struct {
	// Model settings
	Model        string
	SidecarModel string
	AutoModel    bool // true = auto-select based on RAM

	// LLM settings
	MaxTokens     int
	Temperature   float64
	ContextWindow int

	// Provider selection
	Provider string // "ollama" (default), "openrouter", "openai", "anthropic", "google", etc.

	// Ollama settings
	OllamaHost    string
	OllamaNumCtx  int // Ollama num_ctx override (0 = use Ollama default)
	OllamaNumGPU  int // Ollama num_gpu override (-1 = not set, 0+ = explicit)

	// Cloud provider API keys (provider key → API key)
	CloudAPIKeys map[string]string


	// Session settings
	SessionID     string
	ResumeLast    bool
	ListSessions  bool

	// One-shot mode
	Prompt string

	// Auto-approve mode
	AutoApprove bool

	// Debug mode
	Debug bool

	// Sandbox mode — ファイル書き込みをステージングディレクトリで行う
	SandboxMode bool

	// AutoVenv — Python実行時に自動で.venvを作成・activateする
	AutoVenv bool
	// VenvDir — 仮想環境のディレクトリ名（デフォルト: .venv）
	VenvDir string

	// Prompt hints
	IncludePythonHints bool // Python venv instructions をシステムプロンプトに含めるか

	// Platform-specific
	OS   string // "darwin", "linux", "windows"
	Arch string // "amd64", "arm64"

	// OS hints (injected into system prompt)
	OSHints []string
}

// DefaultConfig returns a configuration with default values
func DefaultConfig() *Config {
	return &Config{
		Model:         "",
		SidecarModel:  "",
		AutoModel:     true,
		Provider:      "ollama",
		MaxTokens:     DefaultMaxTokens,
		Temperature:   DefaultTemperature,
		ContextWindow: DefaultContextWindow,
		OllamaHost:    DefaultOllamaHost,
		OllamaNumCtx:  0,
		OllamaNumGPU:  -1, // -1 = not set
		CloudAPIKeys:  make(map[string]string),
		VenvDir:       ".venv",
		OS:            detectOS(),
		Arch:          detectArch(),
	}
}

func detectOS() string {
	return os.Getenv("GOOS")
}

func detectArch() string {
	return os.Getenv("GOARCH")
}

// RecommendModel 推奨モデルを返す
func RecommendModel(memoryGB float64) string {
	switch {
	case memoryGB >= 256:
		return "qwen3:72b" // Tier A
	case memoryGB >= 96:
		return "qwen3:32b" // Tier B
	case memoryGB >= 32:
		return "qwen3-coder:30b" // Tier C 上位
	case memoryGB >= 16:
		return "qwen3:8b" // Tier C 中位
	case memoryGB >= 8:
		return "qwen3:4b" // Tier D
	default:
		return "qwen3:1.7b" // Tier E
	}
}

// GetTierFromRAM RAMからティアを取得
func GetTierFromRAM(memoryGB float64) string {
	switch {
	case memoryGB >= 256:
		return TierA
	case memoryGB >= 96:
		return TierB
	case memoryGB >= 16:
		return TierC
	case memoryGB >= 8:
		return TierD
	default:
		return TierE
	}
}
