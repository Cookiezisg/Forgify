// Package chat is the domain layer for conversation messaging.
// It owns the Message and Block entities, their lifecycle constants,
// sentinel errors, and the storage contract (Repository).
// No LLM orchestration logic lives here — that belongs in app/chat.
//
// Package chat 是对话消息的 domain 层。
// 拥有 Message 和 Block 实体、生命周期常量、sentinel 错误及存储契约（Repository）。
// 不含 LLM 编排逻辑——那部分属于 app/chat。
package chat

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ── Message ───────────────────────────────────────────────────────────────────

// Message is one turn in a conversation. Role identifies the speaker;
// Status tracks the generation lifecycle for assistant messages.
// Content is stored in the associated Blocks, not directly on Message.
//
// Message 是对话中的一个回合。Role 标识发言方；
// Status 追踪 assistant 消息的生成生命周期。
// 内容存储在关联的 Blocks 中，不直接在 Message 上。
type Message struct {
	ID             string         `gorm:"primaryKey;type:text" json:"id"`
	ConversationID string         `gorm:"not null;index;type:text" json:"conversationId"`
	UserID         string         `gorm:"not null;type:text" json:"-"`
	Role           string         `gorm:"not null;type:text" json:"role"` // "user" | "assistant"
	Status         string         `gorm:"not null;type:text;default:'completed'" json:"status"`
	StopReason     string         `gorm:"type:text;default:''" json:"stopReason,omitempty"`
	InputTokens    int            `gorm:"default:0" json:"inputTokens,omitempty"`
	OutputTokens   int            `gorm:"default:0" json:"outputTokens,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`

	// Blocks is not a DB column — populated by the store layer after a query.
	// Blocks 不是 DB 列，由 store 层查询后填充。
	Blocks []Block `gorm:"-" json:"blocks"`
}

// TableName locks the DB table to "messages".
// TableName 把表名锁定为 "messages"。
func (Message) TableName() string { return "messages" }

// Role values for Message.Role.
// Message.Role 的取值。
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Status values for Message.Status.
// Message.Status 的取值。
const (
	StatusPending   = "pending"
	StatusStreaming = "streaming"
	StatusCompleted = "completed"
	StatusError     = "error"
	StatusCancelled = "cancelled"
)

// StopReason values for Message.StopReason (assistant messages only).
// Message.StopReason 的取值（仅 assistant 消息）。
const (
	StopReasonEndTurn   = "end_turn"
	StopReasonMaxTokens = "max_tokens"
	StopReasonCancelled = "cancelled"
	StopReasonError     = "error"
)

// ── Block ─────────────────────────────────────────────────────────────────────

// Block is one typed content element within a Message.
// All content lives in Blocks — a Message row holds only metadata.
//
// Block 是 Message 中一个有类型的内容元素。
// 所有内容都在 Block 中——Message 行只存元数据。
type Block struct {
	ID        string    `gorm:"primaryKey;type:text" json:"id"`
	MessageID string    `gorm:"not null;index;type:text" json:"-"`
	Seq       int       `gorm:"not null" json:"seq"`
	Type      string    `gorm:"not null;type:text" json:"type"`
	Data      string    `gorm:"not null;type:text" json:"data"` // JSON, structure varies by Type
	CreatedAt time.Time `json:"createdAt"`
}

// TableName locks the DB table to "message_blocks".
// TableName 把表名锁定为 "message_blocks"。
func (Block) TableName() string { return "message_blocks" }

// Block type constants.
// Block 类型常量。
const (
	BlockTypeText          = "text"
	BlockTypeReasoning     = "reasoning"
	BlockTypeToolCall      = "tool_call"
	BlockTypeToolResult    = "tool_result"
	BlockTypeAttachmentRef = "attachment_ref"
)

// ── Block data shapes ─────────────────────────────────────────────────────────
// These structs are used by app/chat to marshal/unmarshal Block.Data JSON.
// 这些结构体供 app/chat 序列化/反序列化 Block.Data JSON。

// TextData is the Data payload for BlockTypeText and BlockTypeReasoning.
// TextData 是 BlockTypeText 和 BlockTypeReasoning 的 Data 载荷。
type TextData struct {
	Text string `json:"text"`
}

// ToolCallData is the Data payload for BlockTypeToolCall.
// Arguments never contains "summary" — that is stored separately.
//
// ToolCallData 是 BlockTypeToolCall 的 Data 载荷。
// Arguments 不含 "summary"，summary 单独存储。
type ToolCallData struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Summary   string         `json:"summary"`   // LLM-provided one-liner, may be empty
	Arguments map[string]any `json:"arguments"` // stripped of "summary"
}

