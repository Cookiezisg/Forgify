// Package reqctx carries per-request metadata (user identity, locale)
// through ctx so any layer can read it without reverse dependencies.
//
// Package reqctx 通过 ctx 传递每次请求的元数据（用户身份、locale），
// 让任何层都能读取而不制造反向依赖。
package reqctx

import "context"

// DefaultLocalUserID is the hardcoded user ID used by Phase 2 single-user mode.
// Will be replaced by real auth extraction later.
//
// DefaultLocalUserID 是 Phase 2 单用户模式的硬编码 ID，未来被真实 auth 替换。
const DefaultLocalUserID = "local-user"

type userIDKey struct{}

// SetUserID returns a copy of ctx carrying the given user ID.
//
// SetUserID 返回携带给定 user ID 的 ctx 拷贝。
func SetUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey{}, id)
}

// GetUserID retrieves the user ID. A false result means the auth
// middleware didn't run or an empty string was stored — treat as
// a server-side wiring bug (respond 500), not as 401.
//
// GetUserID 取用户 ID。返回 false 表示 auth 中间件未跑或存的是空串——
// 视为服务端接线 bug 返回 500，而非 401。
func GetUserID(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey{}).(string)
	return id, ok && id != ""
}
