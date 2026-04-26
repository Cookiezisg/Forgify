// Package agent holds the System Tool implementations that the ReAct Agent
// uses in chat conversations. Each file in this package corresponds to one
// capability domain:
//
//   - forge.go  — user tool library (search / get / create / edit / run)
//   - web.go    — (Phase 5) web_search, fetch_url
//   - workflow.go — (Phase 4) run_workflow, …
//
// The chat service assembles these tools and injects them into the ReAct
// Agent via compose.ToolsNodeConfig. Tool implementations must not import
// the chat domain to avoid circular dependencies.
//
// Package agent 存放 ReAct Agent 在对话中使用的 System Tool 实现。
// 每个文件对应一个能力域：
//
//   - forge.go    — 用户工具库（search / get / create / edit / run）
//   - web.go      — (Phase 5) web_search, fetch_url
//   - workflow.go — (Phase 4) run_workflow, …
//
// chat service 组装这些工具并通过 compose.ToolsNodeConfig 注入 ReAct Agent。
// Tool 实现不得 import chat domain，避免循环依赖。
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	"github.com/sunweilin/forgify/backend/internal/domain/events"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	einoinfra "github.com/sunweilin/forgify/backend/internal/infra/eino"
)

// ── Context helpers ───────────────────────────────────────────────────────────

type agentContextKey int

const (
	convIDKey agentContextKey = iota
)

// WithConversationID stores conversationID in ctx for system tools to read.
// The chat service calls this before running the ReAct agent.
//
// WithConversationID 把 conversationID 存入 ctx，供 system tool 读取。
// chat service 在启动 ReAct agent 前调用此函数。
func WithConversationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, convIDKey, id)
}

// GetConversationID retrieves the conversationID set by WithConversationID.
//
// GetConversationID 取出由 WithConversationID 存入的 conversationID。
func GetConversationID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(convIDKey).(string)
	return v, ok && v != ""
}

// ── ForgeTools ────────────────────────────────────────────────────────────────

// ForgeTools returns the 5 System Tools for the user tool library.
// Assembly of all agent tools (Forge + future domains) happens in app/chat.
//
// ForgeTools 返回用户工具库的 5 个 System Tool。
// 所有 agent tool 的组装（Forge + 未来域）在 app/chat 完成。
func ForgeTools(
	toolSvc *toolapp.Service,
	attachRepo chatdomain.Repository,
	modelPicker modeldomain.ModelPicker,
	keyProvider apikeydomain.KeyProvider,
	modelFactory einoinfra.ChatModelFactory,
	bridge events.Bridge,
) []tool.BaseTool {
	return []tool.BaseTool{
		&SearchTool{svc: toolSvc, picker: modelPicker, keys: keyProvider, factory: modelFactory},
		&GetTool{svc: toolSvc},
		&CreateTool{svc: toolSvc, picker: modelPicker, keys: keyProvider, factory: modelFactory, bridge: bridge},
		&EditTool{svc: toolSvc, picker: modelPicker, keys: keyProvider, factory: modelFactory, bridge: bridge},
		&RunTool{svc: toolSvc, attachRepo: attachRepo},
	}
}

// ── Shared model builder ──────────────────────────────────────────────────────

func buildModel(
	ctx context.Context,
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory einoinfra.ChatModelFactory,
) (*einoinfra.BuiltModel, error) {
	provider, modelID, err := picker.PickForChat(ctx)
	if err != nil {
		return nil, fmt.Errorf("forge: pick model: %w", err)
	}
	creds, err := keys.ResolveCredentials(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("forge: resolve credentials: %w", err)
	}
	built, err := factory.Build(ctx, einoinfra.ModelConfig{
		Provider: provider, ModelID: modelID,
		Key: creds.Key, BaseURL: creds.BaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("forge: build model: %w", err)
	}
	return built, nil
}

// ── resolveAttachments ────────────────────────────────────────────────────────

// resolveAttachments replaces any string value starting with "att_" with the
// absolute file path from chat_attachments. Other values pass through unchanged.
//
// resolveAttachments 把 input 中以 "att_" 开头的字符串值替换为
// chat_attachments 表中的绝对路径，其他值直接透传。
func resolveAttachments(ctx context.Context, repo chatdomain.Repository, input map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(input))
	for k, v := range input {
		s, ok := v.(string)
		if !ok || !strings.HasPrefix(s, "att_") {
			out[k] = v
			continue
		}
		att, err := repo.GetAttachment(ctx, s)
		if err != nil {
			return nil, fmt.Errorf("resolveAttachments: %w", err)
		}
		out[k] = att.StoragePath
	}
	return out, nil
}

