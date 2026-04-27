// pipeline.go — Queue management and the ReAct loop.
// One worker goroutine per conversation drains tasks sequentially.
//
// DB persistence contract:
//   - Intermediate steps: saved with status=streaming (skipped by buildLLMHistory)
//   - Final step: saved with status=completed/cancelled/error + correct stopReason
//   - All blocks from all steps accumulate into ONE assistant message (assistantMsgID)
//
// pipeline.go — 队列管理和 ReAct loop。
//
// DB 持久化约定：
//   - 中间步骤：status=streaming 保存（buildLLMHistory 会跳过）
//   - 最终步骤：status=completed/cancelled/error + 正确的 stopReason 一次性保存
//   - 所有步骤的 blocks 累积在同一条 assistant 消息（assistantMsgID）中
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	agentapp "github.com/sunweilin/forgify/backend/internal/app/agent"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	eventsdomain "github.com/sunweilin/forgify/backend/internal/domain/events"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	"github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// ── Queue / worker ────────────────────────────────────────────────────────────

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
	client, baseURL, err := s.llmFactory.Build(llminfra.Config{
		Provider: provider, ModelID: modelID,
		Key: creds.Key, BaseURL: creds.BaseURL,
	})
	if err != nil {
		s.publishError(ctx, conversationID, "LLM_PROVIDER_ERROR", err.Error())
		return
	}

	history, err := s.buildLLMHistory(ctx, conversationID, task.userMsgID)
	if err != nil {
		s.publishError(ctx, conversationID, "INTERNAL_ERROR", err.Error())
		return
	}

	assistantMsgID := newMsgID()

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

	agentCtx = agentapp.WithConversationID(agentCtx, conversationID)

	baseReq := llminfra.Request{
		ModelID: modelID,
		Key:     creds.Key,
		BaseURL: baseURL,
		System:  s.buildSystemPrompt(agentCtx, conv),
		Tools:   agentapp.ToLLMDefs(s.tools),
	}

	finalContent, stopReason, inputTokens, outputTokens := s.runReactLoop(
		agentCtx, client, baseReq, history, conversationID, assistantMsgID, uid,
	)

	s.bridge.Publish(agentCtx, conversationID, eventsdomain.ChatDone{
		ConversationID: conversationID,
		MessageID:      assistantMsgID,
		StopReason:     stopReason,
		InputTokens:    inputTokens,
		OutputTokens:   outputTokens,
	})

	s.log.Info("chat task done",
		zap.String("conversation_id", conversationID),
		zap.String("stop_reason", stopReason))

	if conv.Title == "" && !conv.AutoTitled {
		go s.autoTitle(context.Background(), conv, uid, finalContent)
	}
}

// ── ReAct loop ────────────────────────────────────────────────────────────────

