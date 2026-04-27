// system.go — General-purpose system tools: file I/O, shell execution,
// Python scripting, and datetime. All use Go stdlib only — no extra deps.
//
// system.go — 通用系统 tools：文件读写、shell 执行、Python 脚本、时间。
// 仅使用 Go 标准库，无额外依赖。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SystemTools returns the 6 general-purpose system tools.
//
// SystemTools 返回 6 个通用系统 tool。
func SystemTools() []Tool {
	return []Tool{
		&ReadFileTool{},
		&WriteFileTool{},
		&ListDirTool{},
		&RunShellTool{},
		&RunPythonTool{},
		&DatetimeTool{},
	}
}

// ── read_file ─────────────────────────────────────────────────────────────────

// ReadFileTool reads a local file and returns its content.
//
// ReadFileTool 读取本地文件并返回内容。
type ReadFileTool struct{}

func (t *ReadFileTool) Name() string { return "read_file" }
func (t *ReadFileTool) Description() string {
	return "Read the content of a local file and return it as text. " +
		"Supports text files, code, JSON, CSV, Markdown, etc. " +
		"Binary files (images, executables) are not supported."
}
func (t *ReadFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Absolute or relative path to the file"}
		},
		"required": ["path"]
	}`)
}
func (t *ReadFileTool) Execute(_ context.Context, argsJSON string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("read_file: bad args: %w", err)
	}
	data, err := os.ReadFile(args.Path)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	// Limit to 100 KB to protect LLM context.
	// 限制 100 KB，保护 LLM context。
	if len(data) > 100*1024 {
		return string(data[:100*1024]) + "\n\n[content truncated at 100 KB]", nil
	}
	return string(data), nil
}

// ── write_file ────────────────────────────────────────────────────────────────

// WriteFileTool writes text content to a local file.
//
// WriteFileTool 将文本内容写入本地文件。
type WriteFileTool struct{}

func (t *WriteFileTool) Name() string { return "write_file" }
func (t *WriteFileTool) Description() string {
	return "Write text content to a local file. Creates the file (and parent directories) " +
		"if they don't exist. Overwrites the file if it already exists."
}
func (t *WriteFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path":    {"type": "string", "description": "Absolute or relative path to write to"},
			"content": {"type": "string", "description": "Text content to write"}
		},
		"required": ["path", "content"]
	}`)
}
func (t *WriteFileTool) Execute(_ context.Context, argsJSON string) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("write_file: bad args: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(args.Path), 0o755); err != nil {
		return "", fmt.Errorf("write_file: create dirs: %w", err)
	}
	if err := os.WriteFile(args.Path, []byte(args.Content), 0o644); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(args.Content), args.Path), nil
}

// ── list_dir ──────────────────────────────────────────────────────────────────

// ListDirTool lists the contents of a local directory.
//
// ListDirTool 列出本地目录内容。
type ListDirTool struct{}

func (t *ListDirTool) Name() string { return "list_dir" }
func (t *ListDirTool) Description() string {
	return "List the files and subdirectories in a local directory. " +
		"Returns names, types (file/dir), and sizes."
}
func (t *ListDirTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Directory path to list"}
		},
		"required": ["path"]
	}`)
}
func (t *ListDirTool) Execute(_ context.Context, argsJSON string) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("list_dir: bad args: %w", err)
	}
	entries, err := os.ReadDir(args.Path)
	if err != nil {
		return "", fmt.Errorf("list_dir: %w", err)
	}
	type entry struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		Bytes int64  `json:"bytes,omitempty"`
	}
	result := make([]entry, 0, len(entries))
	for _, e := range entries {
		item := entry{Name: e.Name(), Type: "file"}
		if e.IsDir() {
			item.Type = "dir"
		} else if info, err := e.Info(); err == nil {
			item.Bytes = info.Size()
		}
		result = append(result, item)
	}
	b, _ := json.Marshal(result)
	return string(b), nil
}

// ── run_shell ─────────────────────────────────────────────────────────────────

// RunShellTool executes a shell command and returns its combined output.
//
// RunShellTool 执行 shell 命令并返回合并输出。
type RunShellTool struct{}

func (t *RunShellTool) Name() string { return "run_shell" }
func (t *RunShellTool) Description() string {
	return "Execute a shell command and return its stdout and stderr output. " +
		"Timeout: 60 seconds. Working directory: current process directory."
}
func (t *RunShellTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {"type": "string", "description": "Shell command to execute"}
		},
		"required": ["command"]
	}`)
}
func (t *RunShellTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("run_shell: bad args: %w", err)
	}
	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	out, err := exec.CommandContext(runCtx, "sh", "-c", args.Command).CombinedOutput()
	result := map[string]any{
		"command": args.Command,
		"output":  strings.TrimSpace(string(out)),
		"ok":      err == nil,
	}
	if err != nil {
		result["error"] = err.Error()
	}
	b, _ := json.Marshal(result)
	return string(b), nil
}

// ── run_python ────────────────────────────────────────────────────────────────

// RunPythonTool executes a one-off Python script and returns its stdout.
//
// RunPythonTool 执行一次性 Python 脚本并返回 stdout。
type RunPythonTool struct{}

func (t *RunPythonTool) Name() string { return "run_python" }
func (t *RunPythonTool) Description() string {
	return "Execute a Python script and return its output (stdout). " +
		"Write a complete script — not just a function. " +
		"Use print() to produce output. Timeout: 30 seconds."
}
func (t *RunPythonTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"code": {"type": "string", "description": "Complete Python script to execute"}
		},
		"required": ["code"]
	}`)
}
func (t *RunPythonTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("run_python: bad args: %w", err)
	}
	tmp, err := os.CreateTemp("", "forgify-py-*.py")
	if err != nil {
		return "", fmt.Errorf("run_python: create temp: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.WriteString(args.Code); err != nil {
		tmp.Close()
		return "", fmt.Errorf("run_python: write: %w", err)
	}
	tmp.Close()

	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	out, execErr := exec.CommandContext(runCtx, "python3", tmp.Name()).CombinedOutput()
	result := map[string]any{
		"output": strings.TrimSpace(string(out)),
		"ok":     execErr == nil,
	}
	if execErr != nil {
		result["error"] = execErr.Error()
	}
	b, _ := json.Marshal(result)
	return string(b), nil
}

// ── datetime ──────────────────────────────────────────────────────────────────

// DatetimeTool returns the current date and time.
// LLMs lack real-time clock access — this tool bridges that gap.
//
// DatetimeTool 返回当前日期时间。LLM 没有实时时钟访问，此工具填补该空缺。
type DatetimeTool struct{}

func (t *DatetimeTool) Name() string { return "datetime" }
func (t *DatetimeTool) Description() string {
	return "Get the current local date and time. Use this whenever you need to know " +
		"today's date, current time, day of week, or to perform date calculations."
}
func (t *DatetimeTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"format": {
				"type": "string",
				"description": "Optional: \"date\" (date only), \"time\" (time only), or omit for full datetime"
			}
		}
	}`)
}
func (t *DatetimeTool) Execute(_ context.Context, argsJSON string) (string, error) {
	now := time.Now()
	b, _ := json.Marshal(map[string]string{
		"datetime": now.Format("2006-01-02 15:04:05"),
		"date":     now.Format("2006-01-02"),
		"time":     now.Format("15:04:05"),
		"weekday":  now.Weekday().String(),
		"timezone": now.Location().String(),
		"unix":     fmt.Sprintf("%d", now.Unix()),
	})
	return string(b), nil
}
