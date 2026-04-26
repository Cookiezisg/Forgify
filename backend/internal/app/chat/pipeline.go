package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
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

// invokable is the execution interface required to run a tool.
//
// invokable 是执行 tool 所需的接口。
type invokable interface {
	InvokableRun(ctx context.Context, argsJSON string, opts ...tool.Option) (string, error)
}

// tcAccum accumulates streaming ToolCall fragments for one tool call position.
// OpenAI-style streaming splits a single ToolCall across many chunks; each
// chunk carries a fragment keyed by Index.
//
// tcAccum 用于累积一个 tool call 位置的流式 ToolCall 片段。
// OpenAI 风格流式传输把单个 ToolCall 拆成多个带 Index 的片段。
type tcAccum struct {
	id   string
	name string
	args strings.Builder
}

// ── Queue / worker ────────────────────────────────────────────────────────────

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

// ── processTask ───────────────────────────────────────────────────────────────

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

	// 3. Bind tools to a fresh model instance (WithTools is safe; no mutation).
	// 把 tools 绑定到新的 model 实例（WithTools 返回新实例，无竞争）。
	toolInfos := collectToolInfos(ctx, s.tools)
	activeModel, err := built.Model.WithTools(toolInfos)
	if err != nil {
		s.publishError(ctx, conversationID, "LLM_PROVIDER_ERROR", err.Error())
		return
	}

	// 4. Load conversation history.
	history, err := s.buildEinoMessages(ctx, conversationID, provider)
	if err != nil {
		s.publishError(ctx, conversationID, "INTERNAL_ERROR", err.Error())
		return
	}

	// 5. Pre-generate the final assistant message ID for SSE correlation.
	//    The message is written to DB only when the final response arrives.
	//
	//    预生成 assistant 消息 ID，只有最终响应到达时才写入 DB。
	assistantMsgID := newMsgID()

	// 6. Cancellable context.
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

	// 7. Run our own ReAct loop — no black-box agent, full control over every step.
	// 运行我们自己的 ReAct loop——不使用黑盒 agent，完全掌控每一步。
	systemMsg := schema.SystemMessage(s.buildSystemPrompt(agentCtx, conv))
	finalContent, stopReason, tokenUsage := s.runReactLoop(
		agentCtx,
		activeModel,
		systemMsg,
		history,
		conversationID,
		assistantMsgID,
		uid,
	)

	// 8. Publish chat.done.
	s.bridge.Publish(agentCtx, conversationID, events.ChatDone{
		ConversationID: conversationID,
		MessageID:      assistantMsgID,
		StopReason:     stopReason,
		TokenUsage:     tokenUsage,
	})

	s.log.Info("chat task done",
		zap.String("conversation_id", conversationID),
		zap.String("stop_reason", stopReason))

	// 9. Auto-title after first exchange.
	if conv.Title == "" && !conv.AutoTitled {
		go s.autoTitle(context.Background(), conv, uid, finalContent)
	}
}

// ── ReAct loop ────────────────────────────────────────────────────────────────