// runReactLoop drives the ReAct (Reasoning + Acting) cycle. It accumulates
// all blocks from every step into a single assistant message (assistantMsgID)
// so the full tool-call history is preserved in one DB row per user turn.
// DB is written after every step: streaming during tool steps, final status last.
//
// runReactLoop 驱动 ReAct 循环。所有步骤的 blocks 累积到同一条 assistant 消息中
// （assistantMsgID），保证一次用户发言对应一条完整的 DB 记录。
// 每步写 DB：工具调用中间步骤写 streaming，最后一步写最终状态。
func (s *Service) runReactLoop(
	ctx context.Context,
	client llminfra.Client,
	baseReq llminfra.Request,
	history []llminfra.LLMMessage,
	convID, assistantMsgID, uid string,
) (finalContent, stopReason string, inputTokens, outputTokens int) {
	stopReason = chatdomain.StopReasonEndTurn
	current := make([]llminfra.LLMMessage, len(history))
	copy(current, history)

	var allBlocks []chatdomain.Block // accumulates blocks from every step

	const maxSteps = 20
	for step := 0; step < maxSteps; step++ {
		req := baseReq
		req.Messages = current

		stepBlocks, hasToolCalls, sr, iT, oT := s.runStep(ctx, client, req, convID, assistantMsgID)
		allBlocks = append(allBlocks, stepBlocks...)
		inputTokens += iT
		outputTokens += oT
		if sr != "" {
			stopReason = sr
		}

		switch {
		case stopReason == chatdomain.StopReasonCancelled:
			s.finalPersist(ctx, assistantMsgID, convID, uid, allBlocks,
				chatdomain.StatusCancelled, stopReason, inputTokens, outputTokens)
			finalContent = extractTextContent(allBlocks)
			return

		case stopReason == chatdomain.StopReasonError:
			s.finalPersist(ctx, assistantMsgID, convID, uid, allBlocks,
				chatdomain.StatusError, stopReason, inputTokens, outputTokens)
			finalContent = extractTextContent(allBlocks)
			return

		case !hasToolCalls:
			// Final response — write completed status with correct stopReason.
			// 最终回复——写 completed 状态和正确的 stopReason。
			s.finalPersist(ctx, assistantMsgID, convID, uid, allBlocks,
				chatdomain.StatusCompleted, stopReason, inputTokens, outputTokens)
			finalContent = extractTextContent(allBlocks)
			return

		default:
			// Intermediate tool-calling step — persist streaming state so the DB
			// reflects current progress; buildLLMHistory skips streaming messages.
			// Failure here is non-fatal: the final persist will overwrite anyway.
			//
			// 中间 tool call 步骤——写 streaming 状态反映当前进度。
			// 此处失败非致命：最终 persist 会覆盖。
			if err := s.persistMsg(ctx, assistantMsgID, convID, uid, allBlocks,
				chatdomain.StatusStreaming, "", inputTokens, outputTokens); err != nil {
				s.log.Warn("intermediate persist failed, continuing",
					zap.String("msg_id", assistantMsgID), zap.Error(err))
			}

			// Add this step's contribution to the LLM history for the next call.
			// 把本步贡献加入 LLM 历史，供下一次调用使用。
			stepMsgs, err := blocksToAssistantLLM(stepBlocks)
			if err != nil {
				s.log.Error("history reconstruction failed mid-loop, aborting",
					zap.String("conversation_id", convID), zap.Error(err))
				stopReason = chatdomain.StopReasonError
				s.finalPersist(ctx, assistantMsgID, convID, uid, allBlocks,
					chatdomain.StatusError, stopReason, inputTokens, outputTokens)
				return
			}
			current = append(current, stepMsgs...)
		}
	}

	// Reached step limit — save with max_tokens stopReason (fixes the DB mismatch).
	// 达到步骤上限——用 max_tokens 保存（修复 DB 状态不一致问题）。
	stopReason = chatdomain.StopReasonMaxTokens
	s.finalPersist(ctx, assistantMsgID, convID, uid, allBlocks,
		chatdomain.StatusCompleted, stopReason, inputTokens, outputTokens)
	finalContent = extractTextContent(allBlocks)
	return
}

// runStep executes one LLM call and any resulting tool calls.
// It does NOT write to the database — all persistence is managed by runReactLoop.
// Returns the blocks produced in this step and whether tool calls were made.
//
// runStep 执行一次 LLM 调用及其产生的 tool call。
// 不写 DB——所有持久化由 runReactLoop 统一管理。
// 返回本步产生的 blocks 和是否有 tool call。
func (s *Service) runStep(
	ctx context.Context,
	client llminfra.Client,
	req llminfra.Request,
	convID, assistantMsgID string,
) (blocks []chatdomain.Block, hasToolCalls bool, stopReason string, inputTokens, outputTokens int) {
	assistantBlocks, stopReason, inputTokens, outputTokens := s.consumeStream(
		ctx, client.Stream(ctx, req), convID, assistantMsgID,
	)

	if stopReason == chatdomain.StopReasonCancelled || stopReason == chatdomain.StopReasonError {
		return assistantBlocks, false, stopReason, inputTokens, outputTokens
	}

	toolCalls := collectToolCalls(assistantBlocks)
	if len(toolCalls) == 0 {
		return assistantBlocks, false, stopReason, inputTokens, outputTokens
	}

	resultBlocks := s.executeToolCalls(ctx, toolCalls, convID, assistantMsgID)
	return append(assistantBlocks, resultBlocks...), true, stopReason, inputTokens, outputTokens
}

// persistMsg writes the assistant message to the DB and returns any error.
// Used for intermediate streaming saves where failure is non-fatal.
//
// persistMsg 把 assistant 消息写入 DB，返回错误。用于中间 streaming 保存，失败非致命。
func (s *Service) persistMsg(
	ctx context.Context,
	msgID, convID, uid string,
	blocks []chatdomain.Block,
	status, stopReason string,
	inputTokens, outputTokens int,
) error {
	stamped := make([]chatdomain.Block, len(blocks))
	copy(stamped, blocks)
	for i := range stamped {
		stamped[i].MessageID = msgID
		stamped[i].Seq = i // global seq across all steps; per-step values would collide
		if stamped[i].ID == "" {
			stamped[i].ID = newBlockID()
		}
	}
	msg := &chatdomain.Message{
		ID:             msgID,
		ConversationID: convID,
		UserID:         uid,
		Role:           chatdomain.RoleAssistant,
		Status:         status,
		StopReason:     stopReason,
		InputTokens:    inputTokens,
		OutputTokens:   outputTokens,
		Blocks:         stamped,
	}
	if err := s.repo.Save(ctx, msg); err != nil {
		return fmt.Errorf("persistMsg: %w", err)
	}
	return nil
}

