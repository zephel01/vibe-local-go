package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// テスト用: 一時ディレクトリに config.json を作成して ParseConfigFile を呼ぶヘルパー
func setupTestConfig(t *testing.T, content string) (*Config, string) {
	t.Helper()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	if err := os.WriteFile(configPath, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// configFilePaths をテスト用に一時的に上書き
	origPaths := configFilePaths
	configFilePaths = []string{configPath}
	t.Cleanup(func() { configFilePaths = origPaths })

	cfg := DefaultConfig()
	if err := cfg.ParseConfigFile(); err != nil {
		t.Fatalf("ParseConfigFile failed: %v", err)
	}

	return cfg, configPath
}

// --- 後方互換テスト: 旧フォーマット ---

func TestParseConfigFile_LegacyFormat(t *testing.T) {
	cfg, _ := setupTestConfig(t, `{
		"MODEL": "qwen3:8b",
		"OLLAMA_HOST": "http://192.168.1.100:11434",
		"MAX_TOKENS": 4096,
		"TEMPERATURE": 0.5,
		"CONTEXT_WINDOW": 16384
	}`)

	if cfg.Model != "qwen3:8b" {
		t.Errorf("Model = %q, want %q", cfg.Model, "qwen3:8b")
	}
	if cfg.OllamaHost != "http://192.168.1.100:11434" {
		t.Errorf("OllamaHost = %q, want %q", cfg.OllamaHost, "http://192.168.1.100:11434")
	}
	if cfg.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want %d", cfg.MaxTokens, 4096)
	}
	if cfg.Temperature != 0.5 {
		t.Errorf("Temperature = %f, want %f", cfg.Temperature, 0.5)
	}
	if cfg.ContextWindow != 16384 {
		t.Errorf("ContextWindow = %d, want %d", cfg.ContextWindow, 16384)
	}
	if cfg.AutoModel {
		t.Error("AutoModel should be false when MODEL is specified")
	}
	// Provider はデフォルトのまま
	if cfg.Provider != "ollama" {
		t.Errorf("Provider = %q, want %q", cfg.Provider, "ollama")
	}
}

// --- マルチプロバイダー: Ollama プロファイル ---

func TestParseConfigFile_OllamaProvider(t *testing.T) {
	cfg, _ := setupTestConfig(t, `{
		"PROVIDER": "ollama",
		"PROVIDERS": {
			"ollama": {
				"type": "ollama",
				"host": "http://10.0.0.5:11434",
				"model": "qwen3:32b",
				"max_tokens": 16384,
				"temperature": 0.3
			}
		}
	}`)

	if cfg.Provider != "ollama" {
		t.Errorf("Provider = %q, want %q", cfg.Provider, "ollama")
	}
	if cfg.OllamaHost != "http://10.0.0.5:11434" {
		t.Errorf("OllamaHost = %q, want %q", cfg.OllamaHost, "http://10.0.0.5:11434")
	}
	if cfg.Model != "qwen3:32b" {
		t.Errorf("Model = %q, want %q", cfg.Model, "qwen3:32b")
	}
	if cfg.MaxTokens != 16384 {
		t.Errorf("MaxTokens = %d, want %d", cfg.MaxTokens, 16384)
	}
	if cfg.Temperature != 0.3 {
		t.Errorf("Temperature = %f, want %f", cfg.Temperature, 0.3)
	}
	if cfg.AutoModel {
		t.Error("AutoModel should be false")
	}
}

// --- マルチプロバイダー: OpenRouter プロファイル ---

