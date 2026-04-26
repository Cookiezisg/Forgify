// Package chat (app/chat) orchestrates the chat pipeline: LLM streaming,
// attachment handling, auto-titling, and SSE event publishing. It owns
// no SQL — persistence is delegated to infra/store/chat.
//
// Concurrency model: each conversation has a convQueue with a buffered task
// channel. A single worker goroutine drains the channel sequentially, so
// messages within one conversation always execute one at a time in order.
//
// Package chat（app/chat）编排聊天管线：LLM 流式输出、附件处理、自动命名、
// SSE 事件推送。不含 SQL——持久化委托给 infra/store/chat。
//
// 并发模型：每个 conversation 拥有一个带缓冲任务 channel 的 convQueue。
// 单个 worker goroutine 顺序消费队列，保证同一 conversation 的消息始终按序、逐条执行。
//
// Files:
//
//	chat.go     — public API
//	pipeline.go — ReAct loop: stream, tools, SSE, DB writes
//	util.go     — shared helpers
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/cloudwego/eino/components/tool"
	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	"github.com/sunweilin/forgify/backend/internal/domain/events"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	einoinfra "github.com/sunweilin/forgify/backend/internal/infra/eino"
	"github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// queueCapacity is the maximum number of messages that can be queued
// behind the currently running Agent for one conversation.
//
// queueCapacity 是单个 conversation 在当前 Agent 之后最多排队的消息数。
const queueCapacity = 5

// convQueue manages sequential Agent execution for one conversation.
//
// convQueue 管理单个 conversation 的顺序 Agent 执行。
type convQueue struct {
	ch     chan queuedTask
	mu     sync.Mutex
	cancel context.CancelFunc // nil when idle; set while Agent is running
}

// queuedTask is one pending chat turn waiting to be processed.
//
// queuedTask 是等待处理的一次对话请求。
type queuedTask struct {
	ctx  context.Context
	conv *convdomain.Conversation
	uid  string
}

// Service orchestrates LLM calls, attachment handling, and SSE event publishing.
//
// Service 编排 LLM 调用、附件处理和 SSE 事件推送。
type Service struct {
	repo         chatdomain.Repository
	convRepo     convdomain.Repository
	modelPicker  modeldomain.ModelPicker
	keyProvider  apikeydomain.KeyProvider
	modelFactory einoinfra.ChatModelFactory
	tools        []tool.BaseTool
	bridge       events.Bridge
	dataDir      string
	log          *zap.Logger
	queues       sync.Map // conversationID → *convQueue
}

// NewService wires Service dependencies. Panics on nil logger.
//
// NewService 装配依赖。nil logger 立刻 panic。
func NewService(
	repo chatdomain.Repository,
	convRepo convdomain.Repository,
	modelPicker modeldomain.ModelPicker,
	keyProvider apikeydomain.KeyProvider,
	modelFactory einoinfra.ChatModelFactory,
	bridge events.Bridge,
	dataDir string,
	log *zap.Logger,
) *Service {
	if log == nil {
		panic("chat.NewService: logger is nil")
	}
	if dataDir == "" {
		dataDir = filepath.Join(os.TempDir(), "forgify")
	}
	return &Service{
		repo:         repo,
		convRepo:     convRepo,
		modelPicker:  modelPicker,
		keyProvider:  keyProvider,
		modelFactory: modelFactory,
		bridge:       bridge,
		dataDir:      dataDir,
		log:          log,
	}
}

// SetTools injects system tools into the ReAct Agent.
// Safe to call before any conversation starts.
//
// SetTools 将 system tools 注入 ReAct Agent，在任何对话启动前调用均安全。
func (s *Service) SetTools(tools []tool.BaseTool) {
	s.tools = tools
}

// SendInput is the payload for Service.Send.
//
// SendInput 是 Service.Send 的请求载荷。
type SendInput struct {
	Content       string
	AttachmentIDs []string
}

