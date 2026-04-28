// stream.go — One LLM call: consume stream events, push SSE, assemble Blocks.
// No database writes happen here. The caller (agentRun) owns persistence.
//
// stream.go — 单次 LLM 调用：消费流事件、推 SSE、组装 Block。
// 不写 DB——持久化由调用方 agentRun 负责。
package chat

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	agentapp "github.com/sunweilin/forgify/backend/internal/app/agent"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventsdomain "github.com/sunweilin/forgify/backend/internal/domain/events"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// toolAccum accumulates streaming fragments for one tool call.
//
// toolAccum 累积单个 tool call 的流式片段。
type toolAccum struct {
	id, name string
	args     strings.Builder
}

// streamLLM executes one LLM call, publishing SSE events in real-time as each
// stream event arrives. It assembles typed Blocks from the accumulated buffers
// and extracts any tool calls the LLM requested.
//
// streamLLM 执行一次 LLM 调用，每个流事件到达时实时推送 SSE。
// 从累积缓冲中组装 Block，并提取 LLM 请求的工具调用。
func (s *Service) streamLLM(
	ctx context.Context,
	client llminfra.Client,
	req llminfra.Request,
	convID, msgID string,
) (blocks []chatdomain.Block, toolCalls []chatdomain.ToolCallData, stopReason string, inputTokens, outputTokens int) {
	var textBuf, reasonBuf strings.Builder
	accums := map[int]*toolAccum{}
	stopReason = chatdomain.StopReasonEndTurn

	for event := range client.Stream(ctx, req) {
		switch event.Type {
		case llminfra.EventText:
			textBuf.WriteString(event.Delta)
			s.bridge.Publish(ctx, convID, eventsdomain.ChatToken{
				ConversationID: convID, MessageID: msgID, Delta: event.Delta,
			})

		case llminfra.EventReasoning:
			reasonBuf.WriteString(event.Delta)
			s.bridge.Publish(ctx, convID, eventsdomain.ChatReasoningToken{
				ConversationID: convID, MessageID: msgID, Delta: event.Delta,
			})

		case llminfra.EventToolStart:
			accums[event.ToolIndex] = &toolAccum{id: event.ToolID, name: event.ToolName}
			s.bridge.Publish(ctx, convID, eventsdomain.ChatToolCallStart{
				ConversationID: convID, MessageID: msgID,
				ToolCallID: event.ToolID, ToolName: event.ToolName,
			})
			// TODO (A1): mid-stream tool execution — when EventToolStart(N+1) arrives,
			// accums[N].args is complete; start executing tool N without waiting for EventFinish.
			// TODO (A1)：mid-stream 工具执行——当 EventToolStart(N+1) 到达时，
			// accums[N].args 已完整，无需等 EventFinish 即可开始执行工具 N。

		case llminfra.EventToolDelta:
			if a := accums[event.ToolIndex]; a != nil {
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

	blocks = assembleBlocks(textBuf.String(), reasonBuf.String(), accums)
	toolCalls = extractToolCalls(blocks)
	return
}

// assembleBlocks builds the final Block slice from accumulated stream buffers.
// Order: reasoning → text → tool_calls (by ToolIndex). Seq is stamped locally
// here and overwritten globally by stampBlocks when written to the database.
//
// assembleBlocks 从流缓冲组装最终的 Block 列表。
// 顺序：reasoning → text → tool_calls（按 ToolIndex）。
// Seq 在此打本地值，写 DB 时由 stampBlocks 覆盖为全局值。
func assembleBlocks(text, reasoning string, accums map[int]*toolAccum) []chatdomain.Block {
	var blocks []chatdomain.Block
	seq := 0

	if reasoning != "" {
		d, _ := json.Marshal(chatdomain.TextData{Text: reasoning})
		blocks = append(blocks, chatdomain.Block{
			ID: newBlockID(), Seq: seq, Type: chatdomain.BlockTypeReasoning,
			Data: string(d), CreatedAt: time.Now().UTC(),
		})
		seq++
	}
	if text != "" {
		d, _ := json.Marshal(chatdomain.TextData{Text: text})
		blocks = append(blocks, chatdomain.Block{
			ID: newBlockID(), Seq: seq, Type: chatdomain.BlockTypeText,
			Data: string(d), CreatedAt: time.Now().UTC(),
		})
		seq++
	}
	for i := range len(accums) {
		a, ok := accums[i]
		if !ok {
			continue
		}
		summary, args := parseToolArgs(a.args.String())
		d, _ := json.Marshal(chatdomain.ToolCallData{
			ID: a.id, Name: a.name, Summary: summary, Arguments: args,
		})
		blocks = append(blocks, chatdomain.Block{
			ID: newBlockID(), Seq: seq, Type: chatdomain.BlockTypeToolCall,
			Data: string(d), CreatedAt: time.Now().UTC(),
		})
		seq++
	}
	return blocks
}

// extractToolCalls pulls ToolCallData out of tool_call typed blocks.
//
// extractToolCalls 从 tool_call 类型的 block 中提取 ToolCallData。
func extractToolCalls(blocks []chatdomain.Block) []chatdomain.ToolCallData {
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

// parseToolArgs extracts the "summary" field from a raw args JSON string and
// returns the summary and the remaining arguments as a map.
// Falls back to {"raw": original} when the JSON is malformed.
//
// parseToolArgs 从原始 args JSON 中提取 "summary"，返回 summary 和剩余参数 map。
// JSON 不合法时兜底为 {"raw": original}。
func parseToolArgs(argsJSON string) (summary string, args map[string]any) {
	summary, stripped := agentapp.StripSummary(argsJSON)
	if err := json.Unmarshal([]byte(stripped), &args); err != nil {
		args = map[string]any{"raw": argsJSON}
	}
	return summary, args
}
