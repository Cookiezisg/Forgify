// Package events defines typed event types for SSE streaming and
// in-process notification. Every event is a concrete Go struct — no
// map[string]any allowed — so the compiler catches shape drift.
//
// Naming: snake_case, dot-separated by domain. e.g. "chat.token",
// "tool.code_updated".
//
// Package events 定义 SSE 流推送和进程内通知的类型化事件。每个事件都是
// 具体的 Go struct——禁止 map[string]any——让编译器捕获载荷形状漂移。
//
// 命名：snake_case，按 domain 加点号前缀。如 "chat.token"、"tool.code_updated"。
package events

// Event is any typed message flowing through a Bridge.
//
// Event 是在 Bridge 中流动的类型化消息。
type Event interface {
	EventName() string
}

// ChatToken fires for every streamed token from the LLM.
// Expect hundreds to thousands per conversation turn.
//
// ChatToken 在 LLM 流式返回的每个 token 到达时触发，单轮对话会产生几百到几千条。
type ChatToken struct {
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId"` // 当前 assistant 消息 id
	Delta          string `json:"delta"`     // 增量文本
}

// EventName returns "chat.token".
// EventName 返回 "chat.token"。
func (ChatToken) EventName() string { return "chat.token" }

// ChatToolCall fires when the Agent decides to call a system tool.
//
// ChatToolCall 在 Agent 决定调用某个 system tool 时触发。
type ChatToolCall struct {
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId"`
	ToolCallID     string `json:"toolCallId"`
	ToolName       string `json:"toolName"`
	ToolInput      string `json:"toolInput"` // JSON string
}

// EventName returns "chat.tool_call".
// EventName 返回 "chat.tool_call"。
func (ChatToolCall) EventName() string { return "chat.tool_call" }

// ChatToolResult fires when a tool execution completes.
//
// ChatToolResult 在 tool 执行完成时触发。
type ChatToolResult struct {
	ConversationID string `json:"conversationId"`
	ToolCallID     string `json:"toolCallId"`
	Result         string `json:"result"`
	OK             bool   `json:"ok"`
}

// EventName returns "chat.tool_result".
// EventName 返回 "chat.tool_result"。
func (ChatToolResult) EventName() string { return "chat.tool_result" }

// ChatDone fires when the Agent finishes the full response.
// StopReason distinguishes normal completion from truncation or cancellation.
//
// ChatDone 在 Agent 完成完整回复时触发。
// StopReason 区分正常完成、截断和取消。
type ChatDone struct {
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId"`
	StopReason     string `json:"stopReason"`           // end_turn | max_tokens | cancelled | error
	TokenUsage     string `json:"tokenUsage,omitempty"` // JSON: {inputTokens,outputTokens,cacheReadTokens}
}

// EventName returns "chat.done".
// EventName 返回 "chat.done"。
func (ChatDone) EventName() string { return "chat.done" }

// ChatError fires when the Agent encounters a non-recoverable error.
// Code matches the SCREAMING_SNAKE_CASE error codes in error-codes.md.
//
// ChatError 在 Agent 遇到不可恢复错误时触发。
// Code 与 error-codes.md 中的 SCREAMING_SNAKE_CASE 错误码对应。
type ChatError struct {
	ConversationID string `json:"conversationId"`
	Code           string `json:"code"`
	Message        string `json:"message"`
}

// EventName returns "chat.error".
// EventName 返回 "chat.error"。
func (ChatError) EventName() string { return "chat.error" }

// ConversationTitleUpdated fires after auto-titling writes a generated
// title back to the conversation, so the frontend sidebar updates without
// a manual refresh.
//
// ConversationTitleUpdated 在 auto-titling 把生成的标题写回对话后触发，
// 让前端侧边栏无需手动刷新即可更新。
type ConversationTitleUpdated struct {
	ConversationID string `json:"conversationId"`
	Title          string `json:"title"`
	AutoTitled     bool   `json:"autoTitled"`
}

// EventName returns "conversation.title_updated".
// EventName 返回 "conversation.title_updated"。
func (ConversationTitleUpdated) EventName() string { return "conversation.title_updated" }
