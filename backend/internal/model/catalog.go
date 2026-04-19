package model

type ModelInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Tier     string `json:"tier"`
	Provider string `json:"provider"`
}

var ProviderModels = map[string][]ModelInfo{
	"anthropic": {
		{ID: "claude-opus-4-7", Name: "Claude Opus 4.7", Tier: "powerful"},
		{ID: "claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Tier: "balanced"},
		{ID: "claude-haiku-4-5-20251001", Name: "Claude Haiku 4.5", Tier: "cheap"},
	},
	"openai": {
		{ID: "gpt-4o", Name: "GPT-4o", Tier: "powerful"},
		{ID: "gpt-4o-mini", Name: "GPT-4o mini", Tier: "cheap"},
		{ID: "o3-mini", Name: "o3-mini", Tier: "balanced"},
	},
	"deepseek": {
		{ID: "deepseek-chat", Name: "DeepSeek V3", Tier: "balanced"},
		{ID: "deepseek-reasoner", Name: "DeepSeek R1", Tier: "powerful"},
	},
	"moonshot": {
		{ID: "moonshot-v1-8k", Name: "Moonshot 8k", Tier: "cheap"},
		{ID: "moonshot-v1-128k", Name: "Moonshot 128k", Tier: "balanced"},
	},
	"groq": {
		{ID: "llama-3.3-70b-versatile", Name: "Llama 3.3 70B", Tier: "balanced"},
		{ID: "llama-3.1-8b-instant", Name: "Llama 3.1 8B (fast)", Tier: "cheap"},
		{ID: "mixtral-8x7b-32768", Name: "Mixtral 8x7B", Tier: "cheap"},
		{ID: "gemma2-9b-it", Name: "Gemma 2 9B", Tier: "cheap"},
	},
	"mistral": {
		{ID: "mistral-large-latest", Name: "Mistral Large", Tier: "powerful"},
		{ID: "mistral-small-latest", Name: "Mistral Small", Tier: "balanced"},
		{ID: "codestral-latest", Name: "Codestral", Tier: "balanced"},
		{ID: "open-mistral-nemo", Name: "Mistral Nemo", Tier: "cheap"},
	},
	"gemini": {
		{ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash", Tier: "balanced"},
		{ID: "gemini-2.0-flash-lite", Name: "Gemini 2.0 Flash Lite", Tier: "cheap"},
		{ID: "gemini-1.5-pro", Name: "Gemini 1.5 Pro", Tier: "powerful"},
	},
	"siliconflow": {
		{ID: "deepseek-ai/DeepSeek-V3", Name: "DeepSeek V3", Tier: "balanced"},
		{ID: "deepseek-ai/DeepSeek-R1", Name: "DeepSeek R1", Tier: "powerful"},
		{ID: "Qwen/Qwen2.5-72B-Instruct", Name: "Qwen 2.5 72B", Tier: "balanced"},
		{ID: "THUDM/glm-4-9b-chat", Name: "GLM-4 9B", Tier: "cheap"},
	},
	"openrouter": {
		{ID: "anthropic/claude-sonnet-4-6", Name: "Claude Sonnet 4.6", Tier: "balanced"},
		{ID: "google/gemini-2.0-flash-001", Name: "Gemini 2.0 Flash", Tier: "balanced"},
		{ID: "meta-llama/llama-3.3-70b-instruct", Name: "Llama 3.3 70B", Tier: "balanced"},
		{ID: "deepseek/deepseek-r1", Name: "DeepSeek R1", Tier: "powerful"},
	},
	"zhipu": {
		{ID: "glm-4-plus", Name: "GLM-4 Plus", Tier: "powerful"},
		{ID: "glm-4-flash", Name: "GLM-4 Flash", Tier: "cheap"},
	},
	"openai_compat": {
		{ID: "custom", Name: "自定义模型", Tier: "balanced"},
	},
	"ollama": {
		{ID: "llama3.2", Name: "Llama 3.2", Tier: "cheap"},
		{ID: "qwen2.5", Name: "Qwen 2.5", Tier: "balanced"},
		{ID: "deepseek-r1", Name: "DeepSeek R1", Tier: "powerful"},
	},
}

// ProviderBaseURLs contains the default base URL for OpenAI-compatible providers.
// Providers listed here use the OpenAI adapter with a custom base URL.
var ProviderBaseURLs = map[string]string{
	"deepseek":    "https://api.deepseek.com/v1",
	"moonshot":    "https://api.moonshot.cn/v1",
	"groq":        "https://api.groq.com/openai/v1",
	"mistral":     "https://api.mistral.ai/v1",
	"gemini":      "https://generativelanguage.googleapis.com/v1beta/openai",
	"siliconflow": "https://api.siliconflow.cn/v1",
	"openrouter":  "https://openrouter.ai/api/v1",
	"zhipu":       "https://open.bigmodel.cn/api/paas/v4",
}

func AvailableModels(configuredProviders []string) []ModelInfo {
	var result []ModelInfo
	for _, p := range configuredProviders {
		for _, m := range ProviderModels[p] {
			m.Provider = p
			result = append(result, m)
		}
	}
	return result
}
