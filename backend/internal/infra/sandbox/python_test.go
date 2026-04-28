package sandbox

import (
	"context"
	"testing"
	"time"
)

func TestRun_BasicExecution(t *testing.T) {
	s := New("python3")
	code := `
def add(a: int, b: int) -> int:
    return a + b
`
	result, err := s.Run(context.Background(), code, map[string]any{"a": 1, "b": 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected ok=true, got error: %s", result.ErrorMsg)
	}
	// JSON numbers unmarshal as float64
	if result.Output.(float64) != 3 {
		t.Errorf("expected output 3, got %v", result.Output)
	}
}

func TestRun_StringOutput(t *testing.T) {
	s := New("python3")
	code := `
def greet(name: str) -> str:
    return f"hello {name}"
`
	result, err := s.Run(context.Background(), code, map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected ok=true, got: %s", result.ErrorMsg)
	}
	if result.Output.(string) != "hello world" {
		t.Errorf("expected 'hello world', got %v", result.Output)
	}
}

func TestRun_DictOutput(t *testing.T) {
	s := New("python3")
	code := `
def parse(csv_text: str) -> dict:
    rows = [r.split(",") for r in csv_text.strip().split("\n")]
    return {"rows": rows, "count": len(rows)}
`
	result, err := s.Run(context.Background(), code, map[string]any{"csv_text": "a,b\n1,2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected ok=true, got: %s", result.ErrorMsg)
	}
	m, ok := result.Output.(map[string]any)
	if !ok {
		t.Fatalf("expected dict output, got %T", result.Output)
	}
	if m["count"].(float64) != 2 {
		t.Errorf("expected count=2, got %v", m["count"])
	}
}

func TestRun_PythonException_ReturnsOKFalse(t *testing.T) {
	s := New("python3")
	code := `
def divide(a: int, b: int) -> float:
    return a / b
`
	result, err := s.Run(context.Background(), code, map[string]any{"a": 1, "b": 0})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	// ZeroDivisionError is caught as ok=false, not a Go error
	if result.OK {
		t.Errorf("expected ok=false for division by zero")
	}
	if result.ErrorMsg == "" {
		t.Errorf("expected non-empty error message")
	}
}

func TestRun_ContextCancel(t *testing.T) {
	// Cancelling ctx must kill the subprocess and return ok=false.
	// ctx cancel 必须杀掉子进程并返回 ok=false。
	s := New("python3")
	code := `
def slow() -> str:
    import time
    time.sleep(10)
    return "done"
`
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	result, err := s.Run(ctx, code, map[string]any{})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.OK {
		t.Errorf("expected ok=false when context is cancelled")
	}
}

func TestRun_DefaultArgument(t *testing.T) {
	s := New("python3")
	code := `
def repeat(text: str, times: int = 2) -> str:
    return text * times
`
	result, err := s.Run(context.Background(), code, map[string]any{"text": "ab"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected ok=true, got: %s", result.ErrorMsg)
	}
	if result.Output.(string) != "abab" {
		t.Errorf("expected 'abab', got %v", result.Output)
	}
}

func TestExtractFuncName(t *testing.T) {
	cases := []struct {
		code string
		want string
	}{
		{"def parse_csv(text: str) -> list:\n    pass", "parse_csv"},
		{"def add(a, b):\n    return a+b", "add"},
		{"\n\ndef my_func() -> dict:\n    pass", "my_func"},
	}
	for _, tc := range cases {
		got, err := extractFuncName(tc.code)
		if err != nil {
			t.Errorf("extractFuncName(%q): unexpected error: %v", tc.code[:20], err)
			continue
		}
		if got != tc.want {
			t.Errorf("extractFuncName: want %q, got %q", tc.want, got)
		}
	}
}

func TestExtractFuncName_NoFunction(t *testing.T) {
	_, err := extractFuncName("x = 1\ny = 2")
	if err == nil {
		t.Error("expected error for code with no function definition")
	}
}
