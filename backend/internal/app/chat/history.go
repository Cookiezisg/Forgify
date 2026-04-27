// history.go — LLM message history reconstruction from stored Blocks,
// and the shared block→LLM-message conversion used by both the pipeline
// (in-memory accumulation) and the DB loader (history rebuild).
//
// history.go — 从已存储 Block 重建 LLM 消息历史，
// 以及被 pipeline（内存累积）和 DB 加载器（历史重建）共用的 block→LLM 消息转换。
package chat

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	chatinfra "github.com/sunweilin/forgify/backend/internal/infra/chat"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// buildLLMHistory loads up to 200 messages for the conversation and converts
// each one to the LLM wire format. Streaming/pending messages are skipped —
// they represent in-progress turns that should not appear in history.
//
// buildLLMHistory 加载最多 200 条消息并转为 LLM 协议格式。
// streaming/pending 消息跳过——它们是正在处理中的回合，不应出现在历史中。
// maxHistoryMessages is the maximum number of past messages loaded per LLM call.
// Older messages beyond this limit are silently dropped.
//
// maxHistoryMessages 是每次 LLM 调用加载的历史消息上限，超出的旧消息静默丢弃。
const maxHistoryMessages = 200

// buildLLMHistory loads up to maxHistoryMessages completed messages and converts
// them to LLM wire format. currentUserMsgID is the user message that triggered
// the current task: it is excluded from the history scan and appended last so
// the conversation always ends with the user's latest message regardless of DB
// insertion order. This prevents the queued-message race where a later user
// message has an earlier created_at than the assistant that follows the first.
//
// buildLLMHistory 加载最多 maxHistoryMessages 条完整消息并转为 LLM 协议格式。
// currentUserMsgID 是触发当前任务的 user 消息：从历史扫描中排除，最后追加，
// 保证对话始终以用户最新消息结尾，避免队列消息在 DB 插入顺序上的竞态。
func (s *Service) buildLLMHistory(ctx context.Context, conversationID, currentUserMsgID string) ([]llminfra.LLMMessage, error) {
	rows, _, err := s.repo.ListByConversation(ctx, conversationID, chatdomain.ListFilter{Limit: maxHistoryMessages})
	if err != nil {
		return nil, err
	}
	var out []llminfra.LLMMessage
	var currentUserMsg *chatdomain.Message
	for _, m := range rows {
		if m.Status == chatdomain.StatusStreaming || m.Status == chatdomain.StatusPending {
			continue
		}
		if m.ID == currentUserMsgID {
			currentUserMsg = m
			continue
		}
		msgs, err := s.toLLMMessages(ctx, m)
		if err != nil {
			return nil, fmt.Errorf("buildLLMHistory: message %q: %w", m.ID, err)
		}
		out = append(out, msgs...)
	}
	// Append the current user message last so the LLM always sees it as the
	// turn to respond to, regardless of its created_at relative to other rows.
	// 最后追加 current user message，让 LLM 始终以其为待回复轮次。
	if currentUserMsg != nil {
		msg, err := s.buildUserLLMMessage(ctx, currentUserMsg)
		if err != nil {
			return nil, fmt.Errorf("buildLLMHistory: current user message %q: %w", currentUserMsgID, err)
		}
		out = append(out, msg)
	}
	return out, nil
}

func (s *Service) toLLMMessages(ctx context.Context, m *chatdomain.Message) ([]llminfra.LLMMessage, error) {
	switch m.Role {
	case chatdomain.RoleUser:
		msg, err := s.buildUserLLMMessage(ctx, m)
		if err != nil {
			return nil, err
		}
		return []llminfra.LLMMessage{msg}, nil
	case chatdomain.RoleAssistant:
		return blocksToAssistantLLM(m.Blocks)
	}
	return nil, nil
}

// ── Shared block→LLM conversion ───────────────────────────────────────────────

