package types

// TestStatus records the OUTCOME of the most recent connectivity test —
// this is a snapshot field, NOT a streaming state machine. Tests are
// synchronous blocking calls that write the outcome once.
//
// TestStatus 记录**最近一次**连通性测试的**结果**——这是快照字段，
// **不是**流式状态机。测试是同步阻塞调用，完成后一次性写入结果。
const (
	TestStatusPending = "pending" // never tested yet / 从未测试过
	TestStatusOK      = "ok"      // last test succeeded / 最近一次成功
	TestStatusError   = "error"   // last test failed / 最近一次失败
)

// APIFormat values for APIKey.APIFormat (used by custom provider only).
//
// APIKey.APIFormat 的取值（仅 custom provider 使用）。
const (
	APIFormatOpenAICompatible    = "openai-compatible"
	APIFormatAnthropicCompatible = "anthropic-compatible"
)
