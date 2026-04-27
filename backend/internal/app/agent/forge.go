// Package agent holds System Tool implementations used by the ReAct loop.
//
//   - forge.go  — user tool library (search / get / create / edit / run)
//   - web.go    — web_search, fetch_url
//   - system.go — file I/O, shell, python, datetime
//
// Tool implementations must not import the chat domain to avoid circular deps.
//
// Package agent 存放 ReAct loop 使用的 System Tool 实现。
// Tool 实现不得 import chat domain，避免循环依赖。
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	toolapp "github.com/sunweilin/forgify/backend/internal/app/tool"
	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventsdomain "github.com/sunweilin/forgify/backend/internal/domain/events"
	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	llminfra "github.com/sunweilin/forgify/backend/internal/infra/llm"
)

// ── Context helpers ───────────────────────────────────────────────────────────

type agentContextKey int

const (
	convIDKey agentContextKey = iota
	msgIDKey
	toolCallIDKey
)

// WithConversationID stores conversationID in ctx for system tools to read.
//
// WithConversationID 把 conversationID 存入 ctx，供 system tool 读取。
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

// WithMessageID stores the current assistant message ID in ctx.
//
// WithMessageID 把当前 assistant 消息 ID 存入 ctx。
func WithMessageID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, msgIDKey, id)
}

// GetMessageID retrieves the message ID set by WithMessageID.
//
// GetMessageID 取出由 WithMessageID 存入的消息 ID。
func GetMessageID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(msgIDKey).(string)
	return v, ok && v != ""
}

// WithToolCallID stores the LLM-assigned tool call ID in ctx.
//
// WithToolCallID 把 LLM 分配的 tool call ID 存入 ctx。
func WithToolCallID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, toolCallIDKey, id)
}

// GetToolCallID retrieves the tool call ID set by WithToolCallID.
//
// GetToolCallID 取出由 WithToolCallID 存入的 tool call ID。
func GetToolCallID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(toolCallIDKey).(string)
	return v, ok && v != ""
}

// ── ForgeTools ────────────────────────────────────────────────────────────────

// ForgeTools returns the 5 System Tools for the user tool library.
//
// ForgeTools 返回用户工具库的 5 个 System Tool。
func ForgeTools(
	toolSvc *toolapp.Service,
	attachRepo chatdomain.Repository,
	modelPicker modeldomain.ModelPicker,
	keyProvider apikeydomain.KeyProvider,
	llmFactory *llminfra.Factory,
	bridge eventsdomain.Bridge,
) []Tool {
	return []Tool{
		&SearchTool{svc: toolSvc, picker: modelPicker, keys: keyProvider, factory: llmFactory},
		&GetTool{svc: toolSvc},
		&CreateTool{svc: toolSvc, picker: modelPicker, keys: keyProvider, factory: llmFactory, bridge: bridge},
		&EditTool{svc: toolSvc, picker: modelPicker, keys: keyProvider, factory: llmFactory, bridge: bridge},
		&RunTool{svc: toolSvc, attachRepo: attachRepo},
	}
}

// ── Shared LLM client builder ─────────────────────────────────────────────────

type builtClient struct {
	client  llminfra.Client
	modelID string
	key     string
	baseURL string
}

