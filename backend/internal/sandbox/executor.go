package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	_ "embed"
)

//go:embed runner.py.tmpl
var runnerTemplate string

const DefaultTimeout = 30 * time.Second

// RunResult holds the outcome of a tool execution.
type RunResult struct {
	Output     any    `json:"output"`
	DurationMs int64  `json:"durationMs"`
	Error      string `json:"error,omitempty"`
	Traceback  string `json:"traceback,omitempty"`
}

// Run executes a Python tool function in an isolated subprocess.
func Run(ctx context.Context, code, funcName string, requirements []string, params map[string]any, timeout time.Duration) (*RunResult, error) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Ensure virtual environment
	venvDir, err := EnsureVenv(ctx, requirements)
	if err != nil {
		return nil, err
	}

	// Generate runner script
	runnerCode, err := buildRunner(code, funcName)
	if err != nil {
		return nil, fmt.Errorf("build runner: %w", err)
	}

	// Write to temp directory
	tmpDir, err := os.MkdirTemp("", "forgify-run-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	runnerFile := filepath.Join(tmpDir, "runner.py")
	if err := os.WriteFile(runnerFile, []byte(runnerCode), 0644); err != nil {
		return nil, err
	}

	// Prepare input
	inputJSON, _ := json.Marshal(params)
	if params == nil {
		inputJSON = []byte("{}")
	}

	// Execute
	pythonBin := PythonBin(venvDir)
	cmd := commandContext(ctx, pythonBin, runnerFile)
	cmd.Stdin = bytes.NewReader(inputJSON)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	runErr := cmd.Run()
	durationMs := time.Since(start).Milliseconds()

	result := &RunResult{DurationMs: durationMs}

	// Check timeout
	if ctx.Err() == context.DeadlineExceeded {
		result.Error = fmt.Sprintf("工具运行超过 %d 秒，已停止", int(timeout.Seconds()))
		return result, nil
	}

	// Check execution error
	if runErr != nil {
		stderrStr := strings.TrimSpace(stderr.String())
		// Try to parse structured error from stderr
		var errObj struct {
			Error     string `json:"error"`
			Type      string `json:"type"`
			Traceback string `json:"traceback"`
		}
		if json.Unmarshal([]byte(stderrStr), &errObj) == nil {
			result.Error = errObj.Type + ": " + errObj.Error
			result.Traceback = errObj.Traceback
		} else {
			result.Error = classifySandboxError(stderrStr)
		}
		return result, nil
	}

	// Parse output
	stdoutStr := strings.TrimSpace(stdout.String())
	if stdoutStr == "" {
		result.Output = nil
		return result, nil
	}
	var output any
	if err := json.Unmarshal([]byte(stdoutStr), &output); err != nil {
		result.Error = "工具返回了非 JSON 格式的数据"
		return result, nil
	}
	result.Output = output
	return result, nil
}

func buildRunner(toolCode, funcName string) (string, error) {
	tmpl, err := template.New("runner").Parse(runnerTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]string{
		"ToolCode": toolCode,
		"FuncName": funcName,
	}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func classifySandboxError(stderr string) string {
	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "syntaxerror"):
		return "代码存在语法错误：" + extractLastLine(stderr)
	case strings.Contains(lower, "modulenotfounderror"):
		return "依赖模块未找到：" + extractLastLine(stderr)
	case strings.Contains(lower, "nameerror"):
		return "变量未定义：" + extractLastLine(stderr)
	case strings.Contains(lower, "typeerror"):
		return "类型错误：" + extractLastLine(stderr)
	default:
		// Return last meaningful line
		last := extractLastLine(stderr)
		if last != "" {
			return last
		}
		return stderr
	}
}

func extractLastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}