// runReactLoop drives the full ReAct (Reasoning + Acting) conversation loop.
// It calls the LLM, streams tokens to the frontend, executes any tool calls,
// and repeats until the LLM produces a final text response or the step limit
// is reached. All SSE events and DB writes happen inside this function.
//
// runReactLoop 驱动完整的 ReAct 对话循环。
// 调用 LLM、把 token 流式推给前端、执行 tool call，循环直到 LLM 给出
// 最终文字回复或达到步骤上限。所有 SSE 事件和 DB 写入均在此函数内完成。
func (s *Service) runReactLoop(
	ctx context.Context,
	m model.ToolCallingChatModel,
	systemMsg *schema.Message,
	history []*schema.Message,
	convID, assistantMsgID, uid string,
) (finalContent, stopReason, tokenUsage string) {
	stopReason = chatdomain.StopReasonEndTurn

	// currentMessages holds the growing conversation for this turn.
	// systemMsg is prepended on every LLM call (not stored in DB history).
	// currentMessages 保存本轮对话增量；systemMsg 每次 LLM 调用前插入，不入 DB 历史。
	currentMessages := make([]*schema.Message, len(history))
	copy(currentMessages, history)

	const maxSteps = 20
	for step := 0; step < maxSteps; step++ {
		// Prepend system prompt for every LLM call.
		// 每次 LLM 调用前插入 system prompt。
		callMsgs := append([]*schema.Message{systemMsg}, currentMessages...)

		sr, err := m.Stream(ctx, callMsgs)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				stopReason = chatdomain.StopReasonCancelled
			} else {
				stopReason = chatdomain.StopReasonError
				s.publishError(ctx, convID, "LLM_PROVIDER_ERROR", err.Error())
			}
			s.saveTerminalMessage(ctx, assistantMsgID, convID, uid, "", stopReason)
			return finalContent, stopReason, ""
		}

		// Read the stream: push tokens + assemble the full message.
		// 读流：推 token + 组装完整消息。
		response, sr_stop, usage := s.collectStream(ctx, sr, convID, assistantMsgID)
		sr.Close()

		if sr_stop != "" {
			stopReason = sr_stop
		}

		// Append the response to history for the next LLM call.
		// 追加响应到历史，供下次 LLM 调用使用。
		currentMessages = append(currentMessages, response)

		if len(response.ToolCalls) == 0 {
			// Final response — save to DB and return.
			// 最终响应——写 DB 并返回。
			finalContent = response.Content
			tokenUsage = usage
			s.saveFinalMessage(ctx, assistantMsgID, convID, uid, response, stopReason, usage)
			return finalContent, stopReason, tokenUsage
		}

		// Intermediate response: has tool calls.
		// Save the assistant message, then execute each tool.
		// 中间响应（有 tool calls）：保存 assistant 消息，然后逐个执行 tool。
		s.saveIntermediateMessage(ctx, convID, uid, response)

		for _, tc := range response.ToolCalls {
			summary := s.toolSummary(tc.Function.Name, tc.Function.Arguments)
			s.bridge.Publish(ctx, convID, events.ChatToolCall{
				ConversationID: convID,
				MessageID:      assistantMsgID,
				ToolCallID:     tc.ID,
				ToolName:       tc.Function.Name,
				ToolInput:      tc.Function.Arguments,
				Summary:        summary,
			})

			result, ok := s.executeTool(ctx, tc)
			s.bridge.Publish(ctx, convID, events.ChatToolResult{
				ConversationID: convID,
				ToolCallID:     tc.ID,
				Result:         result,
				OK:             ok,
			})

			if err := s.repo.Save(ctx, &chatdomain.Message{
				ID:             newMsgID(),
				ConversationID: convID,
				UserID:         uid,
				Role:           chatdomain.RoleTool,
				Content:        result,
				ToolCallID:     tc.ID,
				Status:         chatdomain.StatusCompleted,
			}); err != nil {
				s.log.Warn("save tool result message failed",
					zap.String("tool", tc.Function.Name), zap.Error(err))
			}

			currentMessages = append(currentMessages, &schema.Message{
				Role:       schema.Tool,
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	// Reached step limit without a final text response.
	// 达到步骤上限仍未收到最终文字响应。
	stopReason = chatdomain.StopReasonMaxTokens
	s.saveTerminalMessage(ctx, assistantMsgID, convID, uid, finalContent, stopReason)
	return finalContent, stopReason, tokenUsage
}

// collectStream reads one LLM streaming response to completion.
// It pushes chat.token SSE events as tokens arrive and assembles the full
// schema.Message (including ToolCall fragments) before returning.
//
// collectStream 读完一次 LLM 流式响应。
// 边读边推 chat.token SSE，并在返回前组装完整的 schema.Message（含 ToolCall 片段）。
func (s *Service) collectStream(
	ctx context.Context,
	sr *schema.StreamReader[*schema.Message],
	convID, assistantMsgID string,
) (msg *schema.Message, stopReason, tokenUsageJSON string) {
	var contentBuf strings.Builder
	var reasoningBuf strings.Builder // reasoning_content must be echoed back to APIs that require it
	accums := map[int]*tcAccum{}
	var finishReason string
	var usage *schema.TokenUsage

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
			}
			break
		}
		if chunk == nil {
			continue
		}

		// Stream reasoning tokens and accumulate for history echo-back.
		// Thinking-mode APIs (e.g. DeepSeek-R1) require the full reasoning_content
		// to be passed back unchanged in subsequent requests.
		//
		// 推送 reasoning token 并累积，用于历史回传。
		// Thinking 模式 API（如 DeepSeek-R1）要求后续请求原样带上 reasoning_content。
		if chunk.ReasoningContent != "" {
			reasoningBuf.WriteString(chunk.ReasoningContent)
			s.bridge.Publish(ctx, convID, events.ChatReasoningToken{
				ConversationID: convID,
				MessageID:      assistantMsgID,
				Delta:          chunk.ReasoningContent,
			})
		}

		// Stream text tokens to the frontend immediately.
		// 立即把文字 token 流式推送给前端。
		if chunk.Content != "" {
			contentBuf.WriteString(chunk.Content)
			s.bridge.Publish(ctx, convID, events.ChatToken{
				ConversationID: convID,
				MessageID:      assistantMsgID,
				Delta:          chunk.Content,
			})
		}

		// Accumulate streaming ToolCall fragments by Index.
		// 按 Index 累积流式 ToolCall 片段。
		for _, tc := range chunk.ToolCalls {
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}
			a := accums[idx]
			if a == nil {
				a = &tcAccum{}
				accums[idx] = a
			}
			if tc.ID != "" {
				a.id = tc.ID
			}
			if tc.Function.Name != "" {
				a.name = tc.Function.Name
			}
			a.args.WriteString(tc.Function.Arguments)
		}

		if chunk.ResponseMeta != nil {
			if r := chunk.ResponseMeta.FinishReason; r != "" {
				finishReason = r
			}
			if u := chunk.ResponseMeta.Usage; u != nil {
				usage = u
			}
		}
	}

	if finishReason == "length" {
		stopReason = chatdomain.StopReasonMaxTokens
	}

	if usage != nil {
		b, _ := json.Marshal(struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
		}{
			InputTokens:  usage.PromptTokens,
			OutputTokens: usage.CompletionTokens,
		})
		tokenUsageJSON = string(b)
	}

	msg = &schema.Message{
		Role:             schema.Assistant,
		Content:          contentBuf.String(),
		ReasoningContent: reasoningBuf.String(),
		ToolCalls:        assembleToolCalls(accums),
	}
	return msg, stopReason, tokenUsageJSON
}

