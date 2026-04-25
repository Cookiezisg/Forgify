// Package chat (infra/store/chat) is the GORM-backed implementation of
// the domain chat Repository port. Every method scopes queries by the
// userID carried in ctx — callers MUST have run the InjectUserID middleware.
//
// Package chat（infra/store/chat）是 domain chat Repository port 的 GORM 实现。
// 所有方法按 ctx 中的 userID 过滤——调用方必须先经过 InjectUserID 中间件。
package chat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	"github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Store is the GORM implementation of chatdomain.Repository.
//
// Store 是 chatdomain.Repository 的 GORM 实现。
type Store struct {
	db *gorm.DB
}

// New constructs a Store bound to the given *gorm.DB.
//
// New 基于给定 *gorm.DB 构造 Store。
func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

func userID(ctx context.Context) (string, error) {
	id, ok := reqctx.GetUserID(ctx)
	if !ok {
		return "", fmt.Errorf("chatstore: missing user id in context")
	}
	return id, nil
}

// Save inserts or updates a Message by primary key.
//
// Save 按主键插入或更新 Message。
func (s *Store) Save(ctx context.Context, m *chatdomain.Message) error {
	if err := s.db.WithContext(ctx).Save(m).Error; err != nil {
		return fmt.Errorf("chatstore.Save: %w", err)
	}
	return nil
}

// Get fetches a single Message by id, scoped to the current user.
// Returns ErrMessageNotFound if no live record matches.
//
// Get 按 id 查单条，按当前用户过滤。未命中活跃记录返回 ErrMessageNotFound。
func (s *Store) Get(ctx context.Context, id string) (*chatdomain.Message, error) {
	uid, err := userID(ctx)
	if err != nil {
		return nil, err
	}
	var m chatdomain.Message
	err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, uid).
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, chatdomain.ErrMessageNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("chatstore.Get: %w", err)
	}
	return &m, nil
}

// ListByConversation returns a cursor-paginated page of messages ordered
// by created_at ASC (oldest first — chronological chat history).
// Uses a (created_at, id) tuple cursor for stable pagination.
//
// ListByConversation 返回按 created_at ASC（最旧优先）排序的 cursor 分页消息。
// 使用 (created_at, id) 元组 cursor 保证分页稳定。
func (s *Store) ListByConversation(ctx context.Context, conversationID string, filter chatdomain.ListFilter) ([]*chatdomain.Message, string, error) {
	uid, err := userID(ctx)
	if err != nil {
		return nil, "", err
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	q := s.db.WithContext(ctx).
		Where("conversation_id = ? AND user_id = ?", conversationID, uid)
	if filter.Cursor != "" {
		c, err := decodeCursor(filter.Cursor)
		if err != nil {
			return nil, "", fmt.Errorf("chatstore.ListByConversation: %w", err)
		}
		// Rows strictly "newer" than cursor (ASC order).
		// 严格比 cursor "更新"的行（ASC 顺序）。
		q = q.Where("(created_at, id) > (?, ?)", c.CreatedAt, c.ID)
	}
	var rows []*chatdomain.Message
	if err := q.Order("created_at ASC, id ASC").
		Limit(limit + 1).
		Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("chatstore.ListByConversation: %w", err)
	}
	var next string
	if len(rows) > limit {
		last := rows[limit-1]
		next, err = encodeCursor(pageCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, "", fmt.Errorf("chatstore.ListByConversation: %w", err)
		}
		rows = rows[:limit]
	}
	return rows, next, nil
}

// SaveAttachment inserts an Attachment record (no upsert — attachments are
// write-once; the file on disk is immutable after upload).
//
// SaveAttachment 插入 Attachment 记录（不 upsert——附件上传后磁盘文件不可变）。
func (s *Store) SaveAttachment(ctx context.Context, a *chatdomain.Attachment) error {
	if err := s.db.WithContext(ctx).Create(a).Error; err != nil {
		return fmt.Errorf("chatstore.SaveAttachment: %w", err)
	}
	return nil
}

// GetAttachment fetches an Attachment by id, scoped to the current user.
//
// GetAttachment 按 id 查 Attachment，按当前用户过滤。
func (s *Store) GetAttachment(ctx context.Context, id string) (*chatdomain.Attachment, error) {
	uid, err := userID(ctx)
	if err != nil {
		return nil, err
	}
	var a chatdomain.Attachment
	err = s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", id, uid).
		First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("chatstore.GetAttachment: attachment %q not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("chatstore.GetAttachment: %w", err)
	}
	return &a, nil
}

// pageCursor is the opaque continuation token for ListByConversation.
//
// pageCursor 是 ListByConversation 的不透明续传 token。
type pageCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        string    `json:"i"`
}

func encodeCursor(c pageCursor) (string, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("encode cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeCursor(s string) (pageCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return pageCursor{}, fmt.Errorf("decode cursor: %w", err)
	}
	var c pageCursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return pageCursor{}, fmt.Errorf("decode cursor: %w", err)
	}
	return c, nil
}
