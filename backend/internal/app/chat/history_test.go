// history_test.go — unit tests for LLM message history reconstruction.
// Tests blocksToAssistantLLM and blocksToAssistantLLM with synthetic blocks.
//
// history_test.go — LLM 消息历史重建的单元测试。
// 用合成 block 测试 blocksToAssistantLLM 和 blocksToAssistantLLM。
package chat

import (
	"encoding/json"
	"testing"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func makeBlock(id string, seq int, blockType string, data any) chatdomain.Block {
	d, _ := json.Marshal(data)
	return chatdomain.Block{ID: id, Seq: seq, Type: blockType, Data: string(d)}
}

func msgWithBlocks(blocks ...chatdomain.Block) *chatdomain.Message {
	return &chatdomain.Message{
		ID:     "msg-1",
		Role:   chatdomain.RoleAssistant,
		Status: chatdomain.StatusCompleted,
		Blocks: blocks,
	}
}

// ── blocksToAssistantLLM ─────────────────────────────────────────────────

func TestBuildAssistant_TextOnly(t *testing.T) {
	m := msgWithBlocks(
		makeBlock("b1", 0, chatdomain.BlockTypeText, chatdomain.TextData{Text: "Hello world"}),
	)
	msgs, err := blocksToAssistantLLM(m.Blocks)
	if err != nil {
		t.Fatalf("blocksToAssistantLLM: %v", err)
	}

	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != llminfra.RoleAssistant {
		t.Errorf("role = %q, want assistant", msgs[0].Role)
	}
	if msgs[0].Content != "Hello world" {
		t.Errorf("content = %q, want 'Hello world'", msgs[0].Content)
	}
}

func TestBuildAssistant_WithReasoning(t *testing.T) {
	m := msgWithBlocks(
		makeBlock("b1", 0, chatdomain.BlockTypeReasoning, chatdomain.TextData{Text: "Let me think"}),
		makeBlock("b2", 1, chatdomain.BlockTypeText, chatdomain.TextData{Text: "Answer"}),
	)
	msgs, err := blocksToAssistantLLM(m.Blocks)
	if err != nil {
		t.Fatalf("blocksToAssistantLLM: %v", err)
	}

	if len(msgs) != 1 {
		t.Fatalf("want 1 assistant message, got %d", len(msgs))
	}
	if msgs[0].ReasoningContent != "Let me think" {
		t.Errorf("reasoning = %q", msgs[0].ReasoningContent)
	}
	if msgs[0].Content != "Answer" {
		t.Errorf("content = %q", msgs[0].Content)
	}
}

func TestBuildAssistant_WithToolCall(t *testing.T) {
	args := map[string]any{"city": "Beijing"}
	m := msgWithBlocks(
		makeBlock("b1", 0, chatdomain.BlockTypeToolCall, chatdomain.ToolCallData{
			ID: "call_1", Name: "get_weather", Summary: "Checking weather", Arguments: args,
		}),
		makeBlock("b2", 1, chatdomain.BlockTypeToolResult, chatdomain.ToolResultData{
			ToolCallID: "call_1", OK: true, Result: "晴，25°C",
		}),
	)
	msgs, err := blocksToAssistantLLM(m.Blocks)
	if err != nil {
		t.Fatalf("blocksToAssistantLLM: %v", err)
	}

	// Should produce: [assistant(tool_calls), tool(result)]
	// 应产生：[assistant(tool_calls), tool(result)]
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}

	assistant := msgs[0]
	if assistant.Role != llminfra.RoleAssistant {
		t.Errorf("msgs[0] role = %q", assistant.Role)
	}
	if len(assistant.ToolCalls) != 1 {
		t.Fatalf("want 1 tool call, got %d", len(assistant.ToolCalls))
	}
	if assistant.ToolCalls[0].Name != "get_weather" || assistant.ToolCalls[0].ID != "call_1" {
		t.Errorf("tool call: %+v", assistant.ToolCalls[0])
	}
	// Summary must NOT be in the arguments sent back to the LLM.
	// Summary 不得出现在回传给 LLM 的 arguments 中。
	var argMap map[string]any
	json.Unmarshal([]byte(assistant.ToolCalls[0].Arguments), &argMap)
	if _, hasSummary := argMap["summary"]; hasSummary {
		t.Error("summary should not be in LLM tool call arguments")
	}
	if argMap["city"] != "Beijing" {
		t.Errorf("city = %v", argMap["city"])
	}

	toolResult := msgs[1]
	if toolResult.Role != llminfra.RoleTool {
		t.Errorf("msgs[1] role = %q, want tool", toolResult.Role)
	}
	if toolResult.ToolCallID != "call_1" {
		t.Errorf("tool_call_id = %q", toolResult.ToolCallID)
	}
	if toolResult.Content != "晴，25°C" {
		t.Errorf("result = %q", toolResult.Content)
	}
}

