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

// ChatToken fires for every token streamed from the LLM during a response.
// Expect hundreds to thousands per conversation turn.
//
// ChatToken 在 LLM 流式返回的每个 token 到达时触发，单轮对话会产生几百到几千条。
type ChatToken struct {
	ConversationID string `json:"conversationId"`
	StreamID       string `json:"streamId"`
	Token          string `json:"token"`
}

// EventName returns "chat.token".
// EventName 返回 "chat.token"。
func (ChatToken) EventName() string { return "chat.token" }
