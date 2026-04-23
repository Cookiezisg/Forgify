// Package ports holds the two interfaces that external code implements
// or consumes: Repository (storage, implemented by infra/gorm) and
// KeyProvider (cross-domain consumer API).
//
// Package ports 存放两个被外部实现或消费的接口：Repository（存储，
// 由 infra/gorm 实现）和 KeyProvider（跨 domain 消费 API）。
package ports

import (
	"context"

	"github.com/sunweilin/forgify/backend/internal/domain/apikey/types"
)

// Repository is the storage contract for APIKey. Implementations filter
// by the userID in ctx — callers MUST ensure InjectUserID middleware has
// run before invoking any method here.
//
// Implemented by: infra/gorm.APIKeyRepo
// Consumer:       app/apikey.Service (only)
//
// Repository 是 APIKey 的存储契约。实现按 ctx 中的 userID 过滤——调用方
// 必须保证 InjectUserID 中间件已在链中运行。
//
// 实现：infra/gorm.APIKeyRepo
// 消费：仅 app/apikey.Service
type Repository interface {
	// Get fetches a single APIKey by id, scoped to the user in ctx.
	// Returns types.ErrNotFound if no live record matches.
	//
	// Get 按 id 查询单条 APIKey，按 ctx 中的用户过滤。
	// 未命中活跃记录返回 types.ErrNotFound。
	Get(ctx context.Context, id string) (*types.APIKey, error)

	// List returns a page of keys for the current user, with optional
	// provider filter. Returns (rows, nextCursor, err).
	//
	// List 返回当前用户的一页 Key，可选按 provider 过滤。
	// 返回 (rows, nextCursor, err)。
	List(ctx context.Context, filter types.ListFilter) ([]*types.APIKey, string, error)

	// GetByProvider picks the most suitable live APIKey for the given
	// provider under the current user. Selection order:
	//   1. test_status = 'ok' preferred
	//   2. last_tested_at DESC (most recently validated)
	//   3. created_at DESC (most recently created)
	// Returns types.ErrNotFoundForProvider if none exists.
	//
	// GetByProvider 为当前用户在指定 provider 下挑选**最适合**的活跃 Key。
	// 挑选顺序：
	//   1. test_status = 'ok' 优先
	//   2. last_tested_at DESC（最近验证过）
	//   3. created_at DESC（最近创建）
	// 未命中返回 types.ErrNotFoundForProvider。
	GetByProvider(ctx context.Context, provider string) (*types.APIKey, error)

	// Save inserts or updates based on whether k.ID already exists. The
	// caller must have set UserID on k (typically from ctx).
	//
	// Save 按 k.ID 决定插入或更新。调用方需确保已设置 UserID（通常从 ctx 取）。
	Save(ctx context.Context, k *types.APIKey) error

	// Delete soft-deletes by id, scoped to current user.
	//
	// Delete 软删除（按 ctx 中用户过滤）。
	Delete(ctx context.Context, id string) error

	// UpdateTestResult writes only test_status / test_error / last_tested_at.
	// Used by Service.Test and MarkInvalid to avoid a full-record round-trip.
	//
	// UpdateTestResult 只写 test_status / test_error / last_tested_at。
	// Service.Test 和 MarkInvalid 使用，避免读写整条记录。
	UpdateTestResult(ctx context.Context, id, status, errMsg string) error
}