func TestParseConfigFile_OpenRouterProvider(t *testing.T) {
	cfg, _ := setupTestConfig(t, `{
		"PROVIDER": "openrouter",
		"PROVIDERS": {
			"ollama": {
				"type": "ollama",
				"host": "http://localhost:11434",
				"model": "qwen3:8b"
			},
			"openrouter": {
				"type": "openrouter",
				"api_key": "sk-or-v1-test-key-12345",
				"model": "google/gemini-2.5-flash",
				"max_tokens": 32768,
				"temperature": 0.5
			}
		}
	}`)

	if cfg.Provider != "openrouter" {
		t.Errorf("Provider = %q, want %q", cfg.Provider, "openrouter")
	}
	if cfg.CloudAPIKeys["openrouter"] != "sk-or-v1-test-key-12345" {
		t.Errorf("CloudAPIKeys[openrouter] = %q, want %q", cfg.CloudAPIKeys["openrouter"], "sk-or-v1-test-key-12345")
	}
	if cfg.Model != "google/gemini-2.5-flash" {
		t.Errorf("Model = %q, want %q", cfg.Model, "google/gemini-2.5-flash")
	}
	if cfg.MaxTokens != 32768 {
		t.Errorf("MaxTokens = %d, want %d", cfg.MaxTokens, 32768)
	}
	if cfg.Temperature != 0.5 {
		t.Errorf("Temperature = %f, want %f", cfg.Temperature, 0.5)
	}
	// Ollama のプロファイルは適用されないこと
	if cfg.OllamaHost != DefaultOllamaHost {
		t.Errorf("OllamaHost should remain default, got %q", cfg.OllamaHost)
	}
}

// --- プロバイダー個別トークン設定 ---

func TestParseConfigFile_PerProviderTokens(t *testing.T) {
	jsonData := `{
		"PROVIDER": "ollama",
		"MAX_TOKENS": 8192,
		"PROVIDERS": {
			"ollama": {
				"type": "ollama",
				"host": "http://localhost:11434",
				"model": "qwen3:8b",
				"max_tokens": 4096,
				"temperature": 0.7
			},
			"openrouter": {
				"type": "openrouter",
				"api_key": "sk-or-test",
				"model": "google/gemini-2.5-flash",
				"max_tokens": 32768,
				"temperature": 0.3
			}
		}
	}`

	// Ollama選択時
	cfg, _ := setupTestConfig(t, jsonData)
	if cfg.MaxTokens != 4096 {
		t.Errorf("[ollama] MaxTokens = %d, want 4096 (provider profile overrides global)", cfg.MaxTokens)
	}
	if cfg.Temperature != 0.7 {
		t.Errorf("[ollama] Temperature = %f, want 0.7", cfg.Temperature)
	}

	// OpenRouter選択時: PROVIDER を変えて再テスト
	jsonDataOR := `{
		"PROVIDER": "openrouter",
		"MAX_TOKENS": 8192,
		"PROVIDERS": {
			"ollama": {
				"type": "ollama",
				"host": "http://localhost:11434",
				"model": "qwen3:8b",
				"max_tokens": 4096,
				"temperature": 0.7
			},
			"openrouter": {
				"type": "openrouter",
				"api_key": "sk-or-test",
				"model": "google/gemini-2.5-flash",
				"max_tokens": 32768,
				"temperature": 0.3
			}
		}
	}`

	cfgOR, _ := setupTestConfig(t, jsonDataOR)
	if cfgOR.MaxTokens != 32768 {
		t.Errorf("[openrouter] MaxTokens = %d, want 32768 (provider profile overrides global)", cfgOR.MaxTokens)
	}
	if cfgOR.Temperature != 0.3 {
		t.Errorf("[openrouter] Temperature = %f, want 0.3", cfgOR.Temperature)
	}
}

// --- SaveConfigFile テスト ---

func TestSaveConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "config.json")

	// defaultConfigPath を一時的に上書き
	origDefault := defaultConfigPath
	// defaultConfigPath は const なので直接変更できない → SaveConfigFile を直接テストする代わりに
	// configFilePaths を上書きして GetConfigFilePath が正しく返すかテスト

	// 代替: Config を作成して手動で保存パスをテスト
	cfg := DefaultConfig()
	cfg.Provider = "openrouter"
	cfg.CloudAPIKeys["openrouter"] = "sk-or-v1-save-test"
	cfg.Model = "anthropic/claude-sonnet-4"
	cfg.MaxTokens = 16384
	cfg.Temperature = 0.4
	cfg.ContextWindow = 65536

	// 手動でファイルに書き出し（SaveConfigFile の内部ロジックを再現）
	var cf ConfigFile
	cf.Providers = make(map[string]ProviderProfile)
	cf.Provider = cfg.Provider
	cf.MaxTokens = cfg.MaxTokens
	cf.Temperature = cfg.Temperature
	cf.ContextWindow = cfg.ContextWindow
	cf.Model = cfg.Model
	cf.OllamaHost = cfg.OllamaHost

	profile := ProviderProfile{
		Type:   "openrouter",
		APIKey: cfg.CloudAPIKeys["openrouter"],
		Model:  cfg.Model,
	}
	cf.Providers["openrouter"] = profile

	data, err := json.MarshalIndent(cf, "", "    ")
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := os.WriteFile(savePath, data, 0600); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	// 保存したファイルを読み戻し
	origPaths := configFilePaths
	configFilePaths = []string{savePath}
	t.Cleanup(func() { configFilePaths = origPaths; _ = origDefault })

	loaded := DefaultConfig()
	if err := loaded.ParseConfigFile(); err != nil {
		t.Fatalf("ParseConfigFile failed: %v", err)
	}

	if loaded.Provider != "openrouter" {
		t.Errorf("Provider = %q, want %q", loaded.Provider, "openrouter")
	}
	if loaded.CloudAPIKeys["openrouter"] != "sk-or-v1-save-test" {
		t.Errorf("CloudAPIKeys[openrouter] = %q, want %q", loaded.CloudAPIKeys["openrouter"], "sk-or-v1-save-test")
	}
	if loaded.Model != "anthropic/claude-sonnet-4" {
		t.Errorf("Model = %q, want %q", loaded.Model, "anthropic/claude-sonnet-4")
	}
	if loaded.MaxTokens != 16384 {
		t.Errorf("MaxTokens = %d, want %d", loaded.MaxTokens, 16384)
	}
	if loaded.ContextWindow != 65536 {
		t.Errorf("ContextWindow = %d, want %d", loaded.ContextWindow, 65536)
	}
}

// --- config.json が存在しない場合 ---

func TestParseConfigFile_NoFile(t *testing.T) {
	// 存在しないパスだけを設定
	origPaths := configFilePaths
	configFilePaths = []string{"/nonexistent/path/config.json"}
	t.Cleanup(func() { configFilePaths = origPaths })

	cfg := DefaultConfig()
	err := cfg.ParseConfigFile()
	if err != nil {
		t.Errorf("ParseConfigFile should not error on missing file, got: %v", err)
	}

	// デフォルト値のまま
	if cfg.Provider != "ollama" {
		t.Errorf("Provider = %q, want default %q", cfg.Provider, "ollama")
	}
	if cfg.MaxTokens != DefaultMaxTokens {
		t.Errorf("MaxTokens = %d, want default %d", cfg.MaxTokens, DefaultMaxTokens)
	}
}

// --- 不正な JSON ---

func TestParseConfigFile_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	os.WriteFile(configPath, []byte(`{ invalid json }`), 0600)

	origPaths := configFilePaths
	configFilePaths = []string{configPath}
	t.Cleanup(func() { configFilePaths = origPaths })

	cfg := DefaultConfig()
	err := cfg.ParseConfigFile()
	// エラーは返さない（Warning表示のみ）
	if err != nil {
		t.Errorf("ParseConfigFile should not return error on invalid JSON, got: %v", err)
	}

	// デフォルト値のまま
	if cfg.Provider != "ollama" {
		t.Errorf("Provider should be default 'ollama', got %q", cfg.Provider)
	}
}

// --- applyConfigFile 直接テスト ---

func TestApplyConfigFile_ProviderOverridesGlobal(t *testing.T) {
	cfg := DefaultConfig()

	cf := &ConfigFile{
		MaxTokens:   8192,
		Temperature: 0.7,
		Provider:    "openrouter",
		Providers: map[string]ProviderProfile{
			"openrouter": {
				Type:        "openrouter",
				APIKey:      "test-key",
				Model:       "meta-llama/llama-3.1-70b",
				MaxTokens:   65536,
				Temperature: 0.2,
			},
		},
	}

	cfg.applyConfigFile(cf)

	// グローバル MAX_TOKENS=8192 がプロバイダープロファイルの 65536 で上書き
	if cfg.MaxTokens != 65536 {
		t.Errorf("MaxTokens = %d, want 65536 (provider profile overrides global)", cfg.MaxTokens)
	}
	if cfg.Temperature != 0.2 {
		t.Errorf("Temperature = %f, want 0.2 (provider profile overrides global)", cfg.Temperature)
	}
	if cfg.Model != "meta-llama/llama-3.1-70b" {
		t.Errorf("Model = %q, want %q", cfg.Model, "meta-llama/llama-3.1-70b")
	}
}