func buildClient(
	ctx context.Context,
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
) (*builtClient, error) {
	provider, modelID, err := picker.PickForChat(ctx)
	if err != nil {
		return nil, fmt.Errorf("forge: pick model: %w", err)
	}
	creds, err := keys.ResolveCredentials(ctx, provider)
	if err != nil {
		return nil, fmt.Errorf("forge: resolve credentials: %w", err)
	}
	client, baseURL, err := factory.Build(llminfra.Config{
		Provider: provider, ModelID: modelID,
		Key: creds.Key, BaseURL: creds.BaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("forge: build client: %w", err)
	}
	return &builtClient{client: client, modelID: modelID, key: creds.Key, baseURL: baseURL}, nil
}

func (b *builtClient) newRequest(system, prompt string) llminfra.Request {
	return llminfra.Request{
		ModelID: b.modelID,
		Key:     b.key,
		BaseURL: b.baseURL,
		System:  system,
		Messages: []llminfra.LLMMessage{
			{Role: llminfra.RoleUser, Content: prompt},
		},
	}
}

// ── resolveAttachments ────────────────────────────────────────────────────────

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
//
// SearchTool 实现 search_tools system tool。
type SearchTool struct {
	svc     *toolapp.Service
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
}

func (t *SearchTool) Name() string { return "search_tools" }
func (t *SearchTool) Description() string {
	return "Search the user's tool library for relevant tools given a query. " +
		"Returns up to limit tools ranked by relevance. " +
		"Use get_tool to inspect the full code of a candidate before running it."
}
func (t *SearchTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {"type": "string", "description": "Natural language description of what you're looking for"},
			"limit": {"type": "integer", "description": "Maximum results to return (default 3, max 5)"}
		},
		"required": ["query"]
	}`)
}

func (t *SearchTool) Execute(ctx context.Context, argsJSON string) (string, error) {
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

	var sb strings.Builder
	fmt.Fprintf(&sb, "Query: %s\n\nTools:\n", args.Query)
	for _, tool := range tools {
		fmt.Fprintf(&sb, "- id: %s, name: %s, description: %s\n", tool.ID, tool.Name, tool.Description)
	}
	fmt.Fprintf(&sb, "\nReturn the %d most relevant tool IDs as JSON: "+
		`[{"id":"t_xxx","score":0.95},...]`+
		"\nRespond with valid JSON only.", args.Limit)

	bc, err := buildClient(ctx, t.picker, t.keys, t.factory)
	if err != nil {
		return "", err
	}
	resp, err := llminfra.Generate(ctx, bc.client, bc.newRequest("", sb.String()))
	if err != nil {
		return "", fmt.Errorf("search_tools: llm: %w", err)
	}

	var ranked []struct {
		ID    string  `json:"id"`
		Score float32 `json:"score"`
	}
	jsonStr, ok := extractJSON(resp)
	if !ok {
		return "", fmt.Errorf("search_tools: LLM response contained no JSON: %q", resp)
	}
	if err = json.Unmarshal([]byte(jsonStr), &ranked); err != nil {
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
		// Unmarshal errors here mean DB data is corrupted for this tool;
		// we still include the tool with nil schema rather than aborting the search.
		// DB 数据损坏时保留工具但 schema 为 nil，不中止整个搜索。
		var params, ret any
		json.Unmarshal([]byte(tool.Parameters), &params) //nolint:errcheck
		json.Unmarshal([]byte(tool.ReturnSchema), &ret)  //nolint:errcheck
		out = append(out, result{
			ID: tool.ID, Name: tool.Name, Description: tool.Description,
			Parameters: params, ReturnSchema: ret, Similarity: scoreMap[tool.ID],
		})
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// ── get_tool ──────────────────────────────────────────────────────────────────

// GetTool implements the get_tool system tool.
//
// GetTool 实现 get_tool system tool。
type GetTool struct{ svc *toolapp.Service }

func (t *GetTool) Name() string { return "get_tool" }
func (t *GetTool) Description() string {
	return "Get the full details of a specific tool including its complete Python code " +
		"and recent test summary. Use this to verify a candidate tool before running it."
}
func (t *GetTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"tool_id": {"type": "string", "description": "The tool ID (t_xxx) to retrieve"}
		},
		"required": ["tool_id"]
	}`)
}

func (t *GetTool) Execute(ctx context.Context, argsJSON string) (string, error) {
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
	if err := json.Unmarshal([]byte(detail.Parameters), &params); err != nil {
		return "", fmt.Errorf("get_tool: corrupted parameters for tool %q: %w", args.ToolID, err)
	}
	if err := json.Unmarshal([]byte(detail.ReturnSchema), &ret); err != nil {
		return "", fmt.Errorf("get_tool: corrupted return_schema for tool %q: %w", args.ToolID, err)
	}
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
//
// CreateTool 实现 create_tool system tool。
type CreateTool struct {
	svc     *toolapp.Service
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
	bridge  eventsdomain.Bridge
}

func (t *CreateTool) Name() string { return "create_tool" }
func (t *CreateTool) Description() string {
	return "Create a new Python tool in the user's tool library. " +
		"You provide a name, description, and natural-language instruction; " +
		"the system generates the code. The user will see the code appear in real time."
}
func (t *CreateTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name":        {"type": "string", "description": "Short unique tool name (snake_case)"},
			"description": {"type": "string", "description": "What this tool does"},
			"instruction": {"type": "string", "description": "Detailed code generation instruction"}
		},
		"required": ["name", "description", "instruction"]
	}`)
}

func (t *CreateTool) Execute(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Instruction string `json:"instruction"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("create_tool: bad args: %w", err)
	}
	convID, _ := GetConversationID(ctx)
	msgID, _ := GetMessageID(ctx)
	toolCallID, _ := GetToolCallID(ctx)
	code, err := streamCode(ctx, convID, "", "create",
		buildCreatePrompt(args.Name, args.Description, args.Instruction),
		t.picker, t.keys, t.factory, t.bridge)
	if err != nil {
		return "", fmt.Errorf("create_tool: generate code: %w", err)
	}

	tool, err := t.svc.Create(ctx, toolapp.CreateInput{
		Name: args.Name, Description: args.Description, Code: code,
	})
	if err != nil {
		return "", fmt.Errorf("create_tool: save: %w", err)
	}

	t.bridge.Publish(ctx, convID, eventsdomain.ToolCreated{
		ConversationID: convID, MessageID: msgID, ToolCallID: toolCallID,
		ToolID: tool.ID, ToolName: tool.Name,
	})

	var params any
	if err := json.Unmarshal([]byte(tool.Parameters), &params); err != nil {
		return "", fmt.Errorf("create_tool: corrupted parameters after save for tool %q: %w", tool.ID, err)
	}
	b, _ := json.Marshal(map[string]any{
		"tool_id": tool.ID, "name": tool.Name, "parameters": params,
	})
	return string(b), nil
}

