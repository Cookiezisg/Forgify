// Integration tests for infra/llm. These tests hit the real DeepSeek API
// and require DEEPSEEK_API_KEY to be set (or the constant below).
// Run with: go test -v -run TestIntegration ./internal/infra/llm/...
//
// infra/llm 集成测试，直接调用真实 DeepSeek API。
// 需设置 DEEPSEEK_API_KEY 环境变量（或使用下方常量）。
// 运行：go test -v -run TestIntegration ./internal/infra/llm/...
package llm

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// testKey returns the DeepSeek API key from env or the hardcoded fallback.
//
// testKey 从环境变量或硬编码兜底返回 DeepSeek API Key。
func testKey() string {
	if k := os.Getenv("DEEPSEEK_API_KEY"); k != "" {
		return k
	}
	return "shabi"
}

// TestIntegration_TextStream verifies that text tokens are streamed correctly.
//
// TestIntegration_TextStream 验证文字 token 流式传输正确。
func TestIntegration_TextStream(t *testing.T) {
	ctx := context.Background()
	factory := NewFactory()
	client, baseURL, err := factory.Build(Config{
		Provider: "deepseek",
		ModelID:  "deepseek-chat",
		Key:      testKey(),
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	req := Request{
		ModelID: "deepseek-chat",
		Key:     testKey(),
		BaseURL: baseURL,
		System:  "Reply in exactly 5 words.",
		Messages: []LLMMessage{
			{Role: RoleUser, Content: "Say hello."},
		},
	}

	var tokens []string
	var gotFinish bool
	for event := range client.Stream(ctx, req) {
		switch event.Type {
		case EventText:
			tokens = append(tokens, event.Delta)
		case EventFinish:
			gotFinish = true
			if event.InputTokens == 0 {
				t.Error("InputTokens should be > 0")
			}
			if event.OutputTokens == 0 {
				t.Error("OutputTokens should be > 0")
			}
		case EventError:
			t.Fatalf("stream error: %v", event.Err)
		}
	}

	if len(tokens) == 0 {
		t.Fatal("received no text tokens")
	}
	if !gotFinish {
		t.Fatal("never received EventFinish")
	}
	t.Logf("response: %q", strings.Join(tokens, ""))
	t.Logf("tokens: in=%d out=%d", 0, 0)
}

// TestIntegration_Generate verifies the Generate helper (non-streaming).
//
// TestIntegration_Generate 验证 Generate helper（非流式调用）。
func TestIntegration_Generate(t *testing.T) {
	ctx := context.Background()
	factory := NewFactory()
	client, baseURL, err := factory.Build(Config{Provider: "deepseek", ModelID: "deepseek-chat", Key: testKey()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	req := Request{
		ModelID: "deepseek-chat", Key: testKey(), BaseURL: baseURL,
		System:   "Reply with only the number, nothing else.",
		Messages: []LLMMessage{{Role: RoleUser, Content: "What is 2+2?"}},
	}

	result, err := Generate(ctx, client, req)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(result, "4") {
		t.Errorf("expected '4' in response, got %q", result)
	}
	t.Logf("Generate result: %q", result)
}

// TestIntegration_ToolCall verifies that tool call events are emitted in order:
// EventToolStart (name known) before EventToolDelta (arguments).
//
// TestIntegration_ToolCall 验证 tool call 事件按顺序发出：
// EventToolStart（name 已知）先于 EventToolDelta（arguments）。
func TestIntegration_ToolCall(t *testing.T) {
	ctx := context.Background()
	factory := NewFactory()
	client, baseURL, err := factory.Build(Config{Provider: "deepseek", ModelID: "deepseek-chat", Key: testKey()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	weatherTool := ToolDef{
		Name:        "get_weather",
		Description: "Get current weather for a city.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"city": {"type": "string", "description": "City name"}
			},
			"required": ["city"]
		}`),
	}

	req := Request{
		ModelID: "deepseek-chat", Key: testKey(), BaseURL: baseURL,
		System:   "Use tools when asked.",
		Messages: []LLMMessage{{Role: RoleUser, Content: "What's the weather in Beijing?"}},
		Tools:    []ToolDef{weatherTool},
	}

	var eventOrder []StreamEventType
	argsBuf := map[int]string{}
	var gotToolStart, gotToolDelta, gotFinish bool
	var toolName string

	for event := range client.Stream(ctx, req) {
		eventOrder = append(eventOrder, event.Type)
		switch event.Type {
		case EventToolStart:
			gotToolStart = true
			toolName = event.ToolName
		case EventToolDelta:
			gotToolDelta = true
			argsBuf[event.ToolIndex] += event.ArgsDelta
		case EventFinish:
			gotFinish = true
		case EventError:
			t.Fatalf("stream error: %v", event.Err)
		}
	}

	if !gotToolStart {
		t.Fatal("never received EventToolStart")
	}
	if toolName != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got %q", toolName)
	}
	if !gotToolDelta {
		t.Fatal("never received EventToolDelta")
	}
	if !gotFinish {
		t.Fatal("never received EventFinish")
	}

	// Verify EventToolStart appears before any EventToolDelta.
	// 验证 EventToolStart 出现在任意 EventToolDelta 之前。
	startIdx, deltaIdx := -1, -1
	for i, et := range eventOrder {
		if et == EventToolStart && startIdx < 0 {
			startIdx = i
		}
		if et == EventToolDelta && deltaIdx < 0 {
			deltaIdx = i
		}
	}
	if startIdx > deltaIdx {
		t.Errorf("EventToolStart (idx %d) came after EventToolDelta (idx %d)", startIdx, deltaIdx)
	}

	// Verify assembled arguments are valid JSON containing "city".
	// 验证拼接后的 arguments 是合法 JSON 且含 "city"。
	args := argsBuf[0]
	var parsed map[string]any
	if err := json.Unmarshal([]byte(args), &parsed); err != nil {
		t.Errorf("assembled arguments not valid JSON: %q — %v", args, err)
	}
	if _, ok := parsed["city"]; !ok {
		t.Errorf("expected 'city' in arguments, got: %q", args)
	}
	t.Logf("tool: %s, args: %s", toolName, args)
}

// TestIntegration_MultiTurn verifies that conversation history is correctly
// passed back to the LLM (the model should remember the previous exchange).
//
// TestIntegration_MultiTurn 验证对话历史被正确回传（模型应记得上一轮）。
func TestIntegration_MultiTurn(t *testing.T) {
	ctx := context.Background()
	factory := NewFactory()
	client, baseURL, err := factory.Build(Config{Provider: "deepseek", ModelID: "deepseek-chat", Key: testKey()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	req := Request{
		ModelID: "deepseek-chat", Key: testKey(), BaseURL: baseURL,
		System: "You are a concise assistant.",
		Messages: []LLMMessage{
			{Role: RoleUser, Content: "My favourite number is 42. Remember it."},
			{Role: RoleAssistant, Content: "Got it, your favourite number is 42."},
			{Role: RoleUser, Content: "What is my favourite number?"},
		},
	}

	result, err := Generate(ctx, client, req)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(result, "42") {
		t.Errorf("model forgot the number: %q", result)
	}
	t.Logf("multi-turn result: %q", result)
}

// TestIntegration_ContextCancel verifies that cancelling the context stops
// the stream without hanging.
//
// TestIntegration_ContextCancel 验证取消 ctx 能干净停止流，不挂起。
func TestIntegration_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	factory := NewFactory()
	client, baseURL, err := factory.Build(Config{Provider: "deepseek", ModelID: "deepseek-chat", Key: testKey()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	req := Request{
		ModelID: "deepseek-chat", Key: testKey(), BaseURL: baseURL,
		Messages: []LLMMessage{{Role: RoleUser, Content: "Count from 1 to 100 slowly."}},
	}

	count := 0
	for event := range client.Stream(ctx, req) {
		if event.Type == EventText {
			count++
			if count == 3 {
				cancel() // cancel after 3 text events
			}
		}
		if event.Type == EventError {
			// After cancel, some providers may return an error — that's acceptable.
			// 取消后某些 provider 可能返回错误，可接受。
			t.Logf("error after cancel (expected): %v", event.Err)
			break
		}
	}
	if count < 3 {
		t.Errorf("expected at least 3 text events before cancel, got %d", count)
	}
	t.Logf("cancelled after %d text events", count)
}
