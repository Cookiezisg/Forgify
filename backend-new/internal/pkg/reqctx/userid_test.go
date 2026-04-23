// userid_test.go — unit tests for SetUserID / GetUserID.
//
// userid_test.go — SetUserID / GetUserID 的单元测试。
package reqctx

import (
	"context"
	"testing"
)

func TestSetGetUserID_RoundTrip(t *testing.T) {
	ctx := SetUserID(context.Background(), "alice-123")

	id, ok := GetUserID(ctx)
	if !ok {
		t.Fatal("ok: got false, want true after SetUserID")
	}
	if id != "alice-123" {
		t.Errorf("id: got %q, want \"alice-123\"", id)
	}
}

func TestGetUserID_MissingReturnsFalse(t *testing.T) {
	// Without SetUserID, GetUserID must return ("", false) so handlers
	// can treat it as a wiring bug.
	//
	// 没调 SetUserID 时，GetUserID 必须返回 ("", false)，让 handler 当
	// 接线 bug 处理。
	id, ok := GetUserID(context.Background())
	if ok {
		t.Errorf("ok: got true for empty ctx, want false")
	}
	if id != "" {
		t.Errorf("id: got %q, want empty", id)
	}
}

func TestGetUserID_EmptyStringReturnsFalse(t *testing.T) {
	// Someone might accidentally call SetUserID(ctx, ""). We must not
	// treat empty string as a valid userID.
	//
	// 有人可能误调 SetUserID(ctx, "")。我们不能把空字符串当成有效 userID。
	ctx := SetUserID(context.Background(), "")
	id, ok := GetUserID(ctx)
	if ok {
		t.Errorf("ok: got true for empty-string userID, want false")
	}
	if id != "" {
		t.Errorf("id: got %q, want empty", id)
	}
}

func TestGetUserID_PrivateKeyIsolation(t *testing.T) {
	// External code might try to inject a userID using a string key
	// "userID". Our private empty-struct key must not collide with that.
	//
	// 外部代码可能用 string key "userID" 注入 userID。我们的私有空结构体
	// key **不得**与之冲突。
	ctx := context.WithValue(context.Background(), "userID", "attacker") //nolint:staticcheck // intentional bad key type
	id, ok := GetUserID(ctx)
	if ok {
		t.Errorf("string-keyed value leaked into private key: got id=%q", id)
	}
}

func TestSetUserID_CopiesContext(t *testing.T) {
	// SetUserID must return a NEW ctx — the parent must be unaffected.
	//
	// SetUserID 必须返回**新**的 ctx，父 ctx 不应受影响。
	parent := context.Background()
	_ = SetUserID(parent, "child")

	id, ok := GetUserID(parent)
	if ok || id != "" {
		t.Errorf("parent ctx was mutated: id=%q ok=%v", id, ok)
	}
}

func TestDefaultLocalUserID_IsNotEmpty(t *testing.T) {
	// Guard against someone renaming/emptying the constant.
	// 防止有人改名或改空该常量。
	if DefaultLocalUserID == "" {
		t.Error("DefaultLocalUserID should never be empty")
	}
}