// ── edit_tool ─────────────────────────────────────────────────────────────────

// EditTool implements the edit_tool system tool.
//
// EditTool 实现 edit_tool system tool。
type EditTool struct {
	svc     *toolapp.Service
	picker  modeldomain.ModelPicker
	keys    apikeydomain.KeyProvider
	factory *llminfra.Factory
	bridge  eventsdomain.Bridge
}

func (t *EditTool) Name() string { return "edit_tool" }
func (t *EditTool) Description() string {
	return "Propose a change to an existing tool. You can update the code (via instruction), " +
		"name, description, or tags. All changes become a pending proposal that the user must confirm."
}
func (t *EditTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"tool_id":     {"type": "string", "description": "Tool to edit"},
			"instruction": {"type": "string", "description": "Code modification instruction"},
			"name":        {"type": "string", "description": "New tool name"},
			"description": {"type": "string", "description": "New description"}
		},
		"required": ["tool_id"]
	}`)
}

func (t *EditTool) Execute(ctx context.Context, argsJSON string) (string, error) {
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
	msgID, _ := GetMessageID(ctx)
	toolCallID, _ := GetToolCallID(ctx)
	snap := toolapp.PendingSnapshot{
		Name: args.Name, Description: args.Description, Instruction: args.Instruction,
	}

	if args.Instruction != "" {
		current, err := t.svc.Get(ctx, args.ToolID)
		if err != nil {
			return "", fmt.Errorf("edit_tool: get tool: %w", err)
		}
		newCode, err := streamCode(ctx, convID, args.ToolID, "edit",
			buildEditPrompt(current.Code, args.Instruction),
			t.picker, t.keys, t.factory, t.bridge)
		if err != nil {
			return "", fmt.Errorf("edit_tool: generate code: %w", err)
		}
		snap.Code = newCode
	}

	pending, err := t.svc.CreatePending(ctx, args.ToolID, snap)
	if err != nil {
		return "", fmt.Errorf("edit_tool: create pending: %w", err)
	}

	t.bridge.Publish(ctx, convID, eventsdomain.ToolPendingCreated{
		ConversationID: convID, MessageID: msgID, ToolCallID: toolCallID,
		ToolID: args.ToolID, PendingID: pending.ID, Instruction: args.Instruction,
	})

	b, _ := json.Marshal(map[string]string{"pending_id": pending.ID, "tool_id": args.ToolID})
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

func (t *RunTool) Name() string { return "run_tool" }
func (t *RunTool) Description() string {
	return "Execute a user tool with the given input. " +
		"Returns the tool output. Execution failures return ok=false (not an error). " +
		"File paths starting with 'att_' are resolved automatically from uploaded attachments."
}
func (t *RunTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"tool_id": {"type": "string", "description": "Tool to execute"},
			"input":   {"type": "object", "description": "Input parameters matching the tool's signature"}
		},
		"required": ["tool_id", "input"]
	}`)
}

func (t *RunTool) Execute(ctx context.Context, argsJSON string) (string, error) {
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

// ── Shared streaming code generation ─────────────────────────────────────────

// streamCode calls the LLM to generate Python code, pushing ToolCodeStreaming
// SSE events for each text token. Returns the clean extracted code.
//
// streamCode 调用 LLM 生成 Python 代码，每个文字 token 推 ToolCodeStreaming 事件。
// 返回提取出的干净代码。
func streamCode(
	ctx context.Context,
	convID, toolID, actionType, prompt string,
	picker modeldomain.ModelPicker,
	keys apikeydomain.KeyProvider,
	factory *llminfra.Factory,
	bridge eventsdomain.Bridge,
) (string, error) {
	bc, err := buildClient(ctx, picker, keys, factory)
	if err != nil {
		return "", err
	}

	msgID, _ := GetMessageID(ctx)
	toolCallID, _ := GetToolCallID(ctx)

	var buf strings.Builder
	for event := range bc.client.Stream(ctx, bc.newRequest("", prompt)) {
		switch event.Type {
		case llminfra.EventText:
			buf.WriteString(event.Delta)
			bridge.Publish(ctx, convID, eventsdomain.ToolCodeStreaming{
				ConversationID: convID,
				MessageID:      msgID,
				ToolCallID:     toolCallID,
				ToolID:         toolID,
				ActionType:     actionType,
				Delta:          event.Delta,
			})
		case llminfra.EventError:
			return "", fmt.Errorf("streamCode: %w", event.Err)
		}
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("streamCode: %w", err)
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

// ── Helpers ───────────────────────────────────────────────────────────────────

func extractCode(raw string) string {
	raw = strings.TrimSpace(raw)
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

func extractJSON(s string) (string, bool) {
	s = strings.TrimSpace(s)
	for _, pair := range [][2]byte{{'[', ']'}, {'{', '}'}} {
		start := strings.IndexByte(s, pair[0])
		end := strings.LastIndexByte(s, pair[1])
		if start >= 0 && end > start {
			return s[start : end+1], true
		}
	}
	return "", false
}
