// stream_test.go — unit tests for assembleBlocks and parseToolArgs.
//
// stream_test.go — assembleBlocks 和 parseToolArgs 的单元测试。
package chat

import (
	"encoding/json"
	"strings"
	"testing"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
)

// makeAccums is a test helper that builds a toolAccum map from (id, name, argsJSON) triples.
func makeAccums(triples ...string) map[int]*toolAccum {
	m := map[int]*toolAccum{}
	for i := 0; i+2 < len(triples); i += 3 {
		a := &toolAccum{id: triples[i], name: triples[i+1]}
		a.args.WriteString(triples[i+2])
		m[i/3] = a
	}
	return m
}

// ── assembleBlocks ────────────────────────────────────────────────────────────

func TestAssemble_TextOnly(t *testing.T) {
	blocks := assembleBlocks("Hello world", "", nil)
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

func TestAssemble_ReasoningThenText(t *testing.T) {
	blocks := assembleBlocks("The answer is 42.", "Let me think...", nil)
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

func TestAssemble_TextThenToolCall(t *testing.T) {
	accums := makeAccums("call_1", "get_weather", `{"city":"Beijing"}`)
	blocks := assembleBlocks("Let me check the weather.", "", accums)
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks (text + tool_call), got %d", len(blocks))
	}
	if blocks[0].Type != chatdomain.BlockTypeText {
		t.Errorf("blocks[0] = %q, want text", blocks[0].Type)
	}
	if blocks[1].Type != chatdomain.BlockTypeToolCall {
		t.Errorf("blocks[1] = %q, want tool_call", blocks[1].Type)
	}
}

func TestAssemble_ToolCallOnly(t *testing.T) {
	accums := makeAccums("call_1", "get_weather", `{"summary":"Checking Beijing weather","city":"Beijing"}`)
	blocks := assembleBlocks("", "", accums)
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

func TestAssemble_ParallelToolCalls(t *testing.T) {
	accums := map[int]*toolAccum{}
	a0 := &toolAccum{id: "call_1", name: "get_weather"}
	a0.args.WriteString(`{"city":"Beijing"}`)
	a1 := &toolAccum{id: "call_2", name: "get_time"}
	a1.args.WriteString(`{"tz":"UTC"}`)
	accums[0] = a0
	accums[1] = a1

	blocks := assembleBlocks("", "", accums)
	if len(blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(blocks))
	}
	var d0, d1 chatdomain.ToolCallData
	json.Unmarshal([]byte(blocks[0].Data), &d0)
	json.Unmarshal([]byte(blocks[1].Data), &d1)
	// assembleBlocks iterates accums in ToolIndex order.
	// assembleBlocks 按 ToolIndex 顺序迭代。
	if d0.Name != "get_weather" || d1.Name != "get_time" {
		t.Errorf("names = %q %q, want get_weather get_time", d0.Name, d1.Name)
	}
}

func TestAssemble_FullReactStep(t *testing.T) {
	// reasoning → text preamble → tool_call (expected stream order).
	// reasoning → 前置文字 → tool_call（预期流顺序）。
	accums := makeAccums("call_1", "get_weather", `{"city":"Beijing"}`)
	blocks := assembleBlocks("Let me look that up.", "I'll check the weather first.", accums)
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

func TestAssemble_Empty(t *testing.T) {
	blocks := assembleBlocks("", "", nil)
	if len(blocks) != 0 {
		t.Errorf("want 0 blocks, got %d", len(blocks))
	}
}

func TestAssemble_BlockIDs(t *testing.T) {
	blocks := assembleBlocks("hi", "", nil)
	for _, bl := range blocks {
		if bl.ID == "" {
			t.Error("block ID must not be empty")
		}
	}
}

func TestAssemble_SeqIncremental(t *testing.T) {
	// seq values within one assembled call must be 0, 1, 2 in order.
	// 单次 assembleBlocks 内 seq 值必须按顺序为 0, 1, 2。
	accums := makeAccums("c1", "t1", `{}`)
	blocks := assembleBlocks("answer", "thinking", accums)
	// reasoning(0), text(1), tool_call(2)
	for i, bl := range blocks {
		if bl.Seq != i {
			t.Errorf("blocks[%d].Seq = %d, want %d", i, bl.Seq, i)
		}
	}
}

func TestAssemble_CreatedAtSet(t *testing.T) {
	blocks := assembleBlocks("hi", "", nil)
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

// ── extractToolCalls ──────────────────────────────────────────────────────────

func TestExtractToolCalls_Mixed(t *testing.T) {
	args := map[string]any{"x": 1.0}
	d, _ := json.Marshal(chatdomain.ToolCallData{ID: "c1", Name: "t1", Arguments: args})
	blocks := []chatdomain.Block{
		{Type: chatdomain.BlockTypeText, Data: `{"text":"hi"}`},
		{Type: chatdomain.BlockTypeToolCall, Data: string(d)},
	}
	calls := extractToolCalls(blocks)
	if len(calls) != 1 {
		t.Fatalf("want 1 tool call, got %d", len(calls))
	}
	if calls[0].ID != "c1" || calls[0].Name != "t1" {
		t.Errorf("call = %+v", calls[0])
	}
}

func TestExtractToolCalls_None(t *testing.T) {
	blocks := []chatdomain.Block{
		{Type: chatdomain.BlockTypeText, Data: `{"text":"hi"}`},
	}
	if calls := extractToolCalls(blocks); len(calls) != 0 {
		t.Errorf("want 0 calls, got %d", len(calls))
	}
}

// ── makeAccums helper ─────────────────────────────────────────────────────────

func TestMakeAccums_WritesArgs(t *testing.T) {
	accums := makeAccums("id1", "name1", `{"k":"v"}`)
	a, ok := accums[0]
	if !ok {
		t.Fatal("accums[0] not set")
	}
	if a.id != "id1" || a.name != "name1" {
		t.Errorf("id/name = %q/%q", a.id, a.name)
	}
	if got := a.args.String(); !strings.Contains(got, `"k"`) {
		t.Errorf("args = %q", got)
	}
}
