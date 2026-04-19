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
	"openai_compat": {
		{ID: "custom", Name: "自定义模型", Tier: "balanced"},
	},
	"ollama": {
		{ID: "llama3.2", Name: "Llama 3.2", Tier: "cheap"},
		{ID: "qwen2.5", Name: "Qwen 2.5", Tier: "balanced"},
	},
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
