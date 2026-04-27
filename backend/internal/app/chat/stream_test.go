// stream_test.go — unit tests for streamBlockBuilder and parseToolArgs.
// Blocks are assembled in stream-arrival order, not grouped by type.
//
// stream_test.go — streamBlockBuilder 和 parseToolArgs 的单元测试。
// Block 按流到达顺序组装，非按类型分组。
package chat

import (
	"encoding/json"
	"testing"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
)

// build is a test helper that feeds fn to a fresh builder and finalises it.
func build(fn func(*streamBlockBuilder)) []chatdomain.Block {
	b := newStreamBlockBuilder()
	fn(b)
	b.finalize()
	return b.blocks
}

// ── streamBlockBuilder ────────────────────────────────────────────────────────

func TestBuilder_TextOnly(t *testing.T) {
	blocks := build(func(b *streamBlockBuilder) { b.appendText("Hello world") })
	if len(blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(blocks))
	}
	if blocks[0].Type != chatdomain.BlockTypeText {
		t.Errorf("type = %q, want text", blocks[0].Type)
	}
	var d chatdomain.TextData
	json.Unmarshal([]byte(blocks[0].Data), &d)
	if d.Text != "Hello world" {
		t.Errorf("text = %q, want 'Hello world'", d.Text)
	}
}

func TestBuilder_ReasoningThenText(t *testing.T) {
	blocks := build(func(b *streamBlockBuilder) {
		b.appendReasoning("Let me think...")
		b.appendText("The answer is 42.")
	})
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Type != chatdomain.BlockTypeReasoning {
		t.Errorf("blocks[0] = %q, want reasoning", blocks[0].Type)
	}
	if blocks[1].Type != chatdomain.BlockTypeText {
		t.Errorf("blocks[1] = %q, want text", blocks[1].Type)
	}
}

func TestBuilder_TextThenToolCall(t *testing.T) {
	// Preamble text must appear BEFORE tool_call in stream order.
	// Preamble 文字必须按流顺序排在 tool_call 之前。
	blocks := build(func(b *streamBlockBuilder) {
		b.appendText("Let me check the weather.")
		a := &toolAccum{id: "call_1", name: "get_weather"}
		a.args.WriteString(`{"city":"Beijing"}`)
		b.startTool(0, "call_1", "get_weather")
		b.accums[0] = a
	})
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks (text + tool_call), got %d", len(blocks))
	}
	if blocks[0].Type != chatdomain.BlockTypeText {
		t.Errorf("blocks[0] = %q, want text (preamble)", blocks[0].Type)
	}
	if blocks[1].Type != chatdomain.BlockTypeToolCall {
		t.Errorf("blocks[1] = %q, want tool_call", blocks[1].Type)
	}
}

func TestBuilder_ToolCallOnly(t *testing.T) {
	blocks := build(func(b *streamBlockBuilder) {
		a := &toolAccum{id: "call_1", name: "get_weather"}
		a.args.WriteString(`{"summary":"Checking Beijing weather","city":"Beijing"}`)
		b.startTool(0, "call_1", "get_weather")
		b.accums[0] = a
	})
	if len(blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(blocks))
	}
	if blocks[0].Type != chatdomain.BlockTypeToolCall {
		t.Errorf("type = %q, want tool_call", blocks[0].Type)
	}
	var d chatdomain.ToolCallData
	json.Unmarshal([]byte(blocks[0].Data), &d)
	if d.ID != "call_1" || d.Name != "get_weather" {
		t.Errorf("id/name = %q/%q", d.ID, d.Name)
	}
	if d.Summary != "Checking Beijing weather" {
		t.Errorf("summary = %q", d.Summary)
	}
	if _, ok := d.Arguments["summary"]; ok {
		t.Error("summary key should be stripped from arguments")
	}
	if d.Arguments["city"] != "Beijing" {
		t.Errorf("city = %v", d.Arguments["city"])
	}
}

