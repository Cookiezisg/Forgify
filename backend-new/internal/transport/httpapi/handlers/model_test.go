// model_test.go — E2E contract tests for /api/v1/model-configs/* endpoints.
// Full stack: in-memory SQLite → real Store → real Service → Handler,
// wrapped by InjectUserID middleware.
//
// model_test.go — /api/v1/model-configs/* 端点的端到端契约测试。
// 完整栈：内存 SQLite → 真 Store → 真 Service → Handler，
// 用 InjectUserID 中间件包裹。
package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"
	gormlogger "gorm.io/gorm/logger"

	modelapp "github.com/sunweilin/forgify/backend/internal/app/model"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	"github.com/sunweilin/forgify/backend/internal/infra/db"
	modelstore "github.com/sunweilin/forgify/backend/internal/infra/store/model"
	"github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

func newModelTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	gdb, err := db.Open(db.Config{LogLevel: gormlogger.Silent})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close(gdb) })
	if err := db.Migrate(gdb, &modeldomain.ModelConfig{}); err != nil {
		t.Fatalf("db.Migrate: %v", err)
	}
	log := zaptest.NewLogger(t)
	svc := modelapp.NewService(modelstore.New(gdb), log)
	h := NewModelConfigHandler(svc, log)

	mux := http.NewServeMux()
	h.Register(mux)
	return httptest.NewServer(middleware.InjectUserID(mux))
}

// ---- GET /api/v1/model-configs ----

func TestModelHandler_List_EmptyReturnsArray(t *testing.T) {
	srv := newModelTestServer(t)
	defer srv.Close()

	status, env := do(t, srv, "GET", "/api/v1/model-configs", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	items, ok := env["data"].([]any)
	if !ok {
		t.Fatalf("data is not an array: %+v", env["data"])
	}
	if len(items) != 0 {
		t.Errorf("len(data) = %d, want 0", len(items))
	}
}

// ---- PUT /api/v1/model-configs/{scenario} ----

func TestModelHandler_Upsert_Success(t *testing.T) {
	srv := newModelTestServer(t)
	defer srv.Close()

	status, env := do(t, srv, "PUT", "/api/v1/model-configs/chat", map[string]any{
		"provider": "openai",
		"modelId":  "gpt-4o",
	})
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %+v", status, env)
	}
	d := dataMap(t, env)
	if got := d["scenario"].(string); got != "chat" {
		t.Errorf("scenario = %q, want chat", got)
	}
	if got := d["provider"].(string); got != "openai" {
		t.Errorf("provider = %q, want openai", got)
	}
	if got := d["modelId"].(string); got != "gpt-4o" {
		t.Errorf("modelId = %q, want gpt-4o", got)
	}
	// UserID must not appear in response (json:"-").
	// UserID 不得出现在响应里（json:"-"）。
	if _, has := d["userId"]; has {
		t.Error("userId leaked into response")
	}
}

func TestModelHandler_Upsert_UpdateKeepsOneRow(t *testing.T) {
	// Two PUTs to the same scenario must result in a single row.
	// 同一 scenario 两次 PUT 必须只产生一条行。
	srv := newModelTestServer(t)
	defer srv.Close()

	do(t, srv, "PUT", "/api/v1/model-configs/chat", map[string]any{
		"provider": "openai", "modelId": "gpt-4o",
	})
	do(t, srv, "PUT", "/api/v1/model-configs/chat", map[string]any{
		"provider": "anthropic", "modelId": "claude-3-5-sonnet-latest",
	})

	status, env := do(t, srv, "GET", "/api/v1/model-configs", nil)
	if status != http.StatusOK {
		t.Fatalf("GET status = %d", status)
	}
	items := env["data"].([]any)
	if len(items) != 1 {
		t.Errorf("len(data) = %d, want 1 after two PUTs on same scenario", len(items))
	}
	d := items[0].(map[string]any)
	if got := d["provider"].(string); got != "anthropic" {
		t.Errorf("provider = %q, want anthropic (second PUT should win)", got)
	}
}

func TestModelHandler_Upsert_InvalidScenario(t *testing.T) {
	srv := newModelTestServer(t)
	defer srv.Close()

	status, env := do(t, srv, "PUT", "/api/v1/model-configs/workflow_llm", map[string]any{
		"provider": "openai", "modelId": "gpt-4o",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if code := errorCode(t, env); code != "INVALID_SCENARIO" {
		t.Errorf("code = %q, want INVALID_SCENARIO", code)
	}
}

func TestModelHandler_Upsert_ProviderRequired(t *testing.T) {
	srv := newModelTestServer(t)
	defer srv.Close()

	status, env := do(t, srv, "PUT", "/api/v1/model-configs/chat", map[string]any{
		"provider": "", "modelId": "gpt-4o",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if code := errorCode(t, env); code != "PROVIDER_REQUIRED" {
		t.Errorf("code = %q, want PROVIDER_REQUIRED", code)
	}
}

func TestModelHandler_Upsert_ModelIDRequired(t *testing.T) {
	srv := newModelTestServer(t)
	defer srv.Close()

	status, env := do(t, srv, "PUT", "/api/v1/model-configs/chat", map[string]any{
		"provider": "openai", "modelId": "",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if code := errorCode(t, env); code != "MODEL_ID_REQUIRED" {
		t.Errorf("code = %q, want MODEL_ID_REQUIRED", code)
	}
}

func TestModelHandler_Upsert_MalformedJSON(t *testing.T) {
	srv := newModelTestServer(t)
	defer srv.Close()

	status, env := do(t, srv, "PUT", "/api/v1/model-configs/chat", "{bad json")
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
	if code := errorCode(t, env); code != "INVALID_REQUEST" {
		t.Errorf("code = %q, want INVALID_REQUEST", code)
	}
}
