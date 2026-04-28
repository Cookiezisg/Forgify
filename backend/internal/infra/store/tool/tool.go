// Package tool (infra/store/tool) is the GORM-backed implementation of the
// domain tool Repository port. Every method scopes queries to the userID
// carried in ctx — callers MUST have run the InjectUserID middleware.
//
// The package shares its name with domain/tool and app/tool by design;
// external callers alias at import: `toolstore "…/infra/store/tool"`.
//
// Package tool（infra/store/tool）是 domain tool Repository port 的 GORM 实现。
// 所有方法按 ctx 中的 userID 过滤——调用方必须先经过 InjectUserID 中间件。
//
// 本包与 domain/tool、app/tool 同名是刻意的；外部调用方 import 时起别名，
// 如 `toolstore "…/infra/store/tool"`。
package tool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	tooldomain "github.com/sunweilin/forgify/backend/internal/domain/tool"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Store is the GORM implementation of tooldomain.Repository.
//
// Store 是 tooldomain.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

// uid extracts the user ID from ctx. A missing value is a server wiring bug.
//
// uid 从 ctx 取 user ID。缺失代表服务端接线 bug，不是 401。
func uid(ctx context.Context) (string, error) {
	id, ok := reqctxpkg.GetUserID(ctx)
	if !ok {
		return "", fmt.Errorf("toolstore: missing user id in context")
	}
	return id, nil
}

// ── Tool CRUD ─────────────────────────────────────────────────────────────────

// SaveTool inserts or updates a Tool by primary key.
//
// SaveTool 按主键插入或更新 Tool。
func (s *Store) SaveTool(ctx context.Context, t *tooldomain.Tool) error {
	if err := s.db.WithContext(ctx).Save(t).Error; err != nil {
		return fmt.Errorf("toolstore.SaveTool: %w", err)
	}
	return nil
}

// GetTool fetches a single live Tool by id for the current user.
//
// GetTool 按 id 查当前用户的单条活跃 Tool。
func (s *Store) GetTool(ctx context.Context, id string) (*tooldomain.Tool, error) {
	userID, err := uid(ctx)
	if err != nil {
		return nil, err
	}
	var t tooldomain.Tool
	err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, tooldomain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("toolstore.GetTool: %w", err)
	}
	return &t, nil
}

// GetToolsByIDs fetches multiple live Tools by id slice, preserving the
// input order. IDs that don't exist or belong to another user are silently omitted.
//
// GetToolsByIDs 按 id 切片批量查活跃 Tool，保持输入顺序。
// 不存在或属于其他用户的 ID 静默忽略。
func (s *Store) GetToolsByIDs(ctx context.Context, ids []string) ([]*tooldomain.Tool, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	userID, err := uid(ctx)
	if err != nil {
		return nil, err
	}
	var rows []*tooldomain.Tool
	if err = s.db.WithContext(ctx).
		Where("id IN ? AND user_id = ?", ids, userID).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("toolstore.GetToolsByIDs: %w", err)
	}
	// Re-order to match the requested id slice.
	// 按请求的 id 顺序重排。
	idx := make(map[string]*tooldomain.Tool, len(rows))
	for _, r := range rows {
		idx[r.ID] = r
	}
	ordered := make([]*tooldomain.Tool, 0, len(ids))
	for _, id := range ids {
		if t, ok := idx[id]; ok {
			ordered = append(ordered, t)
		}
	}
	return ordered, nil
}

// ListTools returns a cursor-paginated page of live tools for the current user,
// ordered by created_at DESC with id as tiebreaker.
//
// ListTools 返回当前用户活跃工具的 cursor 分页结果，按 created_at DESC 排序。
func (s *Store) ListTools(ctx context.Context, filter tooldomain.ListFilter) ([]*tooldomain.Tool, string, error) {
	userID, err := uid(ctx)
	if err != nil {
		return nil, "", err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	q := s.db.WithContext(ctx).Where("user_id = ?", userID)
	if filter.Cursor != "" {
		c, err := decodeCursor(filter.Cursor)
		if err != nil {
			return nil, "", fmt.Errorf("toolstore.ListTools: %w", err)
		}
		q = q.Where("(created_at, id) < (?, ?)", c.CreatedAt, c.ID)
	}
	var rows []*tooldomain.Tool
	if err = q.Order("created_at DESC, id DESC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("toolstore.ListTools: %w", err)
	}
	var next string
	if len(rows) > limit {
		last := rows[limit-1]
		next, err = encodeCursor(pageCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, "", fmt.Errorf("toolstore.ListTools: %w", err)
		}
		rows = rows[:limit]
	}
	return rows, next, nil
}

// ListAllTools returns all live tools for the current user without pagination.
//
// ListAllTools 返回当前用户全部活跃工具，不分页。
func (s *Store) ListAllTools(ctx context.Context) ([]*tooldomain.Tool, error) {
	userID, err := uid(ctx)
	if err != nil {
		return nil, err
	}
	var rows []*tooldomain.Tool
	if err = s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("toolstore.ListAllTools: %w", err)
	}
	return rows, nil
}

