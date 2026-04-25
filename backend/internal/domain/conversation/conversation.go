// Package conversation is the domain layer for conversation thread management.
// A Conversation is a named container for a chat session. It holds no
// messages itself — the chat domain owns message history.
//
// Package conversation 是对话线程管理的 domain 层。
// Conversation 是聊天会话的命名容器，本身不含消息——消息历史由 chat domain 管理。
package conversation

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// Conversation is a chat thread container. Title may be empty until the
// user renames it or Phase 5 auto-names it from the first exchange.
//
// Conversation 是对话线程容器。Title 可为空，待用户手动改名或
// Phase 5 根据首轮对话自动命名。
type Conversation struct {
	ID           string         `gorm:"primaryKey;type:text" json:"id"`
	UserID       string         `gorm:"not null;index;type:text" json:"-"`
	Title        string         `gorm:"not null;type:text;default:''" json:"title"`
	AutoTitled   bool           `gorm:"not null;default:false" json:"autoTitled"`
	SystemPrompt string         `gorm:"type:text;default:''" json:"systemPrompt,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName locks the DB table to "conversations".
//
// TableName 把表名锁定为 "conversations"。
func (Conversation) TableName() string { return "conversations" }

// ListFilter is the query shape accepted by Repository.List.
//
// ListFilter 是 Repository.List 接受的查询形状。
type ListFilter struct {
	Cursor string
	Limit  int
}

// ErrNotFound is returned when a conversation id does not match any live record.
//
// ErrNotFound：conversation id 未命中任何活跃记录。
var ErrNotFound = errors.New("conversation: not found")

// Repository is the storage contract for Conversation.
// Implementations scope every query to the userID in ctx.
//
// Repository 是 Conversation 的存储契约。实现按 ctx 中的 userID 过滤。
type Repository interface {
	// Save inserts or updates by primary key.
	//
	// Save 按主键插入或更新。
	Save(ctx context.Context, c *Conversation) error

	// Get fetches one Conversation by id, scoped to the current user.
	// Returns ErrNotFound if no live record matches.
	//
	// Get 按 id 查单条，按当前用户过滤。未命中活跃记录返回 ErrNotFound。
	Get(ctx context.Context, id string) (*Conversation, error)

	// List returns a page of conversations for the current user, newest first.
	//
	// List 返回当前用户的一页对话，最新优先。
	List(ctx context.Context, filter ListFilter) ([]*Conversation, string, error)

	// Delete soft-deletes by id, scoped to the current user.
	// Returns ErrNotFound if no live record was matched.
	//
	// Delete 按 id 软删除，按当前用户过滤。未命中返回 ErrNotFound。
	Delete(ctx context.Context, id string) error
}
