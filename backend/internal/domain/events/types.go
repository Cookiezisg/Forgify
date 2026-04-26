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

// ChatReasoningToken fires for every streamed token of the model's reasoning
// (thinking) content. Only produced by reasoning-capable models such as
// DeepSeek-R1. Clients may display this in a collapsible "Thinking…" block.
//
// ChatReasoningToken 在 LLM 推理内容（thinking）流式返回时触发，
// 仅推理型模型（如 DeepSeek-R1）产生。前端可折叠展示。
type ChatReasoningToken struct {
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId"`
	Delta          string `json:"delta"`
}

// EventName returns "chat.reasoning_token".
// EventName 返回 "chat.reasoning_token"。
func (ChatReasoningToken) EventName() string { return "chat.reasoning_token" }

// ChatToolCall fires when the Agent decides to call a system tool.
//
// ChatToolCall 在 Agent 决定调用某个 system tool 时触发。
type ChatToolCall struct {
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId"`
	ToolCallID     string `json:"toolCallId"`
	ToolName       string `json:"toolName"`
	ToolInput      string `json:"toolInput"` // JSON string of full arguments
	Summary        string `json:"summary"`   // human-readable core info, e.g. "$ git status"
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

// ── Tool events (Phase 3) ─────────────────────────────────────────────────────

// ToolCodeStreaming fires for every LLM token during code generation inside
// create_tool or edit_tool. MessageID and ToolCallID bind the stream to the
// specific conversation turn that triggered it, so the frontend can associate
// the code panel update with the right message.
// ToolID is empty during create_tool (the tool does not exist yet).
//
// ToolCodeStreaming 在 create_tool / edit_tool 内部 LLM 代码生成阶段
// 逐 token 触发。MessageID 和 ToolCallID 把流绑定到触发它的对话轮次，
// 前端据此将代码面板更新关联到正确的消息。
// create_tool 期间 ToolID 为空（工具尚未创建）。
type ToolCodeStreaming struct {
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId"`  // assistant message that triggered the tool call
	ToolCallID     string `json:"toolCallId"` // LLM-assigned tool call id
	ToolID         string `json:"toolId"`     // empty for create_tool; existing id for edit_tool
	ActionType     string `json:"actionType"` // "create" | "edit"
	Delta          string `json:"delta"`
}

// EventName returns "tool.code_streaming".
// EventName 返回 "tool.code_streaming"。
func (ToolCodeStreaming) EventName() string { return "tool.code_streaming" }

// ToolCreated fires after create_tool successfully saves the new tool.
//
// ToolCreated 在 create_tool 成功保存新工具后触发。
type ToolCreated struct {
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId"`
	ToolCallID     string `json:"toolCallId"`
	ToolID         string `json:"toolId"`
	ToolName       string `json:"toolName"`
}

// EventName returns "tool.created".
// EventName 返回 "tool.created"。
func (ToolCreated) EventName() string { return "tool.created" }

// ToolPendingCreated fires after edit_tool saves a pending change awaiting
// user review.
//
// ToolPendingCreated 在 edit_tool 保存待用户审核的 pending 变更后触发。
type ToolPendingCreated struct {
	ConversationID string `json:"conversationId"`
	MessageID      string `json:"messageId"`
	ToolCallID     string `json:"toolCallId"`
	ToolID         string `json:"toolId"`
	PendingID      string `json:"pendingId"`   // ToolVersion id with status='pending'
	Instruction    string `json:"instruction"` // the LLM instruction that produced this change
}

// EventName returns "tool.pending_created".
// EventName 返回 "tool.pending_created"。
func (ToolPendingCreated) EventName() string { return "tool.pending_created" }

// ToolTestCaseGenerated fires once per test case during generate-test-cases.
// Each event carries a complete test case (not individual tokens), so the
// frontend can render it immediately as it arrives.
//
// ToolTestCaseGenerated 在 generate-test-cases 流程中每生成一条完整测试用例时触发。
// 每个事件携带完整用例（非 token），前端收到即可直接渲染。
type ToolTestCaseGenerated struct {
	ToolID         string `json:"toolId"`
	TestCaseID     string `json:"testCaseId"`
	Name           string `json:"name"`
	InputData      string `json:"inputData"`      // JSON object string
	ExpectedOutput string `json:"expectedOutput"` // JSON string
}

// EventName returns "tool.test_case_generated".
// EventName 返回 "tool.test_case_generated"。
func (ToolTestCaseGenerated) EventName() string { return "tool.test_case_generated" }

// ToolTestCasesDone fires when generate-test-cases has finished producing all
// test cases and saved them to the database.
//
// ToolTestCasesDone 在 generate-test-cases 生成全部测试用例并写入数据库后触发。
type ToolTestCasesDone struct {
	ToolID string `json:"toolId"`
	Count  int    `json:"count"` // number of test cases generated and saved
}

// EventName returns "tool.test_cases_done".
// EventName 返回 "tool.test_cases_done"。
func (ToolTestCasesDone) EventName() string { return "tool.test_cases_done" }

// ToolTestCasesNotSupported fires when the LLM determines that the tool
// cannot be reliably tested automatically (e.g. it depends on local file
// paths, network calls, or randomness). No test cases are saved.
//
// ToolTestCasesNotSupported 在 LLM 判断工具无法可靠地自动生成测试用例时触发
// （如依赖本地文件路径、网络请求、随机性等）。不保存任何测试用例。
type ToolTestCasesNotSupported struct {
	ToolID string `json:"toolId"`
	Reason string `json:"reason"` // LLM explanation; shown directly to the user
}

// EventName returns "tool.test_cases_not_supported".
// EventName 返回 "tool.test_cases_not_supported"。
func (ToolTestCasesNotSupported) EventName() string { return "tool.test_cases_not_supported" }
