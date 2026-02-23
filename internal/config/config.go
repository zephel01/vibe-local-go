package config

import "os"

// Default configuration values
const (
	DefaultOllamaHost    = "http://localhost:11434"
	DefaultMaxTokens     = 8192
	DefaultTemperature  = 0.7
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

	// Ollama settings
	OllamaHost string

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
		MaxTokens:     DefaultMaxTokens,
		Temperature:   DefaultTemperature,
		ContextWindow: DefaultContextWindow,
		OllamaHost:    DefaultOllamaHost,
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
		return "qwen2.5-72b-instruct" // Tier A
	case memoryGB >= 96:
		return "llama3.1-70b-instruct" // Tier B
	case memoryGB >= 32:
		return "qwen2.5-32b-instruct" // Tier C 上位
	case memoryGB >= 16:
		return "llama3.1-8b-instruct" // Tier C 中位
	case memoryGB >= 8:
		return "llama3.2-3b-instruct" // Tier D
	default:
		return "qwen2.5-1.5b-instruct" // Tier E
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
