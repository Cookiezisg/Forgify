// store_test.go — integration tests for Store using an in-memory
// SQLite. Covers CRUD, user scoping, pagination, and the GetByProvider
// selection order.
//
// store_test.go — Store 的集成测试（内存 SQLite）。覆盖 CRUD、
// 用户隔离、分页、GetByProvider 选择顺序。
package apikey

import (
	"context"
	"errors"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	"github.com/sunweilin/forgify/backend/internal/infra/db"
	"github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const (
	userAlice = "u-alice"
	userBob   = "u-bob"
)

// newStore opens an in-memory DB, migrates APIKey, and returns a Store.
//
// newStore 打开内存 DB，迁移 APIKey 表，返回 Store。
func newStore(t *testing.T) *Store {
	t.Helper()
	database, err := db.Open(db.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(database) })
	if err := db.Migrate(database, &apikeydomain.APIKey{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(database)
}

func ctxFor(userID string) context.Context {
	return reqctx.SetUserID(context.Background(), userID)
}

// mkKey builds a minimal APIKey for insertion. Caller can override fields.
//
// mkKey 构造一个最小的 APIKey 用于插入。调用方可覆盖字段。
func mkKey(id, userID, provider string) *apikeydomain.APIKey {
	return &apikeydomain.APIKey{
		ID:           id,
		UserID:       userID,
		Provider:     provider,
		DisplayName:  "test-" + id,
		KeyEncrypted: "v1:cipher-" + id,
		KeyMasked:    "sk-...xxxx",
		TestStatus:   apikeydomain.TestStatusPending,
	}
}

func TestSave_Insert(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	k := mkKey("k1", userAlice, "openai")
	if err := s.Save(ctx, k); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("Get after Save: %v", err)
	}
	if got.Provider != "openai" || got.UserID != userAlice {
		t.Errorf("mismatch: got %+v", got)
	}
}

func TestSave_UpdateExisting(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	k := mkKey("k1", userAlice, "openai")
	if err := s.Save(ctx, k); err != nil {
		t.Fatalf("Save insert: %v", err)
	}

	k.DisplayName = "renamed"
	if err := s.Save(ctx, k); err != nil {
		t.Fatalf("Save update: %v", err)
	}

	got, err := s.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DisplayName != "renamed" {
		t.Errorf("DisplayName = %q, want %q", got.DisplayName, "renamed")
	}
}

