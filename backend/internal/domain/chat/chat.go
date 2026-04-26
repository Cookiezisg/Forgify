// Package chat is the domain layer for conversation messaging.
// It owns the Message and Attachment entities, their status lifecycle,
// sentinel errors, and the storage contract (Repository). The chat
// domain does NOT contain any LLM orchestration logic — that lives in
// app/chat, which depends on Eino.
//
// Package chat 是对话消息的 domain 层。拥有 Message 和 Attachment 实体、
// 状态生命周期、sentinel 错误及存储契约（Repository）。
// chat domain 不含任何 LLM 编排逻辑——那部分在 app/chat，依赖 Eino。
package chat

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ── Message ──────────────────────────────────────────────────────────────────

// Message is one turn in a conversation. Role determines whose voice it carries;
// Status tracks the generation lifecycle for assistant messages.
//
// Message 是对话中的一个回合。Role 决定发言方；
// Status 追踪 assistant 消息的生成生命周期。
type Message struct {
	ID               string         `gorm:"primaryKey;type:text" json:"id"`
	ConversationID   string         `gorm:"not null;index;type:text" json:"conversationId"`
	UserID           string         `gorm:"not null;type:text" json:"-"`
	Role             string         `gorm:"not null;type:text" json:"role"`
	Content          string         `gorm:"not null;type:text" json:"content"`
	Status           string         `gorm:"not null;type:text;default:'completed'" json:"status"`
	StopReason       string         `gorm:"type:text;default:''" json:"stopReason,omitempty"`
	TokenUsage       string         `gorm:"type:text;default:''" json:"tokenUsage,omitempty"`       // JSON: {inputTokens,outputTokens,cacheReadTokens}
	ToolCalls        string         `gorm:"type:text;default:''" json:"toolCalls,omitempty"`        // JSON array of tool call objects
	ToolCallID       string         `gorm:"type:text;default:''" json:"toolCallId,omitempty"`       // tool role: links to assistant message
	ReasoningContent string         `gorm:"type:text;default:''" json:"reasoningContent,omitempty"` // thinking-mode: must be echoed back to API
	AttachmentIDs    string         `gorm:"type:text;default:''" json:"attachmentIds,omitempty"`    // JSON array of att_<16hex>
	CreatedAt        time.Time      `json:"createdAt"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName locks the DB table to "messages".
//
// TableName 把表名锁定为 "messages"。
func (Message) TableName() string { return "messages" }

// Role values for Message.Role.
//
// Message.Role 的取值。
const (
	RoleUser      = "user"      // 用户输入
	RoleAssistant = "assistant" // LLM 输出（文字回复或 tool call 指令）
	RoleTool      = "tool"      // Tool 执行结果，ToolCallID 关联 assistant 消息
)

// Status values for Message.Status.
//
// Message.Status 的取值。
const (
	StatusPending   = "pending"   // 已入队，等待 Agent 处理
	StatusStreaming = "streaming" // 正在流式输出
	StatusCompleted = "completed" // 正常完成
	StatusError     = "error"     // 出错中止
	StatusCancelled = "cancelled" // 用户取消
)

// StopReason values for Message.StopReason (assistant messages only).
//
// Message.StopReason 的取值（仅 assistant 消息）。
const (
	StopReasonEndTurn   = "end_turn"   // 正常结束
	StopReasonMaxTokens = "max_tokens" // 达到 token 上限，回复被截断
	StopReasonCancelled = "cancelled"  // 用户主动取消
	StopReasonError     = "error"      // 出错中止
)

// ListFilter is the query shape for Repository.ListByConversation.
//
// ListFilter 是 Repository.ListByConversation 的查询形状。
type ListFilter struct {
	Cursor string
	Limit  int
}

// ── Attachment ────────────────────────────────────────────────────────────────

// Attachment is a file uploaded by the user and associated with a message.
// The file bytes are stored on disk at StoragePath; the DB row holds only
// metadata. StoragePath is not exposed in API responses (json:"-").
//
// Attachment 是用户上传并关联到消息的文件。文件字节存在 StoragePath 的磁盘上；
// DB 行只存元数据。StoragePath 不在 API 响应中暴露（json:"-"）。
type Attachment struct {
	ID          string    `gorm:"primaryKey;type:text" json:"id"` // att_<16hex>
	UserID      string    `gorm:"not null;type:text" json:"-"`
	FileName    string    `gorm:"not null;type:text" json:"fileName"`
	MimeType    string    `gorm:"not null;type:text" json:"mimeType"`
	SizeBytes   int64     `gorm:"not null" json:"sizeBytes"`
	StoragePath string    `gorm:"not null;type:text" json:"-"` // 相对 dataDir，不对外暴露
	CreatedAt   time.Time `json:"createdAt"`
}

// TableName locks the DB table to "chat_attachments".
//
// TableName 把表名锁定为 "chat_attachments"。
func (Attachment) TableName() string { return "chat_attachments" }

// MaxAttachmentBytes is the upload size limit (50 MB).
//
// MaxAttachmentBytes 是上传大小限制（50 MB）。
const MaxAttachmentBytes = 50 * 1024 * 1024

// ── Sentinel errors ───────────────────────────────────────────────────────────

// Sentinel errors. Mapped to HTTP responses by
// transport/httpapi/response/errmap.go.
//
// Sentinel 错误。由 transport/httpapi/response/errmap.go 映射到 HTTP 响应。
var (
	// ErrMessageNotFound: message id does not match any live record.
	// ErrMessageNotFound：message id 未命中任何活跃记录。
	ErrMessageNotFound = errors.New("chat: message not found")

	// ErrStreamNotFound: cancel request on a conversation with no active Agent.
	// ErrStreamNotFound：取消请求时对话没有正在运行的 Agent。
	ErrStreamNotFound = errors.New("chat: no active stream for conversation")

	// ErrStreamInProgress: send a request while a stream is already running.
	// ErrStreamInProgress：发送消息时该对话已有 Agent 在运行。
	ErrStreamInProgress = errors.New("chat: stream already in progress")

	// ErrProviderUnavailable: upstream LLM returned a non-401 error.
	// ErrProviderUnavailable：上游 LLM 返回非 401 错误。
	ErrProviderUnavailable = errors.New("chat: LLM provider unavailable")

	// ErrAttachmentTooLarge: file exceeds MaxAttachmentBytes (50 MB).
	// ErrAttachmentTooLarge：文件超过 MaxAttachmentBytes（50 MB）。
	ErrAttachmentTooLarge = errors.New("chat: attachment exceeds 50 MB limit")

	// ErrAttachmentTypeUnsupported: no extractor can handle this MIME type.
	// ErrAttachmentTypeUnsupported：没有提取器能处理该 MIME 类型。
	ErrAttachmentTypeUnsupported = errors.New("chat: attachment type not supported")

	// ErrAttachmentParseFailed: file is corrupt or extraction failed.
	// ErrAttachmentParseFailed：文件损坏或内容提取失败。
	ErrAttachmentParseFailed = errors.New("chat: attachment parse failed")

	// ErrVisionNotSupported: the selected provider does not support image input.
	// ErrVisionNotSupported：当前 provider 不支持图片输入。
	ErrVisionNotSupported = errors.New("chat: provider does not support vision")
)

// ── Repository ────────────────────────────────────────────────────────────────

// Repository is the storage contract for Message and Attachment.
// Implementations scope every query to the userID carried in ctx.
//
// Implemented by: infra/store/chat.Store
// Consumer:       app/chat.Service (only)
//
// Repository 是 Message 和 Attachment 的存储契约。
// 实现按 ctx 中的 userID 过滤所有查询。
//
// 实现：infra/store/chat.Store
// 消费：仅 app/chat.Service
type Repository interface {
	// Save inserts or updates a Message by primary key.
	//
	// Save 按主键插入或更新 Message。
	Save(ctx context.Context, m *Message) error

	// Get fetches a single Message by id, scoped to the current user.
	// Returns ErrMessageNotFound if no live record matches.
	//
	// Get 按 id 查单条，按当前用户过滤。未命中活跃记录返回 ErrMessageNotFound。
	Get(ctx context.Context, id string) (*Message, error)

	// ListByConversation returns a cursor-paginated page of messages for
	// the given conversation, ordered by created_at ASC (chronological).
	//
	// ListByConversation 返回指定对话的 cursor 分页消息，按 created_at ASC 排序。
	ListByConversation(ctx context.Context, conversationID string, filter ListFilter) ([]*Message, string, error)

	// SaveAttachment inserts an Attachment record.
	//
	// SaveAttachment 插入 Attachment 记录。
	SaveAttachment(ctx context.Context, a *Attachment) error

	// GetAttachment fetches an Attachment by id, scoped to the current user.
	//
	// GetAttachment 按 id 查 Attachment，按当前用户过滤。
	GetAttachment(ctx context.Context, id string) (*Attachment, error)
}
