// Package errors declares cross-domain sentinel errors. Per-domain
// sentinels (tool.ErrNotFound, etc.) live in their own packages.
//
// Package errors 声明跨 domain 的通用 sentinel 错误。按 domain 特定的
// sentinel（tool.ErrNotFound 等）放在各自包内。
package errors

import "errors"

var (
	// ErrInvalidRequest signals a malformed or semantically invalid request
	// detected before domain logic runs.
	//
	// ErrInvalidRequest 表示在进入 domain 逻辑前被发现的请求格式错误
	// 或语义无效。
	ErrInvalidRequest = errors.New("invalid request")

	// ErrInternal signals an unexpected failure — a bug or infra outage.
	//
	// ErrInternal 表示意外失败——bug 或基础设施故障。
	ErrInternal = errors.New("internal error")
)