// ── search_tools ──────────────────────────────────────────────────────────────

// SearchTool implements the search_tools system tool.
// It fetches all user tools and asks the LLM to rank them by relevance.
//
// SearchTool 实现 search_tools system tool。
// 获取全部用户工具，让 LLM 按相关度排序。
type SearchTool struct {
	svc     *toolapp.Service
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory einoinfra.ChatModelFactory
}

func (t *SearchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "search_tools",
		Desc: "Search the user's tool library for relevant tools given a query. " +
			"Returns up to limit tools ranked by relevance with similarity scores. " +
			"Use get_tool to inspect the full code of a candidate before running it.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {Type: schema.String, Required: true,
				Desc: "Natural language description of what you're looking for"},
			"limit": {Type: schema.Integer, Required: false,
				Desc: "Maximum results to return (default 3, max 5)"},
		}),
	}, nil
}

// CoreInfo returns a human-readable summary of the tool invocation,
// used as the summary field in the SSE chat.tool_call event.
//
// CoreInfo 返回 tool 调用的人类可读摘要，
// 用于 SSE chat.tool_call 事件的 summary 字段。
func (t *SearchTool) CoreInfo(argsJSON string) string {
	var args struct {
		Query string `json:"query"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	return args.Query
}

func (t *SearchTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("search_tools: bad args: %w", err)
	}
	if args.Limit <= 0 || args.Limit > 5 {
		args.Limit = 3
	}

	tools, err := t.svc.ListAll(ctx)
	if err != nil {
		return "", fmt.Errorf("search_tools: list: %w", err)
	}
	if len(tools) == 0 {
		b, _ := json.Marshal([]any{})
		return string(b), nil
	}

	// Build ranking prompt.
	var sb strings.Builder
	fmt.Fprintf(&sb, "Query: %s\n\nTools:\n", args.Query)
	for _, tool := range tools {
		fmt.Fprintf(&sb, "- id: %s, name: %s, description: %s\n",
			tool.ID, tool.Name, tool.Description)
	}
	fmt.Fprintf(&sb,
		"\nReturn the %d most relevant tool IDs as JSON: "+
			`[{"id":"t_xxx","score":0.95},...]`+
			"\nRespond with valid JSON only.", args.Limit)

	built, err := buildModel(ctx, t.picker, t.keys, t.factory)
	if err != nil {
		return "", err
	}
	resp, err := built.Model.Generate(ctx, []*schema.Message{
		schema.UserMessage(sb.String()),
	})
	if err != nil {
		return "", fmt.Errorf("search_tools: llm: %w", err)
	}

	var ranked []struct {
		ID    string  `json:"id"`
		Score float32 `json:"score"`
	}
	content := extractJSON(resp.Content)
	if err = json.Unmarshal([]byte(content), &ranked); err != nil {
		return "", fmt.Errorf("search_tools: parse ranking: %w", err)
	}

	ids := make([]string, len(ranked))
	scoreMap := make(map[string]float32, len(ranked))
	for i, r := range ranked {
		ids[i] = r.ID
		scoreMap[r.ID] = r.Score
	}

	fetched, err := t.svc.GetToolsByIDs(ctx, ids)
	if err != nil {
		return "", fmt.Errorf("search_tools: fetch: %w", err)
	}

	type result struct {
		ID           string  `json:"id"`
		Name         string  `json:"name"`
		Description  string  `json:"description"`
		Parameters   any     `json:"parameters"`
		ReturnSchema any     `json:"returnSchema"`
		Similarity   float32 `json:"similarity"`
	}
	out := make([]result, 0, len(fetched))
	for _, tool := range fetched {
		var params, ret any
		_ = json.Unmarshal([]byte(tool.Parameters), &params)
		_ = json.Unmarshal([]byte(tool.ReturnSchema), &ret)
		out = append(out, result{
			ID: tool.ID, Name: tool.Name, Description: tool.Description,
			Parameters: params, ReturnSchema: ret,
			Similarity: scoreMap[tool.ID],
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ── get_tool ──────────────────────────────────────────────────────────────────

// GetTool implements the get_tool system tool.
//
// GetTool 实现 get_tool system tool。
type GetTool struct {
	svc *toolapp.Service
}

func (t *GetTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "get_tool",
		Desc: "Get the full details of a specific tool including its complete Python code " +
			"and recent test summary. Use this to verify a candidate tool before running it.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"tool_id": {Type: schema.String, Required: true,
				Desc: "The tool ID (t_xxx) to retrieve"},
		}),
	}, nil
}

// CoreInfo returns a human-readable summary of the tool invocation,
// used as the summary field in the SSE chat.tool_call event.
//
// CoreInfo 返回 tool 调用的人类可读摘要，
// 用于 SSE chat.tool_call 事件的 summary 字段。
func (t *GetTool) CoreInfo(argsJSON string) string {
	var args struct {
		ToolID string `json:"tool_id"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	return args.ToolID
}

func (t *GetTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		ToolID string `json:"tool_id"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("get_tool: bad args: %w", err)
	}
	detail, err := t.svc.GetDetail(ctx, args.ToolID)
	if err != nil {
		return "", fmt.Errorf("get_tool: %w", err)
	}
	var params, ret any
	_ = json.Unmarshal([]byte(detail.Parameters), &params)
	_ = json.Unmarshal([]byte(detail.ReturnSchema), &ret)
	out := map[string]any{
		"id": detail.ID, "name": detail.Name, "description": detail.Description,
		"code": detail.Code, "parameters": params, "returnSchema": ret,
		"tags": detail.Tags, "versionCount": detail.VersionCount,
		"testSummary": map[string]any{
			"total":        detail.TestSummary.Total,
			"lastPassRate": detail.TestSummary.LastPassRate,
			"lastRunAt":    detail.TestSummary.LastRunAt,
		},
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ── create_tool ───────────────────────────────────────────────────────────────

// CreateTool implements the create_tool system tool.
// It calls the LLM to generate Python code (streaming) and saves the tool.
//
// CreateTool 实现 create_tool system tool。
// 调用 LLM 流式生成 Python 代码并保存工具。
type CreateTool struct {
	svc     *toolapp.Service
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory einoinfra.ChatModelFactory
	bridge  events.Bridge
}

func (t *CreateTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "create_tool",
		Desc: "Create a new Python tool in the user's tool library. " +
			"You provide a name, description, and natural-language instruction; " +
			"the system generates the code. The user will see the code appear in real time.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"name":        {Type: schema.String, Required: true, Desc: "Short unique tool name (snake_case)"},
			"description": {Type: schema.String, Required: true, Desc: "What this tool does"},
			"instruction": {Type: schema.String, Required: true, Desc: "Detailed code generation instruction"},
		}),
	}, nil
}

// CoreInfo returns a human-readable summary of the tool invocation,
// used as the summary field in the SSE chat.tool_call event.
//
// CoreInfo 返回 tool 调用的人类可读摘要，
// 用于 SSE chat.tool_call 事件的 summary 字段。
func (t *CreateTool) CoreInfo(argsJSON string) string {
	var args struct {
		Name string `json:"name"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	return args.Name
}

func (t *CreateTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Instruction string `json:"instruction"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("create_tool: bad args: %w", err)
	}

	convID, _ := GetConversationID(ctx)
	code, err := t.streamCode(ctx, convID, "", "create", buildCreatePrompt(args.Name, args.Description, args.Instruction))
	if err != nil {
		return "", fmt.Errorf("create_tool: generate code: %w", err)
	}

	tool, err := t.svc.Create(ctx, toolapp.CreateInput{
		Name: args.Name, Description: args.Description, Code: code,
	})
	if err != nil {
		return "", fmt.Errorf("create_tool: save: %w", err)
	}

	t.bridge.Publish(ctx, convID, events.ToolCreated{
		ConversationID: convID, ToolID: tool.ID, ToolName: tool.Name,
	})

	var params any
	_ = json.Unmarshal([]byte(tool.Parameters), &params)
	b, _ := json.Marshal(map[string]any{
		"tool_id": tool.ID, "name": tool.Name, "parameters": params,
	})
	return string(b), nil
}

// ── edit_tool ─────────────────────────────────────────────────────────────────

// EditTool implements the edit_tool system tool.
// It optionally generates new code (streaming) and creates a pending change.
//
// EditTool 实现 edit_tool system tool。
// 可选流式生成新代码，并创建待用户确认的 pending 变更。
type EditTool struct {
	svc     *toolapp.Service
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory einoinfra.ChatModelFactory
	bridge  events.Bridge
}

func (t *EditTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "edit_tool",
		Desc: "Propose a change to an existing tool. You can update the code (via instruction), " +
			"name, description, or tags — or any combination. All changes become a pending " +
			"proposal that the user must confirm before they take effect.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"tool_id":     {Type: schema.String, Required: true, Desc: "Tool to edit"},
			"instruction": {Type: schema.String, Required: false, Desc: "Code modification instruction"},
			"name":        {Type: schema.String, Required: false, Desc: "New tool name"},
			"description": {Type: schema.String, Required: false, Desc: "New description"},
		}),
	}, nil
}

// CoreInfo returns a human-readable summary of the tool invocation,
// used as the summary field in the SSE chat.tool_call event.
//
// CoreInfo 返回 tool 调用的人类可读摘要，
// 用于 SSE chat.tool_call 事件的 summary 字段。
func (t *EditTool) CoreInfo(argsJSON string) string {
	var args struct {
		ToolID      string `json:"tool_id"`
		Instruction string `json:"instruction"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	instr := args.Instruction
	if len(instr) > 40 {
		instr = instr[:40] + "…"
	}
	if args.ToolID != "" && instr != "" {
		return args.ToolID + ": " + instr
	}
	return args.ToolID + instr
}

func (t *EditTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		ToolID      string `json:"tool_id"`
		Instruction string `json:"instruction"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("edit_tool: bad args: %w", err)
	}

	convID, _ := GetConversationID(ctx)
	snap := toolapp.PendingSnapshot{
		Name:        args.Name,
		Description: args.Description,
		Instruction: args.Instruction,
	}

	if args.Instruction != "" {
		current, err := t.svc.Get(ctx, args.ToolID)
		if err != nil {
			return "", fmt.Errorf("edit_tool: get tool: %w", err)
		}
		newCode, err := t.streamCode(ctx, convID, args.ToolID, "edit",
			buildEditPrompt(current.Code, args.Instruction))
		if err != nil {
			return "", fmt.Errorf("edit_tool: generate code: %w", err)
		}
		snap.Code = newCode
	}

	pending, err := t.svc.CreatePending(ctx, args.ToolID, snap)
	if err != nil {
		return "", fmt.Errorf("edit_tool: create pending: %w", err)
	}

	t.bridge.Publish(ctx, convID, events.ToolPendingCreated{
		ConversationID: convID,
		ToolID:         args.ToolID,
		PendingID:      pending.ID,
		Instruction:    args.Instruction,
	})

	b, _ := json.Marshal(map[string]string{
		"pending_id": pending.ID, "tool_id": args.ToolID,
	})
	return string(b), nil
}

// ── run_tool ──────────────────────────────────────────────────────────────────

// RunTool implements the run_tool system tool.
//
// RunTool 实现 run_tool system tool。
type RunTool struct {
	svc        *toolapp.Service
	attachRepo chatdomain.Repository
}

func (t *RunTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "run_tool",
		Desc: "Execute a user tool with the given input. " +
			"Returns the tool output. Execution failures return ok=false (not an error). " +
			"File paths starting with 'att_' are resolved automatically from uploaded attachments.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"tool_id": {Type: schema.String, Required: true, Desc: "Tool to execute"},
			"input":   {Type: schema.Object, Required: true, Desc: "Input parameters matching the tool's signature"},
		}),
	}, nil
}

// CoreInfo returns a human-readable summary of the tool invocation,
// used as the summary field in the SSE chat.tool_call event.
//
// CoreInfo 返回 tool 调用的人类可读摘要，
// 用于 SSE chat.tool_call 事件的 summary 字段。
func (t *RunTool) CoreInfo(argsJSON string) string {
	var args struct {
		ToolID string `json:"tool_id"`
	}
	json.Unmarshal([]byte(argsJSON), &args)
	return args.ToolID
}

func (t *RunTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args struct {
		ToolID string         `json:"tool_id"`
		Input  map[string]any `json:"input"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("run_tool: bad args: %w", err)
	}

	resolved, err := resolveAttachments(ctx, t.attachRepo, args.Input)
	if err != nil {
		return "", fmt.Errorf("run_tool: resolve attachments: %w", err)
	}

	result, err := t.svc.RunTool(ctx, args.ToolID, resolved)
	if err != nil {
		return "", fmt.Errorf("run_tool: %w", err)
	}

	b, _ := json.Marshal(map[string]any{
		"ok": result.OK, "output": result.Output,
		"error": result.ErrorMsg, "elapsed_ms": result.ElapsedMs,
	})
	return string(b), nil
}