// DeleteTool soft-deletes a tool by id for the current user.
//
// DeleteTool 软删除当前用户的指定工具。
func (s *Store) DeleteTool(ctx context.Context, id string) error {
	userID, err := uid(ctx)
	if err != nil {
		return err
	}
	if err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&tooldomain.Tool{}).Error; err != nil {
		return fmt.Errorf("toolstore.DeleteTool: %w", err)
	}
	return nil
}

// ── Versions (including pending) ──────────────────────────────────────────────

// SaveVersion inserts a ToolVersion record.
//
// SaveVersion 插入一条 ToolVersion 记录。
func (s *Store) SaveVersion(ctx context.Context, v *tooldomain.ToolVersion) error {
	if err := s.db.WithContext(ctx).Create(v).Error; err != nil {
		return fmt.Errorf("toolstore.SaveVersion: %w", err)
	}
	return nil
}

// GetVersion fetches the accepted ToolVersion with the given version number.
//
// GetVersion 查询指定版本号的已接受版本记录。
func (s *Store) GetVersion(ctx context.Context, toolID string, version int) (*tooldomain.ToolVersion, error) {
	userID, err := uid(ctx)
	if err != nil {
		return nil, err
	}
	var v tooldomain.ToolVersion
	err = s.db.WithContext(ctx).
		Where("tool_id = ? AND user_id = ? AND version = ? AND status = ?",
			toolID, userID, version, tooldomain.VersionStatusAccepted).
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, tooldomain.ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("toolstore.GetVersion: %w", err)
	}
	return &v, nil
}

// GetActivePending returns the pending ToolVersion for the tool.
//
// GetActivePending 返回工具当前的 pending ToolVersion。
func (s *Store) GetActivePending(ctx context.Context, toolID string) (*tooldomain.ToolVersion, error) {
	userID, err := uid(ctx)
	if err != nil {
		return nil, err
	}
	var v tooldomain.ToolVersion
	err = s.db.WithContext(ctx).
		Where("tool_id = ? AND user_id = ? AND status = ?",
			toolID, userID, tooldomain.VersionStatusPending).
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, tooldomain.ErrPendingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("toolstore.GetActivePending: %w", err)
	}
	return &v, nil
}

// ListAcceptedVersions returns all accepted versions for a tool, newest first.
//
// ListAcceptedVersions 返回工具所有已接受版本，最新在前。
func (s *Store) ListAcceptedVersions(ctx context.Context, toolID string) ([]*tooldomain.ToolVersion, error) {
	userID, err := uid(ctx)
	if err != nil {
		return nil, err
	}
	var rows []*tooldomain.ToolVersion
	if err = s.db.WithContext(ctx).
		Where("tool_id = ? AND user_id = ? AND status = ?",
			toolID, userID, tooldomain.VersionStatusAccepted).
		Order("version DESC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("toolstore.ListAcceptedVersions: %w", err)
	}
	return rows, nil
}

