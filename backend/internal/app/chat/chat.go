// Package chat (app/chat) orchestrates the chat pipeline: LLM streaming,
// attachment handling, auto-titling, and SSE event publishing.
// It owns no SQL — persistence is delegated to infra/store/chat.
//
// Concurrency model: each conversation has a convQueue with a buffered task
// channel. A single worker goroutine drains it sequentially, so messages
// within one conversation always execute one at a time in order.
//
// Package chat（app/chat）编排聊天管线：LLM 流式输出、附件处理、
// 自动命名、SSE 事件推送。不含 SQL——持久化委托给 infra/store/chat。
//
// 并发模型：每个 conversation 拥有带缓冲任务 channel 的 convQueue。
// 单个 worker goroutine 顺序消费队列，保证同一 conversation 的消息按序逐条执行。
//
// Files:
//
//	chat.go     — public API (Send, Cancel, ListMessages, UploadAttachment)
//	runner.go   — queue management, agentRun (ReAct loop), writeDB
//	stream.go   — streamLLM, assembleBlocks, extractToolCalls, parseToolArgs
//	tools.go    — runTools (parallel), executeTool
//	history.go  — buildHistory, extendHistory, blocksToAssistantLLM
//	util.go     — ID generators, file helpers, truncate
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"

	agentapp "github.com/sunweilin/forgify/backend/internal/app/agent"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	eventsdomain "github.com/sunweilin/forgify/backend/internal/domain/events"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// queueCapacity is the maximum number of messages that can queue behind
// the currently running Agent for one conversation.
//
// queueCapacity 是单个 conversation 在当前 Agent 之后最多排队的消息数。
const queueCapacity = 5

// convQueue manages sequential Agent execution for one conversation.
//
// convQueue 管理单个 conversation 的顺序 Agent 执行。
type convQueue struct {
	ch     chan queuedTask
	mu     sync.Mutex
	cancel context.CancelFunc // nil when idle
}

// queuedTask is one pending chat turn waiting to be processed.
//
// queuedTask 是等待处理的一次对话请求。
type queuedTask struct {
	ctx       context.Context
	conv      *convdomain.Conversation
	uid       string
	userMsgID string // ID of the user message that triggered this task
}

// Service orchestrates LLM calls, attachment handling, and SSE event publishing.
//
// Service 编排 LLM 调用、附件处理和 SSE 事件推送。
type Service struct {
	repo        chatdomain.Repository
	convRepo    convdomain.Repository
	modelPicker modeldomain.ModelPicker
	keyProvider apikeydomain.KeyProvider
	llmFactory  *llminfra.Factory
	tools       []agentapp.Tool
	bridge      eventsdomain.Bridge
	dataDir     string
	log         *zap.Logger
	queues      sync.Map // conversationID → *convQueue
}

// NewService wires Service dependencies. Panics on nil logger.
//
// NewService 装配依赖。nil logger 立刻 panic。
func NewService(
	repo chatdomain.Repository,
	convRepo convdomain.Repository,
	modelPicker modeldomain.ModelPicker,
	keyProvider apikeydomain.KeyProvider,
	llmFactory *llminfra.Factory,
	bridge eventsdomain.Bridge,
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
		repo:        repo,
		convRepo:    convRepo,
		modelPicker: modelPicker,
		keyProvider: keyProvider,
		llmFactory:  llmFactory,
		bridge:      bridge,
		dataDir:     dataDir,
		log:         log,
	}
}

// SetTools injects system tools into the ReAct Agent.
// Safe to call before any conversation starts.
//
// SetTools 将 system tools 注入 ReAct Agent，在任何对话启动前调用均安全。
func (s *Service) SetTools(tools []agentapp.Tool) {
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
// UploadAttachment 把 fileBytes 复制到 data 目录，把元数据存入 DB，返回创建好的 Attachment。
func (s *Service) UploadAttachment(ctx context.Context, fileBytes []byte, mimeType, fileName string) (*chatdomain.Attachment, error) {
	if int64(len(fileBytes)) > chatdomain.MaxAttachmentBytes {
		return nil, chatdomain.ErrAttachmentTooLarge
	}
	uid, ok := reqctxpkg.GetUserID(ctx)
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
		if cleanErr := os.RemoveAll(storageDir); cleanErr != nil {
			s.log.Warn("failed to clean up attachment directory after save error",
				zap.String("dir", storageDir), zap.Error(cleanErr))
		}
		return nil, err
	}
	return a, nil
}