// UploadAttachment copies fileBytes to the data directory, stores metadata
// in DB, and returns the created Attachment.
//
// UploadAttachment 把 fileBytes 复制到 data 目录，把元数据存入 DB，
// 返回创建好的 Attachment。
func (s *Service) UploadAttachment(ctx context.Context, fileBytes []byte, mimeType, fileName string) (*chatdomain.Attachment, error) {
	if int64(len(fileBytes)) > chatdomain.MaxAttachmentBytes {
		return nil, chatdomain.ErrAttachmentTooLarge
	}
	uid, ok := reqctx.GetUserID(ctx)
	if !ok {
		return nil, fmt.Errorf("chat.Service.UploadAttachment: missing user id in context")
	}

	id := newAttachmentID()
	ext := filepath.Ext(fileName)
	storageDir := filepath.Join(s.dataDir, "attachments", id)
	storagePath := filepath.Join(storageDir, "original"+ext)

	if err := os.MkdirAll(storageDir, 0750); err != nil {
		return nil, fmt.Errorf("chat.Service.UploadAttachment: mkdir: %w", err)
	}
	if err := os.WriteFile(storagePath, fileBytes, 0640); err != nil {
		return nil, fmt.Errorf("chat.Service.UploadAttachment: write: %w", err)
	}

	a := &chatdomain.Attachment{
		ID:          id,
		UserID:      uid,
		FileName:    fileName,
		MimeType:    mimeType,
		SizeBytes:   int64(len(fileBytes)),
		StoragePath: storagePath,
	}
	if err := s.repo.SaveAttachment(ctx, a); err != nil {
		_ = os.RemoveAll(storageDir)
		return nil, err
	}
	return a, nil
}

// Send saves the user message and enqueues an Agent task for this conversation.
// Returns immediately with the user message ID (202 semantics). If a previous
// message is still streaming, this one waits in the queue rather than failing.
// Returns ErrStreamInProgress only when the queue is full.
//
// Send 保存用户消息并把 Agent 任务加入该 conversation 的队列，立刻返回
// 用户消息 ID（202 语义）。若上一条消息仍在流式输出，本条在队列中等待而非报错；
// 仅在队列已满时返回 ErrStreamInProgress。
func (s *Service) Send(ctx context.Context, conversationID string, in SendInput) (string, error) {
	conv, err := s.convRepo.Get(ctx, conversationID)
	if err != nil {
		return "", err
	}
	uid, ok := reqctx.GetUserID(ctx)
	if !ok {
		return "", fmt.Errorf("chat.Service.Send: missing user id in context")
	}

	attIDsJSON, _ := json.Marshal(in.AttachmentIDs)
	userMsg := &chatdomain.Message{
		ID:             newMsgID(),
		ConversationID: conversationID,
		UserID:         uid,
		Role:           chatdomain.RoleUser,
		Content:        in.Content,
		Status:         chatdomain.StatusCompleted,
		AttachmentIDs:  string(attIDsJSON),
	}
	if err := s.repo.Save(ctx, userMsg); err != nil {
		return "", err
	}

	// Agent runs in a background context so it outlives the HTTP request.
	// Agent 在后台 context 中运行，生命周期超过 HTTP 请求。
	agentCtx := reqctx.SetUserID(context.Background(), uid)
	agentCtx = reqctx.SetLocale(agentCtx, reqctx.GetLocale(ctx))

	q := s.getOrCreateQueue(conversationID)
	task := queuedTask{ctx: agentCtx, conv: conv, uid: uid}

	select {
	case q.ch <- task:
	default:
		return "", chatdomain.ErrStreamInProgress
	}

	s.log.Info("chat task enqueued",
		zap.String("conversation_id", conversationID),
		zap.String("user_message_id", userMsg.ID),
		zap.Int("queue_depth", len(q.ch)))
	return userMsg.ID, nil
}

// Cancel stops the currently running Agent for the conversation and drains
// any pending queued tasks.
//
// Cancel 停止当前正在运行的 Agent 并清空队列中待处理的任务。
func (s *Service) Cancel(_ context.Context, conversationID string) error {
	v, ok := s.queues.Load(conversationID)
	if !ok {
		return chatdomain.ErrStreamNotFound
	}
	q := v.(*convQueue)

	q.mu.Lock()
	cancel := q.cancel
	q.mu.Unlock()

	if cancel == nil {
		return chatdomain.ErrStreamNotFound
	}
	cancel()

	for {
		select {
		case <-q.ch:
		default:
			return nil
		}
	}
}

// ListMessages returns a paginated list of messages for the conversation.
//
// ListMessages 返回对话的分页消息列表。
func (s *Service) ListMessages(ctx context.Context, conversationID string, filter chatdomain.ListFilter) ([]*chatdomain.Message, string, error) {
	return s.repo.ListByConversation(ctx, conversationID, filter)
}
