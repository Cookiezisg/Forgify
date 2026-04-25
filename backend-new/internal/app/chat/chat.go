// Package chat (app/chat) orchestrates the chat pipeline: LLM streaming,
// attachment handling, auto-titling, and SSE event publishing. It owns
// no SQL — persistence is delegated to infra/store/chat.
//
// Concurrency model: each conversation has a convQueue with a buffered task
// channel. A single worker goroutine drains the channel sequentially, so
// messages within one conversation always execute one at a time in order.
//
// All three chat packages (domain / app / store) declare `package chat`.
// External callers alias at import:
//
//	chatapp "…/internal/app/chat"
//
// Package chat（app/chat）编排聊天管线：LLM 流式输出、附件处理、自动命名、
// SSE 事件推送。不含 SQL——持久化委托给 infra/store/chat。
//
// 并发模型：每个 conversation 拥有一个带缓冲任务 channel 的 convQueue。
// 单个 worker goroutine 顺序消费队列，保证同一 conversation 的消息始终
// 按序、逐条执行。
package chat

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	"github.com/sunweilin/forgify/backend/internal/domain/events"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	chatextract "github.com/sunweilin/forgify/backend/internal/infra/chat"
	einoinfra "github.com/sunweilin/forgify/backend/internal/infra/eino"
	"github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// queueCapacity is the maximum number of messages that can be queued
// behind the currently running Agent for one conversation.
//
// queueCapacity 是单个 conversation 在当前 Agent 之后最多排队的消息数。
const queueCapacity = 5

// convQueue manages sequential Agent execution for one conversation.
// The worker goroutine reads from ch and processes tasks one at a time.
//
// convQueue 管理单个 conversation 的顺序 Agent 执行。
// Worker goroutine 从 ch 读取任务，逐条处理。
type convQueue struct {
	ch     chan queuedTask
	mu     sync.Mutex
	cancel context.CancelFunc // nil when idle; set while Agent is running
}

// queuedTask is one pending chat turn waiting to be processed.
//
// queuedTask 是等待处理的一次对话请求。
type queuedTask struct {
	ctx  context.Context // carries userID + locale
	conv *convdomain.Conversation
	uid  string
}

// ── Service ───────────────────────────────────────────────────────────────────

// Service orchestrates LLM calls, attachment handling, and SSE event publishing.
//
// Service 编排 LLM 调用、附件处理和 SSE 事件推送。
type Service struct {
	repo         chatdomain.Repository
	convRepo     convdomain.Repository
	modelPicker  modeldomain.ModelPicker
	keyProvider  apikeydomain.KeyProvider
	modelFactory einoinfra.ChatModelFactory
	tools        []tool.BaseTool // Phase 2: nil; Phase 3+: system tools injected via main.go
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

// SendInput is the payload for Service.Send.
//
// SendInput 是 Service.Send 的请求载荷。
type SendInput struct {
	Content       string
	AttachmentIDs []string
}

// ── UploadAttachment ──────────────────────────────────────────────────────────

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
		_ = os.RemoveAll(storageDir) // best-effort rollback / 尽力回滚
		return nil, err
	}
	return a, nil
}

// ── Send ──────────────────────────────────────────────────────────────────────

// Send saves the user message and enqueues an Agent task for this conversation.
// Returns immediately with the user message ID (202 semantics). If a previous
// message is still streaming, this one waits in the queue rather than failing.
// Returns ErrStreamInProgress only when the queue is full (queueCapacity reached).
//
// Send 保存用户消息并把 Agent 任务加入该 conversation 的队列，立刻返回
// 用户消息 ID（202 语义）。若上一条消息仍在流式输出，本条在队列中等待而非报错。
// 仅在队列已满（达到 queueCapacity）时返回 ErrStreamInProgress。
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
	// Locale is copied from the request context.
	// Agent 在后台 context 中运行，生命周期超过 HTTP 请求。
	// Locale 从请求 context 复制。
	agentCtx := reqctx.SetUserID(context.Background(), uid)
	agentCtx = reqctx.SetLocale(agentCtx, reqctx.GetLocale(ctx))

	q := s.getOrCreateQueue(conversationID)
	task := queuedTask{ctx: agentCtx, conv: conv, uid: uid}

	select {
	case q.ch <- task:
	default:
		// Queue full — the conversation is very busy.
		// 队列已满——该 conversation 当前请求量过大。
		return "", chatdomain.ErrStreamInProgress
	}

	s.log.Info("chat task enqueued",
		zap.String("conversation_id", conversationID),
		zap.String("user_message_id", userMsg.ID),
		zap.Int("queue_depth", len(q.ch)))
	return userMsg.ID, nil
}