// --- SaveConfigFile → ParseConfigFile ラウンドトリップ ---

func TestSaveAndReload_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	savePath := filepath.Join(tmpDir, "config.json")

	// configFilePaths をテスト用に設定
	origPaths := configFilePaths
	configFilePaths = []string{savePath}
	t.Cleanup(func() { configFilePaths = origPaths })

	// 元の Config を作成
	original := DefaultConfig()
	original.Provider = "openrouter"
	original.CloudAPIKeys["openrouter"] = "sk-or-v1-roundtrip"
	original.Model = "google/gemini-2.5-flash"
	original.MaxTokens = 24576
	original.Temperature = 0.6
	original.ContextWindow = 131072

	// 手動で保存（SaveConfigFile は const defaultConfigPath を使うため）
	var cf ConfigFile
	cf.Providers = make(map[string]ProviderProfile)
	cf.Provider = original.Provider
	cf.MaxTokens = original.MaxTokens
	cf.Temperature = original.Temperature
	cf.ContextWindow = original.ContextWindow
	cf.Model = original.Model
	cf.OllamaHost = original.OllamaHost
	cf.Providers["openrouter"] = ProviderProfile{
		Type:   "openrouter",
		APIKey: original.CloudAPIKeys["openrouter"],
		Model:  original.Model,
	}

	data, _ := json.MarshalIndent(cf, "", "    ")
	os.WriteFile(savePath, data, 0600)

	// 読み戻し
	reloaded := DefaultConfig()
	reloaded.ParseConfigFile()

	if reloaded.Provider != original.Provider {
		t.Errorf("Provider: got %q, want %q", reloaded.Provider, original.Provider)
	}
	if reloaded.CloudAPIKeys["openrouter"] != original.CloudAPIKeys["openrouter"] {
		t.Errorf("APIKey: got %q, want %q", reloaded.CloudAPIKeys["openrouter"], original.CloudAPIKeys["openrouter"])
	}
	if reloaded.Model != original.Model {
		t.Errorf("Model: got %q, want %q", reloaded.Model, original.Model)
	}
	if reloaded.MaxTokens != original.MaxTokens {
		t.Errorf("MaxTokens: got %d, want %d", reloaded.MaxTokens, original.MaxTokens)
	}
	if reloaded.ContextWindow != original.ContextWindow {
		t.Errorf("ContextWindow: got %d, want %d", reloaded.ContextWindow, original.ContextWindow)
	}
}

// --- GetProviderProfiles テスト ---

func TestGetProviderProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	jsonData := `{
		"PROVIDER": "ollama",
		"PROVIDERS": {
			"ollama": {
				"type": "ollama",
				"host": "http://localhost:11434",
				"model": "qwen3:8b"
			},
			"openrouter": {
				"type": "openrouter",
				"api_key": "sk-test",
				"model": "google/gemini-2.5-flash"
			}
		}
	}`
	os.WriteFile(configPath, []byte(jsonData), 0600)

	origPaths := configFilePaths
	configFilePaths = []string{configPath}
	t.Cleanup(func() { configFilePaths = origPaths })

	cfg := DefaultConfig()
	profiles := cfg.GetProviderProfiles()

	if profiles == nil {
		t.Fatal("GetProviderProfiles returned nil")
	}
	if len(profiles) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(profiles))
	}

	ollama, ok := profiles["ollama"]
	if !ok {
		t.Fatal("missing 'ollama' profile")
	}
	if ollama.Type != "ollama" {
		t.Errorf("ollama type = %q, want %q", ollama.Type, "ollama")
	}
	if ollama.Model != "qwen3:8b" {
		t.Errorf("ollama model = %q, want %q", ollama.Model, "qwen3:8b")
	}

	or, ok := profiles["openrouter"]
	if !ok {
		t.Fatal("missing 'openrouter' profile")
	}
	if or.APIKey != "sk-test" {
		t.Errorf("openrouter api_key = %q, want %q", or.APIKey, "sk-test")
	}
}