// ── Shared streaming helper ───────────────────────────────────────────────────

// streamCode calls the LLM to generate Python code, pushing ToolCodeStreaming
// SSE events for each token. Returns the clean extracted code.
//
// streamCode 调用 LLM 生成 Python 代码，每个 token 推送 ToolCodeStreaming 事件。
// 返回提取出的干净代码。
func (t *CreateTool) streamCode(ctx context.Context, convID, toolID, actionType, prompt string) (string, error) {
	return streamCodeGeneration(ctx, convID, toolID, actionType, prompt, t.picker, t.keys, t.factory, t.bridge)
}

func (t *EditTool) streamCode(ctx context.Context, convID, toolID, actionType, prompt string) (string, error) {
	return streamCodeGeneration(ctx, convID, toolID, actionType, prompt, t.picker, t.keys, t.factory, t.bridge)
}

func streamCodeGeneration(
	ctx context.Context,
	convID, toolID, actionType, prompt string,
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory einoinfra.ChatModelFactory,
	bridge events.Bridge,
) (string, error) {
	built, err := buildModel(ctx, picker, keys, factory)
	if err != nil {
		return "", err
	}
	sr, err := built.Model.Stream(ctx, []*schema.Message{schema.UserMessage(prompt)})
	if err != nil {
		return "", fmt.Errorf("streamCode: stream: %w", err)
	}
	defer sr.Close()

	var buf strings.Builder
	for {
		chunk, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("streamCode: recv: %w", err)
		}
		if chunk.Content == "" {
			continue
		}
		buf.WriteString(chunk.Content)
		bridge.Publish(ctx, convID, events.ToolCodeStreaming{
			ConversationID: convID,
			ToolID:         toolID,
			ActionType:     actionType,
			Delta:          chunk.Content,
		})
	}
	return extractCode(buf.String()), nil
}

