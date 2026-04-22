// Forgify backend — clean architecture skeleton.
//
// Phase 0/1: bootstrap that wires up logger, DB (via infra/gorm), HTTP
// router (with all middlewares), and graceful shutdown. Per-domain
// handlers and services are wired in during Phase 2 through router.Deps.
//
// Forgify 后端 — 清晰架构骨架。
//
// Phase 0/1：启动流程，组装 logger、DB（通过 infra/gorm）、HTTP 路由
// （含全部中间件）、优雅关闭。各 domain 的 handler 和 service 在 Phase 2
// 通过 router.Deps 接入。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	gormdb "github.com/sunweilin/forgify/backend/internal/infra/gorm"
	"github.com/sunweilin/forgify/backend/internal/infra/logger"
	"github.com/sunweilin/forgify/backend/internal/transport/httpapi/router"
)

func main() {
	port := flag.Int("port", 0, "HTTP port (0 = pick a free port, print it)")
	dataDir := flag.String("data-dir", "", "Data directory (empty = in-memory SQLite)")
	dev := flag.Bool("dev", false, "Development mode (colored console logs)")
	flag.Parse()

	log, err := logger.New(*dev)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logger: %v\n", err)
		os.Exit(1)
	}
	// Flush any buffered logs on exit. Error ignored because we're exiting anyway.
	//
	// 退出时刷出缓存日志。错误忽略，因为进程反正要退。
	defer log.Sync() //nolint:errcheck

	db, err := gormdb.Open(gormdb.Config{DataDir: *dataDir})
	if err != nil {
		log.Error("open db", zap.Error(err))
		os.Exit(1)
	}
	defer func() {
		if err := gormdb.Close(db); err != nil {
			log.Warn("close db", zap.Error(err))
		}
	}()

	// Phase 2 will extend this call with real domain models:
	//   gormdb.Migrate(db, &apikey.APIKey{}, &tool.Tool{}, ...)
	//
	// Phase 2 会扩展这里为真实 domain model：
	//   gormdb.Migrate(db, &apikey.APIKey{}, &tool.Tool{}, ...)
	if err := gormdb.Migrate(db); err != nil {
		log.Error("migrate db", zap.Error(err))
		os.Exit(1)
	}

	// Assemble the HTTP handler: routes + middleware chain.
	// All route registration lives in router/ and handlers/, not here.
	//
	// 组装 HTTP handler：路由 + 中间件链。
	// 所有路由注册都在 router/ 和 handlers/，main.go 不沾具体路由。
	handler := router.New(router.Deps{Log: log})

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Error("listen", zap.Error(err))
		os.Exit(1)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port

	// Electron reads this line from stdout to discover the port.
	//
	// Electron 从 stdout 读取这一行来发现后端端口。
	fmt.Printf("BACKEND_PORT=%d\n", actualPort)

	srv := &http.Server{
		Handler:     handler,
		ReadTimeout: 15 * time.Second,
		// WriteTimeout intentionally 0: SSE streams may run for minutes.
		//
		// WriteTimeout 特意设为 0：SSE 流可能持续几分钟。
		IdleTimeout: 60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("serve", zap.Error(err))
			stop()
		}
	}()
	log.Info("backend started", zap.Int("port", actualPort), zap.Bool("dev", *dev))

	<-ctx.Done()
	log.Info("shutdown requested")

	// Give in-flight requests up to 5s to complete before forcing shutdown.
	//
	// 给进行中的请求最多 5 秒完成，之后强制关闭。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", zap.Error(err))
	}
}