// UpdateVersionStatus updates the status and optionally the version number.
//
// UpdateVersionStatus 更新 status 字段，可选分配版本号。
func (s *Store) UpdateVersionStatus(ctx context.Context, id, status string, version *int) error {
	updates := map[string]any{"status": status}
	if version != nil {
		updates["version"] = *version
	}
	if err := s.db.WithContext(ctx).
		Model(&tooldomain.ToolVersion{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("toolstore.UpdateVersionStatus: %w", err)
	}
	return nil
}

// CountAcceptedVersions returns the number of accepted versions for a tool.
//
// CountAcceptedVersions 返回工具已接受版本数。
func (s *Store) CountAcceptedVersions(ctx context.Context, toolID string) (int64, error) {
	var n int64
	if err := s.db.WithContext(ctx).Model(&tooldomain.ToolVersion{}).
		Where("tool_id = ? AND status = ?", toolID, tooldomain.VersionStatusAccepted).
		Count(&n).Error; err != nil {
		return 0, fmt.Errorf("toolstore.CountAcceptedVersions: %w", err)
	}
	return n, nil
}

// DeleteOldestAcceptedVersion hard-deletes the accepted version with the
// lowest version number for the given tool.
//
// DeleteOldestAcceptedVersion 硬删除指定工具版本号最小的已接受版本。
func (s *Store) DeleteOldestAcceptedVersion(ctx context.Context, toolID string) error {
	var v tooldomain.ToolVersion
	err := s.db.WithContext(ctx).
		Where("tool_id = ? AND status = ?", toolID, tooldomain.VersionStatusAccepted).
		Order("version ASC").
		First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("toolstore.DeleteOldestAcceptedVersion: find: %w", err)
	}
	if err = s.db.WithContext(ctx).Delete(&v).Error; err != nil {
		return fmt.Errorf("toolstore.DeleteOldestAcceptedVersion: delete: %w", err)
	}
	return nil
}

// ── Test cases ────────────────────────────────────────────────────────────────

// SaveTestCase inserts a ToolTestCase.
//
// SaveTestCase 插入 ToolTestCase。
func (s *Store) SaveTestCase(ctx context.Context, tc *tooldomain.ToolTestCase) error {
	if err := s.db.WithContext(ctx).Create(tc).Error; err != nil {
		return fmt.Errorf("toolstore.SaveTestCase: %w", err)
	}
	return nil
}

// GetTestCase fetches a test case by id.
//
// GetTestCase 按 id 查测试用例。
func (s *Store) GetTestCase(ctx context.Context, id string) (*tooldomain.ToolTestCase, error) {
	userID, err := uid(ctx)
	if err != nil {
		return nil, err
	}
	var tc tooldomain.ToolTestCase
	err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		First(&tc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, tooldomain.ErrTestCaseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("toolstore.GetTestCase: %w", err)
	}
	return &tc, nil
}

// ListTestCases returns all test cases for the given tool, ordered by created_at ASC.
//
// ListTestCases 返回指定工具所有测试用例，按 created_at ASC 排序。
func (s *Store) ListTestCases(ctx context.Context, toolID string) ([]*tooldomain.ToolTestCase, error) {
	userID, err := uid(ctx)
	if err != nil {
		return nil, err
	}
	var rows []*tooldomain.ToolTestCase
	if err = s.db.WithContext(ctx).
		Where("tool_id = ? AND user_id = ?", toolID, userID).
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("toolstore.ListTestCases: %w", err)
	}
	return rows, nil
}

// DeleteTestCase hard-deletes a test case by id.
//
// DeleteTestCase 硬删除测试用例。
func (s *Store) DeleteTestCase(ctx context.Context, id string) error {
	userID, err := uid(ctx)
	if err != nil {
		return err
	}
	if err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, userID).
		Delete(&tooldomain.ToolTestCase{}).Error; err != nil {
		return fmt.Errorf("toolstore.DeleteTestCase: %w", err)
	}
	return nil
}

// ── Run history ───────────────────────────────────────────────────────────────

// SaveRunHistory inserts a ToolRunHistory record.
//
// SaveRunHistory 插入 ToolRunHistory 记录。
func (s *Store) SaveRunHistory(ctx context.Context, h *tooldomain.ToolRunHistory) error {
	if err := s.db.WithContext(ctx).Create(h).Error; err != nil {
		return fmt.Errorf("toolstore.SaveRunHistory: %w", err)
	}
	return nil
}

// ListRunHistory returns the most recent limit records, ordered by created_at DESC.
//
// ListRunHistory 返回最近 limit 条运行历史，按 created_at DESC。
func (s *Store) ListRunHistory(ctx context.Context, toolID string, limit int) ([]*tooldomain.ToolRunHistory, error) {
	userID, err := uid(ctx)
	if err != nil {
		return nil, err
	}
	var rows []*tooldomain.ToolRunHistory
	if err = s.db.WithContext(ctx).
		Where("tool_id = ? AND user_id = ?", toolID, userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("toolstore.ListRunHistory: %w", err)
	}
	return rows, nil
}

