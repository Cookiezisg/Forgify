// recover_test.go — unit tests for the Recover middleware.
//
// recover_test.go — Recover 中间件的单元测试。
package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// newObservedLogger returns a logger writing to an in-memory observable sink
// plus the observer that test code reads entries from.
//
// newObservedLogger 返回一个写入内存可观测接收器的 logger，以及测试代码用来
// 读取日志条目的 observer。
func newObservedLogger(t *testing.T) (*zap.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, obs := observer.New(zap.DebugLevel)
	return zap.New(core), obs
}

// handlerThatPanics makes a handler that panics with the given value.
//
// handlerThatPanics 构造一个会用给定值 panic 的 handler。
func handlerThatPanics(v any) http.Handler {
	return http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic(v)
	})
}

func TestRecover_NormalRequestPassesThrough(t *testing.T) {
	log, obs := newObservedLogger(t)
	handler := Recover(log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("brew"))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if rec.Code != http.StatusTeapot {
		t.Errorf("status: got %d, want 418", rec.Code)
	}
	if rec.Body.String() != "brew" {
		t.Errorf("body: got %q, want %q", rec.Body.String(), "brew")
	}
	if obs.Len() != 0 {
		t.Errorf("expected no log entries on normal request, got %d", obs.Len())
	}
}

func TestRecover_StringPanic(t *testing.T) {
	log, obs := newObservedLogger(t)
	handler := Recover(log)(handlerThatPanics("boom"))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	assertEnvelope500(t, rec)

	entries := obs.FilterMessage("panic recovered").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 panic log entry, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["panic"] != "boom" {
		t.Errorf("panic field: got %v, want \"boom\"", fields["panic"])
	}
	if !strings.Contains(fields["stack"].(string), "runtime/debug.Stack") &&
		!strings.Contains(fields["stack"].(string), "recover_test.go") {
		t.Errorf("stack field does not look like a real stack trace: %v", fields["stack"])
	}
}

func TestRecover_ErrorPanic(t *testing.T) {
	log, obs := newObservedLogger(t)
	handler := Recover(log)(handlerThatPanics(errors.New("db died")))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	assertEnvelope500(t, rec)

	entries := obs.FilterMessage("panic recovered").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 panic log entry, got %d", len(entries))
	}
	// zap stringifies errors; exact representation varies, but must mention "db died"
	//
	// zap 会把 error 字符串化；具体表达各异，但必须含 "db died"
	panicVal := entries[0].ContextMap()["panic"]
	if s, ok := panicVal.(string); !ok || !strings.Contains(s, "db died") {
		// zap may also preserve as error; both acceptable
		// zap 也可能保留为 error；两种都可以接受
		if e, ok := panicVal.(error); !ok || !strings.Contains(e.Error(), "db died") {
			t.Errorf("panic value does not carry 'db died': %v (%T)", panicVal, panicVal)
		}
	}
}

func TestRecover_RuntimePanic(t *testing.T) {
	// Slice out-of-range is a runtime.Error, exercising the path where the
	// panic value is a typed error rather than a string or custom error.
	//
	// 切片越界会产生 runtime.Error，用于覆盖 panic 值是运行时类型错误
	// 而非字符串或自定义 error 的路径。
	log, obs := newObservedLogger(t)
	handler := Recover(log)(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		xs := []int{}
		_ = xs[10]
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	assertEnvelope500(t, rec)

	if got := obs.FilterMessage("panic recovered").Len(); got != 1 {
		t.Errorf("expected 1 panic log, got %d", got)
	}
}

func TestRecover_DoesNotLeakPanicValueToClient(t *testing.T) {
	log, _ := newObservedLogger(t)
	secret := "SECRET_DATABASE_PASSWORD=hunter2"
	handler := Recover(log)(handlerThatPanics(secret))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("GET", "/x", nil))

	if strings.Contains(rec.Body.String(), secret) {
		t.Fatalf("response leaks panic value to client: %s", rec.Body.String())
	}

	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("response not valid JSON: %v", err)
	}
	if env.Error.Code != "INTERNAL_ERROR" {
		t.Errorf("error.code: got %q, want INTERNAL_ERROR", env.Error.Code)
	}
	if env.Error.Message != "internal server error" {
		t.Errorf("error.message: got %q, want generic message", env.Error.Message)
	}
}

func TestRecover_LogsRequestContext(t *testing.T) {
	log, obs := newObservedLogger(t)
	handler := Recover(log)(handlerThatPanics("x"))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/tools/123:run", nil))

	entries := obs.FilterMessage("panic recovered").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 panic log, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["method"] != "POST" {
		t.Errorf("method: got %v, want POST", fields["method"])
	}
	if fields["path"] != "/api/v1/tools/123:run" {
		t.Errorf("path: got %v, want /api/v1/tools/123:run", fields["path"])
	}
}

// assertEnvelope500 checks the response is a 500 with INTERNAL_ERROR envelope.
//
// assertEnvelope500 校验响应是 500 + INTERNAL_ERROR envelope。
func assertEnvelope500(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type: got %q, want application/json", ct)
	}
	if !strings.Contains(rec.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Errorf("body missing INTERNAL_ERROR code: %s", rec.Body.String())
	}
}
