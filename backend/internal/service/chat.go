package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/sunweilin/forgify/internal/attachment"
	ctxcompress "github.com/sunweilin/forgify/internal/context"
	"github.com/sunweilin/forgify/internal/events"
	"github.com/sunweilin/forgify/internal/model"
	"github.com/sunweilin/forgify/internal/storage"
)

type ChatService struct {
	gateway    *model.ModelGateway
	bridge     *events.Bridge
	convSvc    *ConversationService
	compressor *ctxcompress.Compressor
	mu         sync.Mutex
	cancels    map[string]context.CancelFunc
}

func NewChatService(gateway *model.ModelGateway, bridge *events.Bridge, convSvc *ConversationService) *ChatService {
	return &ChatService{
		gateway:    gateway,
		bridge:     bridge,
		convSvc:    convSvc,
		compressor: ctxcompress.NewCompressor(gateway, bridge),
		cancels:    make(map[string]context.CancelFunc),
	}
}

// FullCompact exposes the compressor's full compact operation to the server layer.
func (s *ChatService) FullCompact(ctx context.Context, conversationID string) error {
	return s.compressor.FullCompact(ctx, conversationID)
}

type chatTokenPayload struct {
	ConversationID string `json:"conversationId"`
	Token          string `json:"token"`
}

type chatDonePayload struct {
	ConversationID string `json:"conversationId"`
	ModelID        string `json:"modelId,omitempty"`
}

type chatErrorPayload struct {
	ConversationID string `json:"conversationId"`
	Error          string `json:"error"`
}

type notificationPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Level string `json:"level"`
}

func (s *ChatService) SendMessageWithAttachments(ctx context.Context, conversationID, userMessage string, files []*attachment.FileInfo) error {
	// Build attachment summary for persistence (file names + sizes)
	var attachSummary string
	if len(files) > 0 {
		var names []string
		for _, f := range files {
			names = append(names, fmt.Sprintf("%s (%s)", f.Name, formatSize(f.Size)))
		}
		attachSummary = "\n📎 " + strings.Join(names, ", ")
	}

	userMsgID := uuid.NewString()
	if _, err := storage.DB().Exec(
		`INSERT INTO messages (id, conversation_id, role, content, content_type) VALUES (?, ?, 'user', ?, 'text')`,
		userMsgID, conversationID, userMessage+attachSummary,
	); err != nil {
		return fmt.Errorf("save user message: %w", err)
	}
	s.convSvc.TouchUpdatedAt(conversationID)

	history, err := s.loadHistory(conversationID)
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}

	// Inject file content into the message history for the LLM
	if len(files) > 0 {
		history, err = attachment.InjectIntoMessages(history, files)
		if err != nil {
			return fmt.Errorf("inject attachments: %w", err)
		}
	}

	// Apply context compression if needed (model limit defaults to 128k tokens)
	const defaultModelLimit = 128000
	history, compressLevel, _ := s.compressor.MaybeCompress(ctx, conversationID, history, defaultModelLimit)
	if compressLevel == ctxcompress.LevelAuto {
		s.bridge.Emit(events.ChatCompacted, map[string]any{
			"conversationId": conversationID,
			"level":          string(compressLevel),
		})
	}

	llm, modelID, getErr := s.gateway.GetModel(ctx, model.PurposeConversation)
	if getErr != nil {
		var fallbackErr model.ErrUsedFallback
		if errors.As(getErr, &fallbackErr) {
			modelID = fallbackErr.Fallback
			s.bridge.Emit(events.Notification, notificationPayload{
				Title: "已自动切换模型",
				Body:  fmt.Sprintf("主模型不可用，已切换到 %s", fallbackErr.Fallback),
				Level: "info",
			})
		} else {
			s.bridge.Emit(events.ChatError, chatErrorPayload{
				ConversationID: conversationID,
				Error:          classifyError(getErr),
			})
			return getErr
		}
	}

	streamCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[conversationID] = cancel
	s.mu.Unlock()

	go s.doStream(streamCtx, cancel, conversationID, llm, history, modelID, userMessage)
	return nil
}

func formatSize(bytes int64) string {
	const kb = 1024
	const mb = kb * 1024
	switch {
	case bytes >= mb:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.0fKB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func (s *ChatService) StopGeneration(conversationID string) {
	s.mu.Lock()
	cancel, ok := s.cancels[conversationID]
	s.mu.Unlock()
	if ok {
		cancel()
	}
}

func (s *ChatService) doStream(
	ctx context.Context,
	cancel context.CancelFunc,
	conversationID string,
	llm einomodel.BaseChatModel,
	history []*schema.Message,
	modelID string,
	userMessage string,
) {
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.cancels, conversationID)
		s.mu.Unlock()
	}()

	sr, err := llm.Stream(ctx, history)
	if err != nil {
		s.bridge.Emit(events.ChatError, chatErrorPayload{
			ConversationID: conversationID,
			Error:          classifyError(err),
		})
		return
	}
	defer sr.Close()

	var buf strings.Builder
	for {
		chunk, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			content := buf.String()
			assistantMsgID := uuid.NewString()
			storage.DB().Exec(
				`INSERT INTO messages (id, conversation_id, role, content, content_type, model_id) VALUES (?, ?, 'assistant', ?, 'text', ?)`,
				assistantMsgID, conversationID, content, modelID,
			)
			s.convSvc.TouchUpdatedAt(conversationID)
			s.bridge.Emit(events.ChatDone, chatDonePayload{
				ConversationID: conversationID,
				ModelID:        modelID,
			})

			// Trigger auto-title for first exchange
			s.convSvc.AutoTitle(ctx, conversationID, userMessage, content)
			return
		}
		if err != nil {
			s.bridge.Emit(events.ChatError, chatErrorPayload{
				ConversationID: conversationID,
				Error:          classifyError(err),
			})
			return
		}
		if chunk != nil && chunk.Content != "" {
			buf.WriteString(chunk.Content)
			s.bridge.Emit(events.ChatToken, chatTokenPayload{
				ConversationID: conversationID,
				Token:          chunk.Content,
			})
		}
	}
}

func (s *ChatService) loadHistory(conversationID string) ([]*schema.Message, error) {
	rows, err := storage.DB().Query(`
		SELECT role, content FROM messages
		WHERE conversation_id=? ORDER BY created_at ASC`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*schema.Message
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			return nil, err
		}
		switch role {
		case "user":
			msgs = append(msgs, schema.UserMessage(content))
		case "assistant":
			msgs = append(msgs, schema.AssistantMessage(content, nil))
		case "system":
			msgs = append(msgs, schema.SystemMessage(content))
		}
	}
	return msgs, rows.Err()
}

func classifyError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "401") || strings.Contains(msg, "invalid api key") || strings.Contains(msg, "authentication"):
		return "API Key 可能已失效，请前往设置检查"
	case strings.Contains(msg, "429") || strings.Contains(msg, "rate limit"):
		return "请求过于频繁，请稍后重试"
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return "连接超时，请检查网络后重试"
	case strings.Contains(msg, "no model configured"):
		return "尚未配置模型，请前往设置页面配置"
	case strings.Contains(msg, "no api key") || strings.Contains(msg, "no key for"):
		return "尚未配置该提供商的 API Key，请前往设置检查"
	default:
		return "AI 服务暂时不可用：" + err.Error()
	}
}
