// history.go — LLM history construction from DB messages and in-loop extension.
// buildHistory is called once per user turn (before the loop).
// extendHistory is called after each tool-calling step (inside the loop).
// Both paths share blocksToAssistantLLM — there is only one converter.
//
// history.go — 从 DB 消息构建 LLM 历史 + 循环内扩展。
// buildHistory 每个用户回合调用一次（循环前）。
// extendHistory 在每个工具调用步骤后调用（循环内）。
// 两条路径共用 blocksToAssistantLLM——只有一个转换器。
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

// maxHistoryMessages is the maximum number of past messages loaded per LLM call.
// Older messages beyond this limit are silently dropped.
//
// maxHistoryMessages 是每次 LLM 调用加载的历史消息上限，超出的旧消息静默丢弃。
const maxHistoryMessages = 200

// buildHistory loads completed messages from the DB and returns them as LLM
// wire messages. currentUserMsgID is excluded from the main scan and appended
// last, ensuring the LLM always sees the triggering message at the end
// regardless of created_at ordering (prevents the fast-send race condition).
//
// buildHistory 从 DB 加载已完成消息并转为 LLM 协议格式。
// currentUserMsgID 排除在主扫描外并追加到末尾，保证 LLM 以该消息作为待回复轮次，
// 不受快速连发时 created_at 竞态的影响。
func (s *Service) buildHistory(ctx context.Context, convID, currentUserMsgID string) ([]llminfra.LLMMessage, error) {
	rows, _, err := s.repo.ListByConversation(ctx, convID, chatdomain.ListFilter{Limit: maxHistoryMessages})
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
		msgs, err := s.blocksToLLM(ctx, m)
		if err != nil {
			return nil, fmt.Errorf("buildHistory: message %q: %w", m.ID, err)
		}
		out = append(out, msgs...)
	}

	if currentUserMsg != nil {
		msg, err := s.buildUserLLMMessage(ctx, currentUserMsg)
		if err != nil {
			return nil, fmt.Errorf("buildHistory: current user msg %q: %w", currentUserMsgID, err)
		}
		out = append(out, msg)
	}
	return out, nil
}

// extendHistory appends one ReAct step's contribution to the running history.
// aBlocks are the assistant's response; rBlocks are the tool results.
// This is the single point where in-loop history grows — same blocksToAssistantLLM
// converter used by buildHistory for DB-loaded messages.
//
// extendHistory 把一个 ReAct 步骤的贡献追加到运行中的历史。
// aBlocks 是 assistant 回复；rBlocks 是工具结果。
// 这是循环内历史增长的唯一入口，与 buildHistory 使用相同的转换器。
func extendHistory(history []llminfra.LLMMessage, aBlocks, rBlocks []chatdomain.Block) ([]llminfra.LLMMessage, error) {
	msgs, err := blocksToAssistantLLM(append(aBlocks, rBlocks...))
	if err != nil {
		return nil, err
	}
	return append(history, msgs...), nil
}

// blocksToLLM converts one persisted Message to LLM wire messages.
//
// blocksToLLM 把一条已持久化的 Message 转为 LLM 协议消息。
func (s *Service) blocksToLLM(ctx context.Context, m *chatdomain.Message) ([]llminfra.LLMMessage, error) {
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

// blocksToAssistantLLM converts an assistant turn's blocks into LLM wire
// messages. A turn with tool calls expands to:
//
//	[assistant{text, reasoning, toolCalls}] + [N × role=tool messages]
//
// Used by both buildHistory (DB-loaded) and extendHistory (in-loop accumulation).
//
// blocksToAssistantLLM 把一个 assistant 回合的 blocks 转为 LLM 协议消息。
// 含工具调用的回合展开为：
//
//	[assistant{text, reasoning, toolCalls}] + [N 条 role=tool 消息]
func blocksToAssistantLLM(blocks []chatdomain.Block) ([]llminfra.LLMMessage, error) {
	assistant := llminfra.LLMMessage{Role: llminfra.RoleAssistant}
	var toolResults []llminfra.LLMMessage

	for _, b := range blocks {
		switch b.Type {
		case chatdomain.BlockTypeReasoning:
			var d chatdomain.TextData
			if err := json.Unmarshal([]byte(b.Data), &d); err != nil {
				return nil, fmt.Errorf("blocksToAssistantLLM: reasoning block %q: %w", b.ID, err)
			}
			assistant.ReasoningContent = d.Text

		case chatdomain.BlockTypeText:
			var d chatdomain.TextData
			if err := json.Unmarshal([]byte(b.Data), &d); err != nil {
				return nil, fmt.Errorf("blocksToAssistantLLM: text block %q: %w", b.ID, err)
			}
			assistant.Content = d.Text

		case chatdomain.BlockTypeToolCall:
			var d chatdomain.ToolCallData
			if err := json.Unmarshal([]byte(b.Data), &d); err != nil {
				return nil, fmt.Errorf("blocksToAssistantLLM: tool_call block %q: %w", b.ID, err)
			}
			// summary is not sent back to the LLM; re-marshal arguments without it.
			// summary 不回传给 LLM；重新序列化 arguments 以排除 summary。
			argsJSON, _ := json.Marshal(d.Arguments)
			assistant.ToolCalls = append(assistant.ToolCalls, llminfra.LLMToolCall{
				ID: d.ID, Name: d.Name, Arguments: string(argsJSON),
			})

		case chatdomain.BlockTypeToolResult:
			var d chatdomain.ToolResultData
			if err := json.Unmarshal([]byte(b.Data), &d); err != nil {
				return nil, fmt.Errorf("blocksToAssistantLLM: tool_result block %q: %w", b.ID, err)
			}
			toolResults = append(toolResults, llminfra.LLMMessage{
				Role: llminfra.RoleTool, Content: d.Result, ToolCallID: d.ToolCallID,
			})
		}
	}
	return append([]llminfra.LLMMessage{assistant}, toolResults...), nil
}

// buildUserLLMMessage converts a user message's blocks to a single LLM message.
// Text blocks become inline content; attachment blocks resolve to ContentParts
// (image → base64, document → extracted text). Attachment failures are soft:
// logged and skipped so the rest of the message still reaches the LLM.
//
// buildUserLLMMessage 把 user 消息的 blocks 转为单条 LLM 消息。
// text block 变为内联 content；attachment block 解析为 ContentPart
// （图片 → base64，文档 → 提取文本）。附件失败属于软失败：记录后跳过。
func (s *Service) buildUserLLMMessage(ctx context.Context, m *chatdomain.Message) (llminfra.LLMMessage, error) {
	msg := llminfra.LLMMessage{Role: llminfra.RoleUser}
	var parts []llminfra.ContentPart

	for _, b := range m.Blocks {
		switch b.Type {
		case chatdomain.BlockTypeText:
			var d chatdomain.TextData
			if err := json.Unmarshal([]byte(b.Data), &d); err != nil {
				return llminfra.LLMMessage{}, fmt.Errorf("buildUserLLMMessage: text block %q: %w", b.ID, err)
			}
			parts = append(parts, llminfra.ContentPart{Type: "text", Text: d.Text})
		case chatdomain.BlockTypeAttachmentRef:
			part, err := s.attachmentToPart(ctx, b)
			if err != nil {
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
// Images → image_url (base64 data URL); documents → inlined text part.
//
// attachmentToPart 把 attachment_ref block 解析为 ContentPart。
// 图片 → image_url（base64 data URL）；文档 → 内联文本。
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
