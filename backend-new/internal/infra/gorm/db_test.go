// db_test.go — tests covering Open, Close, Migrate, and schema extras.
//
// db_test.go — 覆盖 Open、Close、Migrate 和 schema extras 的测试。
package gorm

import (
	"os"
	"path/filepath"
	"testing"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// newTestDB opens an in-memory DB with a quiet logger. Fails the test on error.
//
// newTestDB 打开带静音 logger 的内存 DB。出错时 fail 测试。
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := Open(Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = Close(db) })
	return db
}

func TestOpen_InMemoryDB(t *testing.T) {
	db := newTestDB(t)

	// Simple sanity query: SELECT 1 should return 1.
	// 基础检查：SELECT 1 应返回 1。
	var got int
	if err := db.Raw("SELECT 1").Scan(&got).Error; err != nil {
		t.Fatalf("SELECT 1: %v", err)
	}
	if got != 1 {
		t.Errorf("got %d, want 1", got)
	}
}

func TestOpen_FileDB(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(Config{DataDir: dir, LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open file db: %v", err)
	}
	t.Cleanup(func() { _ = Close(db) })

	// The DB file should exist after Open.
	// Open 之后 DB 文件应存在。
	dbFile := filepath.Join(dir, "forgify.db")
	if _, err := os.Stat(dbFile); err != nil {
		t.Errorf("forgify.db not created: %v", err)
	}
}

func TestOpen_ForeignKeysEnabled(t *testing.T) {
	// Critical regression test: the old backend ran with FK off (SQLite default).
	// We MUST have them on so orphan rows become a DB-level error, not a bug.
	//
	// 关键回归测试：老后端 FK 是关的（SQLite 默认）。我们**必须**开启，
	// 让孤儿行成为数据库层的错误，而不是代码 bug 的温床。
	db := newTestDB(t)

	var fk int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&fk).Error; err != nil {
		t.Fatalf("query pragma: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}
}

func TestOpen_WALEnabled(t *testing.T) {
	// WAL mode is essential for concurrent read performance. Verify it's on.
	// (In-memory DBs may report "memory" instead of "wal" — skip that case.)
	//
	// WAL 模式是并发读性能的关键。确认开启。
	// （内存 DB 可能返回 "memory" 而非 "wal" —— 跳过这种情况。）
	dir := t.TempDir()
	db, err := Open(Config{DataDir: dir, LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = Close(db) })

	var mode string
	if err := db.Raw("PRAGMA journal_mode").Scan(&mode).Error; err != nil {
		t.Fatalf("query pragma: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want \"wal\"", mode)
	}
}

func TestOpen_InvalidDataDir(t *testing.T) {
	// A path that can't be created as a directory (it's an existing file)
	// should fail fast. Create a temp file first, then pass its path as DataDir.
	//
	// 无法创建为目录的路径（它是一个已存在的文件）应快速失败。先建临时文件，
	// 再把它的路径作为 DataDir 传入。
	tmpfile, err := os.CreateTemp("", "notadir-*")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	_ = tmpfile.Close()

	if _, err := Open(Config{DataDir: tmpfile.Name(), LogLevel: gormlogger.Silent}); err == nil {
		t.Errorf("expected error opening DB in path that's a file, got nil")
	}
}

func TestClose_NilSafe(t *testing.T) {
	// Close(nil) should not panic.
	// Close(nil) 不应 panic。
	if err := Close(nil); err != nil {
		t.Errorf("Close(nil) returned error: %v", err)
	}
}

// dummyModel is a minimal test-only model used to exercise Migrate before
// any real domain models exist (Phase 1).
//
// dummyModel 是一个最小的测试专用 model，用于在 Phase 1 尚无真实 domain
// model 时验证 Migrate 能跑。
type dummyModel struct {
	ID   string `gorm:"primaryKey;type:text"`
	Name string `gorm:"not null"`
}

// TableName forces a predictable, lowercase table name rather than GORM's
// default pluralization of the Go type. Keeps test assertions simple.
//
// TableName 强制使用可预测的小写表名，替代 GORM 默认的复数化。让测试断言简单。
func (dummyModel) TableName() string { return "dummy_models" }

func TestMigrate_CreatesTable(t *testing.T) {
	db := newTestDB(t)
	if err := Migrate(db, &dummyModel{}); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if !db.Migrator().HasTable(&dummyModel{}) {
		t.Errorf("table dummy_models was not created")
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	db := newTestDB(t)
	if err := Migrate(db, &dummyModel{}); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := Migrate(db, &dummyModel{}); err != nil {
		t.Fatalf("second migrate (should be idempotent): %v", err)
	}
}

func TestMigrate_MultipleModels(t *testing.T) {
	type other struct {
		ID string `gorm:"primaryKey;type:text"`
	}
	db := newTestDB(t)
	if err := Migrate(db, &dummyModel{}, &other{}); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !db.Migrator().HasTable(&dummyModel{}) {
		t.Errorf("dummy_models missing")
	}
	if !db.Migrator().HasTable(&other{}) {
		t.Errorf("others missing")
	}
}

func TestMigrate_NilDB(t *testing.T) {
	if err := Migrate(nil, &dummyModel{}); err == nil {
		t.Errorf("Migrate(nil, ...) should fail, got nil")
	}
}

func TestMigrate_EmptyModelsRuns(t *testing.T) {
	// Migrate with no models should not error — Phase 1 scenario.
	//
	// 不传任何 model 调 Migrate 不应报错——Phase 1 的实际场景。
	db := newTestDB(t)
	if err := Migrate(db); err != nil {
		t.Errorf("Migrate(db) with no models returned: %v", err)
	}
}
