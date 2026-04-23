package types

// Credentials is the bundle returned to consumers (chat, workflow,
// embedding) for LLM calls. Key is plaintext — callers must treat it as
// ephemeral, never log or persist it.
//
// Credentials 是返回给调用方（chat、workflow、embedding）用于调 LLM 的
// 凭证包。Key 是明文——调用方必须当短生命周期对待，禁止日志或持久化。
type Credentials struct {
	Key     string
	BaseURL string
}

// ListFilter is the query shape accepted by Repository.List.
//
// ListFilter 是 Repository.List 接受的查询形状。
type ListFilter struct {
	Cursor   string
	Limit    int
	Provider string // optional filter / 可选按 provider 过滤
}