// finalPersist is used for the terminal save (completed/cancelled/error).
// On failure it publishes ChatError so the frontend knows the message was not persisted,
// and logs the error as critical.
//
// finalPersist 用于最终保存（completed/cancelled/error）。
// 失败时推 ChatError SSE 告知前端消息未落地，并记录为 critical 错误。
func (s *Service) finalPersist(
	ctx context.Context,
	msgID, convID, uid string,
	blocks []chatdomain.Block,
	status, stopReason string,
	inputTokens, outputTokens int,
) {
	// Use a fresh context so a cancelled stream does not prevent the terminal
	// DB write. The user ID must be preserved for the store's scoping query.
	// 用新 context 避免已取消的流阻止终态 DB 写入；需保留 user ID 供 store 过滤。
	saveCtx := reqctx.SetUserID(context.Background(), uid)
	if err := s.persistMsg(saveCtx, msgID, convID, uid, blocks, status, stopReason, inputTokens, outputTokens); err != nil {
		s.log.Error("CRITICAL: final assistant message persist failed — message lost",
			zap.String("msg_id", msgID),
			zap.String("conversation_id", convID),
			zap.Error(err))
		s.bridge.Publish(ctx, convID, eventsdomain.ChatError{
			ConversationID: convID,
			Code:           "INTERNAL_ERROR",
			Message:        "failed to save assistant message to database",
		})
	}
}

// collectToolCalls extracts ToolCallData from tool_call blocks.
//
// collectToolCalls 从 tool_call blocks 中提取 ToolCallData。
func collectToolCalls(blocks []chatdomain.Block) []chatdomain.ToolCallData {
	var out []chatdomain.ToolCallData
	for _, b := range blocks {
		if b.Type != chatdomain.BlockTypeToolCall {
			continue
		}
		var d chatdomain.ToolCallData
		if json.Unmarshal([]byte(b.Data), &d) == nil {
			out = append(out, d)
		}
	}
	return out
}

func extractTextContent(blocks []chatdomain.Block) string {
	var last string
	for _, b := range blocks {
		if b.Type == chatdomain.BlockTypeText {
			var d chatdomain.TextData
			if json.Unmarshal([]byte(b.Data), &d) == nil {
				last = d.Text
			}
		}
	}
	return last
}

// ── System prompt & auto-title ────────────────────────────────────────────────

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
	client, baseURL, err := s.llmFactory.Build(llminfra.Config{
		Provider: provider, ModelID: modelID, Key: creds.Key, BaseURL: creds.BaseURL,
	})
	if err != nil {
		return
	}

	tCtx, cancel := context.WithTimeout(titleCtx, 10*time.Second)
	defer cancel()

	req := llminfra.Request{
		ModelID: modelID, Key: creds.Key, BaseURL: baseURL,
		System: "Generate a short conversation title (5 words or fewer). Reply with ONLY the title, no punctuation.\n只返回标题本身，不超过 10 个字，不加标点。",
		Messages: []llminfra.LLMMessage{
			{Role: llminfra.RoleUser, Content: "Assistant said: " + truncate(assistantContent, 300)},
		},
	}
	title, err := llminfra.Generate(tCtx, client, req)
	if err != nil || title == "" {
		return
	}
	title = strings.TrimSpace(title)

	conv.Title = title
	conv.AutoTitled = true
	if err := s.convRepo.Save(titleCtx, conv); err != nil {
		s.log.Warn("auto-title save failed", zap.Error(err))
		return
	}
	s.bridge.Publish(titleCtx, conv.ID, eventsdomain.ConversationTitleUpdated{
		ConversationID: conv.ID, Title: title, AutoTitled: true,
	})
	s.log.Info("auto-title generated",
		zap.String("conversation_id", conv.ID), zap.String("title", title))
}

func (s *Service) publishError(ctx context.Context, conversationID, code, msg string) {
	s.bridge.Publish(ctx, conversationID, eventsdomain.ChatError{
		ConversationID: conversationID, Code: code, Message: msg,
	})
	s.log.Error("chat error",
		zap.String("conversation_id", conversationID),
		zap.String("code", code), zap.String("message", msg))
}
