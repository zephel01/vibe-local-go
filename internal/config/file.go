package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ProviderProfile プロバイダー固有の設定プロファイル
type ProviderProfile struct {
	Type        string  `json:"type"`                  // "ollama", "openrouter", "openai", "anthropic", "google"
	Host        string  `json:"host,omitempty"`        // ベースURL（Ollama等）
	APIKey      string  `json:"api_key,omitempty"`     // クラウドプロバイダー用APIキー
	Model       string  `json:"model,omitempty"`       // デフォルトモデル名
	MaxTokens   int     `json:"max_tokens,omitempty"`  // プロバイダー固有のmax_tokens
	Temperature float64 `json:"temperature,omitempty"` // プロバイダー固有のtemperature
}

// ConfigFile represents the JSON config file structure
type ConfigFile struct {
	// 既存フィールド（後方互換）
	Model         string  `json:"MODEL,omitempty"`
	SidecarModel  string  `json:"SIDECAR_MODEL,omitempty"`
	OllamaHost    string  `json:"OLLAMA_HOST,omitempty"`
	MaxTokens     int     `json:"MAX_TOKENS,omitempty"`
	Temperature   float64 `json:"TEMPERATURE,omitempty"`
	ContextWindow int     `json:"CONTEXT_WINDOW,omitempty"`

	// Ollama options
	OllamaNumCtx int `json:"OLLAMA_NUM_CTX,omitempty"`
	OllamaNumGPU int `json:"OLLAMA_NUM_GPU,omitempty"`

	// マルチプロバイダー設定
	Provider  string                     `json:"PROVIDER,omitempty"`
	Providers map[string]ProviderProfile `json:"PROVIDERS,omitempty"`
}

// configFilePaths config.json の探索パス（優先順）
var configFilePaths = []string{
	"~/.config/vibe-local-go/config.json",
	"~/.config/vibe-local/config.json",
	"~/.config/vibe-coder/config.json",
	"~/.vibe-local.json",
	"~/.vibe-coder.json",
}

// defaultConfigPath デフォルトの保存先
const defaultConfigPath = "~/.config/vibe-local-go/config.json"

