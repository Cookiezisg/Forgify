// tool_test.go — unit tests for Tool interface utilities:
// injectSummaryField, StripSummary, ToLLMDef, context helpers.
//
// tool_test.go — Tool 接口工具函数的单元测试：
// injectSummaryField、StripSummary、ToLLMDef、context helpers。
package agent

import (
	"context"
	"encoding/json"
	"testing"

	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// ── injectSummaryField ────────────────────────────────────────────────────────

func TestInjectSummaryField_AddsField(t *testing.T) {
	params := json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string"}
		},
		"required": ["path"]
	}`)

	result := injectSummaryField(params)

	var schema map[string]json.RawMessage
	if err := json.Unmarshal(result, &schema); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}

	var props map[string]json.RawMessage
	if err := json.Unmarshal(schema["properties"], &props); err != nil {
		t.Fatalf("properties not valid JSON: %v", err)
	}
	if _, ok := props["summary"]; !ok {
		t.Error("summary field not injected into properties")
	}

	var required []string
	json.Unmarshal(schema["required"], &required)
	if len(required) == 0 || required[0] != "summary" {
		t.Errorf("summary not first in required: %v", required)
	}
	if !contains(required, "path") {
		t.Error("original 'path' field removed from required")
	}
}

func TestInjectSummaryField_NoRequired(t *testing.T) {
	params := json.RawMessage(`{"type":"object","properties":{"x":{"type":"string"}}}`)
	result := injectSummaryField(params)

	var schema map[string]json.RawMessage
	json.Unmarshal(result, &schema)

	var required []string
	json.Unmarshal(schema["required"], &required)
	if !contains(required, "summary") {
		t.Errorf("summary not in required: %v", required)
	}
}

func TestInjectSummaryField_ConflictPanics(t *testing.T) {
	params := json.RawMessage(`{
		"type": "object",
		"properties": {
			"summary": {"type": "string"}
		}
	}`)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on summary conflict, got none")
		}
	}()
	injectSummaryField(params)
}

func TestInjectSummaryField_NonObjectPanics(t *testing.T) {
	// Non-object schemas are programmer errors and must panic.
	// 非 object schema 是编程错误，必须 panic。
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for non-object schema, got none")
		}
	}()
	injectSummaryField(json.RawMessage(`"just a string"`))
}

// ── StripSummary ──────────────────────────────────────────────────────────────

func TestStripSummary_ExtractsAndStrips(t *testing.T) {
	args := `{"summary":"Checking Beijing weather","city":"Beijing"}`
	summary, stripped := StripSummary(args)

	if summary != "Checking Beijing weather" {
		t.Errorf("summary = %q, want 'Checking Beijing weather'", summary)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(stripped), &m); err != nil {
		t.Fatalf("stripped not valid JSON: %v", err)
	}
	if _, ok := m["summary"]; ok {
		t.Error("summary still present in stripped JSON")
	}
	if m["city"] != "Beijing" {
		t.Errorf("city = %v, want Beijing", m["city"])
	}
}

func TestStripSummary_NoSummary(t *testing.T) {
	args := `{"city":"Beijing"}`
	summary, stripped := StripSummary(args)

	if summary != "" {
		t.Errorf("summary = %q, want empty", summary)
	}
	var m map[string]any
	json.Unmarshal([]byte(stripped), &m)
	if m["city"] != "Beijing" {
		t.Errorf("args modified unexpectedly: %s", stripped)
	}
}

func TestStripSummary_InvalidJSON(t *testing.T) {
	args := `not-json`
	summary, stripped := StripSummary(args)
	if summary != "" {
		t.Errorf("expected empty summary for invalid JSON, got %q", summary)
	}
	if stripped != args {
		t.Errorf("stripped should equal input for invalid JSON")
	}
}

func TestStripSummary_EmptySummary(t *testing.T) {
	args := `{"summary":"","city":"Beijing"}`
	summary, stripped := StripSummary(args)
	if summary != "" {
		t.Errorf("empty summary string should return empty, got %q", summary)
	}
	var m map[string]any
	json.Unmarshal([]byte(stripped), &m)
	if _, ok := m["summary"]; ok {
		t.Error("summary key should be removed even when empty")
	}
}

// ── ToLLMDef ─────────────────────────────────────────────────────────────────

type stubTool struct {
	name   string
	desc   string
	params json.RawMessage
}

func (s *stubTool) Name() string                                        { return s.name }
func (s *stubTool) Description() string                                 { return s.desc }
func (s *stubTool) Parameters() json.RawMessage                         { return s.params }
func (s *stubTool) Execute(_ context.Context, _ string) (string, error) { return "", nil }

func TestToLLMDef_SummaryInjected(t *testing.T) {
	tool := &stubTool{
		name:   "read_file",
		desc:   "Reads a file",
		params: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
	}
	def := ToLLMDef(tool)

	if def.Name != "read_file" || def.Description != "Reads a file" {
		t.Errorf("name/desc mismatch: %q %q", def.Name, def.Description)
	}

	var schema map[string]json.RawMessage
	json.Unmarshal(def.Parameters, &schema)
	var props map[string]json.RawMessage
	json.Unmarshal(schema["properties"], &props)
	if _, ok := props["summary"]; !ok {
		t.Error("ToLLMDef should inject summary into parameters")
	}
}

func TestToLLMDefs_BatchConversion(t *testing.T) {
	tools := []Tool{
		&stubTool{name: "t1", params: json.RawMessage(`{"type":"object","properties":{}}`)},
		&stubTool{name: "t2", params: json.RawMessage(`{"type":"object","properties":{}}`)},
	}
	defs := ToLLMDefs(tools)
	if len(defs) != 2 {
		t.Fatalf("want 2 defs, got %d", len(defs))
	}
	if defs[0].Name != "t1" || defs[1].Name != "t2" {
		t.Errorf("names = %q %q", defs[0].Name, defs[1].Name)
	}
}

func TestToLLMDef_OriginalParamsUnchanged(t *testing.T) {
	original := `{"type":"object","properties":{"path":{"type":"string"}}}`
	tool := &stubTool{params: json.RawMessage(original)}
	ToLLMDef(tool)
	// Original tool.Parameters() should not be mutated.
	// 原始 tool.Parameters() 不应被修改。
	if string(tool.params) != original {
		t.Errorf("original params mutated: %s", tool.params)
	}
}

// ── Context helpers ───────────────────────────────────────────────────────────

func TestConversationIDContext(t *testing.T) {
	ctx := context.Background()
	id, ok := GetConversationID(ctx)
	if ok || id != "" {
		t.Errorf("empty context: ok=%v id=%q", ok, id)
	}

	ctx2 := WithConversationID(ctx, "cv-123")
	id2, ok2 := GetConversationID(ctx2)
	if !ok2 || id2 != "cv-123" {
		t.Errorf("with ID: ok=%v id=%q", ok2, id2)
	}

	// Original ctx unchanged.
	// 原始 ctx 不受影响。
	_, ok3 := GetConversationID(ctx)
	if ok3 {
		t.Error("original context should not have conversation ID")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// Compile-time check: stubTool satisfies Tool and is usable as llminfra.ToolDef.
var _ Tool = (*stubTool)(nil)
var _ llminfra.ToolDef = llminfra.ToolDef{}