// ToolResultData is the Data payload for BlockTypeToolResult.
// ToolResultData 是 BlockTypeToolResult 的 Data 载荷。
type ToolResultData struct {
	ToolCallID string `json:"toolCallId"`
	OK         bool   `json:"ok"`
	Result     string `json:"result"`
}

// AttachmentRefData is the Data payload for BlockTypeAttachmentRef.
// AttachmentRefData 是 BlockTypeAttachmentRef 的 Data 载荷。
type AttachmentRefData struct {
	AttachmentID string `json:"attachmentId"`
	FileName     string `json:"fileName"`
	MimeType     string `json:"mimeType"`
}

// ── Attachment ────────────────────────────────────────────────────────────────

// Attachment is a file uploaded by the user. File bytes are stored on disk
// at StoragePath; the DB row holds only metadata.
//
// Attachment 是用户上传的文件。文件字节存在磁盘的 StoragePath；DB 行只存元数据。
type Attachment struct {
	ID          string    `gorm:"primaryKey;type:text" json:"id"`
	UserID      string    `gorm:"not null;type:text" json:"-"`
	FileName    string    `gorm:"not null;type:text" json:"fileName"`
	MimeType    string    `gorm:"not null;type:text" json:"mimeType"`
	SizeBytes   int64     `gorm:"not null" json:"sizeBytes"`
	StoragePath string    `gorm:"not null;type:text" json:"-"`
	CreatedAt   time.Time `json:"createdAt"`
}

// TableName locks the DB table to "chat_attachments".
// TableName 把表名锁定为 "chat_attachments"。
func (Attachment) TableName() string { return "chat_attachments" }

// MaxAttachmentBytes is the upload size limit (50 MB).
// MaxAttachmentBytes 是上传大小限制（50 MB）。
const MaxAttachmentBytes = 50 * 1024 * 1024

// ── ListFilter ────────────────────────────────────────────────────────────────

// ListFilter is the query shape for paginated message listing.
// ListFilter 是分页消息列表的查询形状。
type ListFilter struct {
	Cursor string
	Limit  int
}

// ── Sentinel errors ───────────────────────────────────────────────────────────

var (
	ErrMessageNotFound           = errors.New("chat: message not found")
	ErrStreamNotFound            = errors.New("chat: no active stream for conversation")
	ErrStreamInProgress          = errors.New("chat: stream already in progress")
	ErrProviderUnavailable       = errors.New("chat: LLM provider unavailable")
	ErrAttachmentTooLarge        = errors.New("chat: attachment exceeds 50 MB limit")
	ErrAttachmentTypeUnsupported = errors.New("chat: attachment type not supported")
	ErrAttachmentParseFailed     = errors.New("chat: attachment parse failed")
	ErrVisionNotSupported        = errors.New("chat: provider does not support vision")
)

// ── Repository ────────────────────────────────────────────────────────────────

// Repository is the storage contract for Message, Block, and Attachment.
// Implementations scope every query to the userID in ctx.
//
// Implemented by: infra/store/chat.Store
// Consumer:       app/chat.Service only
//
// Repository 是 Message、Block 和 Attachment 的存储契约。
// 实现按 ctx 中的 userID 过滤所有查询。
type Repository interface {
	// Save inserts or updates a Message and its Blocks atomically.
	// Callers populate m.Blocks before calling; existing blocks are replaced.
	//
	// Save 原子地插入或更新 Message 及其 Blocks。
	// 调用方在调用前填充 m.Blocks；已有 blocks 会被替换。
	Save(ctx context.Context, m *Message) error

	// Get fetches a single Message by id, scoped to the current user.
	// Returns ErrMessageNotFound if no live record matches.
	//
	// Get 按 id 查单条 Message，按当前用户过滤。未命中返回 ErrMessageNotFound。
	Get(ctx context.Context, id string) (*Message, error)

	// ListByConversation returns cursor-paginated messages with their Blocks,
	// ordered by created_at ASC (chronological).
	//
	// ListByConversation 返回带 Blocks 的 cursor 分页消息，按 created_at ASC 排序。
	ListByConversation(ctx context.Context, conversationID string, filter ListFilter) ([]*Message, string, error)

	// SaveAttachment inserts an Attachment record (write-once).
	// SaveAttachment 插入 Attachment 记录（仅写一次）。
	SaveAttachment(ctx context.Context, a *Attachment) error

	// GetAttachment fetches an Attachment by id, scoped to the current user.
	// GetAttachment 按 id 查 Attachment，按当前用户过滤。
	GetAttachment(ctx context.Context, id string) (*Attachment, error)
}
