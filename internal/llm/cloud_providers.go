package llm

// クラウドプロバイダー定義
// 各プロバイダーの接続情報とデフォルト設定を一元管理する

// CloudProviderDef クラウドプロバイダーの定義
type CloudProviderDef struct {
	Name         string   // 表示名
	Key          string   // config内キー ("openrouter", "openai", etc.)
	Category     string   // カテゴリ ("major", "aggregator", "fast", "specialized")
	BaseURL      string   // API基盤URL
	EnvKey       string   // 環境変数名
	DefaultModel string   // デフォルトモデル
	Models       []string // 推奨モデル一覧
}

// ProviderCategory カテゴリの表示定義
type ProviderCategory struct {
	Key   string // "major", "aggregator", etc.
	Label string // 表示名
}

// CloudProviderCategories カテゴリ一覧（表示順）
var CloudProviderCategories = []ProviderCategory{
	{Key: "aggregator", Label: "アグリゲーター（複数モデル対応）"},
	{Key: "major", Label: "主要プロバイダー"},
	{Key: "fast", Label: "高速推論"},
	{Key: "specialized", Label: "特化型"},
	{Key: "china", Label: "中国系プロバイダー"},
}

// CloudProviders 利用可能なクラウドプロバイダー定義
// BaseURL はバージョンパスまで含む（例: /v1, /v4, /v1beta/openai）
// 実際のエンドポイントは BaseURL + "/chat/completions" で構築される
// モデル一覧は 2026年2月時点の最新
var CloudProviders = []CloudProviderDef{
	// === アグリゲーター ===
	{
		Name:         "OpenRouter",
		Key:          "openrouter",
		Category:     "aggregator",
		BaseURL:      OpenRouterBaseURL,
		EnvKey:       "OPENROUTER_API_KEY",
		DefaultModel: OpenRouterDefaultModel,
		Models: []string{
			"google/gemini-2.5-flash",
			"anthropic/claude-sonnet-4",
			"openai/gpt-4.1",
			"meta-llama/llama-4-maverick",
			"deepseek/deepseek-chat-v3-0324",
			"moonshotai/kimi-k2-instruct",
		},
	},
	// === 主要プロバイダー ===
	{
		Name:         "OpenAI",
		Key:          "openai",
		Category:     "major",
		BaseURL:      "https://api.openai.com/v1",
		EnvKey:       "OPENAI_API_KEY",
		DefaultModel: "gpt-4.1",
		Models: []string{
			"gpt-4.1",
			"gpt-4.1-mini",
			"gpt-4.1-nano",
			"o3",
			"o4-mini",
			"gpt-4o",
		},
	},
	{
		Name:         "Anthropic (Claude)",
		Key:          "anthropic",
		Category:     "major",
		BaseURL:      "https://api.anthropic.com/v1",
		EnvKey:       "ANTHROPIC_API_KEY",
		DefaultModel: "claude-sonnet-4-20250514",
		Models: []string{
			"claude-sonnet-4-20250514",
			"claude-opus-4-20250514",
			"claude-sonnet-4-5-20250929",
			"claude-haiku-4-5-20251001",
		},
	},
	{
		Name:         "Google (Gemini)",
		Key:          "google",
		Category:     "major",
		BaseURL:      "https://generativelanguage.googleapis.com/v1beta/openai",
		EnvKey:       "GEMINI_API_KEY",
		DefaultModel: "gemini-2.5-flash",
		Models: []string{
			"gemini-2.5-flash",
			"gemini-2.5-pro",
			"gemini-2.0-flash",
		},
	},
	{
		Name:         "DeepSeek",
		Key:          "deepseek",
		Category:     "major",
		BaseURL:      "https://api.deepseek.com/v1",
		EnvKey:       "DEEPSEEK_API_KEY",
		DefaultModel: "deepseek-chat",
		Models: []string{
			"deepseek-chat",
			"deepseek-reasoner",
		},
	},
	{
		Name:         "Mistral",
		Key:          "mistral",
		Category:     "major",
		BaseURL:      "https://api.mistral.ai/v1",
		EnvKey:       "MISTRAL_API_KEY",
		DefaultModel: "mistral-large-latest",
		Models: []string{
			"mistral-large-latest",
			"mistral-medium-3.1",
			"codestral-latest",
			"magistral-medium-latest",
			"mistral-small-latest",
		},
	},
	// === 高速推論 ===
	{
		Name:         "Groq",
		Key:          "groq",
		Category:     "fast",
		BaseURL:      "https://api.groq.com/openai/v1",
		EnvKey:       "GROQ_API_KEY",
		DefaultModel: "llama-3.3-70b-versatile",
		Models: []string{
			"llama-3.3-70b-versatile",
			"qwen-qwq-32b",
			"deepseek-r1-distill-llama-70b",
			"llama-3.1-8b-instant",
			"gemma2-9b-it",
		},
	},
	{
		Name:         "Together AI",
		Key:          "together",
		Category:     "fast",
		BaseURL:      "https://api.together.xyz/v1",
		EnvKey:       "TOGETHER_API_KEY",
		DefaultModel: "meta-llama/Llama-3.3-70B-Instruct-Turbo",
		Models: []string{
			"meta-llama/Llama-3.3-70B-Instruct-Turbo",
			"Qwen/Qwen2.5-72B-Instruct-Turbo",
			"deepseek-ai/DeepSeek-R1-Distill-Llama-70B",
			"meta-llama/Llama-4-Maverick-17B-128E-Instruct-FP8",
		},
	},
	{
		Name:         "Fireworks AI",
		Key:          "fireworks",
		Category:     "fast",
		BaseURL:      "https://api.fireworks.ai/inference/v1",
		EnvKey:       "FIREWORKS_API_KEY",
		DefaultModel: "accounts/fireworks/models/llama-v3p3-70b-instruct",
		Models: []string{
			"accounts/fireworks/models/llama-v3p3-70b-instruct",
			"accounts/fireworks/models/llama4-maverick-instruct-basic",
			"accounts/fireworks/models/deepseek-v3",
			"accounts/fireworks/models/qwen2p5-72b-instruct",
		},
	},
	// === 特化型 ===
	{
		Name:         "Perplexity (検索特化)",
		Key:          "perplexity",
		Category:     "specialized",
		BaseURL:      "https://api.perplexity.ai/v1",
		EnvKey:       "PERPLEXITY_API_KEY",
		DefaultModel: "sonar-pro",
		Models: []string{
			"sonar-pro",
			"sonar",
			"sonar-reasoning-pro",
			"sonar-reasoning",
		},
	},
	{
		Name:         "Cohere (RAG特化)",
		Key:          "cohere",
		Category:     "specialized",
		BaseURL:      "https://api.cohere.com/compatibility/v1",
		EnvKey:       "COHERE_API_KEY",
		DefaultModel: "command-a-03-2025",
		Models: []string{
			"command-a-03-2025",
			"command-r-plus-08-2024",
			"command-r-08-2024",
		},
	},
	// === 中国系プロバイダー ===
	{
		Name:         "Z.AI (GLM) 国際版",
		Key:          "zai",
		Category:     "china",
		BaseURL:      "https://api.z.ai/api/paas/v4",
		EnvKey:       "ZAI_API_KEY",
		DefaultModel: "glm-4.7",
		Models: []string{
			"glm-4.7",
			"glm-4.7-flash",
			"glm-4.5",
			"glm-4.5v",
		},
	},
	{
		Name:         "Z.AI Coding Plan",
		Key:          "zai-coding",
		Category:     "china",
		BaseURL:      "https://api.z.ai/api/coding/paas/v4",
		EnvKey:       "ZAI_API_KEY",
		DefaultModel: "glm-4.7",
		Models: []string{
			"glm-4.7",
			"glm-4.7-flash",
		},
	},
	{
		Name:         "智谱AI (GLM) 中国版",
		Key:          "zhipu",
		Category:     "china",
		BaseURL:      "https://open.bigmodel.cn/api/paas/v4",
		EnvKey:       "ZHIPU_API_KEY",
		DefaultModel: "glm-4.7",
		Models: []string{
			"glm-4.7",
			"glm-4.7-flash",
			"glm-4.5",
			"glm-4.5v",
		},
	},
	{
		Name:         "Moonshot (Kimi)",
		Key:          "moonshot",
		Category:     "china",
		BaseURL:      "https://api.moonshot.cn/v1",
		EnvKey:       "MOONSHOT_API_KEY",
		DefaultModel: "kimi-k2-instruct",
		Models: []string{
			"kimi-k2-instruct",
			"kimi-k2.5",
			"moonshot-v1-auto",
			"moonshot-v1-128k",
		},
	},
}

