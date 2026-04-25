package db

import (
	"fmt"

	"gorm.io/gorm"
)

// schemaExtras lists SQL that AutoMigrate can't express: partial indexes,
// complex CHECK constraints, triggers, FTS5 virtual tables, etc.
// Every statement MUST be idempotent (prefer CREATE ... IF NOT EXISTS).
// Migrate runs on every server start.
//
// Build requirement: go-sqlite3 must be compiled with FTS5 enabled.
//
//	CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" go build ./...
//
// schemaExtras 列出 AutoMigrate 表达不了的 SQL：部分索引、复杂 CHECK、触发器、
// FTS5 虚拟表等。每条语句**必须**幂等（优先 CREATE ... IF NOT EXISTS）。
// Migrate 在每次启动时运行。
//
// 构建要求：go-sqlite3 必须以 FTS5 方式编译：
//
//	CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" go build ./...
var schemaExtras = []string{
	// messages_fts — FTS5 full-text search on message content.
	// content= mode avoids duplicating data; three triggers keep the index in
	// sync with inserts, updates, and deletes on the messages table.
	//
	// messages_fts — 对 message content 建 FTS5 全文搜索。
	// content= 模式避免数据重复；三个触发器保持索引与 messages 表同步。
	`CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts
		USING fts5(content, content='messages', content_rowid='rowid')`,

	`CREATE TRIGGER IF NOT EXISTS messages_fts_insert
		AFTER INSERT ON messages BEGIN
			INSERT INTO messages_fts(rowid, content) VALUES (new.rowid, new.content);
		END`,

	`CREATE TRIGGER IF NOT EXISTS messages_fts_update
		AFTER UPDATE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, content)
				VALUES ('delete', old.rowid, old.content);
			INSERT INTO messages_fts(rowid, content) VALUES (new.rowid, new.content);
		END`,

	`CREATE TRIGGER IF NOT EXISTS messages_fts_delete
		AFTER DELETE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, content)
				VALUES ('delete', old.rowid, old.content);
		END`,
}

// applySchemaExtras runs every statement in schemaExtras in a single
// transaction. Errors abort and surface to the caller.
//
// The current extras require the messages table (FTS5 content table). When
// Migrate is called for non-chat domains (apikey, model, etc.) the messages
// table does not exist yet — we skip and let the chat Migrate call apply them.
//
// applySchemaExtras 在单个事务中执行 schemaExtras 所有语句。错误回滚并上抛。
//
// 当前 extras 依赖 messages 表（FTS5 content 表）。为其他 domain（apikey、
// model 等）调用 Migrate 时 messages 表尚不存在——跳过，由 chat Migrate 时执行。
func applySchemaExtras(db *gorm.DB) error {
	if len(schemaExtras) == 0 {
		return nil
	}
	// FTS5 virtual table requires messages table to exist first.
	// FTS5 虚拟表要求 messages 表先存在。
	var n int64
	db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='messages'").Scan(&n)
	if n == 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for i, stmt := range schemaExtras {
			if err := tx.Exec(stmt).Error; err != nil {
				return fmt.Errorf("schema extras #%d: %w", i, err)
			}
		}
		return nil
	})
}
