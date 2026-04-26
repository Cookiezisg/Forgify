// Package tool — integration tests for Store using an in-memory SQLite.
// Covers CRUD, user scoping, version/pending lifecycle, history retention,
// and the interface satisfaction compile-time check.
//
// Package tool — Store 集成测试（内存 SQLite）。
// 覆盖 CRUD、用户隔离、版本/pending 生命周期、历史保留、接口满足检查。
package tool

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"

	tooldomain "github.com/sunweilin/forgify/backend/internal/domain/tool"
	"github.com/sunweilin/forgify/backend/internal/infra/db"
	"github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// compile-time interface satisfaction check.
var _ tooldomain.Repository = (*Store)(nil)

const (
	userAlice = "u-alice"
	userBob   = "u-bob"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	database, err := db.Open(db.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(database) })
	if err := db.Migrate(database,
		&tooldomain.Tool{},
		&tooldomain.ToolVersion{},
		&tooldomain.ToolTestCase{},
		&tooldomain.ToolRunHistory{},
		&tooldomain.ToolTestHistory{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(database)
}

func ctxFor(userID string) context.Context {
	return reqctx.SetUserID(context.Background(), userID)
}

func mkTool(id, userID, name string) *tooldomain.Tool {
	return &tooldomain.Tool{
		ID:           id,
		UserID:       userID,
		Name:         name,
		Description:  "desc " + name,
		Code:         "def " + name + "(): pass",
		Parameters:   "[]",
		ReturnSchema: "{}",
		Tags:         "[]",
		VersionCount: 1,
	}
}

// ── Tool CRUD ─────────────────────────────────────────────────────────────────

func TestSaveAndGetTool(t *testing.T) {
	s := newStore(t)
	tool := mkTool("t_001", userAlice, "parse_csv")
	if err := s.SaveTool(ctxFor(userAlice), tool); err != nil {
		t.Fatalf("SaveTool: %v", err)
	}
	got, err := s.GetTool(ctxFor(userAlice), "t_001")
	if err != nil {
		t.Fatalf("GetTool: %v", err)
	}
	if got.Name != "parse_csv" {
		t.Errorf("name: want parse_csv, got %s", got.Name)
	}
}

func TestGetTool_NotFound(t *testing.T) {
	s := newStore(t)
	_, err := s.GetTool(ctxFor(userAlice), "t_missing")
	if !errors.Is(err, tooldomain.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetTool_UserIsolation(t *testing.T) {
	s := newStore(t)
	if err := s.SaveTool(ctxFor(userAlice), mkTool("t_001", userAlice, "tool")); err != nil {
		t.Fatal(err)
	}
	_, err := s.GetTool(ctxFor(userBob), "t_001")
	if !errors.Is(err, tooldomain.ErrNotFound) {
		t.Errorf("Bob should not see Alice's tool, got %v", err)
	}
}

func TestDeleteTool_SoftDelete(t *testing.T) {
	s := newStore(t)
	if err := s.SaveTool(ctxFor(userAlice), mkTool("t_001", userAlice, "tool")); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteTool(ctxFor(userAlice), "t_001"); err != nil {
		t.Fatalf("DeleteTool: %v", err)
	}
	_, err := s.GetTool(ctxFor(userAlice), "t_001")
	if !errors.Is(err, tooldomain.ErrNotFound) {
		t.Errorf("deleted tool should not be found, got %v", err)
	}
}

func TestListAllTools(t *testing.T) {
	s := newStore(t)
	for _, name := range []string{"tool_a", "tool_b", "tool_c"} {
		if err := s.SaveTool(ctxFor(userAlice), mkTool("t_"+name, userAlice, name)); err != nil {
			t.Fatal(err)
		}
	}
	// Bob's tool should not appear in Alice's list.
	if err := s.SaveTool(ctxFor(userBob), mkTool("t_bob", userBob, "bob_tool")); err != nil {
		t.Fatal(err)
	}
	tools, err := s.ListAllTools(ctxFor(userAlice))
	if err != nil {
		t.Fatalf("ListAllTools: %v", err)
	}
	if len(tools) != 3 {
		t.Errorf("want 3 tools, got %d", len(tools))
	}
}

func TestGetToolsByIDs_OrderPreserved(t *testing.T) {
	s := newStore(t)
	for _, id := range []string{"t_1", "t_2", "t_3"} {
		if err := s.SaveTool(ctxFor(userAlice), mkTool(id, userAlice, "tool_"+id)); err != nil {
			t.Fatal(err)
		}
	}
	tools, err := s.GetToolsByIDs(ctxFor(userAlice), []string{"t_3", "t_1"})
	if err != nil {
		t.Fatalf("GetToolsByIDs: %v", err)
	}
	if len(tools) != 2 || tools[0].ID != "t_3" || tools[1].ID != "t_1" {
		t.Errorf("order not preserved: %v", tools)
	}
}

// ── Versions ─────────────────────────────────────────────────────────────────

func mkVersion(id, toolID, userID, status string, version *int) *tooldomain.ToolVersion {
	return &tooldomain.ToolVersion{
		ID:      id,
		ToolID:  toolID,
		UserID:  userID,
		Version: version,
		Status:  status,
		Name:    "tool",
		Code:    "def tool(): pass",
		Message: "initial",
	}
}

func intPtr(n int) *int { return &n }

func TestVersionLifecycle(t *testing.T) {
	s := newStore(t)
	if err := s.SaveTool(ctxFor(userAlice), mkTool("t_001", userAlice, "tool")); err != nil {
		t.Fatal(err)
	}

	// Save a pending version.
	pending := mkVersion("tv_p1", "t_001", userAlice, tooldomain.VersionStatusPending, nil)
	if err := s.SaveVersion(ctxFor(userAlice), pending); err != nil {
		t.Fatalf("SaveVersion pending: %v", err)
	}

	// GetActivePending should return it.
	got, err := s.GetActivePending(ctxFor(userAlice), "t_001")
	if err != nil {
		t.Fatalf("GetActivePending: %v", err)
	}
	if got.ID != "tv_p1" {
		t.Errorf("want tv_p1, got %s", got.ID)
	}

	// Accept it.
	if err := s.UpdateVersionStatus(ctxFor(userAlice), "tv_p1", tooldomain.VersionStatusAccepted, intPtr(1)); err != nil {
		t.Fatalf("UpdateVersionStatus: %v", err)
	}

	// No more pending.
	_, err = s.GetActivePending(ctxFor(userAlice), "t_001")
	if !errors.Is(err, tooldomain.ErrPendingNotFound) {
		t.Errorf("expected ErrPendingNotFound after accept, got %v", err)
	}

	// GetVersion should find it.
	v, err := s.GetVersion(ctxFor(userAlice), "t_001", 1)
	if err != nil {
		t.Fatalf("GetVersion: %v", err)
	}
	if *v.Version != 1 {
		t.Errorf("want version=1, got %d", *v.Version)
	}
}

func TestDeleteOldestAcceptedVersion(t *testing.T) {
	s := newStore(t)
	if err := s.SaveTool(ctxFor(userAlice), mkTool("t_001", userAlice, "tool")); err != nil {
		t.Fatal(err)
	}
	for i, vid := range []string{"tv_v1", "tv_v2", "tv_v3"} {
		v := mkVersion(vid, "t_001", userAlice, tooldomain.VersionStatusAccepted, intPtr(i+1))
		v.CreatedAt = time.Now().Add(time.Duration(i) * time.Second)
		if err := s.SaveVersion(ctxFor(userAlice), v); err != nil {
			t.Fatal(err)
		}
	}
	n, _ := s.CountAcceptedVersions(ctxFor(userAlice), "t_001")
	if n != 3 {
		t.Fatalf("want 3 versions, got %d", n)
	}
	if err := s.DeleteOldestAcceptedVersion(ctxFor(userAlice), "t_001"); err != nil {
		t.Fatalf("DeleteOldestAcceptedVersion: %v", err)
	}
	n, _ = s.CountAcceptedVersions(ctxFor(userAlice), "t_001")
	if n != 2 {
		t.Errorf("want 2 versions after delete, got %d", n)
	}
	// v1 (lowest) should be gone.
	_, err := s.GetVersion(ctxFor(userAlice), "t_001", 1)
	if !errors.Is(err, tooldomain.ErrVersionNotFound) {
		t.Errorf("v1 should be deleted, got %v", err)
	}
}

// ── Test cases ────────────────────────────────────────────────────────────────

func TestTestCaseCRUD(t *testing.T) {
	s := newStore(t)
	if err := s.SaveTool(ctxFor(userAlice), mkTool("t_001", userAlice, "tool")); err != nil {
		t.Fatal(err)
	}
	tc := &tooldomain.ToolTestCase{
		ID:             "tc_001",
		ToolID:         "t_001",
		UserID:         userAlice,
		Name:           "basic",
		InputData:      `{"x":1}`,
		ExpectedOutput: `2`,
	}
	if err := s.SaveTestCase(ctxFor(userAlice), tc); err != nil {
		t.Fatalf("SaveTestCase: %v", err)
	}
	got, err := s.GetTestCase(ctxFor(userAlice), "tc_001")
	if err != nil {
		t.Fatalf("GetTestCase: %v", err)
	}
	if got.Name != "basic" {
		t.Errorf("want name=basic, got %s", got.Name)
	}
	if err := s.DeleteTestCase(ctxFor(userAlice), "tc_001"); err != nil {
		t.Fatalf("DeleteTestCase: %v", err)
	}
	_, err = s.GetTestCase(ctxFor(userAlice), "tc_001")
	if !errors.Is(err, tooldomain.ErrTestCaseNotFound) {
		t.Errorf("expected ErrTestCaseNotFound after delete, got %v", err)
	}
}

// ── Run history ───────────────────────────────────────────────────────────────

func TestRunHistoryRetention(t *testing.T) {
	s := newStore(t)
	if err := s.SaveTool(ctxFor(userAlice), mkTool("t_001", userAlice, "tool")); err != nil {
		t.Fatal(err)
	}
	// Insert 3 records.
	for i := range 3 {
		h := &tooldomain.ToolRunHistory{
			ID:          fmt.Sprintf("trh_%02d", i),
			ToolID:      "t_001",
			UserID:      userAlice,
			ToolVersion: 1,
			Input:       "{}",
			OK:          true,
			CreatedAt:   time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := s.SaveRunHistory(ctxFor(userAlice), h); err != nil {
			t.Fatalf("SaveRunHistory: %v", err)
		}
	}
	n, err := s.CountRunHistory(ctxFor(userAlice), "t_001")
	if err != nil || n != 3 {
		t.Fatalf("want count=3, got %d, err=%v", n, err)
	}
	// Delete oldest, then count.
	if err := s.DeleteOldestRunHistory(ctxFor(userAlice), "t_001"); err != nil {
		t.Fatalf("DeleteOldestRunHistory: %v", err)
	}
	n, _ = s.CountRunHistory(ctxFor(userAlice), "t_001")
	if n != 2 {
		t.Errorf("want 2 after delete, got %d", n)
	}
}

// ── Test history ──────────────────────────────────────────────────────────────

func TestTestHistoryBatch(t *testing.T) {
	s := newStore(t)
	if err := s.SaveTool(ctxFor(userAlice), mkTool("t_001", userAlice, "tool")); err != nil {
		t.Fatal(err)
	}
	pass := true
	for i := range 3 {
		h := &tooldomain.ToolTestHistory{
			ID:          fmt.Sprintf("tth_%02d", i),
			ToolID:      "t_001",
			UserID:      userAlice,
			ToolVersion: 1,
			TestCaseID:  fmt.Sprintf("tc_%02d", i),
			BatchID:     "batch_001",
			Input:       "{}",
			OK:          true,
			Pass:        &pass,
			CreatedAt:   time.Now().Add(time.Duration(i) * time.Second),
		}
		if err := s.SaveTestHistory(ctxFor(userAlice), h); err != nil {
			t.Fatalf("SaveTestHistory: %v", err)
		}
	}
	byBatch, err := s.ListTestHistoryByBatch(ctxFor(userAlice), "batch_001")
	if err != nil {
		t.Fatalf("ListTestHistoryByBatch: %v", err)
	}
	if len(byBatch) != 3 {
		t.Errorf("want 3, got %d", len(byBatch))
	}
	n, _ := s.CountTestHistory(ctxFor(userAlice), "t_001")
	if n != 3 {
		t.Errorf("want count=3, got %d", n)
	}
	if err := s.DeleteOldestTestHistory(ctxFor(userAlice), "t_001"); err != nil {
		t.Fatalf("DeleteOldestTestHistory: %v", err)
	}
	n, _ = s.CountTestHistory(ctxFor(userAlice), "t_001")
	if n != 2 {
		t.Errorf("want 2 after delete, got %d", n)
	}
}
