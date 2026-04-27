// chat_test.go — integration tests for Store using in-memory SQLite.
// Covers Save/Get (with Blocks), ListByConversation pagination, attachment
// CRUD, cross-user isolation, and missing-userID wiring guard.
//
// chat_test.go — Store 的集成测试（内存 SQLite）。
// 覆盖 Save/Get（含 Blocks）、分页、attachment CRUD、跨用户隔离、缺 userID 守卫。
package chat

import (
	"context"
	"encoding/json"
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
	if err := db.Migrate(database, &chatdomain.Message{}, &chatdomain.Block{}, &chatdomain.Attachment{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return New(database)
}

func ctxFor(uid string) context.Context {
	return reqctx.SetUserID(context.Background(), uid)
}

// textBlock creates a single text Block for use in test messages.
//
// textBlock 创建一个用于测试消息的文字 Block。
func textBlock(id, text string) chatdomain.Block {
	d, _ := json.Marshal(chatdomain.TextData{Text: text})
	return chatdomain.Block{
		ID:   id,
		Seq:  0,
		Type: chatdomain.BlockTypeText,
		Data: string(d),
	}
}

func mkMsg(id, uid, convID, role string, blocks ...chatdomain.Block) *chatdomain.Message {
	return &chatdomain.Message{
		ID:             id,
		UserID:         uid,
		ConversationID: convID,
		Role:           role,
		Status:         chatdomain.StatusCompleted,
		Blocks:         blocks,
	}
}

func blockText(b chatdomain.Block) string {
	var d chatdomain.TextData
	json.Unmarshal([]byte(b.Data), &d)
	return d.Text
}

// ── Save / Get ──────────────────────────────────────────────────────────────

func TestSave_InsertAndGet(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	m := mkMsg("msg-1", userAlice, conv1, chatdomain.RoleUser, textBlock("blk-1", "hello"))
	if err := s.Save(ctx, m); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Get(ctx, "msg-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Role != chatdomain.RoleUser {
		t.Errorf("unexpected role: %q", got.Role)
	}
}

func TestSave_WithBlocks(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	blk := textBlock("blk-1", "hello world")
	m := mkMsg("msg-1", userAlice, conv1, chatdomain.RoleAssistant, blk)
	if err := s.Save(ctx, m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Blocks should be persisted and returned via ListByConversation.
	// Blocks 应持久化并通过 ListByConversation 返回。
	rows, _, err := s.ListByConversation(ctx, conv1, chatdomain.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || len(rows[0].Blocks) != 1 {
		t.Fatalf("want 1 message with 1 block, got %d messages", len(rows))
	}
	if blockText(rows[0].Blocks[0]) != "hello world" {
		t.Errorf("block text = %q, want %q", blockText(rows[0].Blocks[0]), "hello world")
	}
}

func TestSave_BlocksReplaced(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	m := mkMsg("msg-1", userAlice, conv1, chatdomain.RoleAssistant, textBlock("blk-1", "draft"))
	if err := s.Save(ctx, m); err != nil {
		t.Fatalf("Save insert: %v", err)
	}

	// Replace blocks on update.
	// 更新时替换 blocks。
	m.Blocks = []chatdomain.Block{textBlock("blk-2", "final")}
	if err := s.Save(ctx, m); err != nil {
		t.Fatalf("Save update: %v", err)
	}

	rows, _, _ := s.ListByConversation(ctx, conv1, chatdomain.ListFilter{Limit: 10})
	if len(rows[0].Blocks) != 1 || blockText(rows[0].Blocks[0]) != "final" {
		t.Errorf("expected 1 block with 'final', got %+v", rows[0].Blocks)
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
	if err := s.Save(ctxFor(userAlice), mkMsg("msg-1", userAlice, conv1, chatdomain.RoleUser)); err != nil {
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
		if err := s.Save(ctx, mkMsg(id, userAlice, conv1, chatdomain.RoleUser)); err != nil {
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
	if rows[0].ID != "m1" || rows[2].ID != "m3" {
		t.Errorf("order wrong: [%s %s %s]", rows[0].ID, rows[1].ID, rows[2].ID)
	}
}

func TestList_Pagination(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	for _, id := range []string{"m1", "m2", "m3", "m4", "m5"} {
		s.Save(ctx, mkMsg(id, userAlice, conv1, chatdomain.RoleUser))
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
	ids := map[string]bool{}
	for _, row := range append(append(page1, page2...), page3...) {
		if ids[row.ID] {
			t.Errorf("duplicate id across pages: %q", row.ID)
		}
		ids[row.ID] = true
	}
}

func TestList_BlocksAttached(t *testing.T) {
	s := newStore(t)
	ctx := ctxFor(userAlice)

	// Message with two blocks.
	// 带两个 block 的消息。
	m := mkMsg("msg-1", userAlice, conv1, chatdomain.RoleAssistant,
		textBlock("blk-1", "reasoning..."),
		textBlock("blk-2", "answer"),
	)
	m.Blocks[0].Type = chatdomain.BlockTypeReasoning
	if err := s.Save(ctx, m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	rows, _, err := s.ListByConversation(ctx, conv1, chatdomain.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 || len(rows[0].Blocks) != 2 {
		t.Fatalf("want 1 message with 2 blocks, got %d messages / %d blocks",
			len(rows), func() int {
				if len(rows) > 0 {
					return len(rows[0].Blocks)
				}
				return 0
			}())
	}
	if rows[0].Blocks[0].Type != chatdomain.BlockTypeReasoning {
		t.Errorf("block[0] type = %q, want reasoning", rows[0].Blocks[0].Type)
	}
}

func TestList_CrossUserIsolation(t *testing.T) {
	s := newStore(t)
	s.Save(ctxFor(userAlice), mkMsg("a1", userAlice, conv1, chatdomain.RoleUser))
	s.Save(ctxFor(userBob), mkMsg("b1", userBob, conv1, chatdomain.RoleUser))

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
