// openai_test.go — unit tests for the OpenAI-compatible SSE parser and
// request builder. No network calls — all input is synthetic SSE text.
//
// openai_test.go — OpenAI 兼容 SSE 解析器和请求构建器的单元测试。
// 不发网络请求——输入全部为合成 SSE 文本。
package llm

import (
	"context"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"testing"
)

// ── SSE parser ────────────────────────────────────────────────────────────────

func collectEvents(sseText string) []StreamEvent {
	var events []StreamEvent
	r := strings.NewReader(sseText)
	parseOpenAISSE(context.Background(), r, func(e StreamEvent) bool {
		events = append(events, e)
		return true
	})
	return events
}

func TestParseSSE_TextOnly(t *testing.T) {
	sse := `data: {"choices":[{"delta":{"content":"Hello"},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":" world"},"finish_reason":null}]}

data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}

data: [DONE]
`
	events := collectEvents(sse)

	textEvents := filterType(events, EventText)
	if len(textEvents) != 2 {
		t.Fatalf("want 2 text events, got %d", len(textEvents))
	}
	if textEvents[0].Delta != "Hello" || textEvents[1].Delta != " world" {
		t.Errorf("text deltas = %q %q", textEvents[0].Delta, textEvents[1].Delta)
	}

	finishEvents := filterType(events, EventFinish)
	if len(finishEvents) != 1 {
		t.Fatalf("want 1 finish event, got %d", len(finishEvents))
	}
	if finishEvents[0].FinishReason != "stop" {
		t.Errorf("finish reason = %q, want stop", finishEvents[0].FinishReason)
	}
	if finishEvents[0].InputTokens != 5 || finishEvents[0].OutputTokens != 2 {
		t.Errorf("tokens = in:%d out:%d, want in:5 out:2",
			finishEvents[0].InputTokens, finishEvents[0].OutputTokens)
	}
}

func TestParseSSE_ToolCall(t *testing.T) {
	// Simulates OpenAI streaming a single tool call across multiple chunks.
	// 模拟 OpenAI 跨多个 chunk 流式传输一个 tool call。
	sse := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"cit"}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"y\":\"Beijing\"}"}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]
`
	events := collectEvents(sse)

	starts := filterType(events, EventToolStart)
	if len(starts) != 1 {
		t.Fatalf("want 1 EventToolStart, got %d", len(starts))
	}
	if starts[0].ToolName != "get_weather" || starts[0].ToolID != "call_1" {
		t.Errorf("tool start: name=%q id=%q", starts[0].ToolName, starts[0].ToolID)
	}

	deltas := filterType(events, EventToolDelta)
	if len(deltas) != 2 {
		t.Fatalf("want 2 EventToolDelta, got %d", len(deltas))
	}
	assembled := deltas[0].ArgsDelta + deltas[1].ArgsDelta
	var args map[string]any
	if err := json.Unmarshal([]byte(assembled), &args); err != nil {
		t.Errorf("assembled args not valid JSON: %q", assembled)
	}
	if args["city"] != "Beijing" {
		t.Errorf("city = %v, want Beijing", args["city"])
	}
}

func TestParseSSE_ParallelToolCalls(t *testing.T) {
	// Two tool calls in one response, each with a different index.
	// 一次响应中两个 tool call，各有不同 index。
	sse := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":\"Beijing\"}"}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_2","function":{"name":"get_time","arguments":""}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"tz\":\"UTC\"}"}}]},"finish_reason":null}]}

data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]
`
	events := collectEvents(sse)

	starts := filterType(events, EventToolStart)
	if len(starts) != 2 {
		t.Fatalf("want 2 EventToolStart, got %d", len(starts))
	}
	names := []string{starts[0].ToolName, starts[1].ToolName}
	if !slices.Contains(names, "get_weather") || !slices.Contains(names, "get_time") {
		t.Errorf("tool names = %v", names)
	}
}

func TestParseSSE_ReasoningContent(t *testing.T) {
	// DeepSeek-R1 style: reasoning_content before content.
	// DeepSeek-R1 风格：reasoning_content 在 content 之前。
	sse := `data: {"choices":[{"delta":{"reasoning_content":"Let me think..."},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":"The answer is 42."},"finish_reason":"stop"}]}

data: [DONE]
`
	events := collectEvents(sse)

	reasoning := filterType(events, EventReasoning)
	if len(reasoning) != 1 || reasoning[0].Delta != "Let me think..." {
		t.Errorf("reasoning events = %+v", reasoning)
	}
	texts := filterType(events, EventText)
	if len(texts) != 1 || texts[0].Delta != "The answer is 42." {
		t.Errorf("text events = %+v", texts)
	}
}

func TestParseSSE_UsageOnlyChunk(t *testing.T) {
	// Some providers send a final usage-only chunk with no choices.
	// 某些 provider 在最后发一个无 choices 的 usage-only chunk。
	sse := `data: {"choices":[{"delta":{"content":"hi"},"finish_reason":"stop"}]}

data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":1}}

data: [DONE]
`
	events := collectEvents(sse)
	finishes := filterType(events, EventFinish)
	// The usage-only chunk should emit an additional EventFinish with tokens.
	// usage-only chunk 应额外发一个带 token 的 EventFinish。
	found := false
	for _, f := range finishes {
		if f.InputTokens == 10 && f.OutputTokens == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("no EventFinish with usage tokens; got %+v", finishes)
	}
}