// getOrCreateQueue returns the convQueue for the conversation, creating a new
// one with a worker goroutine if none exists.
//
// getOrCreateQueue 返回 conversation 的 convQueue，若不存在则创建并启动 worker。
func (s *Service) getOrCreateQueue(conversationID string) *convQueue {
	q := &convQueue{ch: make(chan queuedTask, queueCapacity)}
	actual, loaded := s.queues.LoadOrStore(conversationID, q)
	if loaded {
		return actual.(*convQueue)
	}
	go s.runQueue(conversationID, q)
	return q
}

// runQueue is the per-conversation worker goroutine. It processes tasks from
// q.ch one at a time. Exits after 5 minutes of inactivity to free resources.
//
// runQueue 是每个 conversation 的 worker goroutine，逐条处理 q.ch 中的任务。
// 5 分钟无任务后退出以释放资源。
func (s *Service) runQueue(conversationID string, q *convQueue) {
	const idleTimeout = 5 * time.Minute
	timer := time.NewTimer(idleTimeout)
	defer func() {
		timer.Stop()
		s.queues.Delete(conversationID)
	}()

	for {
		select {
		case task := <-q.ch:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			s.processTask(conversationID, q, task)
			timer.Reset(idleTimeout)
		case <-timer.C:
			return // idle timeout — goroutine exits / 空闲超时，goroutine 退出
		}
	}
}