// executeTool finds the named tool in s.tools and calls InvokableRun.
// Returns the output string and whether execution succeeded.
//
// executeTool 在 s.tools 里查找指定 tool 并调用 InvokableRun。
// 返回输出字符串和执行是否成功。
func (s *Service) executeTool(ctx context.Context, tc schema.ToolCall) (result string, ok bool) {
	name := tc.Function.Name
	args := tc.Function.Arguments

	for _, t := range s.tools {
		info, err := t.Info(ctx)
		if err != nil || info.Name != name {
			continue
		}
		inv, canInvoke := t.(invokable)
		if !canInvoke {
			return fmt.Sprintf("tool %q is not executable", name), false
		}
		output, err := inv.InvokableRun(ctx, args)
		if err != nil {
			result = err.Error()
			if output != "" {
				result = output
			}
			return result, false
		}
		return output, true
	}
	return fmt.Sprintf("tool %q not found", name), false
}

// toolSummary returns a human-readable one-liner for a tool call.
//
// toolSummary 返回 tool 调用的人类可读一句话摘要。
func (s *Service) toolSummary(toolName, argsJSON string) string {
	for _, t := range s.tools {
		info, err := t.Info(context.Background())
		if err != nil || info.Name != toolName {
			continue
		}
		if sum, ok := t.(agentpkg.Summarizable); ok {
			return sum.CoreInfo(argsJSON)
		}
		break
	}
	return agentpkg.ExtractFallbackSummary(argsJSON)
}

// ── DB helpers ────────────────────────────────────────────────────────────────

