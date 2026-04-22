// Package gorm provides the project's sole gateway to a relational database.
//
// It centralizes three concerns:
//   - Connection setup (Open / Close) with SQLite tuned for our workload.
//   - Schema application (Migrate) driven by domain types' GORM tags.
//   - Escape-hatch SQL (schema extras) for features AutoMigrate can't express
//     — FTS5 virtual tables, triggers, complex CHECKs.
//
// No other package in the codebase is allowed to import `gorm.io/gorm`
// directly (S8). Repositories live in infra/gorm alongside this file and
// implement interfaces declared in domain/.
//
// Package gorm 是项目**唯一**对接关系数据库的网关。
//
// 统一管三件事：
//   - 连接建立（Open / Close），SQLite 按我们的负载调优。
//   - 按 domain 类型的 GORM tag 应用 schema（Migrate）。
//   - AutoMigrate 表达不了的 SQL（schema extras），如 FTS5 虚拟表、
//     触发器、复杂 CHECK。
//
// 本项目**禁止**其他包直接 import `gorm.io/gorm`（标准 S8）。Repository
// 实现位于本目录下，实现 domain/ 中声明的接口。
package gorm

import (
	"fmt"
	"os"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Config controls how the database is opened. Zero values are safe defaults
// suitable for tests (in-memory, quiet logger).
//
// Config 控制数据库的打开方式。零值为安全默认（内存 DB、安静 logger），
// 适合测试。
type Config struct {
	// DataDir is the directory holding forgify.db. Empty string → in-memory
	// database (for tests). Non-empty → the directory is created if missing
	// and "{DataDir}/forgify.db" is opened.
	//
	// DataDir 是存放 forgify.db 的目录。空字符串 → 内存数据库（测试用）。
	// 非空 → 目录不存在时自动创建，打开 "{DataDir}/forgify.db"。
	DataDir string

	// LogLevel controls GORM's internal SQL logger. Use Silent in tests,
	// Warn in production, Info when debugging.
	//
	// LogLevel 控制 GORM 内部 SQL 日志。测试用 Silent，生产用 Warn，调试用 Info。
	LogLevel gormlogger.LogLevel
}

// Open establishes a SQLite connection with WAL, foreign keys, and prepared
// statement caching all enabled. Returns an error if the data directory
// can't be created or the connection can't be established.
//
// Open 打开一个 SQLite 连接，启用 WAL 日志、外键约束、prepared statement 缓存。
// 数据目录创建失败或连接建立失败时返回错误。
func Open(cfg Config) (*gorm.DB, error) {
	dsn, err := buildDSN(cfg.DataDir)
	if err != nil {
		return nil, err
	}

	logLevel := cfg.LogLevel
	if logLevel == 0 {
		logLevel = gormlogger.Warn
	}

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		// NowFunc uses UTC everywhere. Naive local time across timezones is
		// a classic source of off-by-hours bugs; UTC at rest + convert at
		// transport is the safe pattern.
		//
		// NowFunc 全用 UTC。跨时区的本地时间是经典的"差几小时"bug 源；
		// 存 UTC，展示时再转是安全模式。
		NowFunc:  func() time.Time { return time.Now().UTC() },
		Logger:   gormlogger.Default.LogMode(logLevel),
		// PrepareStmt caches prepared statements for every unique SQL text,
		// gaining both speed and defense against SQL injection (can't
		// accidentally concatenate into a statement text at runtime).
		//
		// PrepareStmt 缓存每条独特 SQL 文本的 prepared statement，既提升
		// 性能也防 SQL 注入（无法在运行时意外拼接 SQL 字符串）。
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}

	// Extra runtime PRAGMAs that SQLite connection string already sets, but
	// we verify by querying them back. Belt-and-suspenders for safety-critical
	// settings like foreign_keys.
	//
	// 连接字符串已经设置过的 PRAGMA，这里查询回来校验。对 foreign_keys 这类
	// 关键安全配置做双重保险。
	if err := verifyPragmas(db); err != nil {
		_ = Close(db)
		return nil, err
	}

	return db, nil
}

// Close releases the underlying sql.DB connection pool. Safe to call on nil.
//
// Close 释放底层 sql.DB 连接池。对 nil 调用安全。
func Close(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("gorm close: get underlying sql.DB: %w", err)
	}
	return sqlDB.Close()
}

// buildDSN constructs the SQLite connection string. All tuning PRAGMAs are
// passed as query parameters so they're applied during handshake, before
// any application query runs.
//
// buildDSN 构造 SQLite 连接字符串。所有调优 PRAGMA 作为查询参数传入，
// 确保在握手阶段生效，早于任何应用层查询。
func buildDSN(dataDir string) (string, error) {
	params := "_journal_mode=WAL" + // Write-ahead logging: better concurrent reads.
		"&_busy_timeout=5000" + // Wait up to 5s on lock contention before erroring.
		"&_foreign_keys=on" + // Enforce FK constraints (SQLite default is off!).
		"&_synchronous=NORMAL" // Balance between durability and speed.

	if dataDir == "" {
		return ":memory:?" + params, nil
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dataDir, err)
	}
	return fmt.Sprintf("%s/forgify.db?%s", dataDir, params), nil
}

// verifyPragmas queries back critical PRAGMAs to make sure our DSN took
// effect. An unexpected value means either the driver ignored our settings
// or someone modified them before we looked — either way, fail loudly now
// rather than corrupt data later.
//
// verifyPragmas 查询回关键 PRAGMA 以确认 DSN 生效。值不符预期说明驱动忽略了
// 设置或有人提前改动——任一情况立即失败，强过以后数据损坏。
func verifyPragmas(db *gorm.DB) error {
	var fk int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&fk).Error; err != nil {
		return fmt.Errorf("query foreign_keys pragma: %w", err)
	}
	if fk != 1 {
		return fmt.Errorf("foreign_keys pragma is %d, expected 1", fk)
	}
	return nil
}
