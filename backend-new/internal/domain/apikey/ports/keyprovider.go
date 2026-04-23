package ports

import (
	"context"

	"github.com/sunweilin/forgify/backend/internal/domain/apikey/types"
)

// KeyProvider is the cross-domain interface. Other services (chat,
// workflow, knowledge/embedding, etc.) import this to obtain ready-to-use
// decrypted credentials for an LLM call — they never see Repository or
// raw APIKey records.
//
// Implemented by: app/apikey.Service
//
// KeyProvider 是跨 domain 接口。其他 service（chat、workflow、知识库
// embedding 等）通过本接口拿到调 LLM 用的明文凭证——它们看不到
// Repository 或原始 APIKey 记录。
//
// 由 app/apikey.Service 实现。
type KeyProvider interface {
	// ResolveCredentials returns a usable (key, baseURL) pair for the
	// given provider under the current user (from ctx). Internally: picks
	// the best APIKey (tested-OK preferred), decrypts, merges baseURL
	// with the provider default.
	//
	// Returns types.ErrNotFoundForProvider if no live key exists.
	//
	// ResolveCredentials 为当前用户（从 ctx 取）在给定 provider 下返回可用的
	// (key, baseURL)。内部：挑最佳 APIKey（tested-OK 优先）、解密、合并
	// baseURL 与 provider 默认值。
	//
	// 用户在该 provider 无活跃 Key 时返回 types.ErrNotFoundForProvider。
	ResolveCredentials(ctx context.Context, provider string) (types.Credentials, error)

	// MarkInvalid is the feedback channel: call when an LLM call with the
	// returned credentials got 401/403. Updates test_status to error and
	// records the reason so the UI can surface it.
	//
	// MarkInvalid 是反馈通道：用返回的凭证调 LLM 遇到 401/403 时调用。
	// 把 test_status 更新为 error 并记录原因，UI 可向用户展示。
	MarkInvalid(ctx context.Context, provider string, reason string) error
}
