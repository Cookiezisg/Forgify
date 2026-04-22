// Package errors defines cross-domain sentinel errors.
//
// Each domain package (tool, conversation, apikey, ...) declares its own
// sentinels in its own errors.go. This package is for errors that don't
// belong to any single domain — malformed input, unexpected failures, etc.
//
// Handlers should wrap these errors to add context, and transport/httpapi/response
// maps them to HTTP status codes via errmap.go.
//
// Package errors 定义跨 domain 的 sentinel 错误。
//
// 每个 domain 包（tool、conversation、apikey 等）在自己的 errors.go 里
// 声明自己的 sentinel。本包只存放不属于单一 domain 的错误——输入格式错、
// 意外失败等。
//
// Handler 应包裹这些错误以补充上下文，transport/httpapi/response 通过
// errmap.go 把它们映射到 HTTP 状态码。
package errors

import "errors"

var (
	// ErrInvalidRequest signals a malformed or semantically invalid request,
	// detected before any domain logic runs (bad JSON, missing required field).
	//
	// ErrInvalidRequest 表示请求格式错误或语义无效，在进入 domain 逻辑前
	// 就被发现（如 JSON 坏了、必填字段缺失）。
	ErrInvalidRequest = errors.New("invalid request")

	// ErrInternal signals an unexpected failure — a bug, an infra outage,
	// or any error not explicitly mapped.
	//
	// ErrInternal 表示意外失败——bug、基础设施故障、或任何未显式映射的错误。
	ErrInternal = errors.New("internal error")
)