// ParseConfigFile reads and parses the config file
func (c *Config) ParseConfigFile() error {
	var lastErr error
	for _, configPath := range configFilePaths {
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

		c.applyConfigFile(&cf)

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

// applyConfigFile ConfigFile の値を Config に反映
func (c *Config) applyConfigFile(cf *ConfigFile) {
	// --- 既存フィールド（後方互換） ---
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
	if cf.OllamaNumCtx > 0 {
		c.OllamaNumCtx = cf.OllamaNumCtx
	}
	if cf.OllamaNumGPU > 0 {
		c.OllamaNumGPU = cf.OllamaNumGPU
	}

	// --- プロバイダー設定 ---
	if cf.Provider != "" {
		c.Provider = cf.Provider
	}

	// アクティブプロバイダーのプロファイルを適用
	if cf.Providers != nil {
		activeProvider := c.Provider
		if profile, ok := cf.Providers[activeProvider]; ok {
			c.applyProviderProfile(&profile)
		}
	}
}

// applyProviderProfile プロバイダープロファイルの値を Config に反映
func (c *Config) applyProviderProfile(p *ProviderProfile) {
	// モデル設定（全プロバイダー共通）
	if p.Model != "" {
		c.Model = p.Model
		c.AutoModel = false
	}

	// プロバイダー固有設定
	if p.Type == "ollama" {
		if p.Host != "" {
			c.OllamaHost = p.Host
		}
	} else {
		// クラウドプロバイダー: APIキーをmapに格納
		if p.APIKey != "" {
			if c.CloudAPIKeys == nil {
				c.CloudAPIKeys = make(map[string]string)
			}
			c.CloudAPIKeys[p.Type] = p.APIKey
		}
	}

	// プロバイダー固有のLLMパラメータ（グローバル設定より優先）
	if p.MaxTokens > 0 {
		c.MaxTokens = p.MaxTokens
	}
	if p.Temperature > 0 {
		c.Temperature = p.Temperature
	}
}

// SaveConfigFile 現在の設定を config.json に保存
func (c *Config) SaveConfigFile() error {
	savePath := expandPath(defaultConfigPath)

	// ディレクトリを作成
	dir := filepath.Dir(savePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// 既存ファイルを読み込んでマージ（存在する場合）
	var cf ConfigFile
	if data, err := os.ReadFile(savePath); err == nil {
		json.Unmarshal(data, &cf) // エラーは無視（新規作成扱い）
	}

	// Providers マップの初期化
	if cf.Providers == nil {
		cf.Providers = make(map[string]ProviderProfile)
	}

	// アクティブプロバイダーを設定
	cf.Provider = c.Provider

	// グローバル設定
	cf.MaxTokens = c.MaxTokens
	cf.Temperature = c.Temperature
	cf.ContextWindow = c.ContextWindow
	cf.OllamaNumCtx = c.OllamaNumCtx
	cf.OllamaNumGPU = c.OllamaNumGPU

	// プロバイダー別プロファイルを更新
	profile := cf.Providers[c.Provider]
	profile.Type = c.Provider
	profile.Model = c.Model
	if c.Provider == "ollama" {
		profile.Host = c.OllamaHost
	} else if c.CloudAPIKeys != nil {
		if key, ok := c.CloudAPIKeys[c.Provider]; ok {
			profile.APIKey = key
		}
	}
	cf.Providers[c.Provider] = profile

	// 後方互換フィールドもセット
	cf.Model = c.Model
	cf.OllamaHost = c.OllamaHost

	// JSON書き出し（見やすいインデント付き）
	data, err := json.MarshalIndent(cf, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(savePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetConfigFilePath 現在使用中の設定ファイルパスを返す
func GetConfigFilePath() string {
	for _, configPath := range configFilePaths {
		expandedPath := expandPath(configPath)
		if _, err := os.Stat(expandedPath); err == nil {
			return expandedPath
		}
	}
	return expandPath(defaultConfigPath)
}

// GetProviderProfiles config.json からプロバイダー一覧を取得
func (c *Config) GetProviderProfiles() map[string]ProviderProfile {
	for _, configPath := range configFilePaths {
		expandedPath := expandPath(configPath)
		data, err := os.ReadFile(expandedPath)
		if err != nil {
			continue
		}

		var cf ConfigFile
		if err := json.Unmarshal(data, &cf); err != nil {
			continue
		}

		if cf.Providers != nil {
			return cf.Providers
		}
		break
	}
	return nil
}

// DeleteProviderProfile config.json からプロバイダープロファイルを削除
func (c *Config) DeleteProviderProfile(key string) error {
	savePath := expandPath(defaultConfigPath)

	data, err := os.ReadFile(savePath)
	if err != nil {
		return fmt.Errorf("config file not found: %w", err)
	}

	var cf ConfigFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	if cf.Providers == nil {
		return fmt.Errorf("no providers configured")
	}

	if _, ok := cf.Providers[key]; !ok {
		return fmt.Errorf("provider '%s' not found", key)
	}

	delete(cf.Providers, key)

	// アクティブプロバイダーが削除された場合、ollamaにフォールバック
	if cf.Provider == key {
		cf.Provider = "ollama"
	}

	out, err := json.MarshalIndent(cf, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(savePath, out, 0600)
}

// SaveProviderProfile 指定プロバイダーのプロファイルを config.json に保存（他を上書きしない）
func (c *Config) SaveProviderProfile(key string, profile ProviderProfile) error {
	savePath := expandPath(defaultConfigPath)

	dir := filepath.Dir(savePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	var cf ConfigFile
	if data, err := os.ReadFile(savePath); err == nil {
		json.Unmarshal(data, &cf)
	}

	if cf.Providers == nil {
		cf.Providers = make(map[string]ProviderProfile)
	}

	cf.Providers[key] = profile

	out, err := json.MarshalIndent(cf, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	return os.WriteFile(savePath, out, 0600)
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
