// Package model (infra/store/model) is the GORM-backed implementation of
// the domain model Repository port. Every method scopes queries by the
// userID carried in ctx — callers MUST have run the InjectUserID middleware.
//
// The package shares its name with domain/model by design; external callers
// alias at import: `modelstore "…/infra/store/model"`.
//
// Package model（infra/store/model）是 domain model Repository port 的 GORM
// 实现。所有方法按 ctx 中的 userID 过滤——调用方必须先经过 InjectUserID 中间件。
//
// 本包与 domain/model 同名是刻意的；外部调用方 import 时起别名，
// 例如 `modelstore "…/infra/store/model"`。
package model

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	"github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Store is the GORM implementation of modeldomain.Repository.
//
// Store 是 modeldomain.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// userID extracts the user ID from ctx. A missing value is a server wiring
// bug (middleware didn't run), not a 401.
//
// userID 从 ctx 取 user ID。缺失代表服务端接线 bug（中间件未跑），不是 401。
func userID(ctx context.Context) (string, error) {
	id, ok := reqctx.GetUserID(ctx)
	if !ok {
		return "", fmt.Errorf("modelstore: missing user id in context")
	}
	return id, nil
}

// GetByScenario fetches the active config for (current user, scenario).
// Returns modeldomain.ErrNotConfigured if none exists.
//
// GetByScenario 返回 (当前用户, scenario) 的活跃配置；无则返 ErrNotConfigured。
func (s *Store) GetByScenario(ctx context.Context, scenario string) (*modeldomain.ModelConfig, error) {
	uid, err := userID(ctx)
	if err != nil {
		return nil, err
	}
	var m modeldomain.ModelConfig
	err = s.db.WithContext(ctx).
		Where("user_id = ? AND scenario = ?", uid, scenario).
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, modeldomain.ErrNotConfigured
	}
	if err != nil {
		return nil, fmt.Errorf("modelstore.GetByScenario: %w", err)
	}
	return &m, nil
}

// List returns all active configs for the current user, ordered by scenario.
//
// List 返回当前用户所有活跃配置，按 scenario 排序。
func (s *Store) List(ctx context.Context) ([]*modeldomain.ModelConfig, error) {
	uid, err := userID(ctx)
	if err != nil {
		return nil, err
	}
	var rows []*modeldomain.ModelConfig
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", uid).
		Order("scenario").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("modelstore.List: %w", err)
	}
	return rows, nil
}

// Upsert saves m by primary key: INSERT if ID is new, UPDATE if it already
// exists. Caller (app/model.Service) is responsible for deciding which path
// by calling GetByScenario first.
//
// Upsert 按主键保存 m：ID 新则 INSERT，已存在则 UPDATE。
// 调用方（app/model.Service）负责先调 GetByScenario 决定走哪条路径。
func (s *Store) Upsert(ctx context.Context, m *modeldomain.ModelConfig) error {
	if err := s.db.WithContext(ctx).Save(m).Error; err != nil {
		return fmt.Errorf("modelstore.Upsert: %w", err)
	}
	return nil
}
