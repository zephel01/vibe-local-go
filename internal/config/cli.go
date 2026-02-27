package config

import (
	"flag"
	"fmt"
	"os"
	"strconv"
)

// ParseCLI parses command-line arguments and updates the config
func (c *Config) ParseCLI() {
	flag.StringVar(&c.Model, "model", "", "Specify Ollama model name")
	flag.StringVar(&c.Model, "m", "", "Short for --model")
	flag.StringVar(&c.SidecarModel, "sidecar", "", "Specify sidecar model name")
	flag.StringVar(&c.OllamaHost, "host", DefaultOllamaHost, "Ollama API endpoint URL")
	flag.StringVar(&c.OllamaHost, "ollama-host", DefaultOllamaHost, "Alias for --host")
	flag.IntVar(&c.MaxTokens, "max-tokens", DefaultMaxTokens, "Max output tokens per response")
	flag.Float64Var(&c.Temperature, "temperature", DefaultTemperature, "Sampling temperature (0.0-1.0)")
	flag.IntVar(&c.ContextWindow, "context-window", DefaultContextWindow, "Context window size in tokens")
	flag.StringVar(&c.Prompt, "prompt", "", "One-shot prompt (non-interactive)")
	flag.StringVar(&c.Prompt, "p", "", "Short for --prompt")
	flag.BoolVar(&c.AutoApprove, "yes", false, "Auto-approve all tool calls")
	flag.BoolVar(&c.AutoApprove, "y", false, "Short for --yes")
	flag.BoolVar(&c.ResumeLast, "resume", false, "Resume last session")
	flag.StringVar(&c.SessionID, "session-id", "", "Resume a specific session by ID")
	flag.BoolVar(&c.ListSessions, "list-sessions", false, "List all saved sessions")
	flag.BoolVar(&c.Debug, "debug", false, "Enable debug logging")
	flag.BoolVar(&c.AutoApprove, "dangerously-skip-permissions", false, "Alias for -y (compatibility)")
	flag.BoolVar(&c.Debug, "d", false, "Short for --debug")

	flag.Parse()

	// If user specified model manually, disable auto-selection
	if c.Model != "" || c.SidecarModel != "" {
		c.AutoModel = false
	}

	// Handle legacy debug env var
	if os.Getenv("VIBE_LOCAL_DEBUG") == "1" || os.Getenv("VIBE_CODER_DEBUG") == "1" {
		c.Debug = true
	}
}

// ShowVersion displays version information
func ShowVersion() {
	fmt.Println("vibe-local-go v1.0.0")
	os.Exit(0)
}

// ParseEnv parses environment variables and updates the config
func (c *Config) ParseEnv() {
	// Environment variables override config file defaults but CLI args take priority
	if v := os.Getenv("VIBE_CODER_MODEL"); v != "" {
		c.Model = v
		c.AutoModel = false
	}
	if v := os.Getenv("VIBE_LOCAL_MODEL"); v != "" && c.Model == "" {
		c.Model = v
		c.AutoModel = false
	}
	if v := os.Getenv("VIBE_CODER_SIDECAR"); v != "" {
		c.SidecarModel = v
	}
	if v := os.Getenv("VIBE_LOCAL_SIDECAR_MODEL"); v != "" && c.SidecarModel == "" {
		c.SidecarModel = v
	}
	if v := os.Getenv("OLLAMA_HOST"); v != "" {
		c.OllamaHost = v
	}
	if v := os.Getenv("VIBE_CODER_MAX_TOKENS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.MaxTokens = n
		}
	}
	if v := os.Getenv("VIBE_CODER_TEMPERATURE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			c.Temperature = f
		}
	}
	if v := os.Getenv("VIBE_CODER_CONTEXT_WINDOW"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.ContextWindow = n
		}
	}

	// Ollama options from environment variables
	if v := os.Getenv("OLLAMA_NUM_CTX"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.OllamaNumCtx = n
		}
	}
	if v := os.Getenv("OLLAMA_NUM_GPU"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.OllamaNumGPU = n
		}
	}
}