// ── Code generation prompts ───────────────────────────────────────────────────

func buildCreatePrompt(name, description, instruction string) string {
	return fmt.Sprintf(`Write a Python function named %q.

Description: %s
Instruction: %s

Requirements:
- Single function with type annotations
- Google-style docstring with Args: and Returns: sections
- Return value must be JSON-serializable (str, int, float, bool, list, dict)
- Only output the function definition, no main block, no explanation

Output only the Python code.`, name, description, instruction)
}

func buildEditPrompt(currentCode, instruction string) string {
	return fmt.Sprintf(`Modify the following Python function according to the instruction.

Current code:
%s

Instruction: %s

Requirements:
- Keep it a single function with type annotations
- Maintain Google-style docstring
- Return value must be JSON-serializable
- Output only the complete modified function, no explanation

Output only the Python code.`, currentCode, instruction)
}

// ── Code extraction helpers ───────────────────────────────────────────────────

// extractCode strips markdown code fences if the LLM wrapped the output.
//
// extractCode 去掉 LLM 可能添加的 markdown 代码围栏。
func extractCode(raw string) string {
	raw = strings.TrimSpace(raw)
	// Strip ```python ... ``` or ``` ... ```
	for _, fence := range []string{"```python\n", "```\n", "```python", "```"} {
		if after, ok := strings.CutPrefix(raw, fence); ok {
			raw = after
			if idx := strings.LastIndex(raw, "```"); idx >= 0 {
				raw = raw[:idx]
			}
			return strings.TrimSpace(raw)
		}
	}
	return raw
}

// extractJSON finds the first [...] or {...} JSON structure in a string.
// Useful when the LLM adds preamble text around the JSON.
//
// extractJSON 从字符串中找到第一个 [...] 或 {...} JSON 结构。
// 用于 LLM 在 JSON 前后添加了多余文本的情况。
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	for _, pair := range [][2]byte{{'[', ']'}, {'{', '}'}} {
		start := strings.IndexByte(s, pair[0])
		end := strings.LastIndexByte(s, pair[1])
		if start >= 0 && end > start {
			return s[start : end+1]
		}
	}
	return s
}
