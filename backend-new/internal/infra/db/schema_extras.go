package db

import (
	"fmt"

	"gorm.io/gorm"
)

// schemaExtras lists SQL that AutoMigrate can't express: FTS5 virtual
// tables, triggers, complex CHECK constraints, partial indexes, etc.
//
// Every statement MUST be idempotent (prefer CREATE ... IF NOT EXISTS,
// avoid DROP). Migrate runs on every server start.
//
// schemaExtras 列出 AutoMigrate 表达不了的 SQL：FTS5 虚拟表、触发器、
// 跨列的复杂 CHECK、部分索引等。
//
// 每条语句**必须**幂等（优先 CREATE ... IF NOT EXISTS，避免 DROP）。
// Migrate 在每次启动时运行。
var schemaExtras = []string{
	// Phase 2 各 domain 按需追加，例如：
	//   "CREATE VIRTUAL TABLE IF NOT EXISTS tools_fts USING fts5(...)"
}

// applySchemaExtras runs every statement in schemaExtras in a single
// transaction. Errors abort and surface to the caller.
//
// applySchemaExtras 在单个事务中执行 schemaExtras 所有语句。
// 错误回滚并上抛。
func applySchemaExtras(db *gorm.DB) error {
	if len(schemaExtras) == 0 {
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
