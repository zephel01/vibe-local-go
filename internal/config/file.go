package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ConfigFile represents the JSON config file structure
type ConfigFile struct {
	Model         string  `json:"MODEL"`
	SidecarModel  string  `json:"SIDECAR_MODEL"`
	OllamaHost    string  `json:"OLLAMA_HOST"`
	MaxTokens     int     `json:"MAX_TOKENS"`
	Temperature   float64 `json:"TEMPERATURE"`
	ContextWindow int     `json:"CONTEXT_WINDOW"`
}

// ParseConfigFile reads and parses the config file
func (c *Config) ParseConfigFile() error {
	// Try multiple config file locations
	configPaths := []string{
		"~/.config/vibe-local/config.json",
		"~/.config/vibe-coder/config.json",
		"~/.vibe-local.json",
		"~/.vibe-coder.json",
	}

	var lastErr error
	for _, configPath := range configPaths {
		expandedPath := expandPath(configPath)
		if _, err := os.Stat(expandedPath); os.IsNotExist(err) {
			continue
		}

		file, err := os.ReadFile(expandedPath)
		if err != nil {
			lastErr = err
			continue
		}

		var cf ConfigFile
		if err := json.Unmarshal(file, &cf); err != nil {
			lastErr = fmt.Errorf("failed to parse config file %s: %w", configPath, err)
			continue
		}

		// Apply config file values (only if not already set)
		if cf.Model != "" {
			c.Model = cf.Model
			c.AutoModel = false
		}
		if cf.SidecarModel != "" {
			c.SidecarModel = cf.SidecarModel
		}
		if cf.OllamaHost != "" {
			c.OllamaHost = cf.OllamaHost
		}
		if cf.MaxTokens > 0 {
			c.MaxTokens = cf.MaxTokens
		}
		if cf.Temperature > 0 {
			c.Temperature = cf.Temperature
		}
		if cf.ContextWindow > 0 {
			c.ContextWindow = cf.ContextWindow
		}

		if c.Debug {
			fmt.Printf("Loaded config from: %s\n", expandedPath)
		}
		return nil
	}

	if lastErr != nil {
		fmt.Printf("Warning: Could not read config file: %v\n", lastErr)
	}
	return nil
}

// expandPath expands ~ to user's home directory
func expandPath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}
