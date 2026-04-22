// Forgify backend — clean architecture skeleton.
//
// Phase 0: minimal bootstrap. Starts an HTTP server on a free port, opens a
// SQLite database via GORM, and exposes /api/v1/health so Electron can detect
// readiness. Per-domain handlers and services are wired in during Phase 2.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	port := flag.Int("port", 0, "HTTP port (0 = pick a free port, print it)")
	dataDir := flag.String("data-dir", "", "Data directory (empty = in-memory SQLite)")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	db, err := openDB(*dataDir)
	if err != nil {
		logger.Error("open db", "err", err)
		os.Exit(1)
	}
	defer closeDB(db, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", healthHandler)

	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		logger.Error("listen", "err", err)
		os.Exit(1)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port

	// Electron reads this line from stdout to discover the port.
	fmt.Printf("BACKEND_PORT=%d\n", actualPort)

	srv := &http.Server{
		Handler:     mux,
		ReadTimeout: 15 * time.Second,
		// WriteTimeout intentionally 0: SSE streams may run for minutes.
		IdleTimeout: 60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("serve", "err", err)
			stop()
		}
	}()
	logger.Info("backend started", "port", actualPort)

	<-ctx.Done()
	logger.Info("shutdown requested")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "err", err)
	}
}

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

func closeDB(db *gorm.DB, logger *slog.Logger) {
	sqlDB, err := db.DB()
	if err != nil {
		logger.Warn("close db: get underlying DB", "err", err)
		return
	}
	if err := sqlDB.Close(); err != nil {
		logger.Warn("close db", "err", err)
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{"status": "ok"},
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
