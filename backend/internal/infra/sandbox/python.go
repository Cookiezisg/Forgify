// Package sandbox provides a Python subprocess executor for user-forged tools.
// The tool author only defines the function body; the sandbox appends a
// __main__ driver that reads JSON from stdin, calls the function, and prints
// the result as JSON to stdout.
//
// Security model: single local user, no network/filesystem restrictions.
// Lifetime is controlled by the caller's context — cancel to stop.
//
// Package sandbox 为用户锻造的工具提供 Python subprocess 执行器。
// 工具作者只需定义函数体；sandbox 自动追加 __main__ 驱动代码，从 stdin
// 读取 JSON 输入，调用函数，将结果以 JSON 写入 stdout。
//
// 安全模型：本地单用户，不限制网络/文件系统访问。生命周期由调用方 context 控制。
package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tooldomain "github.com/sunweilin/forgify/backend/internal/domain/tool"
)

// driver is appended to user code to bridge stdin → function → stdout.
// The placeholder {FUNC_NAME} is replaced at runtime with the parsed function name.
//
// driver 追加到用户代码尾部，实现 stdin → 函数 → stdout 的桥接。
// 占位符 {FUNC_NAME} 在运行时替换为解析出的函数名。
const driver = `
if __name__ == "__main__":
    import json as _json, sys as _sys
    _input = _json.load(_sys.stdin)
    _result = {FUNC_NAME}(**_input)
    print(_json.dumps(_result))
`

// PythonSandbox executes user tool code in a Python subprocess.
//
// PythonSandbox 在 Python subprocess 中执行用户工具代码。
type PythonSandbox struct {
	pythonPath string // typically "python3"
}

// New constructs a PythonSandbox using the given Python executable.
// Pass "python3" for the system default.
//
// New 基于给定 Python 可执行文件构造 PythonSandbox。
// 传 "python3" 使用系统默认。
func New(pythonPath string) *PythonSandbox {
	return &PythonSandbox{pythonPath: pythonPath}
}

// Run executes code with input as keyword arguments. code must define exactly
// one function; the sandbox extracts the function name automatically.
// Returns ExecutionResult regardless of whether the function succeeded or
// raised an exception — only a subprocess / IO failure elevates to error.
// The process is killed when ctx is cancelled.
//
// Run 以 input 作为关键字参数执行 code。code 必须恰好定义一个函数；
// sandbox 自动提取函数名。无论函数成功或抛出异常，均返回 ExecutionResult——
// 只有 subprocess / IO 层故障才上升为 Go error。ctx cancel 时进程被杀。
func (s *PythonSandbox) Run(
	ctx context.Context,
	code string,
	input map[string]any,
) (*tooldomain.ExecutionResult, error) {
	funcName, err := extractFuncName(code)
	if err != nil {
		return nil, fmt.Errorf("sandbox.Run: %w", err)
	}

	fullCode := code + strings.Replace(driver, "{FUNC_NAME}", funcName, 1)

	tmpFile, err := os.CreateTemp("", "forgify-tool-*.py")
	if err != nil {
		return nil, fmt.Errorf("sandbox.Run: create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err = tmpFile.WriteString(fullCode); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("sandbox.Run: write temp file: %w", err)
	}
	tmpFile.Close()

	inputJSON, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("sandbox.Run: marshal input: %w", err)
	}

	start := time.Now()
	cmd := exec.CommandContext(ctx, s.pythonPath, tmpFile.Name())
	cmd.Stdin = strings.NewReader(string(inputJSON))

	stdout, runErr := cmd.Output()
	elapsed := time.Since(start).Milliseconds()

	if runErr != nil {
		// subprocess failed — could be python not found, timeout, or unhandled exception
		errMsg := runErr.Error()
		if exitErr, ok := runErr.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			errMsg = string(exitErr.Stderr)
		}
		return &tooldomain.ExecutionResult{
			OK:        false,
			ErrorMsg:  strings.TrimSpace(errMsg),
			ElapsedMs: elapsed,
		}, nil
	}

	var output any
	if err = json.Unmarshal(stdout, &output); err != nil {
		// stdout was not valid JSON — treat as raw string output
		output = strings.TrimSpace(string(stdout))
	}

	return &tooldomain.ExecutionResult{
		OK:        true,
		Output:    output,
		ElapsedMs: elapsed,
	}, nil
}

// extractFuncName parses the first "def <name>" line from user code.
// Returns an error if no function definition is found.
//
// extractFuncName 从用户代码中解析第一个 "def <name>" 行。
// 未找到函数定义时返回错误。
func extractFuncName(code string) (string, error) {
	for line := range strings.SplitSeq(code, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "def ") {
			continue
		}
		// "def parse_csv(..." → "parse_csv"
		rest := strings.TrimPrefix(trimmed, "def ")
		if idx := strings.IndexAny(rest, "(: "); idx > 0 {
			return rest[:idx], nil
		}
	}
	return "", fmt.Errorf("sandbox: no function definition found in code")
}
