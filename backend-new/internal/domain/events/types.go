// Package events defines the typed event contract used for server-to-
// client streaming (SSE) and in-process notification. Every event is a
// concrete Go struct — no map[string]any allowed — so the compiler
// catches shape drift.
//
// Adding a new event:
//  1. Declare a struct in this file (or a domain-specific file alongside).
//  2. Set a stable wire name in EventName() — snake_case, dot-separated
//     by domain (e.g. "chat.token", "tool.code_updated").
//  3. Document when it fires and what fields mean.
//
// Package events 定义类型化的事件契约，用于服务端到客户端的 SSE 流推送
// 和进程内通知。每个事件都是**具体的 Go struct**——禁止使用 map[string]any
// ——让编译器帮我们捕获载荷形状漂移。
//
// 新增事件的步骤：
//  1. 在本文件（或按 domain 拆分的相邻文件）声明 struct。
//  2. 在 EventName() 里设置稳定的线上名——snake_case，按 domain 加前缀
//     （如 "chat.token"、"tool.code_updated"）。
//  3. 说明它何时触发、各字段含义。
package events

// Event is any typed message flowing through a Bridge.
//
// Concrete events must provide a stable EventName(). That string is what
// the SSE layer emits in the "event:" line and what future subscribers
// use to identify the type when re-hydrating from another transport
// (e.g. Redis pub/sub in a SaaS setup).
//
// Event 是在 Bridge 间流动的类型化消息。
//
// 具体事件必须提供稳定的 EventName()。该字符串是 SSE 层在 "event:" 行
// 推送的值，也是未来从其他 transport（如 SaaS 下的 Redis pub/sub）
// 重新水化消息时用于识别类型的依据。
type Event interface {
	EventName() string
}

// ChatToken fires for every token streamed from the LLM during an
// in-progress response. Expect hundreds to thousands per conversation turn.
//
// ChatToken 在 LLM 流式返回的每个 token 到达时触发。单轮对话会产生
// 几百到几千条。
type ChatToken struct {
	ConversationID string `json:"conversationId"`
	StreamID       string `json:"streamId"`
	Token          string `json:"token"`
}

// EventName returns the wire name "chat.token".
// EventName 返回线上名 "chat.token"。
func (ChatToken) EventName() string { return "chat.token" }
