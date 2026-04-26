package agent

import (
	"encoding/json"
	"fmt"
)

// Summarizable is an optional interface for system tools to declare their
// human-readable core information. The value is surfaced as the summary
// field in SSE chat.tool_call events so the frontend can render a
// meaningful step label without parsing raw JSON args.
//
// Summarizable 是可选接口，tool 实现后可声明核心信息字符串，
// 写入 SSE chat.tool_call 事件的 summary 字段，前端无需解析原始 JSON。
// 未实现的 tool 由 ExtractFallbackSummary 处理。
type Summarizable interface {
	CoreInfo(argsJSON string) string
}

// ExtractFallbackSummary scans well-known field names to extract a
// human-readable value for tools that don't implement Summarizable
// (e.g. third-party tools like DuckDuckGo search).
//
// ExtractFallbackSummary 扫描常见字段名提取可读值，
// 用于未实现 Summarizable 的 tool（如第三方 DuckDuckGo）。
func ExtractFallbackSummary(argsJSON string) string {
	var args map[string]any
	if json.Unmarshal([]byte(argsJSON), &args) != nil {
		return ""
	}
	prefixes := map[string]string{
		"query":   "Searching: ",
		"url":     "Fetching ",
		"path":    "",
		"command": "$ ",
		"name":    "",
		"tool_id": "Tool: ",
	}
	for _, key := range []string{"query", "url", "path", "command", "name", "tool_id"} {
		if v, ok := args[key]; ok {
			return prefixes[key] + fmt.Sprintf("%v", v)
		}
	}
	return ""
}
