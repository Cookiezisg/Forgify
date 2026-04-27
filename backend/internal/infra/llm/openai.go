// openai.go — OpenAI-compatible streaming client.
// Covers: OpenAI, DeepSeek, Qwen, Moonshot, Ollama (/v1 endpoint), and any
// provider that speaks the OpenAI chat-completions wire format.
//
// openai.go — OpenAI 兼容流式客户端。
// 覆盖：OpenAI / DeepSeek / Qwen / Moonshot / Ollama 及所有兼容 OpenAI
// chat-completions 协议的 provider。
package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"
	"time"
)

// openAIClient implements Client for all OpenAI-compatible providers.
//
// openAIClient 为所有 OpenAI 兼容 provider 实现 Client 接口。
type openAIClient struct {
	http *http.Client
}

func newOpenAIClient() *openAIClient {
	return &openAIClient{
		http: &http.Client{Timeout: 120 * time.Second},
	}
}

// Stream sends a streaming chat-completions request and returns an iter.Seq
// of typed StreamEvents. Break out of the loop to stop early; context
// cancellation also terminates iteration cleanly.
//
// Stream 发起流式 chat-completions 请求，返回类型化 StreamEvent 的 iter.Seq。
// break 可提前退出，ctx 取消时迭代干净终止。
func (c *openAIClient) Stream(ctx context.Context, req Request) iter.Seq[StreamEvent] {
	return func(yield func(StreamEvent) bool) {
		body, err := buildOpenAIBody(req)
		if err != nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/openai: build body: %w", err)})
			return
		}

		httpReq, err := http.NewRequestWithContext(
			ctx, http.MethodPost, req.BaseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/openai: new request: %w", err)})
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+req.Key)

		resp, err := c.http.Do(httpReq)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/openai: do: %w", err)})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			yield(StreamEvent{Type: EventError, Err: classifyHTTPError(resp.StatusCode, raw)})
			return
		}

		parseOpenAISSE(ctx, resp.Body, yield)
	}
}

// ── SSE parser ────────────────────────────────────────────────────────────────

func parseOpenAISSE(ctx context.Context, body io.Reader, yield func(StreamEvent) bool) {
	scanner := bufio.NewScanner(body)
	toolNameSent := map[int]bool{}

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			return
		}
		if data == "" {
			continue // keep-alive or empty line — not an error
		}
		var chunk oaiChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/openai: malformed SSE chunk: %w", err)})
			return
		}
		if !emitOpenAIChunk(chunk, toolNameSent, yield) {
			return
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		yield(StreamEvent{Type: EventError, Err: fmt.Errorf("llm/openai: scan: %w", err)})
	}
}

// emitOpenAIChunk converts one parsed SSE chunk into StreamEvents.
// Returns false when the consumer signals stop.
//
// emitOpenAIChunk 把一个解析好的 SSE chunk 转换为 StreamEvent 发出。
// consumer 发出停止信号时返回 false。
func emitOpenAIChunk(chunk oaiChunk, toolNameSent map[int]bool, yield func(StreamEvent) bool) bool {
	if len(chunk.Choices) == 0 {
		// Usage-only chunk — some providers send this as the final event.
		// 仅含 usage 的 chunk，某些 provider 在流末单独发送。
		if chunk.Usage != nil {
			return yield(StreamEvent{
				Type:         EventFinish,
				InputTokens:  chunk.Usage.PromptTokens,
				OutputTokens: chunk.Usage.CompletionTokens,
			})
		}
		return true
	}

	choice := chunk.Choices[0]
	delta := choice.Delta

	if delta.Content != "" {
		if !yield(StreamEvent{Type: EventText, Delta: delta.Content}) {
			return false
		}
	}
	if delta.ReasoningContent != "" {
		if !yield(StreamEvent{Type: EventReasoning, Delta: delta.ReasoningContent}) {
			return false
		}
	}

	for _, tc := range delta.ToolCalls {
		if !toolNameSent[tc.Index] && tc.Function.Name != "" {
			toolNameSent[tc.Index] = true
			if !yield(StreamEvent{
				Type: EventToolStart, ToolIndex: tc.Index,
				ToolID: tc.ID, ToolName: tc.Function.Name,
			}) {
				return false
			}
		}
		if tc.Function.Arguments != "" {
			if !yield(StreamEvent{
				Type: EventToolDelta, ToolIndex: tc.Index,
				ArgsDelta: tc.Function.Arguments,
			}) {
				return false
			}
		}
	}

	if choice.FinishReason != "" {
		ev := StreamEvent{Type: EventFinish, FinishReason: choice.FinishReason}
		if chunk.Usage != nil {
			ev.InputTokens = chunk.Usage.PromptTokens
			ev.OutputTokens = chunk.Usage.CompletionTokens
		}
		return yield(ev)
	}
	return true
}

// ── Request builder ───────────────────────────────────────────────────────────

func buildOpenAIBody(req Request) ([]byte, error) {
	msgs, err := toOpenAIMsgs(req.Messages, req.System)
	if err != nil {
		return nil, err
	}
	body := oaiRequest{
		Model:         req.ModelID,
		Messages:      msgs,
		Stream:        true,
		StreamOptions: &oaiStreamOptions{IncludeUsage: true},
	}
	if len(req.Tools) > 0 {
		body.Tools = toOpenAITools(req.Tools)
	}
	return json.Marshal(body)
}

func toOpenAIMsgs(msgs []LLMMessage, system string) ([]oaiMessage, error) {
	var out []oaiMessage
	if system != "" {
		out = append(out, oaiMessage{Role: "system", Content: jsonString(system)})
	}
	for _, m := range msgs {
		om, err := toOpenAIMsg(m)
		if err != nil {
			return nil, err
		}
		out = append(out, om)
	}
	return out, nil
}