// CountRunHistory returns the total run history count for a tool.
//
// CountRunHistory 返回工具运行历史总条数。
func (s *Store) CountRunHistory(ctx context.Context, toolID string) (int64, error) {
	var n int64
	if err := s.db.WithContext(ctx).Model(&tooldomain.ToolRunHistory{}).
		Where("tool_id = ?", toolID).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("toolstore.CountRunHistory: %w", err)
	}
	return n, nil
}

// DeleteOldestRunHistory hard-deletes the oldest run history record for a tool.
//
// DeleteOldestRunHistory 硬删除工具最早的运行历史记录。
func (s *Store) DeleteOldestRunHistory(ctx context.Context, toolID string) error {
	var h tooldomain.ToolRunHistory
	err := s.db.WithContext(ctx).
		Where("tool_id = ?", toolID).
		Order("created_at ASC").
		First(&h).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("toolstore.DeleteOldestRunHistory: find: %w", err)
	}
	if err = s.db.WithContext(ctx).Delete(&h).Error; err != nil {
		return fmt.Errorf("toolstore.DeleteOldestRunHistory: delete: %w", err)
	}
	return nil
}

// ── Test history ──────────────────────────────────────────────────────────────

// SaveTestHistory inserts a ToolTestHistory record.
//
// SaveTestHistory 插入 ToolTestHistory 记录。
func (s *Store) SaveTestHistory(ctx context.Context, h *tooldomain.ToolTestHistory) error {
	if err := s.db.WithContext(ctx).Create(h).Error; err != nil {
		return fmt.Errorf("toolstore.SaveTestHistory: %w", err)
	}
	return nil
}

// ListTestHistory returns the most recent limit records for a tool, DESC.
//
// ListTestHistory 返回工具最近 limit 条测试历史，按 created_at DESC。
func (s *Store) ListTestHistory(ctx context.Context, toolID string, limit int) ([]*tooldomain.ToolTestHistory, error) {
	userID, err := uid(ctx)
	if err != nil {
		return nil, err
	}
	var rows []*tooldomain.ToolTestHistory
	if err = s.db.WithContext(ctx).
		Where("tool_id = ? AND user_id = ?", toolID, userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("toolstore.ListTestHistory: %w", err)
	}
	return rows, nil
}

// ListTestHistoryByBatch returns all records sharing a batchID, ordered ASC.
//
// ListTestHistoryByBatch 返回指定 batchID 的所有记录，按 created_at ASC。
func (s *Store) ListTestHistoryByBatch(ctx context.Context, batchID string) ([]*tooldomain.ToolTestHistory, error) {
	var rows []*tooldomain.ToolTestHistory
	if err := s.db.WithContext(ctx).
		Where("batch_id = ?", batchID).
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("toolstore.ListTestHistoryByBatch: %w", err)
	}
	return rows, nil
}

// CountTestHistory returns the total test history count for a tool.
//
// CountTestHistory 返回工具测试历史总条数。
func (s *Store) CountTestHistory(ctx context.Context, toolID string) (int64, error) {
	var n int64
	if err := s.db.WithContext(ctx).Model(&tooldomain.ToolTestHistory{}).
		Where("tool_id = ?", toolID).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("toolstore.CountTestHistory: %w", err)
	}
	return n, nil
}

// DeleteOldestTestHistory hard-deletes the oldest test history record for a tool.
//
// DeleteOldestTestHistory 硬删除工具最早的测试历史记录。
func (s *Store) DeleteOldestTestHistory(ctx context.Context, toolID string) error {
	var h tooldomain.ToolTestHistory
	err := s.db.WithContext(ctx).
		Where("tool_id = ?", toolID).
		Order("created_at ASC").
		First(&h).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("toolstore.DeleteOldestTestHistory: find: %w", err)
	}
	if err = s.db.WithContext(ctx).Delete(&h).Error; err != nil {
		return fmt.Errorf("toolstore.DeleteOldestTestHistory: delete: %w", err)
	}
	return nil
}

// ── Cursor helpers ────────────────────────────────────────────────────────────

type pageCursor struct {
	CreatedAt time.Time `json:"t"`
	ID        string    `json:"i"`
}

func encodeCursor(c pageCursor) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("encodeCursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func decodeCursor(s string) (pageCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return pageCursor{}, fmt.Errorf("decodeCursor: %w", err)
	}
	var c pageCursor
	if err = json.Unmarshal(b, &c); err != nil {
		return pageCursor{}, fmt.Errorf("decodeCursor: %w", err)
	}
	return c, nil
}
