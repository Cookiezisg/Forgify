// factory.go — Provider dispatch: maps (provider, config) to the correct
// Client implementation and resolves the default BaseURL when not supplied.
//
// factory.go — Provider 分派：把（provider, config）映射到正确的 Client 实现，
// 并在未提供 BaseURL 时解析 provider 默认值。
package llm

import "fmt"

// Config carries everything needed to pick and configure a Client.
//
// Config 携带选择和配置 Client 所需的全部信息。
type Config struct {
	Provider  string // "openai" | "anthropic" | "ollama" | "deepseek" | "custom" | …
	APIFormat string // custom provider only: "openai-compatible" | "anthropic-compatible"
	ModelID   string
	Key       string
	BaseURL   string // overrides the provider default when non-empty
}

// Factory creates Clients. It owns one shared HTTP client per wire protocol
// so connections are reused across requests.
//
// Factory 创建 Client。每种协议共用一个 HTTP client，跨请求复用连接。
type Factory struct {
	openai    *openAIClient
	anthropic *anthropicClient
}

// NewFactory constructs a Factory ready for use.
//
// NewFactory 构造一个可直接使用的 Factory。
func NewFactory() *Factory {
	return &Factory{
		openai:    newOpenAIClient(),
		anthropic: newAnthropicClient(),
	}
}

// Build returns the Client and resolved BaseURL for the given Config.
//
// Build 返回给定 Config 对应的 Client 和解析后的 BaseURL。
func (f *Factory) Build(cfg Config) (Client, string, error) {
	baseURL, err := resolveBaseURL(cfg)
	if err != nil {
		return nil, "", err
	}
	switch cfg.Provider {
	case "anthropic":
		return f.anthropic, baseURL, nil
	case "custom":
		if cfg.APIFormat == "anthropic-compatible" {
			return f.anthropic, baseURL, nil
		}
		return f.openai, baseURL, nil
	default:
		return f.openai, baseURL, nil
	}
}

// resolveBaseURL returns cfg.BaseURL when set, or the provider's default.
//
// resolveBaseURL 有 cfg.BaseURL 时直接返回，否则返回 provider 默认值。
func resolveBaseURL(cfg Config) (string, error) {
	if cfg.BaseURL != "" {
		return cfg.BaseURL, nil
	}
	switch cfg.Provider {
	case "openai":
		return "https://api.openai.com/v1", nil
	case "anthropic":
		// Anthropic client appends /v1/messages itself; base is just the host.
		// Anthropic client 自行拼接 /v1/messages，这里只给 host。
		return "https://api.anthropic.com", nil
	case "ollama":
		return "http://localhost:11434/v1", nil
	case "deepseek":
		return "https://api.deepseek.com/v1", nil
	case "qwen", "tongyi":
		return "https://dashscope.aliyuncs.com/compatible-mode/v1", nil
	case "moonshot":
		return "https://api.moonshot.cn/v1", nil
	case "custom":
		return "", fmt.Errorf("llm: custom provider requires base_url")
	default:
		return "", fmt.Errorf("llm: unknown provider %q", cfg.Provider)
	}
}
