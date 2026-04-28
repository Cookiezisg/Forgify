// runner.go — Queue management, processTask, and the main ReAct agent loop.
// One worker goroutine per conversation drains tasks sequentially; each task
// runs agentRun until the LLM stops calling tools or the step limit is reached.
//
// runner.go — 队列管理、processTask 和主 ReAct agent 循环。
// 每个 conversation 一个 worker goroutine 顺序消费任务；每个任务运行
// agentRun，直到 LLM 不再调用工具或达到步骤上限。
package chat

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"go.uber.org/zap"

	agentapp "github.com/sunweilin/forgify/backend/internal/app/agent"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	convdomain "github.com/sunweilin/forgify/backend/internal/domain/conversation"
	eventsdomain "github.com/sunweilin/forgify/backend/internal/domain/events"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
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

	// Per-task cancellable context so Cancel() can stop the running agent.
	// 每任务可取消 context，供 Cancel() 停止运行中的 agent。
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

	provider, modelID, err := s.modelPicker.PickForChat(agentCtx)
	if err != nil {
		s.publishError(agentCtx, conversationID, "MODEL_NOT_CONFIGURED", err.Error())
		return
	}
	creds, err := s.keyProvider.ResolveCredentials(agentCtx, provider)
	if err != nil {
		s.publishError(agentCtx, conversationID, "API_KEY_PROVIDER_NOT_FOUND", err.Error())
		return
	}
	client, baseURL, err := s.llmFactory.Build(llminfra.Config{
		Provider: provider, ModelID: modelID,
		Key: creds.Key, BaseURL: creds.BaseURL,
	})
	if err != nil {
		s.publishError(agentCtx, conversationID, "LLM_PROVIDER_ERROR", err.Error())
		return
	}

	baseReq := llminfra.Request{
		ModelID: modelID,
		Key:     creds.Key,
		BaseURL: baseURL,
		System:  s.buildSystemPrompt(agentCtx, task.conv),
		Tools:   agentapp.ToLLMDefs(s.tools),
	}
	s.agentRun(agentCtx, task.uid, task.conv, task.userMsgID, client, baseReq)
}

// ── agentRun ──────────────────────────────────────────────────────────────────

// maxSteps caps the ReAct loop to prevent runaway tool-calling cycles.
// maxSteps 限制 ReAct 循环次数，防止工具调用无限循环。
const maxSteps = 20

// agentRun runs the ReAct loop for one user turn. It calls streamLLM to get
// the LLM's response, executes any tool calls, writes DB checkpoints, and
// repeats until the LLM produces a final answer or the step limit is reached.
//
// agentRun 为一次用户回合运行 ReAct 循环。调用 streamLLM 获取 LLM 回复，
// 执行工具调用，写 DB checkpoint，重复直到 LLM 产生最终答案或达到步骤上限。
func (s *Service) agentRun(
	ctx context.Context,
	uid string,
	conv *convdomain.Conversation,
	userMsgID string,
	client llminfra.Client,
	baseReq llminfra.Request,
) {
	msgID := newMsgID()

	history, err := s.buildHistory(ctx, conv.ID, userMsgID)
	if err != nil {
		s.publishError(ctx, conv.ID, "INTERNAL_ERROR", err.Error())
		return
	}

	var (
		allBlocks    []chatdomain.Block
		totalInput   int
		totalOutput  int
		stopReason   = chatdomain.StopReasonEndTurn
		finalWritten bool
	)

	for step := range maxSteps {
		req := baseReq
		req.Messages = history

		aBlocks, toolCalls, sr, iT, oT := s.streamLLM(ctx, client, req, conv.ID, msgID)
		allBlocks = append(allBlocks, aBlocks...)
		totalInput += iT
		totalOutput += oT
		if sr != "" {
			stopReason = sr
		}

		if stopReason == chatdomain.StopReasonCancelled || stopReason == chatdomain.StopReasonError {
			status := chatdomain.StatusCancelled
			if stopReason == chatdomain.StopReasonError {
				status = chatdomain.StatusError
			}
			s.writeDB(ctx, msgID, conv.ID, uid, allBlocks, status, stopReason, totalInput, totalOutput, true)
			finalWritten = true
			break
		}

		if len(toolCalls) == 0 {
			// No tool calls — LLM produced its final answer.
			// 无工具调用——LLM 产生最终答案。
			s.writeDB(ctx, msgID, conv.ID, uid, allBlocks, chatdomain.StatusCompleted, stopReason, totalInput, totalOutput, true)
			finalWritten = true
			break
		}

		rBlocks := s.runTools(ctx, toolCalls, conv.ID, msgID)
		allBlocks = append(allBlocks, rBlocks...)

		// Streaming checkpoint — non-fatal; the final write will overwrite.
		// Streaming checkpoint——失败非致命，最终写入会覆盖。
		s.writeDB(ctx, msgID, conv.ID, uid, allBlocks, chatdomain.StatusStreaming, "", totalInput, totalOutput, false)

		history, err = extendHistory(history, aBlocks, rBlocks)
		if err != nil {
			s.log.Error("extend history failed",
				zap.String("conversation_id", conv.ID), zap.Error(err))
			stopReason = chatdomain.StopReasonError
			s.writeDB(ctx, msgID, conv.ID, uid, allBlocks, chatdomain.StatusError, stopReason, totalInput, totalOutput, true)
			finalWritten = true
			break
		}
		// TODO: context compaction — history = s.compactor.MaybeCompact(ctx, history)

		s.log.Debug("react step complete",
			zap.Int("step", step), zap.String("conversation_id", conv.ID))
	}

	if !finalWritten {
		// Step limit reached.
		// 达到步骤上限。
		stopReason = chatdomain.StopReasonMaxTokens
		s.writeDB(ctx, msgID, conv.ID, uid, allBlocks, chatdomain.StatusCompleted, stopReason, totalInput, totalOutput, true)
	}

	s.bridge.Publish(ctx, conv.ID, eventsdomain.ChatDone{
		ConversationID: conv.ID,
		MessageID:      msgID,
		StopReason:     stopReason,
		InputTokens:    totalInput,
		OutputTokens:   totalOutput,
	})
	s.log.Info("agent run complete",
		zap.String("conversation_id", conv.ID),
		zap.String("stop_reason", stopReason),
		zap.Int("input_tokens", totalInput),
		zap.Int("output_tokens", totalOutput))

	if conv.Title == "" && !conv.AutoTitled {
		go s.autoTitle(context.Background(), conv, uid, extractTextContent(allBlocks))
	}
}

