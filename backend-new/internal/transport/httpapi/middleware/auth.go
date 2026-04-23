package middleware

import (
	"context"
	"net/http"
)

// contextKey is a private type used for context keys in this package.
// Using an unexported type prevents accidental key collisions with other
// packages that might use plain strings as keys.
//
// contextKey 是本包私有的 context key 类型。使用未导出类型可以避免
// 与其他使用裸 string 作为 key 的包意外冲突。
type contextKey int

const (
	// userIDKey is the ctx key for the authenticated user's ID.
	// userIDKey 是 ctx 中存放认证用户 ID 的 key。
	userIDKey contextKey = iota
)

// DefaultLocalUserID is the hardcoded user ID used by InjectUserID during
// the Phase 2 single-user phase. When real auth (JWT / session) is added
// later, this constant and its only consumer (InjectUserID) are the ONLY
// things that need to change — all business code calling UserIDFromContext
// stays as-is.
//
// DefaultLocalUserID 是 Phase 2 单用户阶段 InjectUserID 硬编码注入的用户 ID。
// 未来加真实认证（JWT / session）时，只需要改这个常量及它的唯一使用点
// InjectUserID——所有调用 UserIDFromContext 的业务代码零改动。
const DefaultLocalUserID = "local-user"

// InjectUserID is Phase 2's simplified "auth" middleware. It hardcodes
// the user ID to DefaultLocalUserID on every request so that multi-tenant
// code (repositories filtering by user_id, etc.) can be written from day
// one without an actual auth system.
//
// Upgrade path: when real auth lands, this middleware is rewritten to
// parse a JWT / session cookie and inject the real user ID. All downstream
// code that reads UserIDFromContext is unaffected.
//
// Placement in the chain: INSIDE CORS (preflight OPTIONS doesn't need
// auth) and OUTSIDE the route mux (handlers should always see a userID).
//
// InjectUserID 是 Phase 2 的"简化 auth"中间件。每个请求硬编码注入
// DefaultLocalUserID，让多租户代码（repo 按 user_id 过滤等）可以从
// 第一天就按正确模式写，无需等真实认证系统就绪。
//
// 升级路径：加入真实认证时，只需重写本中间件以解析 JWT / session cookie
// 并注入真实用户 ID。所有从 UserIDFromContext 读值的下游代码不受影响。
//
// 在中间件链中的位置：在 CORS **内层**（preflight OPTIONS 不需要认证），
// 在路由 mux **外层**（handler 总能看到 userID）。
func InjectUserID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), userIDKey, DefaultLocalUserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UserIDFromContext retrieves the authenticated user's ID from ctx.
//
// The second return value is false if InjectUserID never ran (wiring bug)
// OR if the stored value is an empty string. Handlers should treat a
// false result as a server-side configuration bug and respond with 500,
// NOT as a normal unauthenticated request (that would be a 401 returned
// by the auth middleware itself).
//
// UserIDFromContext 从 ctx 取出认证用户的 ID。
//
// 第二个返回值为 false 的场景：InjectUserID 未运行（接线 bug）**或**
// 存的是空字符串。Handler 应把 false 视为**服务端配置 bug** 并返回 500，
// 而不是当作普通未认证请求（那应该由 auth 中间件本身返回 401）。
func UserIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey).(string)
	return id, ok && id != ""
}