// Send saves the user message (with attachment_ref blocks) and enqueues an
// Agent task. Returns immediately with the user message ID (202 semantics).
// Returns ErrStreamInProgress only when the queue is full.
//
// Send 保存用户消息（含 attachment_ref blocks）并把 Agent 任务加入队列，立刻返回
// 用户消息 ID（202 语义）。仅在队列已满时返回 ErrStreamInProgress。
func (s *Service) Send(ctx context.Context, conversationID string, in SendInput) (string, error) {
	conv, err := s.convRepo.Get(ctx, conversationID)
	if err != nil {
		return "", err
	}
	uid, ok := reqctxpkg.GetUserID(ctx)
	if !ok {
		return "", fmt.Errorf("chat.Service.Send: missing user id in context")
	}

	blocks, err := s.buildUserBlocks(ctx, in)
	if err != nil {
		return "", fmt.Errorf("chat.Service.Send: build blocks: %w", err)
	}

	msgID := newMsgID()
	userMsg := &chatdomain.Message{
		ID:             msgID,
		ConversationID: conversationID,
		UserID:         uid,
		Role:           chatdomain.RoleUser,
		Status:         chatdomain.StatusCompleted,
		Blocks:         blocks,
	}
	if err := s.repo.Save(ctx, userMsg); err != nil {
		return "", err
	}

	agentCtx := reqctxpkg.SetUserID(context.Background(), uid)
	agentCtx = reqctxpkg.SetLocale(agentCtx, reqctxpkg.GetLocale(ctx))

	q := s.getOrCreateQueue(conversationID)
	task := queuedTask{ctx: agentCtx, conv: conv, uid: uid, userMsgID: msgID}
	select {
	case q.ch <- task:
	default:
		return "", chatdomain.ErrStreamInProgress
	}

	s.log.Info("chat task enqueued",
		zap.String("conversation_id", conversationID),
		zap.String("user_message_id", msgID),
		zap.Int("queue_depth", len(q.ch)))
	return msgID, nil
}

// buildUserBlocks constructs the block slice for a user message.
// Attachment blocks are populated with full metadata from the DB so the
// frontend can display filenames and icons without extra API calls.
//
// buildUserBlocks 构建 user 消息的 block 列表。
// 附件 block 从 DB 查询完整元数据，前端无需额外 API 调用即可展示文件名和图标。
func (s *Service) buildUserBlocks(ctx context.Context, in SendInput) ([]chatdomain.Block, error) {
	var blocks []chatdomain.Block
	seq := 0

	if in.Content != "" {
		d, _ := json.Marshal(chatdomain.TextData{Text: in.Content})
		blocks = append(blocks, chatdomain.Block{
			ID: newBlockID(), Seq: seq, Type: chatdomain.BlockTypeText, Data: string(d),
		})
		seq++
	}

	for _, attID := range in.AttachmentIDs {
		att, err := s.repo.GetAttachment(ctx, attID)
		if err != nil {
			return nil, fmt.Errorf("buildUserBlocks: attachment %q not found: %w", attID, err)
		}
		d, _ := json.Marshal(chatdomain.AttachmentRefData{
			AttachmentID: attID,
			FileName:     att.FileName,
			MimeType:     att.MimeType,
		})
		blocks = append(blocks, chatdomain.Block{
			ID: newBlockID(), Seq: seq, Type: chatdomain.BlockTypeAttachmentRef, Data: string(d),
		})
		seq++
	}
	return blocks, nil
}

// Cancel stops the currently running Agent and drains any pending tasks.
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

// ListMessages returns a paginated list of messages (with Blocks) for the conversation.
//
// ListMessages 返回对话的分页消息列表（含 Blocks）。
func (s *Service) ListMessages(ctx context.Context, conversationID string, filter chatdomain.ListFilter) ([]*chatdomain.Message, string, error) {
	return s.repo.ListByConversation(ctx, conversationID, filter)
}
