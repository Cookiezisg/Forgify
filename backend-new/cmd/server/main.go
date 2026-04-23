// Command server boots the Forgify backend: logger, DB, HTTP router with
// middleware chain, and graceful shutdown. Per-domain handlers and
// services wire in through router.Deps as Phase 2 progresses.
//
// Command server 启动 Forgify 后端：logger、DB、带中间件链的 HTTP 路由、
// 优雅关闭。各 domain 的 handler 和 service 随 Phase 2 推进通过
// router.Deps 接入。
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

	"github.com/sunweilin/forgify/backend/internal/infra/db"
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
	defer log.Sync() //nolint:errcheck // flush on exit; error is noise

	gdb, err := db.Open(db.Config{DataDir: *dataDir})
	if err != nil {
		log.Error("open db", zap.Error(err))
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(gdb); err != nil {
			log.Warn("close db", zap.Error(err))
		}
	}()

	// Phase 2 will pass domain models here, e.g.:
	//   db.Migrate(gdb, &apikey.APIKey{}, &tool.Tool{}, ...)
	//
	// Phase 2 会传入 domain model，例如：
	//   db.Migrate(gdb, &apikey.APIKey{}, &tool.Tool{}, ...)
	if err := db.Migrate(gdb); err != nil {
		log.Error("migrate db", zap.Error(err))
		os.Exit(1)
	}

	handler := router.New(router.Deps{Log: log})

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		log.Error("listen", zap.Error(err))
		os.Exit(1)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port

	// Electron reads this line from stdout to discover the port.
	// Electron 从 stdout 读取此行发现端口。
	fmt.Printf("BACKEND_PORT=%d\n", actualPort)

	srv := &http.Server{
		Handler:     handler,
		ReadTimeout: 15 * time.Second,
		// WriteTimeout=0 because SSE streams may run for minutes.
		// WriteTimeout=0，因为 SSE 流可能持续几分钟。
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

	// Give in-flight requests up to 5s before forcing shutdown.
	// 给进行中的请求最多 5 秒完成，之后强制关闭。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", zap.Error(err))
	}
}
