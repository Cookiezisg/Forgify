// stream.go — Consumes one LLM iter.Seq stream, pushes SSE events, and
// assembles typed Blocks in stream-arrival order.
//
// Block ordering follows the stream: reasoning → text(preamble) → tool_calls.
// seq and created_at are stamped when each block is finalised, so they
// reflect the actual generation sequence rather than a post-hoc grouping.
//
// stream.go — 消费一次 LLM iter.Seq 流，推送 SSE 事件，按流到达顺序组装 Block。
// seq 和 created_at 在每个 block 完成时打入，真实反映生成顺序。
package chat

import (
	"context"
	"encoding/json"
	"iter"
	"strings"
	"time"

	agentapp "github.com/sunweilin/forgify/backend/internal/app/agent"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventsdomain "github.com/sunweilin/forgify/backend/internal/domain/events"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// toolAccum accumulates streaming fragments for one tool call.
//
// toolAccum 累积一个 tool call 的流式片段。
type toolAccum struct {
	id   string
	name string
	args strings.Builder
}

// streamBlockBuilder builds Blocks in the order events arrive from the LLM.
// It keeps track of the "current" open text/reasoning accumulator and closes
// it whenever the event type changes. Tool calls are batched in accums and
// finalised all at once at EventFinish.
//
// streamBlockBuilder 按事件到达顺序构建 Block。
// 跟踪当前正在累积的 text/reasoning block，类型切换时关闭并新开。
// tool call 批量累积，在 EventFinish 时按 index 顺序 finalize。
type streamBlockBuilder struct {
	blocks []chatdomain.Block
	seq    int

	// current open text or reasoning accumulator; "" means nothing open
	curType string
	curBuf  strings.Builder

	accums map[int]*toolAccum // ToolIndex → accumulator
}

func newStreamBlockBuilder() *streamBlockBuilder {
	return &streamBlockBuilder{accums: map[int]*toolAccum{}}
}

// switchTo closes the current open buffer (if any) and records it as a block,
// then resets the buffer for the given type. Pass "" to just close.
func (b *streamBlockBuilder) switchTo(t string) {
	if b.curBuf.Len() > 0 {
		d, _ := json.Marshal(chatdomain.TextData{Text: b.curBuf.String()})
		b.blocks = append(b.blocks, chatdomain.Block{
			ID: newBlockID(), Seq: b.seq, Type: b.curType,
			Data: string(d), CreatedAt: time.Now().UTC(),
		})
		b.seq++
		b.curBuf.Reset()
	}
	b.curType = t
}

func (b *streamBlockBuilder) appendText(s string) {
	if b.curType != chatdomain.BlockTypeText {
		b.switchTo(chatdomain.BlockTypeText)
	}
	b.curBuf.WriteString(s)
}

func (b *streamBlockBuilder) appendReasoning(s string) {
	if b.curType != chatdomain.BlockTypeReasoning {
		b.switchTo(chatdomain.BlockTypeReasoning)
	}
	b.curBuf.WriteString(s)
}

func (b *streamBlockBuilder) startTool(idx int, id, name string) {
	// Close any open text/reasoning block before tool calls begin.
	b.switchTo("")
	b.accums[idx] = &toolAccum{id: id, name: name}
}

// finalize closes any open buffer and appends all accumulated tool_call blocks
// in ToolIndex order. Called at EventFinish or stream end.
func (b *streamBlockBuilder) finalize() {
	b.switchTo("") // close trailing text/reasoning if any
	for i := range len(b.accums) {
		a, ok := b.accums[i]
		if !ok {
			continue
		}
		summary, args := parseToolArgs(a.args.String())
		d, _ := json.Marshal(chatdomain.ToolCallData{
			ID: a.id, Name: a.name, Summary: summary, Arguments: args,
		})
		b.blocks = append(b.blocks, chatdomain.Block{
			ID: newBlockID(), Seq: b.seq, Type: chatdomain.BlockTypeToolCall,
			Data: string(d), CreatedAt: time.Now().UTC(),
		})
		b.seq++
	}
}

// consumeStream reads the event stream, publishes SSE, and assembles Blocks
// in stream-arrival order. Returns the assembled blocks, stop reason, and
// token counts.
//
// consumeStream 读取事件流，推送 SSE，按到达顺序组装 Block。
func (s *Service) consumeStream(
	ctx context.Context,
	stream iter.Seq[llminfra.StreamEvent],
	convID, msgID string,
) (blocks []chatdomain.Block, stopReason string, inputTokens, outputTokens int) {
	builder := newStreamBlockBuilder()
	stopReason = chatdomain.StopReasonEndTurn

	for event := range stream {
		switch event.Type {
		case llminfra.EventText:
			builder.appendText(event.Delta)
			s.bridge.Publish(ctx, convID, eventsdomain.ChatToken{
				ConversationID: convID, MessageID: msgID, Delta: event.Delta,
			})

		case llminfra.EventReasoning:
			builder.appendReasoning(event.Delta)
			s.bridge.Publish(ctx, convID, eventsdomain.ChatReasoningToken{
				ConversationID: convID, MessageID: msgID, Delta: event.Delta,
			})

		case llminfra.EventToolStart:
			builder.startTool(event.ToolIndex, event.ToolID, event.ToolName)
			s.bridge.Publish(ctx, convID, eventsdomain.ChatToolCallStart{
				ConversationID: convID, MessageID: msgID,
				ToolCallID: event.ToolID, ToolName: event.ToolName,
			})

		case llminfra.EventToolDelta:
			if a := builder.accums[event.ToolIndex]; a != nil {
				a.args.WriteString(event.ArgsDelta)
			}

		case llminfra.EventFinish:
			if event.FinishReason == "length" {
				stopReason = chatdomain.StopReasonMaxTokens
			}
			if event.InputTokens > 0 {
				inputTokens = event.InputTokens
			}
			if event.OutputTokens > 0 {
				outputTokens = event.OutputTokens
			}

		case llminfra.EventError:
			if ctx.Err() != nil {
				stopReason = chatdomain.StopReasonCancelled
			} else {
				stopReason = chatdomain.StopReasonError
			}
		}
	}

	if ctx.Err() != nil && stopReason == chatdomain.StopReasonEndTurn {
		stopReason = chatdomain.StopReasonCancelled
	}

	builder.finalize()
	return builder.blocks, stopReason, inputTokens, outputTokens
}

// parseToolArgs extracts the "summary" field from a raw args JSON string and
// returns the summary value and the remaining arguments as a map.
// Falls back to {"raw": original} when the JSON is malformed.
//
// parseToolArgs 从原始 args JSON 中提取 "summary" 字段，
// 返回 summary 值和剩余参数 map。JSON 不合法时兜底为 {"raw": original}。
func parseToolArgs(argsJSON string) (summary string, args map[string]any) {
	summary, stripped := agentapp.StripSummary(argsJSON)
	if err := json.Unmarshal([]byte(stripped), &args); err != nil {
		args = map[string]any{"raw": argsJSON}
	}
	return summary, args
}
