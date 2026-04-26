// system.go — General-purpose system tools: file I/O, shell execution,
// Python scripting, and datetime. All use Go stdlib or the existing
// infra/sandbox — no additional dependencies.
//
// system.go — 通用系统 tools：文件读写、shell 执行、Python 脚本、时间。
// 全部使用 Go 标准库或已有 infra/sandbox，无额外依赖。
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

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// SystemTools returns the 6 general-purpose system tools.
//
// SystemTools 返回 6 个通用系统 tool。
func SystemTools() []tool.BaseTool {
	return []tool.BaseTool{
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

func (t *ReadFileTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "read_file",
		Desc: "Read the content of a local file and return it as text. " +
			"Supports text files, code, JSON, CSV, Markdown, etc. " +
			"Binary files (images, executables) are not supported.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Required: true, Desc: "Absolute or relative path to the file"},
		}),
	}, nil
}

// CoreInfo returns a human-readable summary of the tool invocation,
// used as the summary field in the SSE chat.tool_call event.
//
// CoreInfo 返回 tool 调用的人类可读摘要，
// 用于 SSE chat.tool_call 事件的 summary 字段。
func (t *ReadFileTool) CoreInfo(argsJSON string) string {
	var args struct {
		Path string `json:"path"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	return args.Path
}

func (t *ReadFileTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
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
		data = data[:100*1024]
		return string(data) + "\n\n[content truncated at 100 KB]", nil
	}
	return string(data), nil
}

// ── write_file ────────────────────────────────────────────────────────────────

// WriteFileTool writes text content to a local file.
//
// WriteFileTool 将文本内容写入本地文件。
type WriteFileTool struct{}

func (t *WriteFileTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "write_file",
		Desc: "Write text content to a local file. Creates the file (and parent directories) " +
			"if they don't exist. Overwrites the file if it already exists.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path":    {Type: schema.String, Required: true, Desc: "Absolute or relative path to write to"},
			"content": {Type: schema.String, Required: true, Desc: "Text content to write"},
		}),
	}, nil
}

// CoreInfo returns a human-readable summary of the tool invocation,
// used as the summary field in the SSE chat.tool_call event.
//
// CoreInfo 返回 tool 调用的人类可读摘要，
// 用于 SSE chat.tool_call 事件的 summary 字段。
func (t *WriteFileTool) CoreInfo(argsJSON string) string {
	var args struct {
		Path string `json:"path"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	return args.Path
}

func (t *WriteFileTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
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

func (t *ListDirTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "list_dir",
		Desc: "List the files and subdirectories in a local directory. " +
			"Returns names, types (file/dir), and sizes.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"path": {Type: schema.String, Required: true, Desc: "Directory path to list"},
		}),
	}, nil
}

// CoreInfo returns a human-readable summary of the tool invocation,
// used as the summary field in the SSE chat.tool_call event.
//
// CoreInfo 返回 tool 调用的人类可读摘要，
// 用于 SSE chat.tool_call 事件的 summary 字段。
func (t *ListDirTool) CoreInfo(argsJSON string) string {
	var args struct {
		Path string `json:"path"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	return args.Path
}

func (t *ListDirTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
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
		Type  string `json:"type"` // "file" | "dir"
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

// RunShellTool executes a shell command and returns its output.
// The frontend should confirm with the user before this tool runs.
//
// RunShellTool 执行 shell 命令并返回输出。
// 前端应在执行前向用户确认。
type RunShellTool struct{}

func (t *RunShellTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "run_shell",
		Desc: "Execute a shell command and return its stdout and stderr output. " +
			"⚠️ The user must confirm before this runs. " +
			"Timeout: 60 seconds. Working directory: current process directory.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"command": {Type: schema.String, Required: true, Desc: "Shell command to execute"},
		}),
	}, nil
}

// CoreInfo returns a human-readable summary of the tool invocation,
// used as the summary field in the SSE chat.tool_call event.
//
// CoreInfo 返回 tool 调用的人类可读摘要，
// 用于 SSE chat.tool_call 事件的 summary 字段。
func (t *RunShellTool) CoreInfo(argsJSON string) string {
	var args struct {
		Command string `json:"command"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	return "$ " + args.Command
}

func (t *RunShellTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("run_shell: bad args: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "sh", "-c", args.Command)
	out, err := cmd.CombinedOutput()

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

// RunPythonTool executes an arbitrary Python script and returns its stdout.
// Unlike run_tool (which runs a user-defined persistent function), this
// executes a complete one-off script written by the LLM.
//
// RunPythonTool 执行任意 Python 脚本并返回 stdout。
// 与 run_tool（执行用户定义的持久函数）不同，这里执行 LLM 临时编写的完整脚本。
type RunPythonTool struct{}

func (t *RunPythonTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "run_python",
		Desc: "Execute a Python script and return its output (stdout). " +
			"Write a complete script — not just a function. " +
			"Use print() to produce output. Timeout: 30 seconds.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"code": {Type: schema.String, Required: true, Desc: "Complete Python script to execute"},
		}),
	}, nil
}

// CoreInfo returns a human-readable summary of the tool invocation,
// used as the summary field in the SSE chat.tool_call event.
//
// CoreInfo 返回 tool 调用的人类可读摘要，
// 用于 SSE chat.tool_call 事件的 summary 字段。
func (t *RunPythonTool) CoreInfo(argsJSON string) string {
	var args struct {
		Code string `json:"code"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	first := strings.SplitN(strings.TrimSpace(args.Code), "\n", 2)[0]
	return strings.TrimSpace(first)
}

func (t *RunPythonTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
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

// DatetimeTool returns the current date and time. LLMs don't have real-time
// clock access — this tool bridges that gap.
//
// DatetimeTool 返回当前日期时间。LLM 没有实时时钟访问——此工具填补该空缺。
type DatetimeTool struct{}

func (t *DatetimeTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "datetime",
		Desc: "Get the current local date and time. Use this whenever you need to know " +
			"today's date, current time, day of week, or to perform date calculations.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"format": {
				Type:     schema.String,
				Required: false,
				Desc:     `Optional format: "date" (date only), "time" (time only), or omit for full datetime`,
			},
		}),
	}, nil
}

func (t *DatetimeTool) InvokableRun(_ context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Format string `json:"format"`
	}
	_ = json.Unmarshal([]byte(argsJSON), &args)

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
