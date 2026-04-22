package gorm

import (
	"fmt"

	"gorm.io/gorm"
)

// Migrate applies the schema defined by the given domain models plus any
// schema extras (virtual tables, triggers, etc.) that AutoMigrate can't
// express. Safe to call repeatedly — AutoMigrate is idempotent and each
// extras statement is written to be `CREATE IF NOT EXISTS`.
//
// Ordering matters: models that are referenced by foreign keys should
// appear BEFORE the referring model. The caller is responsible for that
// ordering (we don't sort because dependencies are not always obvious from
// reflection alone).
//
// Typical call from main.go (Phase 2):
//
//	gormdb.Migrate(db,
//	    &apikey.APIKey{},
//	    &tool.Tool{},
//	    &tool.ToolVersion{},       // references Tool
//	    &tool.ToolTag{},            // references Tool
//	    &conversation.Conversation{},
//	    &conversation.Message{},    // references Conversation
//	    // ...
//	)
//
// Migrate 按给定 domain models 的 schema 建表，并执行 AutoMigrate 表达不了的
// schema extras（虚拟表、触发器等）。可重复调用——AutoMigrate 幂等，extras 里
// 每条语句都用 `CREATE IF NOT EXISTS` 风格。
//
// 顺序重要：被外键引用的 model 必须排在引用方**前面**。调用方负责顺序
// （不自动排序，因为反射无法可靠推断依赖图）。
func Migrate(db *gorm.DB, models ...any) error {
	if db == nil {
		return fmt.Errorf("migrate: nil db")
	}
	for i, m := range models {
		if err := db.AutoMigrate(m); err != nil {
			return fmt.Errorf("migrate model #%d (%T): %w", i, m, err)
		}
	}
	return applySchemaExtras(db)
}
