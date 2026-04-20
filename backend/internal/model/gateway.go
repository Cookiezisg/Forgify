package model

import (
	"context"
	"errors"
	"fmt"
	"strings"

	claudemodel "github.com/cloudwego/eino-ext/components/model/claude"
	openaimodel "github.com/cloudwego/eino-ext/components/model/openai"
	einomodel "github.com/cloudwego/eino/components/model"

	"github.com/sunweilin/forgify/internal/events"
)

var ErrNoModelConfigured = errors.New("no model configured; go to Settings → 模型设置")

type ErrUsedFallback struct {
	Primary  string
	Fallback string
}

func (e ErrUsedFallback) Error() string {
	return fmt.Sprintf("primary model %s unavailable, using fallback %s", e.Primary, e.Fallback)
}

type KeyProvider func(provider string) (key, baseURL string, err error)

type ModelGateway struct {
	getKey KeyProvider
	bridge *events.Bridge
}

func New(getKey KeyProvider, bridge *events.Bridge) *ModelGateway {
	return &ModelGateway{getKey: getKey, bridge: bridge}
}

func (g *ModelGateway) GetModel(ctx context.Context, purpose ModelPurpose) (einomodel.ToolCallingChatModel, string, error) {
	cfg, err := LoadModelConfig()
	if err != nil {
		return nil, "", err
	}

	primary := cfg.ForPurpose(purpose)
	if primary.IsEmpty() {
		return nil, "", ErrNoModelConfigured
	}

	m, err := g.buildModel(ctx, primary)
	if err == nil {
		return m, primary.ModelID, nil
	}

	if !shouldFallback(err) || cfg.Fallback.IsEmpty() {
		return nil, "", err
	}

	fallback, ferr := g.buildModel(ctx, cfg.Fallback)
	if ferr != nil {
		return nil, "", fmt.Errorf("primary: %w; fallback: %v", err, ferr)
	}
	return fallback, cfg.Fallback.ModelID, ErrUsedFallback{
		Primary:  primary.ModelID,
		Fallback: cfg.Fallback.ModelID,
	}
}

func (g *ModelGateway) buildModel(ctx context.Context, a ModelAssignment) (einomodel.ToolCallingChatModel, error) {
	key, configuredURL, err := g.getKey(a.Provider)
	if err != nil {
		return nil, fmt.Errorf("no key for %s: %w", a.Provider, err)
	}

	switch a.Provider {
	case "anthropic":
		return claudemodel.NewChatModel(ctx, &claudemodel.Config{
			APIKey:    key,
			Model:     a.ModelID,
			MaxTokens: 4096,
		})

	case "ollama":
		baseURL := configuredURL
		if baseURL == "" {
			baseURL = "http://localhost:11434/v1"
		}
		return openaimodel.NewChatModel(ctx, &openaimodel.ChatModelConfig{
			APIKey:  "ollama",
			Model:   a.ModelID,
			BaseURL: baseURL,
		})

	default:
		// All other providers use OpenAI-compatible API.
		// Priority: key stored base_url > ProviderBaseURLs default > empty (uses openai default)
		baseURL := configuredURL
		if baseURL == "" {
			baseURL = ProviderBaseURLs[a.Provider]
		}
		return openaimodel.NewChatModel(ctx, &openaimodel.ChatModelConfig{
			APIKey:  key,
			Model:   a.ModelID,
			BaseURL: baseURL,
		})
	}
}

func shouldFallback(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "insufficient_quota") ||
		strings.Contains(msg, "model_not_found") ||
		strings.Contains(msg, "overloaded") ||
		strings.Contains(msg, "529")
}
