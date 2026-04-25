// Package eino (infra/eino) bridges Forgify's provider abstraction to the
// Eino LLM framework. It builds ToolCallingChatModel instances at request
// time from runtime credentials resolved by the apikey domain.
//
// All three eino packages referenced here follow the import alias convention:
//
//	einoinfra "…/internal/infra/eino"
//
// Package eino（infra/eino）把 Forgify 的 provider 抽象桥接到 Eino LLM 框架。
// 根据 apikey domain 解析的运行时凭证，在请求时构建 ToolCallingChatModel。
package eino

import (
	"context"
	"fmt"
	"io"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ModelConfig carries everything needed to build a ChatModel for one request.
// BaseURL is already merged with the provider default by apikey.ResolveCredentials
// before reaching here — the factory never needs to look up defaults.
//
// ModelConfig 携带构建一次请求所需的全部信息。
// BaseURL 在到达此处之前，已由 apikey.ResolveCredentials 与 provider 默认值合并——
// factory 无需自行查找默认值。
type ModelConfig struct {
	Provider  string // "openai" | "anthropic" | "ollama" | ...
	ModelID   string
	Key       string
	BaseURL   string
	APIFormat string // 仅 custom provider 使用："openai-compatible" | "anthropic-compatible"
}

// BuiltModel bundles the ChatModel with its provider-specific
// StreamToolCallChecker. chat.Service passes both to react.AgentConfig.
//
// BuiltModel 把 ChatModel 和对应 provider 的 StreamToolCallChecker 打包。
// chat.Service 把两者都传给 react.AgentConfig。
type BuiltModel struct {
	Model   model.ToolCallingChatModel
	Checker func(ctx context.Context, sr *schema.StreamReader[*schema.Message]) (bool, error)
}

// ChatModelFactory builds a provider-appropriate ToolCallingChatModel for
// every chat request. Implementations must be safe for concurrent use.
//
// ChatModelFactory 为每次对话请求构建合适的 ToolCallingChatModel。
// 实现必须并发安全。
type ChatModelFactory interface {
	Build(ctx context.Context, cfg ModelConfig) (*BuiltModel, error)
}

// DefaultFactory is the production implementation. It dispatches by provider:
//   - "ollama" → eino-ext ollama provider
//   - "anthropic" → eino-ext openai provider targeting Anthropic's
//     OpenAI-compatible endpoint, with an Anthropic-specific StreamToolCallChecker
//   - everything else → eino-ext openai provider (all OpenAI-compatible APIs)
//
// DefaultFactory 是生产实现，按 provider 分派：
//   - "ollama"    → eino-ext ollama provider
//   - "anthropic" → eino-ext openai provider 指向 Anthropic 的 OpenAI 兼容端点，
//     并使用 Anthropic 专属 StreamToolCallChecker
//   - 其他        → eino-ext openai provider（所有 OpenAI 兼容 API）
type DefaultFactory struct{}

// NewDefaultFactory constructs a DefaultFactory ready for use.
//
// NewDefaultFactory 构造一个可直接使用的 DefaultFactory。
func NewDefaultFactory() *DefaultFactory {
	return &DefaultFactory{}
}

// Build constructs the appropriate ToolCallingChatModel for cfg.Provider.
//
// Build 为 cfg.Provider 构建合适的 ToolCallingChatModel。
func (f *DefaultFactory) Build(ctx context.Context, cfg ModelConfig) (*BuiltModel, error) {
	switch cfg.Provider {
	case "ollama":
		return buildOllama(ctx, cfg)
	case "anthropic":
		return buildAnthropic(ctx, cfg)
	case "custom":
		if cfg.APIFormat == "anthropic-compatible" {
			return buildAnthropic(ctx, cfg)
		}
		return buildOpenAICompat(ctx, cfg)
	default:
		return buildOpenAICompat(ctx, cfg)
	}
}

// safeStreamChecker reads the full stream and returns true if any chunk
// contains tool calls. Works correctly for all providers including Anthropic,
// which emits text before tool calls in streaming mode.
//
// Note: MUST close the stream — required by Eino's react.AgentConfig contract.
//
// safeStreamChecker 读完整个流，只要任意 chunk 含 tool call 就返回 true。
// 对所有 provider 都正确，包括在流式输出中先输出文本再输出 tool call 的 Anthropic。
//
// 注意：必须关闭 stream——这是 Eino react.AgentConfig 的契约要求。
func safeStreamChecker(_ context.Context, sr *schema.StreamReader[*schema.Message]) (bool, error) {
	defer sr.Close()
	for {
		msg, err := sr.Recv()
		if err != nil {
			if err == io.EOF {
				return false, nil
			}
			return false, fmt.Errorf("einoinfra: stream checker: %w", err)
		}
		if len(msg.ToolCalls) > 0 {
			return true, nil
		}
	}
}