func (s *Service) saveFinalMessage(
	ctx context.Context,
	id, convID, uid string,
	msg *schema.Message,
	stopReason, tokenUsage string,
) {
	if err := s.repo.Save(ctx, &chatdomain.Message{
		ID:               id,
		ConversationID:   convID,
		UserID:           uid,
		Role:             chatdomain.RoleAssistant,
		Content:          msg.Content,
		ReasoningContent: msg.ReasoningContent,
		Status:           chatdomain.StatusCompleted,
		StopReason:       stopReason,
		TokenUsage:       tokenUsage,
	}); err != nil {
		s.log.Error("save final assistant message failed", zap.Error(err))
	}
}

func (s *Service) saveIntermediateMessage(ctx context.Context, convID, uid string, msg *schema.Message) {
	tcJSON, _ := json.Marshal(msg.ToolCalls)
	if err := s.repo.Save(ctx, &chatdomain.Message{
		ID:               newMsgID(),
		ConversationID:   convID,
		UserID:           uid,
		Role:             chatdomain.RoleAssistant,
		Content:          msg.Content,
		ReasoningContent: msg.ReasoningContent,
		ToolCalls:        string(tcJSON),
		Status:           chatdomain.StatusCompleted,
	}); err != nil {
		s.log.Warn("save intermediate assistant message failed", zap.Error(err))
	}
}

// saveTerminalMessage saves a cancelled or error assistant message when the
// loop exits before producing a final response.
//
// saveTerminalMessage 在 loop 未产生最终响应就退出时，保存 cancelled/error 消息。
func (s *Service) saveTerminalMessage(ctx context.Context, id, convID, uid, content, stopReason string) {
	status := chatdomain.StatusError
	if stopReason == chatdomain.StopReasonCancelled {
		status = chatdomain.StatusCancelled
	}
	_ = s.repo.Save(ctx, &chatdomain.Message{
		ID:             id,
		ConversationID: convID,
		UserID:         uid,
		Role:           chatdomain.RoleAssistant,
		Content:        content,
		Status:         status,
		StopReason:     stopReason,
	})
}

// ── Utilities ─────────────────────────────────────────────────────────────────

// collectToolInfos extracts schema.ToolInfo from every BaseTool.
//
// collectToolInfos 从每个 BaseTool 中提取 schema.ToolInfo。
func collectToolInfos(ctx context.Context, tools []tool.BaseTool) []*schema.ToolInfo {
	infos := make([]*schema.ToolInfo, 0, len(tools))
	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		infos = append(infos, info)
	}
	return infos
}

// assembleToolCalls converts the streaming fragment map into a properly
// ordered []schema.ToolCall.
//
// assembleToolCalls 把流式片段 map 转成按序排列的 []schema.ToolCall。
func assembleToolCalls(accums map[int]*tcAccum) []schema.ToolCall {
	if len(accums) == 0 {
		return nil
	}
	result := make([]schema.ToolCall, len(accums))
	for idx, a := range accums {
		if idx < len(result) {
			result[idx] = schema.ToolCall{
				ID:       a.id,
				Function: schema.FunctionCall{Name: a.name, Arguments: a.args.String()},
			}
		}
	}
	return result
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
		// Reconstruct the full assistant message including ReasoningContent and
		// ToolCalls so the LLM has complete context on the next turn.
		// Thinking-mode APIs (e.g. DeepSeek-R1) require ReasoningContent to be
		// echoed back unchanged.
		//
		// 重建完整 assistant 消息，包含 ReasoningContent 和 ToolCalls，
		// 保证下一轮 LLM 能看到完整上下文。
		// Thinking 模式 API（如 DeepSeek-R1）要求原样回传 ReasoningContent。
		msg := &schema.Message{
			Role:             schema.Assistant,
			Content:          m.Content,
			ReasoningContent: m.ReasoningContent,
		}
		if m.ToolCalls != "" {
			var toolCalls []schema.ToolCall
			if err := json.Unmarshal([]byte(m.ToolCalls), &toolCalls); err == nil {
				msg.ToolCalls = toolCalls
			}
		}
		return msg, nil
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
