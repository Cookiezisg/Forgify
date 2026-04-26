package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	agentpkg "github.com/sunweilin/forgify/backend/internal/app/agent"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	"github.com/sunweilin/forgify/backend/internal/domain/events"
	chatextract "github.com/sunweilin/forgify/backend/internal/infra/chat"
	einoinfra "github.com/sunweilin/forgify/backend/internal/infra/eino"
	"github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// getOrCreateQueue returns the convQueue for conversationID, creating a new one
// with a worker goroutine if none exists.
//
// getOrCreateQueue 返回 conversationID 对应的 convQueue，不存在则创建并启动 worker goroutine。
func (s *Service) getOrCreateQueue(conversationID string) *convQueue {
	q := &convQueue{ch: make(chan queuedTask, queueCapacity)}
	actual, loaded := s.queues.LoadOrStore(conversationID, q)
	if loaded {
		return actual.(*convQueue)
	}
	go s.runQueue(conversationID, q)
	return q
}

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
			return
		}
	}
}

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

	// 3. Load conversation history.
	history, err := s.buildEinoMessages(ctx, conversationID, provider)
	if err != nil {
		s.publishError(ctx, conversationID, "INTERNAL_ERROR", err.Error())
		return
	}

	// 4. Build system prompt.
	systemPrompt := s.buildSystemPrompt(ctx, conv)

	// 5. Pre-generate assistant message ID for SSE correlation.
	//    The message is NOT saved to DB here — only after the full turn completes.
	assistantMsgID := newMsgID()

	// 6. Build react.Agent with cancellable context.
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

	agentCtx = agentpkg.WithConversationID(agentCtx, conversationID)

	// 7. Wrap tools: SSE publishing + message buffering, no DB writes during streaming.
	var msgBuf []*chatdomain.Message
	liveTools := s.wrapTools(s.tools, conversationID, assistantMsgID, uid, &msgBuf)

	modifier := react.MessageModifier(func(_ context.Context, msgs []*schema.Message) []*schema.Message {
		return append([]*schema.Message{schema.SystemMessage(systemPrompt)}, msgs...)
	})
	agent, err := react.NewAgent(agentCtx, &react.AgentConfig{
		ToolCallingModel:      built.Model,
		ToolsConfig:           compose.ToolsNodeConfig{Tools: liveTools},
		MessageModifier:       modifier,
		MaxStep:               20,
		StreamToolCallChecker: built.Checker,
	})
	if err != nil {
		s.publishError(ctx, conversationID, "LLM_PROVIDER_ERROR", err.Error())
		return
	}

	// 8. Stream and collect the final text response.
	sr, err := agent.Stream(agentCtx, history)
	if err != nil {
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

	// 9. Persist: flush tool messages first (correct order), then final assistant message.
	//    Nothing was written to DB during streaming, so timestamps are naturally ordered.
	for _, m := range msgBuf {
		if err := s.repo.Save(ctx, m); err != nil {
			s.log.Warn("flush tool message failed", zap.Error(err), zap.String("message_id", m.ID))
		}
	}

	finalContent := contentBuf.String()
	finalStatus := chatdomain.StatusCompleted
	switch stopReason {
	case chatdomain.StopReasonError:
		finalStatus = chatdomain.StatusError
	case chatdomain.StopReasonCancelled:
		finalStatus = chatdomain.StatusCancelled
	}
	s.finaliseMessage(ctx, &chatdomain.Message{
		ID:             assistantMsgID,
		ConversationID: conversationID,
		UserID:         uid,
		Role:           chatdomain.RoleAssistant,
	}, finalContent, finalStatus, stopReason, usage)

	s.bridge.Publish(ctx, conversationID, events.ChatDone{
		ConversationID: conversationID,
		MessageID:      assistantMsgID,
		StopReason:     stopReason,
		TokenUsage:     tokenUsageToJSON(usage),
	})

	s.log.Info("chat task done",
		zap.String("conversation_id", conversationID),
		zap.String("stop_reason", stopReason))

	// 10. Auto-title after first exchange.
	if conv.Title == "" && !conv.AutoTitled {
		go s.autoTitle(context.Background(), conv, uid, finalContent)
	}
}

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
		// Reconstruct tool calls so the LLM has full context on the next turn.
		if m.ToolCalls != "" {
			var toolCalls []schema.ToolCall
			if err := json.Unmarshal([]byte(m.ToolCalls), &toolCalls); err == nil && len(toolCalls) > 0 {
				return schema.AssistantMessage(m.Content, toolCalls), nil
			}
		}
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