// ── writeDB ───────────────────────────────────────────────────────────────────

// writeDB persists the assistant message with its blocks. When fatal is true,
// a write failure publishes ChatError and logs as critical — for terminal saves.
// When fatal is false, failure only warns — for streaming checkpoints.
//
// writeDB 持久化 assistant 消息及 blocks。fatal=true 时写入失败推 ChatError
// 并记录为 critical——用于终态写入。fatal=false 时失败只记 warn——用于 streaming checkpoint。
func (s *Service) writeDB(
	ctx context.Context,
	msgID, convID, uid string,
	blocks []chatdomain.Block,
	status, stopReason string,
	inputTokens, outputTokens int,
	fatal bool,
) {
	saveCtx := ctx
	if fatal {
		// Fresh context: a cancelled stream must not block the terminal write.
		// 新 context：已取消的流不能阻止终态写入。
		saveCtx = reqctxpkg.SetUserID(context.Background(), uid)
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
		Blocks:         stampBlocks(blocks, msgID),
	}
	if err := s.repo.Save(saveCtx, msg); err != nil {
		if fatal {
			s.log.Error("CRITICAL: final assistant message persist failed — message lost",
				zap.String("msg_id", msgID), zap.String("conversation_id", convID), zap.Error(err))
			s.bridge.Publish(ctx, convID, eventsdomain.ChatError{
				ConversationID: convID, Code: "INTERNAL_ERROR",
				Message: "failed to save assistant message to database",
			})
		} else {
			s.log.Warn("streaming checkpoint persist failed, continuing",
				zap.String("msg_id", msgID), zap.Error(err))
		}
	}
}

// stampBlocks assigns global seq and messageID to every block before a DB write.
// stampBlocks 在写 DB 前为每个 block 打上全局 seq 和 messageID。
func stampBlocks(blocks []chatdomain.Block, msgID string) []chatdomain.Block {
	stamped := make([]chatdomain.Block, len(blocks))
	copy(stamped, blocks)
	for i := range stamped {
		stamped[i].MessageID = msgID
		stamped[i].Seq = i
		if stamped[i].ID == "" {
			stamped[i].ID = newBlockID()
		}
	}
	return stamped
}

// ── System prompt & helpers ───────────────────────────────────────────────────

func (s *Service) buildSystemPrompt(ctx context.Context, conv *convdomain.Conversation) string {
	var sb strings.Builder
	sb.WriteString("You are Forgify, an AI assistant that helps users build tools, automate workflows, and work with data.")
	if conv.SystemPrompt != "" {
		sb.WriteString("\n\n")
		sb.WriteString(conv.SystemPrompt)
	}
	if reqctxpkg.GetLocale(ctx) == reqctxpkg.LocaleZhCN {
		sb.WriteString("\n\nPlease respond in Chinese (Simplified) unless the user writes in another language.")
	}
	return sb.String()
}

func (s *Service) autoTitle(ctx context.Context, conv *convdomain.Conversation, uid, assistantContent string) {
	titleCtx := reqctxpkg.SetUserID(ctx, uid)
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
	conv.Title = strings.TrimSpace(title)
	conv.AutoTitled = true
	if err := s.convRepo.Save(titleCtx, conv); err != nil {
		s.log.Warn("auto-title save failed", zap.Error(err))
		return
	}
	s.bridge.Publish(titleCtx, conv.ID, eventsdomain.ConversationTitleUpdated{
		ConversationID: conv.ID, Title: conv.Title, AutoTitled: true,
	})
	s.log.Info("auto-title generated",
		zap.String("conversation_id", conv.ID), zap.String("title", conv.Title))
}

func (s *Service) publishError(ctx context.Context, conversationID, code, msg string) {
	s.bridge.Publish(ctx, conversationID, eventsdomain.ChatError{
		ConversationID: conversationID, Code: code, Message: msg,
	})
	s.log.Error("chat error",
		zap.String("conversation_id", conversationID),
		zap.String("code", code), zap.String("message", msg))
}

// extractTextContent returns the last text block's content from a block slice.
// Used to seed auto-titling after the agent run completes.
//
// extractTextContent 从 block 列表中返回最后一个 text block 的内容。
// 用于 agent 运行完成后提供自动命名的素材。
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
