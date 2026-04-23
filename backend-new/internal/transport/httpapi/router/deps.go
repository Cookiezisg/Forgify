// Package router assembles HTTP routes and the middleware chain into a
// single http.Handler.
//
// Package router 把 HTTP 路由和中间件链组装成一个 http.Handler。
package router

import "go.uber.org/zap"

// Deps bundles everything the HTTP transport layer needs. Constructed
// once in main.go and handed to router.New.
//
// Deps 聚合 HTTP transport 层需要的全部依赖。main.go 里一次性构造后
// 交给 router.New。
type Deps struct {
	Log *zap.Logger

	// Phase 2+ fields (APIKeyService, ConversationService, ToolService, ...)
	// will be appended here as domains come online.
	//
	// Phase 2+ 字段（APIKeyService、ConversationService、ToolService ...）
	// 随各 domain 上线依次追加。
}
