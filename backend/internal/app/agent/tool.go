// tool.go — Tool interface and framework-level summary injection.
// All system tools (system.go, web.go, forge.go) implement Tool.
// The "summary" parameter is injected into every tool's Parameters schema
// here so individual tools never need to know about it.
//
// tool.go — Tool 接口和框架级 summary 注入。
// 所有 system tool 实现 Tool 接口。
// "summary" 参数在此统一注入到每个 tool 的 Parameters schema，tool 实现者无感知。
package agent

import (
	"context"
	"encoding/json"
	"fmt"

	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// Tool is the interface every system tool must implement.
// The "summary" field is injected by the framework and must NOT appear
// in the Parameters() schema returned by implementations.
//
// Tool 是每个 system tool 必须实现的接口。
// "summary" 字段由框架注入，实现方的 Parameters() 不得包含该字段。
type Tool interface {
	// Name returns the tool name used by the LLM to invoke this tool.
	// Name 返回 LLM 调用此 tool 使用的名称。
	Name() string

	// Description explains what the tool does; the LLM reads this to decide
	// when to call it.
	// Description 说明工具的用途；LLM 据此判断何时调用。
	Description() string

	// Parameters returns a JSON Schema object describing the tool's inputs.
	// Must NOT include a "summary" field — the framework adds it automatically.
	//
	// Parameters 返回描述工具输入的 JSON Schema object。
	// 不得包含 "summary" 字段，框架会自动注入。
	Parameters() json.RawMessage

	// Execute runs the tool with the given arguments JSON.
	// argsJSON never contains "summary" — the framework strips it before calling.
	//
	// Execute 用给定的 arguments JSON 执行工具。
	// argsJSON 不含 "summary"，框架在调用前已剥除。
	Execute(ctx context.Context, argsJSON string) (string, error)
}

// ── LLM def conversion ────────────────────────────────────────────────────────

// ToLLMDef converts a Tool to the ToolDef sent to the LLM,
// automatically injecting the "summary" field into the Parameters schema.
//
// ToLLMDef 把 Tool 转成发给 LLM 的 ToolDef，自动注入 "summary" 字段。
func ToLLMDef(t Tool) llminfra.ToolDef {
	return llminfra.ToolDef{
		Name:        t.Name(),
		Description: t.Description(),
		Parameters:  injectSummaryField(t.Parameters()),
	}
}

// ToLLMDefs converts a slice of Tools to ToolDefs.
//
// ToLLMDefs 批量转换 Tool 为 ToolDef。
func ToLLMDefs(tools []Tool) []llminfra.ToolDef {
	defs := make([]llminfra.ToolDef, len(tools))
	for i, t := range tools {
		defs[i] = ToLLMDef(t)
	}
	return defs
}

// injectSummaryField adds "summary" to the tool's parameters schema.
// If "summary" is already present (implementation bug), it panics to catch
// the mistake at development time rather than silently overwriting.
//
// injectSummaryField 向 tool 参数 schema 注入 "summary" 字段。
// 若已存在（实现 bug），直接 panic——开发期快速失败，不静默覆盖。
func injectSummaryField(params json.RawMessage) json.RawMessage {
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(params, &schema); err != nil {
		// Tool parameters must be a valid JSON Schema object — this is a programmer error.
		// Tool parameters 必须是合法的 JSON Schema object——这是编程错误。
		panic(fmt.Sprintf("agent: tool parameters are not a valid JSON object: %v", err))
	}

	var props map[string]json.RawMessage
	if raw, ok := schema["properties"]; ok {
		if err := json.Unmarshal(raw, &props); err != nil {
			panic(fmt.Sprintf("agent: tool parameters.properties is not a valid JSON object: %v", err))
		}
		if _, conflict := props["summary"]; conflict {
			panic("agent: tool parameters already contain 'summary' field; " +
				"rename it to avoid conflict with the framework-injected summary")
		}
	} else {
		props = map[string]json.RawMessage{}
	}

	summaryDef := json.RawMessage(`{
		"type": "string",
		"description": "One sentence describing what you are doing and why. Required."
	}`)
	props["summary"] = summaryDef

	propsRaw, err := json.Marshal(props)
	if err != nil {
		return params
	}
	schema["properties"] = propsRaw

	// Prepend "summary" to required so most LLMs output it first.
	// "summary" 排在 required 首位，引导多数 LLM 优先输出。
	var required []string
	if raw, ok := schema["required"]; ok {
		_ = json.Unmarshal(raw, &required)
	}
	required = append([]string{"summary"}, required...)
	reqRaw, err := json.Marshal(required)
	if err != nil {
		return params
	}
	schema["required"] = reqRaw

	result, err := json.Marshal(schema)
	if err != nil {
		return params
	}
	return result
}

// ── Summary extraction ────────────────────────────────────────────────────────

// StripSummary extracts the "summary" value from argsJSON and returns both
// the summary string and the JSON with "summary" removed.
// If "summary" is absent, summary is empty and strippedJSON equals argsJSON.
//
// StripSummary 从 argsJSON 中提取 "summary" 值，返回 summary 字符串和剥除后的 JSON。
// 不含 "summary" 时，summary 为空，strippedJSON 等于 argsJSON。
func StripSummary(argsJSON string) (summary, strippedJSON string) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(argsJSON), &m); err != nil {
		return "", argsJSON
	}
	if raw, ok := m["summary"]; ok {
		_ = json.Unmarshal(raw, &summary)
		delete(m, "summary")
	}
	b, err := json.Marshal(m)
	if err != nil {
		return summary, argsJSON
	}
	return summary, string(b)
}