// processTask runs one Agent task and updates the assistant message in DB.
//
// processTask 执行一个 Agent 任务并更新 DB 中的 assistant 消息。
func (s *Service) processTask(conversationID string, q *convQueue, task queuedTask) {
	ctx := task.ctx
	conv := task.conv
	uid := task.uid

	// 1. Resolve model + credentials.
	provider, modelID, err := s.modelPicker.PickForChat(ctx)
	if err != nil {
		s.publishError(ctx, conversationID, "MODEL_NOT_CONFIGURED", err.Error())
		return
	}
	creds, err := s.keyProvider.ResolveCredentials(ctx, provider)
	if err != nil {
		s.publishError(ctx, conversationID, "API_KEY_PROVIDER_NOT_FOUND", err.Error())
		return
	}

	// 2. Build LLM model.
	built, err := s.modelFactory.Build(ctx, einoinfra.ModelConfig{
		Provider: provider, ModelID: modelID,
		Key: creds.Key, BaseURL: creds.BaseURL,
	})
	if err != nil {
		s.publishError(ctx, conversationID, "LLM_PROVIDER_ERROR", err.Error())
		return
	}

	// 3. Load conversation history as Eino messages.
	history, err := s.buildEinoMessages(ctx, conversationID, provider)
	if err != nil {
		s.publishError(ctx, conversationID, "INTERNAL_ERROR", err.Error())
		return
	}

	// 4. Build system prompt.
	systemPrompt := s.buildSystemPrompt(ctx, conv)

	// 5. Create streaming assistant message placeholder.
	assistantMsgID := newMsgID()
	assistantMsg := &chatdomain.Message{
		ID: assistantMsgID, ConversationID: conversationID, UserID: uid,
		Role: chatdomain.RoleAssistant, Status: chatdomain.StatusStreaming,
	}
	if err := s.repo.Save(ctx, assistantMsg); err != nil {
		s.publishError(ctx, conversationID, "INTERNAL_ERROR", err.Error())
		return
	}

	// 6. Build react.Agent and attach cancellable context.
	agentCtx, cancel := context.WithCancel(ctx)
	q.mu.Lock()
	q.cancel = cancel
	q.mu.Unlock()
	defer func() {
		cancel()
		q.mu.Lock()
		q.cancel = nil
		q.mu.Unlock()
	}()

	modifier := react.MessageModifier(func(_ context.Context, msgs []*schema.Message) []*schema.Message {
		return append([]*schema.Message{schema.SystemMessage(systemPrompt)}, msgs...)
	})
	agent, err := react.NewAgent(agentCtx, &react.AgentConfig{
		ToolCallingModel:      built.Model,
		ToolsConfig:           compose.ToolsNodeConfig{},
		MessageModifier:       modifier,
		MaxStep:               20,
		StreamToolCallChecker: built.Checker,
	})
	if err != nil {
		s.finaliseMessage(ctx, assistantMsg, "", chatdomain.StatusError, chatdomain.StopReasonError, nil)
		s.publishError(ctx, conversationID, "LLM_PROVIDER_ERROR", err.Error())
		return
	}

	// 7. Stream and collect response.
	sr, err := agent.Stream(agentCtx, history)
	if err != nil {
		s.finaliseMessage(ctx, assistantMsg, "", chatdomain.StatusError, chatdomain.StopReasonError, nil)
		s.publishError(ctx, conversationID, "LLM_PROVIDER_ERROR", err.Error())
		return
	}
	defer sr.Close()

	var contentBuf strings.Builder
	var usage *schema.TokenUsage
	stopReason := chatdomain.StopReasonEndTurn

	for {
		chunk, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				stopReason = chatdomain.StopReasonCancelled
			} else {
				stopReason = chatdomain.StopReasonError
				s.publishError(ctx, conversationID, "LLM_PROVIDER_ERROR", err.Error())
			}
			break
		}
		if chunk.Content != "" {
			contentBuf.WriteString(chunk.Content)
			s.bridge.Publish(ctx, conversationID, events.ChatToken{
				ConversationID: conversationID,
				MessageID:      assistantMsgID,
				Delta:          chunk.Content,
			})
		}
		if chunk.ResponseMeta != nil {
			if chunk.ResponseMeta.Usage != nil {
				usage = chunk.ResponseMeta.Usage
			}
			if chunk.ResponseMeta.FinishReason == "length" {
				stopReason = chatdomain.StopReasonMaxTokens
			}
		}
	}

	// 8. Persist and publish done.
	finalContent := contentBuf.String()
	var finalStatus string
	switch stopReason {
	case chatdomain.StopReasonError:
		finalStatus = chatdomain.StatusError
	case chatdomain.StopReasonCancelled:
		finalStatus = chatdomain.StatusCancelled
	default:
		finalStatus = chatdomain.StatusCompleted
	}
	s.finaliseMessage(ctx, assistantMsg, finalContent, finalStatus, stopReason, usage)
	s.bridge.Publish(ctx, conversationID, events.ChatDone{
		ConversationID: conversationID,
		MessageID:      assistantMsgID,
		StopReason:     stopReason,
		TokenUsage:     tokenUsageToJSON(usage),
	})

	s.log.Info("chat task done",
		zap.String("conversation_id", conversationID),
		zap.String("stop_reason", stopReason))

	// 9. Auto-title after first exchange.
	if conv.Title == "" && !conv.AutoTitled {
		go s.autoTitle(context.Background(), conv, uid, finalContent)
	}
}

// ── Cancel ────────────────────────────────────────────────────────────────────

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
		return chatdomain.ErrStreamNotFound // queue exists but nothing running
	}
	cancel()

	// Drain pending tasks so they don't run after cancel.
	// 清空待处理任务，避免取消后继续执行。
	for {
		select {
		case <-q.ch:
		default:
			return nil
		}
	}
}

// ── ListMessages ──────────────────────────────────────────────────────────────

// ListMessages returns a paginated list of messages for the conversation.
//
// ListMessages 返回对话的分页消息列表。
func (s *Service) ListMessages(ctx context.Context, conversationID string, filter chatdomain.ListFilter) ([]*chatdomain.Message, string, error) {
	return s.repo.ListByConversation(ctx, conversationID, filter)
}

