// store_test.go — integration tests for Store using an in-memory SQLite.
// Covers Upsert (insert + update), GetByScenario, List, cross-user isolation,
// and the unique constraint guard.
//
// store_test.go — Store 的集成测试（内存 SQLite）。覆盖 Upsert（插入 + 更新）、
// GetByScenario、List、跨用户隔离、唯一约束守卫。
package model

import (
	"context"
	"errors"
	"testing"

	gormlogger "gorm.io/gorm/logger"

	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	dbinfra "github.com/sunweilin/forgify/backend/internal/infra/db"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const (
	userAlice = "u-alice"
	userBob   = "u-bob"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	database, err := dbinfra.Open(dbinfra.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = dbinfra.Close(database) })
	if err := dbinfra.Migrate(database, &modeldomain.ModelConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(database)
}

func ctxFor(userID string) context.Context {
	return reqctxpkg.SetUserID(context.Background(), userID)
}

func mkConfig(id, userID, scenario, provider, modelID string) *modeldomain.ModelConfig {
	return &modeldomain.ModelConfig{
		ID:       id,
		UserID:   userID,
		Scenario: scenario,
		Provider: provider,
		ModelID:  modelID,
	}
}

// --- Upsert ---

func TestUpsert_Insert(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	m := mkConfig("mc1", userAlice, modeldomain.ScenarioChat, "openai", "gpt-4o")
	if err := s.Upsert(ctx, m); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := s.GetByScenario(ctx, modeldomain.ScenarioChat)
	if err != nil {
		t.Fatalf("GetByScenario: %v", err)
	}
	if got.Provider != "openai" || got.ModelID != "gpt-4o" {
		t.Errorf("got %+v, want openai/gpt-4o", got)
	}
}

func TestUpsert_UpdatePreservesID(t *testing.T) {
	// Updating an existing row must keep the same ID.
	// 更新已有行必须保持相同 ID。
	s := newStore(t)
	ctx := ctxFor(userAlice)

	m := mkConfig("mc1", userAlice, modeldomain.ScenarioChat, "openai", "gpt-4o")
	if err := s.Upsert(ctx, m); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	m.Provider = "anthropic"
	m.ModelID = "claude-3-5-sonnet-latest"
	if err := s.Upsert(ctx, m); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}

	got, err := s.GetByScenario(ctx, modeldomain.ScenarioChat)
	if err != nil {
		t.Fatalf("GetByScenario: %v", err)
	}
	if got.ID != "mc1" {
		t.Errorf("ID changed: got %q, want %q", got.ID, "mc1")
	}
	if got.Provider != "anthropic" || got.ModelID != "claude-3-5-sonnet-latest" {
		t.Errorf("fields not updated: got %+v", got)
	}
}

func TestUpsert_UniqueConstraintBlocksDuplicate(t *testing.T) {
	// Two different IDs for the same (user, scenario) must be rejected by DB.
	// The service prevents this via GetByScenario+update; this test guards the DB contract.
	//
	// 同一 (user, scenario) 不同 ID 必须被 DB 拒绝。
	// Service 已通过 GetByScenario + 更新避免这种情况；本测试守护 DB 层的约束。
	s := newStore(t)
	ctx := ctxFor(userAlice)

	m1 := mkConfig("mc1", userAlice, modeldomain.ScenarioChat, "openai", "gpt-4o")
	if err := s.Upsert(ctx, m1); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	m2 := mkConfig("mc2", userAlice, modeldomain.ScenarioChat, "anthropic", "claude-3-5-sonnet-latest")
	if err := s.Upsert(ctx, m2); err == nil {
		t.Error("second Upsert with same (user, scenario) different ID: got nil, want unique constraint error")
	}
}

// --- GetByScenario ---

func TestGetByScenario_NotFound(t *testing.T) {
	s := newStore(t)
	_, err := s.GetByScenario(ctxFor(userAlice), modeldomain.ScenarioChat)
	if !errors.Is(err, modeldomain.ErrNotConfigured) {
		t.Errorf("got %v, want ErrNotConfigured", err)
	}
}

func TestGetByScenario_CrossUserIsolation(t *testing.T) {
	// Alice's config must not be visible to Bob.
	// Alice 的配置对 Bob 必须不可见。
	s := newStore(t)

	m := mkConfig("mc1", userAlice, modeldomain.ScenarioChat, "openai", "gpt-4o")
	if err := s.Upsert(ctxFor(userAlice), m); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	_, err := s.GetByScenario(ctxFor(userBob), modeldomain.ScenarioChat)
	if !errors.Is(err, modeldomain.ErrNotConfigured) {
		t.Errorf("Bob sees Alice's config: got %v, want ErrNotConfigured", err)
	}
}

func TestGetByScenario_MissingUserID(t *testing.T) {
	// A ctx without InjectUserID must produce a wiring error, not ErrNotConfigured.
	// 未经 InjectUserID 的 ctx 必须产生接线错误，而非 ErrNotConfigured。
	s := newStore(t)
	_, err := s.GetByScenario(context.Background(), modeldomain.ScenarioChat)
	if err == nil {
		t.Fatal("got nil, want wiring error")
	}
	if errors.Is(err, modeldomain.ErrNotConfigured) {
		t.Errorf("wiring bug leaked as ErrNotConfigured: %v", err)
	}
}

// --- List ---

func TestList_EmptyReturnsNonNilSlice(t *testing.T) {
	s := newStore(t)
	rows, err := s.List(ctxFor(userAlice))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if rows == nil {
		t.Error("List returned nil slice, want empty non-nil")
	}
	if len(rows) != 0 {
		t.Errorf("List returned %d rows, want 0", len(rows))
	}
}

func TestList_ReturnsActiveRows(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	m := mkConfig("mc1", userAlice, modeldomain.ScenarioChat, "openai", "gpt-4o")
	if err := s.Upsert(ctx, m); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	rows, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].ID != "mc1" {
		t.Errorf("ID = %q, want %q", rows[0].ID, "mc1")
	}
}

func TestList_CrossUserIsolation(t *testing.T) {
	s := newStore(t)

	ma := mkConfig("mc-a", userAlice, modeldomain.ScenarioChat, "openai", "gpt-4o")
	if err := s.Upsert(ctxFor(userAlice), ma); err != nil {
		t.Fatalf("Upsert Alice: %v", err)
	}
	mb := mkConfig("mc-b", userBob, modeldomain.ScenarioChat, "anthropic", "claude-3-5-sonnet-latest")
	if err := s.Upsert(ctxFor(userBob), mb); err != nil {
		t.Fatalf("Upsert Bob: %v", err)
	}

	rows, err := s.List(ctxFor(userAlice))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "mc-a" {
		t.Errorf("Alice sees wrong rows: %+v", rows)
	}
}
