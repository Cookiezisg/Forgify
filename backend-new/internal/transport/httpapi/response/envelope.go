// Package response provides envelope-shaped HTTP response helpers that
// enforce the N1 standard: every success carries {"data": ...} and every
// failure carries {"error": {"code", "message", "details"}}.
//
// Handlers must go through this package — no direct w.Write / json.Encode.
// The envelope struct is unexported; callers interact only via helpers.
//
// Package response 提供符合 N1 标准的 HTTP 响应 envelope 辅助函数：
// 每次成功返回 {"data": ...}，每次失败返回 {"error": {"code", "message", "details"}}。
//
// Handler 必须走这个包——禁止直接 w.Write / json.Encode。envelope 结构不对外
// 导出，调用方只通过 helper 交互。
package response

import (
	"encoding/json"
	"net/http"
)

// envelope is the on-the-wire response shape. Exactly one of Data / Error
// is non-nil in any single response; NextCursor + HasMore only appear on
// paged list responses.
//
// envelope 是线上响应格式。单个响应里 Data 和 Error 恰有一个非 nil；
// NextCursor + HasMore 仅在分页列表响应中出现。
type envelope struct {
	Data       any        `json:"data,omitempty"`
	Error      *errorBody `json:"error,omitempty"`
	NextCursor *string    `json:"nextCursor,omitempty"`
	HasMore    *bool      `json:"hasMore,omitempty"`
}

// errorBody is the shape of the "error" field in a failure response.
//
// errorBody 是失败响应中 "error" 字段的形状。
type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Success writes {"data": body} with the given HTTP status.
// Use for GET (200) and successful update operations.
//
// Success 写出 {"data": body} 及给定的 HTTP 状态码。用于 GET（200）和
// 成功的更新操作。
func Success(w http.ResponseWriter, status int, body any) {
	writeJSON(w, status, envelope{Data: body})
}

// Created is shorthand for Success(w, 201, body). Use on POST when a
// new resource is created.
//
// Created 是 Success(w, 201, body) 的快捷方式。用于 POST 创建新资源。
func Created(w http.ResponseWriter, body any) {
	Success(w, http.StatusCreated, body)
}

// NoContent writes HTTP 204 with no body. Use for DELETE and for PATCH
// operations that don't return the updated resource.
//
// NoContent 写出 HTTP 204 且无响应体。用于 DELETE，以及不返回更新后资源的 PATCH。
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Paged writes a paginated list response:
//
//	{"data": items, "nextCursor": "<cursor>", "hasMore": <bool>}
//
// nextCursor="" signals "no more pages"; hasMore should also be false in
// that case. Use for every list endpoint (N4).
//
// Paged 写出分页列表响应：
//
//	{"data": items, "nextCursor": "<cursor>", "hasMore": <bool>}
//
// nextCursor="" 表示"没有下一页"，此时 hasMore 也应为 false。每个列表
// 端点都应使用（N4）。
func Paged(w http.ResponseWriter, items any, nextCursor string, hasMore bool) {
	env := envelope{Data: items, HasMore: &hasMore}
	if nextCursor != "" {
		env.NextCursor = &nextCursor
	}
	writeJSON(w, http.StatusOK, env)
}

// Error writes a structured error envelope. Use this for errors produced
// inside the handler itself (bad JSON, missing field). Service-layer errors
// should go through FromDomainError (see errmap.go).
//
// Error 写出结构化错误 envelope。用于 handler 内部自己发现的错误（JSON 坏了、
// 字段缺失）。Service 层返回的错误应走 FromDomainError（见 errmap.go）。
func Error(w http.ResponseWriter, status int, code, message string, details any) {
	writeJSON(w, status, envelope{Error: &errorBody{
		Code:    code,
		Message: message,
		Details: details,
	}})
}

// writeJSON serializes the envelope and writes it. Encode errors are
// intentionally discarded: by the time encoding fails, the header+status
// are already flushed, and nothing useful can be done.
//
// writeJSON 序列化 envelope 并写出。encode 错误故意丢弃：当 encode 失败时
// header+status 已经刷出，无法再有意义地处理。
func writeJSON(w http.ResponseWriter, status int, body envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