// ── Message building ──────────────────────────────────────────────────────────

func (s *Service) buildEinoMessages(ctx context.Context, conversationID, provider string) ([]*schema.Message, error) {
	rows, _, err := s.repo.ListByConversation(ctx, conversationID, chatdomain.ListFilter{Limit: 200})
	if err != nil {
		return nil, err
	}
	out := make([]*schema.Message, 0, len(rows))
	for _, m := range rows {
		msg, err := s.toEinoMessage(ctx, m, provider)
		if err != nil {
			return nil, err
		}
		if msg != nil {
			out = append(out, msg)
		}
	}
	return out, nil
}

func (s *Service) toEinoMessage(ctx context.Context, m *chatdomain.Message, provider string) (*schema.Message, error) {
	if m.Status == chatdomain.StatusStreaming || m.Status == chatdomain.StatusPending {
		return nil, nil
	}
	switch m.Role {
	case chatdomain.RoleAssistant:
		return schema.AssistantMessage(m.Content, nil), nil
	case chatdomain.RoleTool:
		return &schema.Message{
			Role: schema.Tool, Content: m.Content, ToolCallID: m.ToolCallID,
		}, nil
	case chatdomain.RoleUser:
		return s.buildUserMessage(ctx, m, provider)
	}
	return nil, nil
}

func (s *Service) buildUserMessage(ctx context.Context, m *chatdomain.Message, provider string) (*schema.Message, error) {
	if m.AttachmentIDs == "" || m.AttachmentIDs == "[]" || m.AttachmentIDs == "null" {
		return schema.UserMessage(m.Content), nil
	}
	var attIDs []string
	if err := json.Unmarshal([]byte(m.AttachmentIDs), &attIDs); err != nil || len(attIDs) == 0 {
		return schema.UserMessage(m.Content), nil
	}

	parts := []schema.MessageInputPart{{Type: schema.ChatMessagePartTypeText, Text: m.Content}}
	for _, id := range attIDs {
		att, err := s.repo.GetAttachment(ctx, id)
		if err != nil {
			continue
		}
		if chatextract.IsImage(att.MimeType) {
			part, err := imageToInputPart(att, provider)
			if err != nil {
				s.log.Warn("vision not supported, skipping attachment",
					zap.String("attachment_id", id), zap.String("provider", provider))
				continue
			}
			parts = append(parts, part)
		} else {
			text, err := chatextract.Extract(att.StoragePath, att.MimeType)
			if err != nil {
				s.log.Warn("attachment extraction failed, skipping",
					zap.String("attachment_id", id), zap.Error(err))
				continue
			}
			parts = append(parts, schema.MessageInputPart{
				Type: schema.ChatMessagePartTypeText,
				Text: fmt.Sprintf("\n\n[附件: %s]\n%s", att.FileName, text),
			})
		}
	}
	if len(parts) == 1 {
		return schema.UserMessage(m.Content), nil
	}
	return &schema.Message{Role: schema.User, UserInputMultiContent: parts}, nil
}

func (s *Service) buildSystemPrompt(ctx context.Context, conv *convdomain.Conversation) string {
	var sb strings.Builder
	sb.WriteString("You are Forgify, an AI assistant that helps users build tools, automate workflows, and work with data.")
	if conv.SystemPrompt != "" {
		sb.WriteString("\n\n")
		sb.WriteString(conv.SystemPrompt)
	}
	if reqctx.GetLocale(ctx) == reqctx.LocaleZhCN {
		sb.WriteString("\n\nPlease respond in Chinese (Simplified) unless the user writes in another language.")
	}
	return sb.String()
}

func (s *Service) finaliseMessage(ctx context.Context, m *chatdomain.Message, content, status, stopReason string, usage *schema.TokenUsage) {
	m.Content = content
	m.Status = status
	m.StopReason = stopReason
	m.TokenUsage = tokenUsageToJSON(usage)
	if err := s.repo.Save(ctx, m); err != nil {
		s.log.Error("finalise message failed", zap.Error(err), zap.String("message_id", m.ID))
	}
}

