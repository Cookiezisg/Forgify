// ollama.go — builds ToolCallingChatModel for Ollama (local LLM runner).
// Uses the native eino-ext ollama provider (not OpenAI-compatible path)
// for proper Ollama API semantics.
//
// ollama.go — 为 Ollama（本地 LLM）构建 ToolCallingChatModel。
// 使用原生的 eino-ext ollama provider（不走 OpenAI 兼容路径），
// 以保证正确的 Ollama API 语义。

package eino

import (
	"context"
	"fmt"
	"time"

	einoollama "github.com/cloudwego/eino-ext/components/model/ollama"
)

// buildOllama creates a ToolCallingChatModel targeting a local Ollama server.
// cfg.BaseURL must be set (Ollama has no public default endpoint).
//
// buildOllama 创建指向本地 Ollama server 的 ToolCallingChatModel。
// cfg.BaseURL 必须设置（Ollama 没有公共默认端点）。
func buildOllama(ctx context.Context, cfg ModelConfig) (*BuiltModel, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("einoinfra: ollama requires a base_url (e.g. http://localhost:11434)")
	}
	timeout := 120 * time.Second
	config := &einoollama.ChatModelConfig{
		BaseURL: cfg.BaseURL,
		Model:   cfg.ModelID,
		Timeout: timeout,
	}
	m, err := einoollama.NewChatModel(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("einoinfra: build ollama model: %w", err)
	}
	return &BuiltModel{
		Model:   m,
		Checker: safeStreamChecker,
	}, nil
}