// blocksToAssistantLLM converts a slice of blocks (from one assistant turn)
// into the LLM wire format. Used in two places:
//   - history.go: rebuilding DB history before a new conversation turn
//   - pipeline.go: building per-step history during the ReAct loop
//
// An assistant turn with tool calls expands to:
//
//	[assistant{toolCalls, reasoning}] + [N × role=tool messages]
//
// blocksToAssistantLLM 把一个 assistant 回合的 blocks 转为 LLM 协议格式。
// 在两处使用：
//   - history.go：新对话轮次前从 DB 重建历史
//   - pipeline.go：ReAct loop 中构建逐步历史
//
// 含 tool call 的 assistant 回合展开为：
//
//	[assistant{toolCalls, reasoning}] + [N 条 role=tool 消息]
func blocksToAssistantLLM(blocks []chatdomain.Block) ([]llminfra.LLMMessage, error) {
	assistant := llminfra.LLMMessage{Role: llminfra.RoleAssistant}
	var toolResults []llminfra.LLMMessage

	for _, b := range blocks {
		switch b.Type {
		case chatdomain.BlockTypeReasoning:
			var d chatdomain.TextData
			if err := json.Unmarshal([]byte(b.Data), &d); err != nil {
				return nil, fmt.Errorf("blocksToAssistantLLM: unmarshal reasoning block %q: %w", b.ID, err)
			}
			assistant.ReasoningContent = d.Text

		case chatdomain.BlockTypeText:
			var d chatdomain.TextData
			if err := json.Unmarshal([]byte(b.Data), &d); err != nil {
				return nil, fmt.Errorf("blocksToAssistantLLM: unmarshal text block %q: %w", b.ID, err)
			}
			assistant.Content = d.Text

		case chatdomain.BlockTypeToolCall:
			var d chatdomain.ToolCallData
			if err := json.Unmarshal([]byte(b.Data), &d); err != nil {
				return nil, fmt.Errorf("blocksToAssistantLLM: unmarshal tool_call block %q: %w", b.ID, err)
			}
			// summary is never sent back to the LLM; d.Arguments came from JSON so remarshal is safe.
			// summary 不回传给 LLM；d.Arguments 来源于 JSON，remarshal 安全。
			argsJSON, _ := json.Marshal(d.Arguments)
			assistant.ToolCalls = append(assistant.ToolCalls, llminfra.LLMToolCall{
				ID: d.ID, Name: d.Name, Arguments: string(argsJSON),
			})

		case chatdomain.BlockTypeToolResult:
			var d chatdomain.ToolResultData
			if err := json.Unmarshal([]byte(b.Data), &d); err != nil {
				return nil, fmt.Errorf("blocksToAssistantLLM: unmarshal tool_result block %q: %w", b.ID, err)
			}
			toolResults = append(toolResults, llminfra.LLMMessage{
				Role:       llminfra.RoleTool,
				Content:    d.Result,
				ToolCallID: d.ToolCallID,
			})
		}
	}

	return append([]llminfra.LLMMessage{assistant}, toolResults...), nil
}

// ── User message building ─────────────────────────────────────────────────────

func (s *Service) buildUserLLMMessage(ctx context.Context, m *chatdomain.Message) (llminfra.LLMMessage, error) {
	msg := llminfra.LLMMessage{Role: llminfra.RoleUser}
	var parts []llminfra.ContentPart

	for _, b := range m.Blocks {
		switch b.Type {
		case chatdomain.BlockTypeText:
			var d chatdomain.TextData
			if err := json.Unmarshal([]byte(b.Data), &d); err != nil {
				return llminfra.LLMMessage{}, fmt.Errorf("buildUserLLMMessage: unmarshal text block %q: %w", b.ID, err)
			}
			parts = append(parts, llminfra.ContentPart{Type: "text", Text: d.Text})
		case chatdomain.BlockTypeAttachmentRef:
			part, err := s.attachmentToPart(ctx, b)
			if err != nil {
				// Attachment loading is a soft failure — log and skip so the rest
				// of the message still reaches the LLM.
				// 附件加载属于软失败——记录并跳过，消息其余部分仍发给 LLM。
				s.log.Warn("skipping attachment in LLM history", zap.Error(err))
				continue
			}
			parts = append(parts, *part)
		}
	}

	if len(parts) == 1 && parts[0].Type == "text" {
		msg.Content = parts[0].Text
		return msg, nil
	}
	msg.Parts = parts
	return msg, nil
}

// attachmentToPart resolves an attachment_ref block to a ContentPart.
// Images → image_url (base64); text documents → inlined text part.
//
// attachmentToPart 把 attachment_ref block 解析为 ContentPart。
// 图片 → image_url（base64）；文本文档 → 内联 text。
func (s *Service) attachmentToPart(ctx context.Context, b chatdomain.Block) (*llminfra.ContentPart, error) {
	var d chatdomain.AttachmentRefData
	if err := json.Unmarshal([]byte(b.Data), &d); err != nil {
		return nil, fmt.Errorf("attachmentToPart: unmarshal block %q: %w", b.ID, err)
	}
	att, err := s.repo.GetAttachment(ctx, d.AttachmentID)
	if err != nil {
		return nil, fmt.Errorf("attachmentToPart: get attachment %q: %w", d.AttachmentID, err)
	}

	if chatinfra.IsImage(att.MimeType) {
		data, err := readAndEncode(att.StoragePath)
		if err != nil {
			return nil, fmt.Errorf("attachmentToPart: encode image %q: %w", att.ID, err)
		}
		return &llminfra.ContentPart{
			Type:     "image_url",
			ImageURL: "data:" + att.MimeType + ";base64," + data,
		}, nil
	}

	text, err := chatinfra.Extract(att.StoragePath, att.MimeType)
	if err != nil {
		return nil, fmt.Errorf("attachmentToPart: extract %q: %w", att.ID, err)
	}
	return &llminfra.ContentPart{
		Type: "text",
		Text: fmt.Sprintf("\n\n[附件: %s]\n%s", att.FileName, text),
	}, nil
}
