// Forgify backend — clean architecture skeleton.
//
// Phase 0: minimal bootstrap. Starts an HTTP server on a free port, opens a
// SQLite database via GORM, and exposes /api/v1/health so Electron can detect
// readiness. Per-domain handlers and services are wired in during Phase 2.
//
// Forgify 后端 — 清晰架构骨架。
//
// Phase 0：最小引导。启动一个监听空闲端口的 HTTP 服务器，通过 GORM 打开
// SQLite 数据库，并暴露 /api/v1/health 供 Electron 检测后端就绪。各 domain
// 的 handler 和 service 在 Phase 2 接入。
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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

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

	db, err := openDB(*dataDir)
	if err != nil {
		log.Error("open db", zap.Error(err))
		os.Exit(1)
	}
	defer closeDB(db, log)

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

// openDB opens a SQLite database via GORM. An empty dir yields an in-memory
// database; otherwise a forgify.db file is created (WAL mode, foreign keys on).
//
// openDB 通过 GORM 打开 SQLite 数据库。dir 为空时使用内存数据库；否则在目录下
// 创建 forgify.db 文件（WAL 模式，外键约束开启）。
func openDB(dir string) (*gorm.DB, error) {
	dsn := ":memory:"
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
		dsn = fmt.Sprintf("%s/forgify.db?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on", dir)
	}
	return gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
}

// closeDB closes the underlying SQL connection. Errors are logged as warnings
// since the process is exiting anyway.
//
// closeDB 关闭底层 SQL 连接。错误仅记录 warning，因为进程反正要退。
func closeDB(db *gorm.DB, log *zap.Logger) {
	sqlDB, err := db.DB()
	if err != nil {
		log.Warn("close db: get underlying DB", zap.Error(err))
		return
	}
	if err := sqlDB.Close(); err != nil {
		log.Warn("close db", zap.Error(err))
	}
}