func TestBuilder_ParallelToolCalls(t *testing.T) {
	blocks := build(func(b *streamBlockBuilder) {
		b.startTool(0, "call_1", "get_weather")
		b.startTool(1, "call_2", "get_time")
		a0 := &toolAccum{id: "call_1", name: "get_weather"}
		a0.args.WriteString(`{"city":"Beijing"}`)
		a1 := &toolAccum{id: "call_2", name: "get_time"}
		a1.args.WriteString(`{"tz":"UTC"}`)
		b.accums[0] = a0
		b.accums[1] = a1
	})
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(blocks))
	}
	var d0, d1 chatdomain.ToolCallData
	json.Unmarshal([]byte(blocks[0].Data), &d0)
	json.Unmarshal([]byte(blocks[1].Data), &d1)
	// finalize() iterates accums in ToolIndex order.
	// finalize() 按 ToolIndex 顺序迭代 accums。
	if d0.Name != "get_weather" || d1.Name != "get_time" {
		t.Errorf("names = %q %q, want get_weather get_time", d0.Name, d1.Name)
	}
}

func TestBuilder_FullReactStep(t *testing.T) {
	// Reasoning → text preamble → tool_call (correct stream order).
	// Reasoning → 前置文字 → tool_call（正确的流顺序）。
	blocks := build(func(b *streamBlockBuilder) {
		b.appendReasoning("I'll check the weather first.")
		b.appendText("Let me look that up.")
		a := &toolAccum{id: "call_1", name: "get_weather"}
		a.args.WriteString(`{"city":"Beijing"}`)
		b.startTool(0, "call_1", "get_weather")
		b.accums[0] = a
	})
	if len(blocks) != 3 {
		t.Fatalf("want 3 blocks, got %d: %+v", len(blocks), blocks)
	}
	if blocks[0].Type != chatdomain.BlockTypeReasoning {
		t.Errorf("blocks[0] = %q, want reasoning", blocks[0].Type)
	}
	if blocks[1].Type != chatdomain.BlockTypeText {
		t.Errorf("blocks[1] = %q, want text", blocks[1].Type)
	}
	if blocks[2].Type != chatdomain.BlockTypeToolCall {
		t.Errorf("blocks[2] = %q, want tool_call", blocks[2].Type)
	}
}

func TestBuilder_Empty(t *testing.T) {
	blocks := build(func(_ *streamBlockBuilder) {})
	if len(blocks) != 0 {
		t.Errorf("want 0 blocks, got %d", len(blocks))
	}
}

func TestBuilder_BlockIDs(t *testing.T) {
	blocks := build(func(b *streamBlockBuilder) { b.appendText("hi") })
	for _, bl := range blocks {
		if bl.ID == "" {
			t.Error("block ID must not be empty")
		}
	}
}

func TestBuilder_SeqIncremental(t *testing.T) {
	// seq values must be 0, 1, 2 in order.
	// seq 值必须按顺序为 0, 1, 2。
	blocks := build(func(b *streamBlockBuilder) {
		b.appendReasoning("thinking")
		b.appendText("answer")
		a := &toolAccum{id: "c1", name: "t1"}
		a.args.WriteString(`{}`)
		b.startTool(0, "c1", "t1")
		b.accums[0] = a
	})
	// reasoning(0), text(1), tool_call(2)
	for i, bl := range blocks {
		if bl.Seq != i {
			t.Errorf("blocks[%d].Seq = %d, want %d", i, bl.Seq, i)
		}
	}
}

func TestBuilder_CreatedAtSet(t *testing.T) {
	blocks := build(func(b *streamBlockBuilder) { b.appendText("hi") })
	if blocks[0].CreatedAt.IsZero() {
		t.Error("CreatedAt must be set")
	}
}

// ── parseToolArgs ─────────────────────────────────────────────────────────────

func TestParseToolArgs_WithSummary(t *testing.T) {
	summary, args := parseToolArgs(`{"summary":"doing X","key":"val"}`)
	if summary != "doing X" {
		t.Errorf("summary = %q", summary)
	}
	if args["key"] != "val" {
		t.Errorf("key = %v", args["key"])
	}
	if _, ok := args["summary"]; ok {
		t.Error("summary should be stripped from args map")
	}
}

func TestParseToolArgs_NoSummary(t *testing.T) {
	summary, args := parseToolArgs(`{"key":"val"}`)
	if summary != "" {
		t.Errorf("summary = %q, want empty", summary)
	}
	if args["key"] != "val" {
		t.Errorf("key = %v", args["key"])
	}
}

func TestParseToolArgs_MalformedJSON(t *testing.T) {
	summary, args := parseToolArgs(`not-json`)
	if summary != "" {
		t.Errorf("summary = %q, want empty for bad JSON", summary)
	}
	if args["raw"] != "not-json" {
		t.Errorf("fallback raw = %v", args["raw"])
	}
}