// GetCloudProviderDef プロバイダーキーから定義を取得
func GetCloudProviderDef(key string) *CloudProviderDef {
	for i := range CloudProviders {
		if CloudProviders[i].Key == key {
			return &CloudProviders[i]
		}
	}
	return nil
}

// GetProvidersByCategory カテゴリ別にプロバイダーを取得
func GetProvidersByCategory(category string) []CloudProviderDef {
	var result []CloudProviderDef
	for _, p := range CloudProviders {
		if p.Category == category {
			result = append(result, p)
		}
	}
	return result
}

// NewCloudProvider クラウドプロバイダーを作成（OpenAI互換）
// BaseURL にはバージョンパスまで含まれている前提（例: /v1, /v4, /v1beta/openai）
// OpenRouter のみ固有ヘッダー付きの専用実装を使用
func NewCloudProvider(providerKey, apiKey, model string) LLMProvider {
	def := GetCloudProviderDef(providerKey)
	if def == nil {
		// fallback to openrouter
		return NewOpenRouterProvider(apiKey, model)
	}

	if model == "" {
		model = def.DefaultModel
	}

	// OpenRouter は固有ヘッダーがあるため専用実装
	if providerKey == "openrouter" {
		return NewOpenRouterProvider(apiKey, model)
	}

	// それ以外は全て汎用 OpenAI互換プロバイダー
	// BaseURL + "/chat/completions" でエンドポイントが構築される
	info := ProviderInfo{
		Name:    providerKey,
		Type:    ProviderTypeCloud,
		BaseURL: def.BaseURL,
		Model:   model,
		Features: Features{
			NativeFunctionCalling: true,
			ModelManagement:       false,
			Streaming:             true,
		},
	}
	return NewOpenAICompatProvider(def.BaseURL, apiKey, model, info)
}