func TestBuildAssistant_MultipleToolCalls(t *testing.T) {
	args := map[string]any{}
	m := msgWithBlocks(
		makeBlock("b1", 0, chatdomain.BlockTypeToolCall, chatdomain.ToolCallData{
			ID: "call_1", Name: "t1", Arguments: args,
		}),
		makeBlock("b2", 1, chatdomain.BlockTypeToolCall, chatdomain.ToolCallData{
			ID: "call_2", Name: "t2", Arguments: args,
		}),
		makeBlock("b3", 2, chatdomain.BlockTypeToolResult, chatdomain.ToolResultData{
			ToolCallID: "call_1", OK: true, Result: "r1",
		}),
		makeBlock("b4", 3, chatdomain.BlockTypeToolResult, chatdomain.ToolResultData{
			ToolCallID: "call_2", OK: true, Result: "r2",
		}),
	)
	msgs, err := blocksToAssistantLLM(m.Blocks)
	if err != nil {
		t.Fatalf("blocksToAssistantLLM: %v", err)
	}

	// 1 assistant + 2 tool result messages
	if len(msgs) != 3 {
		t.Fatalf("want 3 messages, got %d", len(msgs))
	}
	if len(msgs[0].ToolCalls) != 2 {
		t.Errorf("want 2 tool calls, got %d", len(msgs[0].ToolCalls))
	}
}

// ── blocksToAssistantLLM ───────────────────────────────────────────────────────

func TestBlocksToLLM_RoundTrip(t *testing.T) {
	args := map[string]any{"city": "Shanghai"}
	input := []chatdomain.Block{
		makeBlock("b1", 0, chatdomain.BlockTypeReasoning, chatdomain.TextData{Text: "thinking"}),
		makeBlock("b2", 1, chatdomain.BlockTypeToolCall, chatdomain.ToolCallData{
			ID: "c1", Name: "t1", Arguments: args,
		}),
		makeBlock("b3", 2, chatdomain.BlockTypeToolResult, chatdomain.ToolResultData{
			ToolCallID: "c1", OK: true, Result: "sunny",
		}),
		makeBlock("b4", 3, chatdomain.BlockTypeText, chatdomain.TextData{Text: "done"}),
	}

	msgs, err := blocksToAssistantLLM(input)
	if err != nil {
		t.Fatalf("blocksToAssistantLLM: %v", err)
	}

	// assistant + 1 tool result
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	a := msgs[0]
	if a.ReasoningContent != "thinking" {
		t.Errorf("reasoning = %q", a.ReasoningContent)
	}
	if a.Content != "done" {
		t.Errorf("content = %q", a.Content)
	}
	if len(a.ToolCalls) != 1 || a.ToolCalls[0].ID != "c1" {
		t.Errorf("tool calls = %+v", a.ToolCalls)
	}

	tr := msgs[1]
	if tr.Role != llminfra.RoleTool || tr.ToolCallID != "c1" || tr.Content != "sunny" {
		t.Errorf("tool result = %+v", tr)
	}
}

func TestBlocksToLLM_TextOnly(t *testing.T) {
	input := []chatdomain.Block{
		makeBlock("b1", 0, chatdomain.BlockTypeText, chatdomain.TextData{Text: "hi"}),
	}
	msgs, err := blocksToAssistantLLM(input)
	if err != nil {
		t.Fatalf("blocksToAssistantLLM: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "hi" {
		t.Errorf("content = %q", msgs[0].Content)
	}
	if len(msgs[0].ToolCalls) != 0 {
		t.Error("should have no tool calls")
	}
}