func TestGet_NotFound(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	_, err := s.Get(ctx, "missing")
	if !errors.Is(err, apikeydomain.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestGet_CrossUserIsolation(t *testing.T) {
	// Alice's key must not be visible to Bob.
	// Alice 的 key 对 Bob 必须不可见。
	s := newStore(t)

	if err := s.Save(ctxFor(userAlice), mkKey("k1", userAlice, "openai")); err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, err := s.Get(ctxFor(userBob), "k1")
	if !errors.Is(err, apikeydomain.ErrNotFound) {
		t.Errorf("Bob Get Alice's key: got %v, want ErrNotFound", err)
	}
}

func TestGet_MissingUserIDInCtx(t *testing.T) {
	// A ctx without InjectUserID should produce a wiring error, not a 401.
	// 未经 InjectUserID 的 ctx 应产生接线错误，而非 401。
	s := newStore(t)
	_, err := s.Get(context.Background(), "k1")
	if err == nil {
		t.Errorf("Get without userID: got nil, want error")
	}
	if errors.Is(err, apikeydomain.ErrNotFound) {
		t.Errorf("wiring bug leaked as ErrNotFound: %v", err)
	}
}

func TestDelete_SoftDeletes(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	if err := s.Save(ctx, mkKey("k1", userAlice, "openai")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Delete(ctx, "k1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "k1"); !errors.Is(err, apikeydomain.ErrNotFound) {
		t.Errorf("Get after Delete: got %v, want ErrNotFound", err)
	}
}

func TestDelete_NotFoundReturnsError(t *testing.T) {
	s := newStore(t)
	err := s.Delete(ctxFor(userAlice), "missing")
	if !errors.Is(err, apikeydomain.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestDelete_CrossUserIsolation(t *testing.T) {
	s := newStore(t)
	if err := s.Save(ctxFor(userAlice), mkKey("k1", userAlice, "openai")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.Delete(ctxFor(userBob), "k1"); !errors.Is(err, apikeydomain.ErrNotFound) {
		t.Errorf("Bob deleting Alice's key: got %v, want ErrNotFound", err)
	}
	// Alice's key should still exist.
	// Alice 的 key 仍应存在。
	if _, err := s.Get(ctxFor(userAlice), "k1"); err != nil {
		t.Errorf("Alice's key missing after Bob's failed delete: %v", err)
	}
}

func TestUpdateTestResult_WritesOnlyTargetedFields(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	k := mkKey("k1", userAlice, "openai")
	k.DisplayName = "original"
	if err := s.Save(ctx, k); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := s.UpdateTestResult(ctx, "k1", apikeydomain.TestStatusOK, ""); err != nil {
		t.Fatalf("UpdateTestResult: %v", err)
	}

	got, err := s.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.TestStatus != apikeydomain.TestStatusOK {
		t.Errorf("TestStatus = %q, want %q", got.TestStatus, apikeydomain.TestStatusOK)
	}
	if got.LastTestedAt == nil {
		t.Errorf("LastTestedAt is nil, expected set")
	}
	if got.DisplayName != "original" {
		t.Errorf("DisplayName changed unexpectedly: %q", got.DisplayName)
	}
}

func TestUpdateTestResult_NotFound(t *testing.T) {
	s := newStore(t)
	err := s.UpdateTestResult(ctxFor(userAlice), "missing", apikeydomain.TestStatusOK, "")
	if !errors.Is(err, apikeydomain.ErrNotFound) {
		t.Errorf("got %v, want ErrNotFound", err)
	}
}

func TestGetByProvider_PrefersOKOverPending(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	pending := mkKey("k-pending", userAlice, "openai")
	if err := s.Save(ctx, pending); err != nil {
		t.Fatalf("Save pending: %v", err)
	}

	okKey := mkKey("k-ok", userAlice, "openai")
	okKey.TestStatus = apikeydomain.TestStatusOK
	if err := s.Save(ctx, okKey); err != nil {
		t.Fatalf("Save ok: %v", err)
	}

	got, err := s.GetByProvider(ctx, "openai")
	if err != nil {
		t.Fatalf("GetByProvider: %v", err)
	}
	if got.ID != "k-ok" {
		t.Errorf("picked %q, want %q (ok should win over pending)", got.ID, "k-ok")
	}
}

func TestGetByProvider_PrefersRecentlyTested(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	older := time.Now().UTC().Add(-2 * time.Hour)
	newer := time.Now().UTC().Add(-10 * time.Minute)

	kOld := mkKey("k-old", userAlice, "openai")
	kOld.TestStatus = apikeydomain.TestStatusOK
	kOld.LastTestedAt = &older
	if err := s.Save(ctx, kOld); err != nil {
		t.Fatalf("Save old: %v", err)
	}

	kNew := mkKey("k-new", userAlice, "openai")
	kNew.TestStatus = apikeydomain.TestStatusOK
	kNew.LastTestedAt = &newer
	if err := s.Save(ctx, kNew); err != nil {
		t.Fatalf("Save new: %v", err)
	}

	got, err := s.GetByProvider(ctx, "openai")
	if err != nil {
		t.Fatalf("GetByProvider: %v", err)
	}
	if got.ID != "k-new" {
		t.Errorf("picked %q, want %q (more recently tested should win)", got.ID, "k-new")
	}
}

func TestGetByProvider_NotFound(t *testing.T) {
	s := newStore(t)
	_, err := s.GetByProvider(ctxFor(userAlice), "openai")
	if !errors.Is(err, apikeydomain.ErrNotFoundForProvider) {
		t.Errorf("got %v, want ErrNotFoundForProvider", err)
	}
}

func TestList_Basic(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	for _, id := range []string{"a", "b", "c"} {
		if err := s.Save(ctx, mkKey(id, userAlice, "openai")); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
		// Spread created_at so DESC ordering is deterministic.
		// 拉开 created_at，让 DESC 排序确定。
		time.Sleep(2 * time.Millisecond)
	}

	rows, next, err := s.List(ctx, apikeydomain.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if next != "" {
		t.Errorf("unexpected nextCursor: %q", next)
	}
	// Most recent first / 最新的在前.
	if rows[0].ID != "c" || rows[2].ID != "a" {
		t.Errorf("order wrong: got [%s %s %s], want [c b a]", rows[0].ID, rows[1].ID, rows[2].ID)
	}
}

func TestList_PaginationWithCursor(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	for _, id := range []string{"a", "b", "c", "d", "e"} {
		if err := s.Save(ctx, mkKey(id, userAlice, "openai")); err != nil {
			t.Fatalf("Save: %v", err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	// Page 1 / 第一页
	page1, next, err := s.List(ctx, apikeydomain.ListFilter{Limit: 2})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 2 || next == "" {
		t.Fatalf("page1: got %d rows next=%q, want 2 rows + cursor", len(page1), next)
	}

	// Page 2 / 第二页
	page2, next2, err := s.List(ctx, apikeydomain.ListFilter{Limit: 2, Cursor: next})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 2 || next2 == "" {
		t.Fatalf("page2: got %d rows next=%q, want 2 rows + cursor", len(page2), next2)
	}

	// No overlap between pages / 页间不应有重复.
	for _, r1 := range page1 {
		for _, r2 := range page2 {
			if r1.ID == r2.ID {
				t.Errorf("overlap: %q in both pages", r1.ID)
			}
		}
	}

	// Page 3 (final) / 第三页（收尾）
	page3, next3, err := s.List(ctx, apikeydomain.ListFilter{Limit: 2, Cursor: next2})
	if err != nil {
		t.Fatalf("page3: %v", err)
	}
	if len(page3) != 1 {
		t.Errorf("page3: got %d rows, want 1", len(page3))
	}
	if next3 != "" {
		t.Errorf("page3 nextCursor = %q, want empty", next3)
	}
}

func TestList_ProviderFilter(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	if err := s.Save(ctx, mkKey("o1", userAlice, "openai")); err != nil {
		t.Fatalf("Save o1: %v", err)
	}
	if err := s.Save(ctx, mkKey("a1", userAlice, "anthropic")); err != nil {
		t.Fatalf("Save a1: %v", err)
	}

	rows, _, err := s.List(ctx, apikeydomain.ListFilter{Limit: 10, Provider: "openai"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "o1" {
		t.Errorf("provider filter wrong: got %+v", rows)
	}
}

func TestList_CrossUserIsolation(t *testing.T) {
	s := newStore(t)

	if err := s.Save(ctxFor(userAlice), mkKey("a1", userAlice, "openai")); err != nil {
		t.Fatalf("Save Alice: %v", err)
	}
	if err := s.Save(ctxFor(userBob), mkKey("b1", userBob, "openai")); err != nil {
		t.Fatalf("Save Bob: %v", err)
	}

	rows, _, err := s.List(ctxFor(userAlice), apikeydomain.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != "a1" {
		t.Errorf("Alice sees wrong rows: %+v", rows)
	}
}

func TestList_InvalidCursor(t *testing.T) {
	s := newStore(t)
	_, _, err := s.List(ctxFor(userAlice), apikeydomain.ListFilter{Cursor: "!!!not-base64!!!"})
	if err == nil {
		t.Errorf("got nil, want error on malformed cursor")
	}
}
