// Package response provides envelope-shaped HTTP response helpers.
// Success → {"data": ...}. Failure → {"error": {"code", "message", "details"}}.
// Handlers must go through this package — no direct w.Write / json.Encode.
//
// Package response 提供 envelope 格式的 HTTP 响应辅助函数。
// 成功 → {"data": ...}。失败 → {"error": {"code", "message", "details"}}。
// Handler 必须走本包——禁止直接 w.Write / json.Encode。
package response

import (
	"encoding/json"
	"net/http"
)

// envelope is the on-wire response shape. Exactly one of Data / Error is
// non-nil; NextCursor + HasMore only appear in paged lists.
//
// envelope 是线上响应形状。Data / Error 恰有一个非 nil；
// NextCursor + HasMore 仅在分页列表中出现。
type envelope struct {
	Data       any        `json:"data,omitempty"`
	Error      *errorBody `json:"error,omitempty"`
	NextCursor *string    `json:"nextCursor,omitempty"`
	HasMore    *bool      `json:"hasMore,omitempty"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Success writes {"data": body} with the given status. Use for GET / updates.
//
// Success 写出 {"data": body} 及给定状态码。用于 GET / 更新。
func Success(w http.ResponseWriter, status int, body any) {
	writeJSON(w, status, envelope{Data: body})
}

// Created is Success(w, 201, body). Use when a new resource was created.
//
// Created 是 Success(w, 201, body) 的快捷方式。创建新资源时使用。
func Created(w http.ResponseWriter, body any) {
	Success(w, http.StatusCreated, body)
}

// NoContent writes HTTP 204 with no body. Use for DELETE / state PATCH.
//
// NoContent 写出 HTTP 204，无响应体。用于 DELETE / 状态 PATCH。
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Paged writes a paginated list: {"data": items, "nextCursor", "hasMore"}.
// Empty nextCursor means "no more pages".
//
// Paged 写出分页列表：{"data": items, "nextCursor", "hasMore"}。
// nextCursor 为空表示"没有下一页"。
func Paged(w http.ResponseWriter, items any, nextCursor string, hasMore bool) {
	env := envelope{Data: items, HasMore: &hasMore}
	if nextCursor != "" {
		env.NextCursor = &nextCursor
	}
	writeJSON(w, http.StatusOK, env)
}

// Error writes a structured error envelope. Use for errors detected inside
// the handler (bad JSON, missing field). Service-layer errors should go
// through FromDomainError.
//
// Error 写出结构化错误 envelope。用于 handler 内部发现的错误（JSON 坏、
// 字段缺失）。service 层返回的错误应走 FromDomainError。
func Error(w http.ResponseWriter, status int, code, message string, details any) {
	writeJSON(w, status, envelope{Error: &errorBody{
		Code:    code,
		Message: message,
		Details: details,
	}})
}

func writeJSON(w http.ResponseWriter, status int, body envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
