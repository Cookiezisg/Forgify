package gorm

import (
	"fmt"

	"gorm.io/gorm"
)

// schemaExtras lists SQL statements that GORM's AutoMigrate cannot express:
// FTS5 virtual tables, triggers, complex CHECK constraints spanning columns,
// partial indexes beyond GORM tag capability, etc.
//
// Each statement MUST be idempotent — prefer `CREATE ... IF NOT EXISTS`
// and avoid DROPs. Migrate is called on every server start, so any
// non-idempotent statement here would break restarts.
//
// Currently empty: Phase 2 will append entries as domains introduce
// features that need them (e.g. tools_fts virtual table).
//
// schemaExtras 列出 GORM AutoMigrate 表达不了的 SQL：FTS5 虚拟表、
// 触发器、跨列的复杂 CHECK、GORM tag 表达不了的部分索引等。
//
// 每条语句**必须**幂等——优先用 `CREATE ... IF NOT EXISTS`，避免 DROP。
// Migrate 在每次启动时都会调用，非幂等语句会让重启失败。
//
// 目前为空：Phase 2 各 domain 引入需要此能力的特性（如 tools_fts 虚拟表）时
// 再逐步添加。
var schemaExtras = []string{
	// Phase 2 examples to be added here:
	//
	//   "CREATE VIRTUAL TABLE IF NOT EXISTS tools_fts USING fts5(" +
	//       "name, display_name, description, content_rowid=rowid)",
	//
	//   "CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(" +
	//       "content, content_rowid=rowid)",
}

// applySchemaExtras runs every statement in schemaExtras within a single
// transaction. Errors abort the transaction and surface to the caller.
//
// applySchemaExtras 在**单个事务**内执行 schemaExtras 中的每条语句。
// 失败时回滚并把错误上抛。
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
