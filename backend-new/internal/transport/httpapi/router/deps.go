// Package router assembles HTTP routes and the middleware chain into a
// single http.Handler.
//
// Package router 把 HTTP 路由和中间件链组装成一个 http.Handler。
package router

import (
	"go.uber.org/zap"

	apikeyapp "github.com/sunweilin/forgify/backend/internal/app/apikey"
)

// Deps bundles everything the HTTP transport layer needs. Constructed
// once in main.go and handed to router.New. Per-domain service fields
// are nil-tolerant — router.New only registers a domain's routes when
// its service is non-nil, so integration tests can stay narrow.
//
// Deps 聚合 HTTP transport 层需要的全部依赖。main.go 里一次性构造后
// 交给 router.New。各 domain 的 service 字段容忍 nil——router.New 仅在
// service 非 nil 时注册对应路由，让集成测试可保持窄切片。
type Deps struct {
	Log *zap.Logger

	// APIKeyService implements CRUD + KeyProvider for /api/v1/api-keys/*.
	// APIKeyService 为 /api/v1/api-keys/* 提供 CRUD + KeyProvider。
	APIKeyService *apikeyapp.Service

	// Phase 2+ fields (ConversationService, ToolService, ...) appended
	// here as domains come online.
	//
	// Phase 2+ 字段（ConversationService、ToolService ...）随各 domain
	// 上线依次追加。
}
