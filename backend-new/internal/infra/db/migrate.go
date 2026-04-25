package db

import (
	"fmt"

	"gorm.io/gorm"
)

// Migrate applies the schema for the given domain models plus schema
// extras (FTS5, triggers) that AutoMigrate can't express. Idempotent —
// safe to call on every server start.
//
// Ordering matters: referenced models must appear BEFORE referring models.
// Caller is responsible for ordering (no dependency auto-detection).
//
// Migrate 为给定 domain model 应用 schema，并执行 AutoMigrate 表达不了
// 的 schema extras（FTS5、触发器）。幂等——每次启动都可安全调用。
//
// 顺序重要：被引用的 model 必须排在引用方**前面**。调用方负责顺序
// （不自动推断依赖）。
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
