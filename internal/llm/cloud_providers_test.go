package llm

import (
	"strings"
	"testing"
)

// TestGetCloudProviderDef_AllProviders 全クラウドプロバイダー定義が正しく取得できるか
func TestGetCloudProviderDef_AllProviders(t *testing.T) {
	knownProviders := []string{
		"openrouter", "openai", "anthropic", "google",
		"deepseek", "mistral", "groq", "together",
		"fireworks", "perplexity", "cohere",
		"zai", "zai-coding", "zhipu", "moonshot",
	}

	for _, key := range knownProviders {
		t.Run(key, func(t *testing.T) {
			def := GetCloudProviderDef(key)
			if def == nil {
				t.Errorf("GetCloudProviderDef(%q) returned nil", key)
				return
			}
			if def.Key != key {
				t.Errorf("expected key %q, got %q", key, def.Key)
			}
			if def.BaseURL == "" {
				t.Errorf("provider %q has empty BaseURL", key)
			}
			if def.DefaultModel == "" {
				t.Errorf("provider %q has empty DefaultModel", key)
			}
			if def.EnvKey == "" {
				t.Errorf("provider %q has empty EnvKey", key)
			}
		})
	}
}

// TestGetCloudProviderDef_Unknown 未知のプロバイダーキーはnilを返す
func TestGetCloudProviderDef_Unknown(t *testing.T) {
	def := GetCloudProviderDef("unknown-provider-xyz")
	if def != nil {
		t.Errorf("expected nil for unknown provider, got %+v", def)
	}
}

// TestGetProvidersByCategory カテゴリ別フィルタリングが正しく動作するか
func TestGetProvidersByCategory(t *testing.T) {
	tests := []struct {
		category    string
		wantMinLen  int
		wantContain string
	}{
		{"major", 3, "openai"},
		{"aggregator", 1, "openrouter"},
		{"fast", 1, "groq"},
		{"china", 1, "zhipu"},
	}

	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			providers := GetProvidersByCategory(tt.category)
			if len(providers) < tt.wantMinLen {
				t.Errorf("category %q: expected at least %d providers, got %d", tt.category, tt.wantMinLen, len(providers))
			}
			found := false
			for _, p := range providers {
				if p.Key == tt.wantContain {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("category %q: expected to contain %q", tt.category, tt.wantContain)
			}
		})
	}
}

// TestGetLocalProviders ローカルプロバイダー一覧が取得できるか
func TestGetLocalProviders(t *testing.T) {
	providers := GetLocalProviders()
	if len(providers) == 0 {
		t.Fatal("GetLocalProviders() returned empty slice")
	}

	// Ollama は必ず含まれているはず
	found := false
	for _, p := range providers {
		if p.Key == "ollama" {
			found = true
			if p.DefaultHost == "" {
				t.Error("ollama provider has empty DefaultHost")
			}
		}
	}
	if !found {
		t.Error("GetLocalProviders() did not contain 'ollama'")
	}
}

// TestNewCloudProvider_KnownProviders 既知プロバイダーでプロバイダーが作成できるか
func TestNewCloudProvider_KnownProviders(t *testing.T) {
	tests := []struct {
		key          string
		wantNamePart string
	}{
		{"openai", "openai"},
		{"anthropic", "anthropic"},
		{"google", "google"},
		{"deepseek", "deepseek"},
		{"groq", "groq"},
		{"openrouter", "openrouter"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			p := NewCloudProvider(tt.key, "test-api-key", "")
			if p == nil {
				t.Fatalf("NewCloudProvider(%q) returned nil", tt.key)
			}
			info := p.Info()
			if !strings.Contains(strings.ToLower(info.Name), tt.wantNamePart) {
				t.Errorf("expected Name to contain %q, got %q", tt.wantNamePart, info.Name)
			}
			if info.Type != ProviderTypeCloud {
				t.Errorf("expected ProviderTypeCloud, got %q", info.Type)
			}
			if info.BaseURL == "" {
				t.Errorf("provider %q has empty BaseURL", tt.key)
			}
			if info.Model == "" {
				t.Errorf("provider %q has empty Model (should use default)", tt.key)
			}
		})
	}
}

// TestNewCloudProvider_UnknownFallsBackToOpenRouter 未知のプロバイダーはOpenRouterにフォールバック
func TestNewCloudProvider_UnknownFallsBackToOpenRouter(t *testing.T) {
	p := NewCloudProvider("unknown-provider", "test-key", "test-model")
	if p == nil {
		t.Fatal("NewCloudProvider with unknown key returned nil")
	}
	info := p.Info()
	// フォールバックはOpenRouterになる
	if info.BaseURL == "" {
		t.Error("fallback provider has empty BaseURL")
	}
}

// TestNewCloudProvider_CustomModel カスタムモデルが正しく設定されるか
func TestNewCloudProvider_CustomModel(t *testing.T) {
	customModel := "gpt-4o-custom"
	p := NewCloudProvider("openai", "test-key", customModel)
	info := p.Info()
	if info.Model != customModel {
		t.Errorf("expected model %q, got %q", customModel, info.Model)
	}
}

// TestNewCloudProvider_DefaultModel モデル未指定時にデフォルトモデルが使われるか
func TestNewCloudProvider_DefaultModel(t *testing.T) {
	p := NewCloudProvider("openai", "test-key", "")
	info := p.Info()
	if info.Model == "" {
		t.Error("expected default model, got empty string")
	}
	def := GetCloudProviderDef("openai")
	if def == nil {
		t.Fatal("openai provider def not found")
	}
	if info.Model != def.DefaultModel {
		t.Errorf("expected default model %q, got %q", def.DefaultModel, info.Model)
	}
}

// TestCloudProviderDef_BaseURLFormat BaseURLが正しい形式か
func TestCloudProviderDef_BaseURLFormat(t *testing.T) {
	for _, p := range CloudProviders {
		t.Run(p.Key, func(t *testing.T) {
			if !strings.HasPrefix(p.BaseURL, "https://") {
				t.Errorf("provider %q BaseURL should start with https://, got %q", p.Key, p.BaseURL)
			}
		})
	}
}

// TestCloudProviderDef_EnvKeyFormat 環境変数名が正しい形式か（大文字 + _API_KEY）
func TestCloudProviderDef_EnvKeyFormat(t *testing.T) {
	for _, p := range CloudProviders {
		t.Run(p.Key, func(t *testing.T) {
			if !strings.HasSuffix(p.EnvKey, "_API_KEY") {
				t.Errorf("provider %q EnvKey should end with _API_KEY, got %q", p.Key, p.EnvKey)
			}
			if p.EnvKey != strings.ToUpper(p.EnvKey) {
				t.Errorf("provider %q EnvKey should be uppercase, got %q", p.Key, p.EnvKey)
			}
		})
	}
}