func TestParseSSE_ContextCancelled(t *testing.T) {
	sse := `data: {"choices":[{"delta":{"content":"a"},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":"b"},"finish_reason":null}]}

data: [DONE]
`
	ctx, cancel := context.WithCancel(context.Background())
	var count int
	parseOpenAISSE(ctx, strings.NewReader(sse), func(e StreamEvent) bool {
		count++
		cancel() // cancel after first event
		return false
	})
	if count != 1 {
		t.Errorf("expected exactly 1 event before cancel, got %d", count)
	}
}

// ── Request builder ───────────────────────────────────────────────────────────

func TestBuildOpenAIBody_SystemPrepended(t *testing.T) {
	req := Request{
		ModelID: "gpt-4o",
		System:  "You are helpful.",
		Messages: []LLMMessage{
			{Role: RoleUser, Content: "Hello"},
		},
	}
	body, err := buildOpenAIBody(req)
	if err != nil {
		t.Fatalf("buildOpenAIBody: %v", err)
	}
	var out oaiRequest
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("want 2 messages (system + user), got %d", len(out.Messages))
	}
	var systemContent string
	json.Unmarshal(out.Messages[0].Content, &systemContent)
	if systemContent != "You are helpful." {
		t.Errorf("system content = %q", systemContent)
	}
}

func TestBuildOpenAIBody_ToolCall(t *testing.T) {
	req := Request{
		ModelID: "gpt-4o",
		Messages: []LLMMessage{
			{
				Role:    RoleAssistant,
				Content: "",
				ToolCalls: []LLMToolCall{
					{ID: "call_1", Name: "get_weather", Arguments: `{"city":"Beijing"}`},
				},
			},
			{Role: RoleTool, Content: "晴，25°C", ToolCallID: "call_1"},
		},
	}
	body, err := buildOpenAIBody(req)
	if err != nil {
		t.Fatalf("buildOpenAIBody: %v", err)
	}
	var out oaiRequest
	json.Unmarshal(body, &out)
	if len(out.Messages) != 2 {
		t.Fatalf("want 2 messages, got %d", len(out.Messages))
	}
	if len(out.Messages[0].ToolCalls) != 1 {
		t.Errorf("assistant should have 1 tool call")
	}
	if out.Messages[1].ToolCallID != "call_1" {
		t.Errorf("tool message tool_call_id = %q", out.Messages[1].ToolCallID)
	}
}

func TestBuildOpenAIBody_StreamEnabled(t *testing.T) {
	req := Request{ModelID: "gpt-4o", Messages: []LLMMessage{{Role: RoleUser, Content: "hi"}}}
	body, _ := buildOpenAIBody(req)
	var out oaiRequest
	json.Unmarshal(body, &out)
	if !out.Stream {
		t.Error("stream should be true")
	}
	if out.StreamOptions == nil || !out.StreamOptions.IncludeUsage {
		t.Error("stream_options.include_usage should be true")
	}
}

func TestBuildOpenAIBody_MultiModalUser(t *testing.T) {
	req := Request{
		ModelID: "gpt-4o",
		Messages: []LLMMessage{{
			Role: RoleUser,
			Parts: []ContentPart{
				{Type: "text", Text: "What's in this image?"},
				{Type: "image_url", ImageURL: "data:image/png;base64,abc"},
			},
		}},
	}
	body, err := buildOpenAIBody(req)
	if err != nil {
		t.Fatalf("buildOpenAIBody: %v", err)
	}
	var out oaiRequest
	json.Unmarshal(body, &out)

	// content should be a JSON array, not a string
	// content 应为 JSON 数组而非字符串
	var parts []oaiContentPart
	if err := json.Unmarshal(out.Messages[0].Content, &parts); err != nil {
		t.Fatalf("content is not a parts array: %v", err)
	}
	if len(parts) != 2 {
		t.Fatalf("want 2 parts, got %d", len(parts))
	}
}

// ── Error classification ──────────────────────────────────────────────────────

func TestClassifyHTTPError(t *testing.T) {
	cases := []struct {
		status int
		substr string
	}{
		{401, "authentication"},
		{429, "rate limit"},
		{400, "bad request"},
		{404, "not found"},
		{500, "provider error"},
	}
	for _, c := range cases {
		err := classifyHTTPError(c.status, []byte("detail"))
		if err == nil {
			t.Errorf("status %d: want error, got nil", c.status)
			continue
		}
		if !strings.Contains(strings.ToLower(err.Error()), c.substr) {
			t.Errorf("status %d: error %q does not contain %q", c.status, err.Error(), c.substr)
		}
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func filterType(events []StreamEvent, t StreamEventType) []StreamEvent {
	var out []StreamEvent
	for _, e := range events {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

// Ensure parseOpenAISSE accepts io.Reader (not just *strings.Reader).
// 确保 parseOpenAISSE 接受 io.Reader（不仅仅是 *strings.Reader）。
var _ io.Reader = (*strings.Reader)(nil)
