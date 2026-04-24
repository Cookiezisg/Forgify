// keyprovider.go — Service's implementation of apikeydomain.KeyProvider.
// Cross-domain consumers (chat, workflow, knowledge) fetch ready-to-use
// credentials here and report invalidation — they never see Repository,
// ciphertext, or raw APIKey rows.
//
// keyprovider.go — Service 实现 apikeydomain.KeyProvider。
// 跨 domain 消费方（chat / workflow / knowledge）在此拿到现成可用的凭证
// 并回报失效——它们看不到 Repository、密文、原始 APIKey 行。

package apikey

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
)

// Compile-time guard: *Service satisfies apikeydomain.KeyProvider.
//
// 编译期守护：*Service 满足 apikeydomain.KeyProvider。
var _ apikeydomain.KeyProvider = (*Service)(nil)

// ResolveCredentials picks the best APIKey for (caller, provider), decrypts,
// and merges baseURL with the provider default.
//
// ResolveCredentials 为 (调用者, provider) 挑选最佳 APIKey，解密，
// 并合并 baseURL 和 provider 默认值。
func (s *Service) ResolveCredentials(ctx context.Context, provider string) (apikeydomain.Credentials, error) {
	k, err := s.repo.GetByProvider(ctx, provider)
	if err != nil {
		return apikeydomain.Credentials{}, err
	}
	plain, err := s.encryptor.Decrypt(ctx, []byte(k.KeyEncrypted))
	if err != nil {
		return apikeydomain.Credentials{}, fmt.Errorf("apikey.Service.ResolveCredentials: decrypt: %w", err)
	}
	baseURL := k.BaseURL
	if baseURL == "" {
		if meta, ok := apikeydomain.GetProviderMeta(provider); ok {
			baseURL = meta.DefaultBaseURL
		}
	}
	return apikeydomain.Credentials{Key: string(plain), BaseURL: baseURL}, nil
}

// MarkInvalid is the feedback channel: consumers call this when the LLM
// rejected the returned credentials (401/403). It updates test_status to
// error on the selected APIKey so the UI can surface the problem.
//
// MarkInvalid 是反馈通道：消费方在 LLM 拒绝返回的凭证（401/403）时调用。
// 把选中 APIKey 的 test_status 更新为 error，UI 可据此展示问题。
func (s *Service) MarkInvalid(ctx context.Context, provider, reason string) error {
	k, err := s.repo.GetByProvider(ctx, provider)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateTestResult(ctx, k.ID, apikeydomain.TestStatusError, reason); err != nil {
		return err
	}
	s.log.Warn("apikey marked invalid",
		zap.String("key_id", k.ID),
		zap.String("provider", provider),
		zap.String("reason", reason))
	return nil
}