func (s *Service) publishError(ctx context.Context, conversationID, code, msg string) {
	s.bridge.Publish(ctx, conversationID, events.ChatError{
		ConversationID: conversationID, Code: code, Message: msg,
	})
	s.log.Error("chat error", zap.String("conversation_id", conversationID),
		zap.String("code", code), zap.String("message", msg))
}

// ── Auto-titling ──────────────────────────────────────────────────────────────

func (s *Service) autoTitle(ctx context.Context, conv *convdomain.Conversation, uid, assistantContent string) {
	titleCtx := reqctx.SetUserID(ctx, uid)
	provider, modelID, err := s.modelPicker.PickForChat(titleCtx)
	if err != nil {
		return
	}
	creds, err := s.keyProvider.ResolveCredentials(titleCtx, provider)
	if err != nil {
		return
	}
	built, err := s.modelFactory.Build(titleCtx, einoinfra.ModelConfig{
		Provider: provider, ModelID: modelID, Key: creds.Key, BaseURL: creds.BaseURL,
	})
	if err != nil {
		return
	}

	tCtx, cancel := context.WithTimeout(titleCtx, 10*time.Second)
	defer cancel()

	msgs := []*schema.Message{
		schema.SystemMessage("Generate a short conversation title (5 words or fewer). Reply with ONLY the title, no punctuation.\n只返回标题本身，不超过 10 个字，不加标点。"),
		schema.UserMessage("Assistant said: " + truncate(assistantContent, 300)),
	}
	result, err := built.Model.Generate(tCtx, msgs)
	if err != nil || result == nil {
		return
	}
	title := strings.TrimSpace(result.Content)
	if title == "" {
		return
	}

	conv.Title = title
	conv.AutoTitled = true
	if err := s.convRepo.Save(titleCtx, conv); err != nil {
		s.log.Warn("auto-title save failed", zap.Error(err))
		return
	}
	s.bridge.Publish(titleCtx, conv.ID, events.ConversationTitleUpdated{
		ConversationID: conv.ID, Title: title, AutoTitled: true,
	})
	s.log.Info("auto-title generated",
		zap.String("conversation_id", conv.ID), zap.String("title", title))
}

// ── Utilities ─────────────────────────────────────────────────────────────────

func newMsgID() string        { return "msg_" + randHex(8) }
func newAttachmentID() string { return "att_" + randHex(8) }

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("chat: crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(b)
}

// imageToInputPart converts an image attachment to a Vision input part.
// Returns ErrVisionNotSupported if the provider does not support Vision.
//
// imageToInputPart 把图片附件转成 Vision 输入 part。
// provider 不支持 Vision 时返回 ErrVisionNotSupported。
func imageToInputPart(att *chatdomain.Attachment, provider string) (schema.MessageInputPart, error) {
	if !supportsVision(provider) {
		return schema.MessageInputPart{}, chatdomain.ErrVisionNotSupported
	}
	data, err := os.ReadFile(att.StoragePath)
	if err != nil {
		return schema.MessageInputPart{}, fmt.Errorf("%w: %v", chatdomain.ErrAttachmentParseFailed, err)
	}
	encoded := base64.StdEncoding.EncodeToString(data)
	return schema.MessageInputPart{
		Type: schema.ChatMessagePartTypeImageURL,
		Image: &schema.MessageInputImage{
			MessagePartCommon: schema.MessagePartCommon{
				Base64Data: &encoded,
				MIMEType:   att.MimeType,
			},
		},
	}, nil
}

// supportsVision reports whether the provider accepts image inputs.
//
// supportsVision 报告 provider 是否支持图片输入。
func supportsVision(provider string) bool {
	switch provider {
	case "openai", "anthropic", "google":
		return true
	default:
		return false
	}
}

type tokenUsageJSON struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
}

func tokenUsageToJSON(u *schema.TokenUsage) string {
	if u == nil {
		return ""
	}
	b, _ := json.Marshal(tokenUsageJSON{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
	})
	return string(b)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
