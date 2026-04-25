// chat_test.go — integration tests for Store using in-memory SQLite.
// Covers Save/Get, ListByConversation pagination, attachment CRUD,
// cross-user isolation, and missing-userID wiring guard.
//
// chat_test.go — Store 的集成测试（内存 SQLite）。覆盖 Save/Get、
// ListByConversation 分页、attachment CRUD、跨用户隔离、缺 userID 接线守卫。
package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	gormlogger "gorm.io/gorm/logger"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	"github.com/sunweilin/forgify/backend/internal/infra/db"
	"github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

const (
	userAlice = "u-alice"
	userBob   = "u-bob"
	conv1     = "cv-001"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	database, err := db.Open(db.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(database) })
	if err := db.Migrate(database, &chatdomain.Message{}, &chatdomain.Attachment{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(database)
}

func ctxFor(uid string) context.Context {
	return reqctx.SetUserID(context.Background(), uid)
}

func mkMsg(id, uid, convID, role, content string) *chatdomain.Message {
	return &chatdomain.Message{
		ID:             id,
		UserID:         uid,
		ConversationID: convID,
		Role:           role,
		Content:        content,
		Status:         chatdomain.StatusCompleted,
	}
}

// ── Save / Get ──────────────────────────────────────────────────────────────

func TestSave_InsertAndGet(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	m := mkMsg("msg-1", userAlice, conv1, chatdomain.RoleUser, "hello")
	if err := s.Save(ctx, m); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Get(ctx, "msg-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != "hello" || got.Role != chatdomain.RoleUser {
		t.Errorf("unexpected message: %+v", got)
	}
}

func TestSave_Update(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	m := mkMsg("msg-1", userAlice, conv1, chatdomain.RoleAssistant, "draft")
	if err := s.Save(ctx, m); err != nil {
		t.Fatalf("Save insert: %v", err)
	}
	m.Content = "final"
	m.Status = chatdomain.StatusCompleted
	if err := s.Save(ctx, m); err != nil {
		t.Fatalf("Save update: %v", err)
	}
	got, _ := s.Get(ctx, "msg-1")
	if got.Content != "final" {
		t.Errorf("content = %q, want final", got.Content)
	}
}

func TestGet_NotFound(t *testing.T) {
	s := newStore(t)
	_, err := s.Get(ctxFor(userAlice), "missing")
	if !errors.Is(err, chatdomain.ErrMessageNotFound) {
		t.Errorf("got %v, want ErrMessageNotFound", err)
	}
}

func TestGet_CrossUserIsolation(t *testing.T) {
	s := newStore(t)
	if err := s.Save(ctxFor(userAlice), mkMsg("msg-1", userAlice, conv1, chatdomain.RoleUser, "hi")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	_, err := s.Get(ctxFor(userBob), "msg-1")
	if !errors.Is(err, chatdomain.ErrMessageNotFound) {
		t.Errorf("Bob sees Alice's message: got %v", err)
	}
}

func TestGet_MissingUserID(t *testing.T) {
	s := newStore(t)
	_, err := s.Get(context.Background(), "msg-1")
	if err == nil {
		t.Fatal("want wiring error, got nil")
	}
}

// ── ListByConversation ───────────────────────────────────────────────────────

func TestList_ChronologicalOrder(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	for _, id := range []string{"m1", "m2", "m3"} {
		if err := s.Save(ctx, mkMsg(id, userAlice, conv1, chatdomain.RoleUser, id)); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	rows, next, err := s.ListByConversation(ctx, conv1, chatdomain.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListByConversation: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	if next != "" {
		t.Errorf("unexpected nextCursor: %q", next)
	}
	// ASC order: oldest first.
	// ASC 顺序：最旧优先。
	if rows[0].ID != "m1" || rows[2].ID != "m3" {
		t.Errorf("order wrong: [%s %s %s]", rows[0].ID, rows[1].ID, rows[2].ID)
	}
}

func TestList_Pagination(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	for _, id := range []string{"m1", "m2", "m3", "m4", "m5"} {
		s.Save(ctx, mkMsg(id, userAlice, conv1, chatdomain.RoleUser, id))
		time.Sleep(2 * time.Millisecond)
	}

	page1, cursor, err := s.ListByConversation(ctx, conv1, chatdomain.ListFilter{Limit: 2})
	if err != nil || len(page1) != 2 || cursor == "" {
		t.Fatalf("page1: len=%d cursor=%q err=%v", len(page1), cursor, err)
	}
	page2, cursor2, err := s.ListByConversation(ctx, conv1, chatdomain.ListFilter{Limit: 2, Cursor: cursor})
	if err != nil || len(page2) != 2 || cursor2 == "" {
		t.Fatalf("page2: len=%d cursor=%q err=%v", len(page2), cursor2, err)
	}
	page3, next, err := s.ListByConversation(ctx, conv1, chatdomain.ListFilter{Limit: 2, Cursor: cursor2})
	if err != nil || len(page3) != 1 || next != "" {
		t.Fatalf("page3: len=%d next=%q err=%v", len(page3), next, err)
	}
	// No overlap between pages.
	// 页间不应有重复。
	ids := map[string]bool{}
	for _, row := range append(append(page1, page2...), page3...) {
		if ids[row.ID] {
			t.Errorf("duplicate id across pages: %q", row.ID)
		}
		ids[row.ID] = true
	}
}

func TestList_CrossUserIsolation(t *testing.T) {
	s := newStore(t)
	s.Save(ctxFor(userAlice), mkMsg("a1", userAlice, conv1, chatdomain.RoleUser, "alice"))
	s.Save(ctxFor(userBob), mkMsg("b1", userBob, conv1, chatdomain.RoleUser, "bob"))

	rows, _, _ := s.ListByConversation(ctxFor(userAlice), conv1, chatdomain.ListFilter{Limit: 10})
	if len(rows) != 1 || rows[0].ID != "a1" {
		t.Errorf("Alice sees wrong rows: %+v", rows)
	}
}

// ── Attachment ───────────────────────────────────────────────────────────────

func mkAtt(id, uid string) *chatdomain.Attachment {
	return &chatdomain.Attachment{
		ID:          id,
		UserID:      uid,
		FileName:    "test.jpg",
		MimeType:    "image/jpeg",
		SizeBytes:   1024,
		StoragePath: "attachments/" + id + "/original.jpg",
	}
}

func TestAttachment_SaveAndGet(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	a := mkAtt("att-1", userAlice)
	if err := s.SaveAttachment(ctx, a); err != nil {
		t.Fatalf("SaveAttachment: %v", err)
	}
	got, err := s.GetAttachment(ctx, "att-1")
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}
	if got.FileName != "test.jpg" || got.MimeType != "image/jpeg" {
		t.Errorf("unexpected attachment: %+v", got)
	}
}

func TestAttachment_CrossUserIsolation(t *testing.T) {
	s := newStore(t)
	s.SaveAttachment(ctxFor(userAlice), mkAtt("att-1", userAlice))

	_, err := s.GetAttachment(ctxFor(userBob), "att-1")
	if err == nil {
		t.Error("Bob can access Alice's attachment")
	}
}
