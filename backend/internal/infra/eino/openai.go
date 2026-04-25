// openai.go — builds ToolCallingChatModel for all OpenAI-compatible providers.
// Covers: openai, deepseek, qwen, zhipu, moonshot, doubao, openrouter,
//         google (OpenAI-compat endpoint), and custom(openai-compatible).
//
// openai.go — 为所有 OpenAI 兼容 provider 构建 ToolCallingChatModel。

// 覆盖：openai / deepseek / qwen / zhipu / moonshot / doubao / openrouter /
//       google（OpenAI 兼容端点）/ custom(openai-compatible)。

package eino

import (
	"context"
	"fmt"
	"time"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
)

// buildOpenAICompat creates a ToolCallingChatModel using the eino-ext OpenAI
// provider. cfg.BaseURL must already be resolved (non-empty) by the time
// this is called for providers that don't have a built-in default.
//
// buildOpenAICompat 用 eino-ext OpenAI provider 创建 ToolCallingChatModel。
// 对没有内置默认值的 provider，cfg.BaseURL 必须在调用前已解析（非空）。
func buildOpenAICompat(ctx context.Context, cfg ModelConfig) (*BuiltModel, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("einoinfra: provider %q requires a base_url", cfg.Provider)
	}
	config := &einoopenai.ChatModelConfig{
		APIKey:  cfg.Key,
		BaseURL: cfg.BaseURL,
		Model:   cfg.ModelID,
		Timeout: 120 * time.Second,
	}
	m, err := einoopenai.NewChatModel(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("einoinfra: build openai-compat model (%s): %w", cfg.Provider, err)
	}
	return &BuiltModel{
		Model:   m,
		Checker: safeStreamChecker,
	}, nil
}
