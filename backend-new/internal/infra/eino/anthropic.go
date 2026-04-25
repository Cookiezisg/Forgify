// anthropic.go — builds ToolCallingChatModel for the Anthropic provider via
// Anthropic's OpenAI-compatible endpoint. The wire format is identical to
// OpenAI, so the eino-ext openai provider handles the HTTP, but we use
// safeStreamChecker to correctly detect tool calls (Claude emits text before
// tool calls in streaming mode, unlike OpenAI).
//
// Prompt caching: pass cache_control blocks via WithExtraFields when building
// the system prompt in chat.Service (not here — the factory has no prompt).
//
// anthropic.go — 通过 Anthropic 的 OpenAI 兼容端点构建 ToolCallingChatModel。
// 线上格式与 OpenAI 相同，所以 eino-ext openai provider 负责 HTTP 通信，
// 但我们使用 safeStreamChecker 正确检测 tool call——Claude 在流式输出中先输出
// 文本再输出 tool call，与 OpenAI 不同。

package eino

import (
	"context"
	"fmt"
	"time"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
)

const defaultAnthropicBaseURL = "https://api.anthropic.com/v1"

// buildAnthropic creates a ToolCallingChatModel targeting Anthropic's
// OpenAI-compatible endpoint.
//
// buildAnthropic 创建指向 Anthropic OpenAI 兼容端点的 ToolCallingChatModel。
func buildAnthropic(ctx context.Context, cfg ModelConfig) (*BuiltModel, error) {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultAnthropicBaseURL
	}
	config := &einoopenai.ChatModelConfig{
		APIKey:  cfg.Key,
		BaseURL: baseURL,
		Model:   cfg.ModelID,
		Timeout: 120 * time.Second,
	}
	m, err := einoopenai.NewChatModel(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("einoinfra: build anthropic model: %w", err)
	}
	// safeStreamChecker reads the full stream — necessary for Claude which
	// outputs text before tool calls, making the first-chunk default wrong.
	//
	// safeStreamChecker 读完整个流——Claude 先输出文本再输出 tool call，
	// 默认的首 chunk 检查会错误地返回 false。
	return &BuiltModel{
		Model:   m,
		Checker: safeStreamChecker,
	}, nil
}
