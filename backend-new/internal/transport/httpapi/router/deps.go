// Package router assembles HTTP routes and the middleware chain into a
// single http.Handler. It is the only place that knows about both middleware
// and all concrete handlers; main.go only knows about router.
//
// Package router 把 HTTP 路由和中间件链组装成一个 http.Handler。它是**唯一**
// 同时认识中间件和所有具体 handler 的地方；main.go 只认识 router。
package router

import "go.uber.org/zap"

// Deps bundles everything the HTTP transport layer needs to operate.
// Constructed once in main.go (from fully initialized services) and handed
// to router.New in one shot.
//
// As Phase 2 adds domain services, new fields are appended here. That's the
// ONLY place the list of services lives — no globals, no hidden injection.
//
// Deps 聚合 HTTP transport 层工作所需的全部依赖。main.go 里一次性构造
// （用完整初始化好的 service），交给 router.New。
//
// Phase 2 每加一个 domain service，就在这里追加一个字段。这里是 service
// 清单的**唯一**归属——不搞全局变量、不搞隐式注入。
type Deps struct {
	// Log is the root zap logger. Middlewares and handlers receive it via
	// this struct rather than through a package-level global.
	//
	// Log 是根 zap logger。中间件和 handler 通过本结构体拿到它，而非通过
	// 包级全局变量。
	Log *zap.Logger

	// Future fields (Phase 2+):
	//   APIKeyService       *apikey.Service
	//   ConversationService *conversation.Service
	//   ToolService         *tool.Service
	//   ChatService         *chat.Service
	//   AttachmentService   *attachment.Service
	//
	// 未来字段（Phase 2+）：按 domain 依次补齐。
}