func toOpenAIMsg(m LLMMessage) (oaiMessage, error) {
	switch m.Role {
	case RoleUser:
		return buildOpenAIUserMsg(m)
	case RoleAssistant:
		return buildOpenAIAssistantMsg(m), nil
	case RoleTool:
		return oaiMessage{
			Role:       "tool",
			Content:    jsonString(m.Content),
			ToolCallID: m.ToolCallID,
		}, nil
	default:
		return oaiMessage{}, fmt.Errorf("llm/openai: unknown role %q", m.Role)
	}
}

func buildOpenAIUserMsg(m LLMMessage) (oaiMessage, error) {
	if len(m.Parts) == 0 {
		return oaiMessage{Role: "user", Content: jsonString(m.Content)}, nil
	}
	parts := make([]oaiContentPart, 0, len(m.Parts))
	for _, p := range m.Parts {
		switch p.Type {
		case "text":
			parts = append(parts, oaiContentPart{Type: "text", Text: p.Text})
		case "image_url":
			parts = append(parts, oaiContentPart{
				Type: "image_url", ImageURL: &oaiImageURL{URL: p.ImageURL},
			})
		default:
			return oaiMessage{}, fmt.Errorf("llm/openai: unknown part type %q", p.Type)
		}
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return oaiMessage{}, fmt.Errorf("llm/openai: marshal parts: %w", err)
	}
	return oaiMessage{Role: "user", Content: raw}, nil
}

func buildOpenAIAssistantMsg(m LLMMessage) oaiMessage {
	om := oaiMessage{
		Role:             "assistant",
		ReasoningContent: m.ReasoningContent,
	}
	if m.Content != "" {
		om.Content = jsonString(m.Content)
	}
	for _, tc := range m.ToolCalls {
		om.ToolCalls = append(om.ToolCalls, oaiToolCall{
			ID:       tc.ID,
			Type:     "function",
			Function: oaiFuncCall{Name: tc.Name, Arguments: tc.Arguments},
		})
	}
	return om
}

func toOpenAITools(defs []ToolDef) []oaiTool {
	out := make([]oaiTool, len(defs))
	for i, d := range defs {
		out[i] = oaiTool{
			Type: "function",
			Function: oaiFuncDef{
				Name: d.Name, Description: d.Description, Parameters: d.Parameters,
			},
		}
	}
	return out
}

// jsonString returns the JSON encoding of a Go string (a quoted JSON string).
// Used to produce json.RawMessage values for string-typed content fields.
//
// jsonString 返回 Go 字符串的 JSON 编码（带引号的 JSON 字符串）。
func jsonString(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// ── HTTP error classification ─────────────────────────────────────────────────

// classifyHTTPError maps HTTP status codes to descriptive errors.
// The raw response body is included for debugging.
//
// classifyHTTPError 把 HTTP 状态码映射为描述性错误，包含原始响应体供调试。
func classifyHTTPError(status int, body []byte) error {
	msg := strings.TrimSpace(string(body))
	if len(msg) > 200 {
		msg = msg[:200] + "..."
	}
	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("llm: authentication failed (401): %s", msg)
	case http.StatusTooManyRequests:
		return fmt.Errorf("llm: rate limited (429): %s", msg)
	case http.StatusBadRequest:
		return fmt.Errorf("llm: bad request (400): %s", msg)
	case http.StatusNotFound:
		return fmt.Errorf("llm: model not found (404): %s", msg)
	default:
		return fmt.Errorf("llm: provider error (%d): %s", status, msg)
	}
}

// ── Wire types ────────────────────────────────────────────────────────────────

type oaiRequest struct {
	Model         string            `json:"model"`
	Messages      []oaiMessage      `json:"messages"`
	Tools         []oaiTool         `json:"tools,omitempty"`
	Stream        bool              `json:"stream"`
	StreamOptions *oaiStreamOptions `json:"stream_options,omitempty"`
}

type oaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// oaiMessage uses json.RawMessage for Content so it can be either a quoted
// string or a JSON array of content parts without a custom marshaler.
//
// oaiMessage 用 json.RawMessage 存 Content，无需自定义 marshaler 即可兼容
// 字符串和 content parts 数组两种格式。
type oaiMessage struct {
	Role             string          `json:"role"`
	Content          json.RawMessage `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ToolCalls        []oaiToolCall   `json:"tool_calls,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
}

type oaiContentPart struct {
	Type     string       `json:"type"`
	Text     string       `json:"text,omitempty"`
	ImageURL *oaiImageURL `json:"image_url,omitempty"`
}

type oaiImageURL struct {
	URL string `json:"url"`
}

type oaiToolCall struct {
	ID       string      `json:"id"`
	Type     string      `json:"type"`
	Function oaiFuncCall `json:"function"`
}

type oaiFuncCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaiTool struct {
	Type     string     `json:"type"`
	Function oaiFuncDef `json:"function"`
}

type oaiFuncDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type oaiChunk struct {
	Choices []oaiChoice `json:"choices"`
	Usage   *oaiUsage   `json:"usage"`
}

type oaiChoice struct {
	Delta        oaiDelta `json:"delta"`
	FinishReason string   `json:"finish_reason"`
}

type oaiDelta struct {
	Content          string             `json:"content"`
	ReasoningContent string             `json:"reasoning_content"`
	ToolCalls        []oaiToolCallDelta `json:"tool_calls"`
}

type oaiToolCallDelta struct {
	Index    int          `json:"index"`
	ID       string       `json:"id"`
	Function oaiFuncDelta `json:"function"`
}

type oaiFuncDelta struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}
