// system_test.go — unit tests for system tools.
// Exercises Execute() with real file system and process calls.
//
// system_test.go — system tool 的单元测试，直接调用真实文件系统和进程。
package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── read_file ─────────────────────────────────────────────────────────────────

func TestReadFileTool_Execute(t *testing.T) {
	f, _ := os.CreateTemp(t.TempDir(), "*.txt")
	f.WriteString("hello content")
	f.Close()

	tool := &ReadFileTool{}
	args, _ := json.Marshal(map[string]string{"path": f.Name()})
	result, err := tool.Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != "hello content" {
		t.Errorf("got %q, want 'hello content'", result)
	}
}

func TestReadFileTool_NotFound(t *testing.T) {
	tool := &ReadFileTool{}
	args, _ := json.Marshal(map[string]string{"path": "/nonexistent/file.txt"})
	_, err := tool.Execute(context.Background(), string(args))
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestReadFileTool_Truncates100KB(t *testing.T) {
	big := strings.Repeat("x", 120*1024)
	f, _ := os.CreateTemp(t.TempDir(), "*.txt")
	f.WriteString(big)
	f.Close()

	tool := &ReadFileTool{}
	args, _ := json.Marshal(map[string]string{"path": f.Name()})
	result, err := tool.Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "truncated") {
		t.Errorf("expected truncation notice, got len=%d", len(result))
	}
	if len(result) > 110*1024 {
		t.Errorf("result too large: %d bytes", len(result))
	}
}

func TestReadFileTool_Interface(t *testing.T) {
	tool := &ReadFileTool{}
	if tool.Name() == "" {
		t.Error("Name() should not be empty")
	}
	if tool.Description() == "" {
		t.Error("Description() should not be empty")
	}
	if len(tool.Parameters()) == 0 {
		t.Error("Parameters() should not be empty")
	}
	// Parameters must be valid JSON.
	// Parameters 必须是合法 JSON。
	var schema map[string]any
	if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
		t.Errorf("Parameters() not valid JSON: %v", err)
	}
}

// ── write_file ────────────────────────────────────────────────────────────────

func TestWriteFileTool_Execute(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	tool := &WriteFileTool{}
	args, _ := json.Marshal(map[string]string{"path": path, "content": "written"})
	result, err := tool.Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(result, "wrote") {
		t.Errorf("result = %q, want write confirmation", result)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "written" {
		t.Errorf("file content = %q, want 'written'", data)
	}
}

func TestWriteFileTool_CreatesParentDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "c.txt")
	tool := &WriteFileTool{}
	args, _ := json.Marshal(map[string]string{"path": path, "content": "deep"})
	_, err := tool.Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "deep" {
		t.Errorf("file content = %q", data)
	}
}

// ── list_dir ──────────────────────────────────────────────────────────────────

func TestListDirTool_Execute(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)
	os.Mkdir(filepath.Join(dir, "sub"), 0755)

	tool := &ListDirTool{}
	args, _ := json.Marshal(map[string]string{"path": dir})
	result, err := tool.Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal([]byte(result), &entries); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("want 3 entries, got %d", len(entries))
	}

	types := map[string]string{}
	for _, e := range entries {
		types[e["name"].(string)] = e["type"].(string)
	}
	if types["a.txt"] != "file" || types["b.txt"] != "file" {
		t.Errorf("file types wrong: %v", types)
	}
	if types["sub"] != "dir" {
		t.Errorf("dir type wrong: %v", types)
	}
}

// ── run_shell ─────────────────────────────────────────────────────────────────

func TestRunShellTool_Execute(t *testing.T) {
	tool := &RunShellTool{}
	args, _ := json.Marshal(map[string]string{"command": "echo hello"})
	result, err := tool.Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	if out["ok"] != true {
		t.Errorf("ok = %v, want true", out["ok"])
	}
	if !strings.Contains(out["output"].(string), "hello") {
		t.Errorf("output = %q, want 'hello'", out["output"])
	}
}

func TestRunShellTool_FailingCommand(t *testing.T) {
	tool := &RunShellTool{}
	args, _ := json.Marshal(map[string]string{"command": "exit 1"})
	result, err := tool.Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("Execute should not return Go error: %v", err)
	}
	var out map[string]any
	json.Unmarshal([]byte(result), &out)
	if out["ok"] != false {
		t.Errorf("ok = %v, want false for exit 1", out["ok"])
	}
}

// ── run_python ────────────────────────────────────────────────────────────────

func TestRunPythonTool_Execute(t *testing.T) {
	tool := &RunPythonTool{}
	args, _ := json.Marshal(map[string]string{"code": "print(2 + 2)"})
	result, err := tool.Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var out map[string]any
	json.Unmarshal([]byte(result), &out)
	if out["ok"] != true {
		t.Errorf("ok = %v, want true; error=%v", out["ok"], out["error"])
	}
	if out["output"] != "4" {
		t.Errorf("output = %q, want '4'", out["output"])
	}
}

func TestRunPythonTool_SyntaxError(t *testing.T) {
	tool := &RunPythonTool{}
	args, _ := json.Marshal(map[string]string{"code": "def broken syntax:"})
	result, err := tool.Execute(context.Background(), string(args))
	if err != nil {
		t.Fatalf("Execute should not return Go error: %v", err)
	}
	var out map[string]any
	json.Unmarshal([]byte(result), &out)
	if out["ok"] != false {
		t.Errorf("ok = %v, want false for syntax error", out["ok"])
	}
}

// ── datetime ──────────────────────────────────────────────────────────────────

func TestDatetimeTool_Execute(t *testing.T) {
	tool := &DatetimeTool{}
	result, err := tool.Execute(context.Background(), "{}")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	for _, key := range []string{"datetime", "date", "time", "weekday", "unix"} {
		if out[key] == "" {
			t.Errorf("field %q is empty", key)
		}
	}
}

// ── interface compliance for all system tools ─────────────────────────────────

func TestSystemTools_AllImplementTool(t *testing.T) {
	tools := SystemTools()
	if len(tools) == 0 {
		t.Fatal("SystemTools() returned empty slice")
	}
	for _, tool := range tools {
		if tool.Name() == "" {
			t.Errorf("tool %T has empty Name()", tool)
		}
		if tool.Description() == "" {
			t.Errorf("tool %T has empty Description()", tool)
		}
		var schema map[string]any
		if err := json.Unmarshal(tool.Parameters(), &schema); err != nil {
			t.Errorf("tool %T Parameters() not valid JSON: %v", tool, err)
		}
	}
}
