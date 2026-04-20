package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	"github.com/sunweilin/forgify/internal/events"
	"github.com/sunweilin/forgify/internal/forge"
	"github.com/sunweilin/forgify/internal/model"
	"github.com/sunweilin/forgify/internal/service"
	"github.com/sunweilin/forgify/internal/storage"
)

var (
	cancelsMu sync.Mutex
	cancels   = make(map[string]context.CancelFunc)
)

func (s *Server) sendMessage(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConversationID string `json:"conversationId"`
		Message        string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request", http.StatusBadRequest)
		return
	}
	if req.ConversationID == "" || req.Message == "" {
		jsonError(w, "conversationId and message are required", http.StatusBadRequest)
		return
	}

	// Save user message
	storage.DB().Exec(
		`INSERT INTO messages (id, conversation_id, role, content, content_type) VALUES (?, ?, 'user', ?, 'text')`,
		uuid.NewString(), req.ConversationID, req.Message,
	)
	s.convSvc.TouchUpdatedAt(req.ConversationID)

	// Build tool summary BEFORE opening DB rows (avoid SQLite single-connection deadlock)
	toolSummary := buildToolSummary(s.toolSvc)

	// Load history
	history, err := loadHistory(req.ConversationID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Inject system prompts
	systemMsgs := []*schema.Message{
		schema.SystemMessage(forge.ForgeSystemPrompt),
	}
	if toolSummary != "" {
		systemMsgs = append(systemMsgs, schema.SystemMessage(toolSummary))
	}

	// If conversation is bound to a tool, inject the tool's current code as context
	conv, _ := s.convSvc.Get(req.ConversationID)
	if conv != nil && conv.AssetID != nil && conv.AssetType != nil && *conv.AssetType == "tool" {
		boundTool, _ := s.toolSvc.Get(*conv.AssetID)
		if boundTool != nil {
			toolContext := fmt.Sprintf("[当前绑定工具: %s]\n函数名: %s\n描述: %s\n状态: %s\n\n当前代码:\n```python\n%s\n```\n\n用户可能要求修改此工具。请使用 update_tool_code 工具进行修改。",
				boundTool.DisplayName, boundTool.Name, boundTool.Description, boundTool.Status, boundTool.Code)
			systemMsgs = append(systemMsgs, schema.SystemMessage(toolContext))
		}
	}

	history = append(systemMsgs, history...)

	// Get model
	keyProvider := func(provider string) (key, baseURL string, err error) {
		return service.GetRawKeyForProvider(provider)
	}
	gateway := model.New(keyProvider, s.Events)
	llm, modelID, err := gateway.GetModel(r.Context(), model.PurposeConversation)
	if err != nil {
		s.Events.Emit(events.ChatError, map[string]any{
			"conversationId": req.ConversationID,
			"error":          classifyError(err),
		})
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Bind forge tools
	boundLLM := bindForgeTools(r.Context(), req.ConversationID, llm, s.convSvc, s.toolSvc, s.Events)

	ctx, cancel := context.WithCancel(context.Background())
	cancelsMu.Lock()
	cancels[req.ConversationID] = cancel
	cancelsMu.Unlock()

	go doStream(ctx, cancel, boundLLM, llm, history, req.ConversationID, req.Message, modelID, s.convSvc, s.toolSvc, s.Events)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) stopGeneration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ConversationID string `json:"conversationId"`
	}
	json.NewDecoder(r.Body).Decode(&req)
	cancelsMu.Lock()
	if cancel, ok := cancels[req.ConversationID]; ok {
		cancel()
	}
	cancelsMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// bindForgeTools attaches create_tool or update_tool_code to the LLM via WithTools.
func bindForgeTools(
	ctx context.Context,
	conversationID string,
	llm einomodel.BaseChatModel,
	convSvc *service.ConversationService,
	toolSvc *service.ToolService,
	bridge *events.Bridge,
) einomodel.BaseChatModel {
	toolLLM, ok := llm.(einomodel.ToolCallingChatModel)
	if !ok {
		return llm
	}

	conv, _ := convSvc.Get(conversationID)
	isBound := conv != nil && conv.AssetID != nil && conv.AssetType != nil && *conv.AssetType == "tool"

	var tools []*schema.ToolInfo
	if isBound {
		t := forge.NewUpdateToolCodeTool(func(ctx context.Context, code, explanation string) error {
			boundToolID := *conv.AssetID
			toolSvc.SetPendingChange(boundToolID, code, explanation)
			bridge.Emit(events.ForgeCodeUpdated, map[string]any{
				"conversationId": conversationID, "toolId": boundToolID, "summary": explanation,
			})
			return nil
		})
		info, _ := t.Info(ctx)
		tools = append(tools, info)
	} else {
		// Unbound conversations: NO tool calling.
		// AI outputs code in markdown blocks, frontend ForgeCodeBlock handles save.
		// This gives the user control: they see the code, click "Save as Tool".
		return llm
		/* DISABLED: auto-create via tool calling
		t := forge.NewCreateToolTool(func(ctx context.Context, code, explanation string) error {
			parsed := forge.ParseFunction(code)
			if parsed.FuncName == "" {
				return fmt.Errorf("invalid code")
			}
			params := make([]service.ToolParameter, len(parsed.Params))
			for i, p := range parsed.Params {
				params[i] = service.ToolParameter{Name: p.Name, Type: p.Type, Required: p.Required, Default: p.Default}
			}
			displayName := parsed.FuncName
			if parsed.Docstring != "" {
				displayName = parsed.Docstring
			}
			tool := &service.Tool{
				Name: parsed.FuncName, DisplayName: displayName, Description: parsed.Docstring,
				Code: code, Requirements: parsed.Requirements, Parameters: params,
				Category: "other", Status: "draft",
			}
			existing, _ := toolSvc.GetByName(parsed.FuncName)
			if existing != nil {
				tool.ID = existing.ID
			}
			if err := toolSvc.Save(tool); err != nil {
				return err
			}
			convSvc.Bind(conversationID, tool.ID, "tool")
			bridge.Emit(events.ForgeCodeDetected, map[string]any{
				"conversationId": conversationID, "toolId": tool.ID, "funcName": parsed.FuncName,
			})
			return nil
		})
		info, _ := t.Info(ctx)
		tools = append(tools, info)
		*/
	}

	withTools, err := toolLLM.WithTools(tools)
	if err != nil {
		return llm
	}
	return withTools
}

// doStream: Phase 1 = Generate (tool calls), Phase 2 = Stream (text response).
func doStream(
	ctx context.Context,
	cancel context.CancelFunc,
	toolLLM, plainLLM einomodel.BaseChatModel,
	history []*schema.Message,
	conversationID, userMessage, modelID string,
	convSvc *service.ConversationService,
	toolSvc *service.ToolService,
	bridge *events.Bridge,
) {
	defer func() {
		if r := recover(); r != nil {
			bridge.Emit(events.ChatError, map[string]any{
				"conversationId": conversationID,
				"error":          fmt.Sprintf("内部错误：%v", r),
			})
		}
		cancel()
		cancelsMu.Lock()
		delete(cancels, conversationID)
		cancelsMu.Unlock()
	}()

	// Check if this is a bound conversation (has tools attached to LLM)
	conv, _ := convSvc.Get(conversationID)
	hasBoundTool := conv != nil && conv.AssetID != nil && conv.AssetType != nil && *conv.AssetType == "tool"

	// Phase 1: Generate (non-streaming) for tool calls — only for bound conversations
	currentHistory := history
	for iteration := 0; iteration < 3; iteration++ {
		if !hasBoundTool {
			break // Unbound conversations skip straight to streaming
		}

		// Only show "generating" indicator for bound tool conversations
		status := "AI 正在修改代码..."
		if iteration == 0 {
			status = "AI 正在分析..."
		}
		bridge.Emit(events.ForgeCodeStreaming, map[string]any{
			"conversationId": conversationID,
			"event":          "generating",
			"status":         status,
		})

		resp, err := toolLLM.Generate(ctx, currentHistory)
		if err != nil {
			bridge.Emit(events.ChatError, map[string]any{
				"conversationId": conversationID,
				"error":          classifyError(err),
			})
			return
		}

		if len(resp.ToolCalls) > 0 {
			assistantMsg := &schema.Message{
				Role: schema.Assistant, Content: resp.Content, ToolCalls: resp.ToolCalls,
			}
			currentHistory = append(currentHistory, assistantMsg)

			for _, tc := range resp.ToolCalls {
				result := executeToolCall(ctx, tc, conversationID, convSvc, toolSvc, bridge)
				currentHistory = append(currentHistory, schema.ToolMessage(result, tc.ID))
			}
			continue
		}
		break
	}

	// Phase 2: Stream text response
	sr, err := plainLLM.Stream(ctx, currentHistory)
	if err != nil {
		bridge.Emit(events.ChatError, map[string]any{
			"conversationId": conversationID,
			"error":          classifyError(err),
		})
		return
	}
	defer sr.Close()

	var buf strings.Builder
	for {
		chunk, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			content := buf.String()
			if content != "" {
				storage.DB().Exec(
					`INSERT INTO messages (id, conversation_id, role, content, content_type, model_id) VALUES (?, ?, 'assistant', ?, 'text', ?)`,
					uuid.NewString(), conversationID, content, modelID,
				)
			}
			convSvc.TouchUpdatedAt(conversationID)

			// For unbound conversations: detect Python code blocks.
			// DON'T create a tool — just notify frontend with metadata from @-comments.
			// User must click "Save as Tool" to actually create the tool.
			conv, _ := convSvc.Get(conversationID)
			isBound := conv != nil && conv.AssetID != nil && conv.AssetType != nil && *conv.AssetType == "tool"
			if !isBound {
				detected := forge.DetectCodeInResponse(content)
				if detected != nil && detected.FuncName != "" {
					// Send code + metadata (from @display_name, @description, @category comments)
					bridge.Emit(events.ForgeCodeDetected, map[string]any{
						"conversationId": conversationID,
						"funcName":       detected.FuncName,
						"code":           detected.Code,
						"displayName":    detected.DisplayName,
						"description":    detected.Description,
						"category":       detected.Category,
					})
				}
			}

			bridge.Emit(events.ChatDone, map[string]any{
				"conversationId": conversationID, "modelId": modelID,
			})
			convSvc.AutoTitle(ctx, conversationID, userMessage, content)
			return
		}
		if err != nil {
			bridge.Emit(events.ChatError, map[string]any{
				"conversationId": conversationID, "error": classifyError(err),
			})
			return
		}
		if chunk != nil && chunk.Content != "" {
			buf.WriteString(chunk.Content)
			bridge.Emit(events.ChatToken, map[string]any{
				"conversationId": conversationID, "token": chunk.Content,
			})
		}
	}
}

func executeToolCall(
	ctx context.Context,
	tc schema.ToolCall,
	conversationID string,
	convSvc *service.ConversationService,
	toolSvc *service.ToolService,
	bridge *events.Bridge,
) string {
	conv, _ := convSvc.Get(conversationID)
	isBound := conv != nil && conv.AssetID != nil && conv.AssetType != nil && *conv.AssetType == "tool"

	switch tc.Function.Name {
	case "update_tool_code":
		if !isBound {
			return "当前对话未绑定工具"
		}
		t := forge.NewUpdateToolCodeTool(func(ctx context.Context, code, explanation string) error {
			toolSvc.SetPendingChange(*conv.AssetID, code, explanation)
			bridge.Emit(events.ForgeCodeUpdated, map[string]any{
				"conversationId": conversationID, "toolId": *conv.AssetID, "summary": explanation,
			})
			return nil
		})
		result, _ := t.InvokableRun(ctx, tc.Function.Arguments)
		return result

	case "create_tool":
		if isBound {
			return "当前对话已绑定工具，如需新工具请开新对话"
		}
		t := forge.NewCreateToolTool(func(ctx context.Context, code, explanation string) error {
			parsed := forge.ParseFunction(code)
			if parsed.FuncName == "" {
				return fmt.Errorf("invalid code")
			}
			params := make([]service.ToolParameter, len(parsed.Params))
			for i, p := range parsed.Params {
				params[i] = service.ToolParameter{Name: p.Name, Type: p.Type, Required: p.Required, Default: p.Default}
			}
			displayName := parsed.FuncName
			if parsed.Docstring != "" {
				displayName = parsed.Docstring
			}
			tool := &service.Tool{
				Name: parsed.FuncName, DisplayName: displayName, Description: parsed.Docstring,
				Code: code, Requirements: parsed.Requirements, Parameters: params,
				Category: "other", Status: "draft",
			}
			existing, _ := toolSvc.GetByName(parsed.FuncName)
			if existing != nil {
				tool.ID = existing.ID
			}
			if err := toolSvc.Save(tool); err != nil {
				return err
			}
			convSvc.Bind(conversationID, tool.ID, "tool")
			bridge.Emit(events.ForgeCodeDetected, map[string]any{
				"conversationId": conversationID, "toolId": tool.ID, "funcName": parsed.FuncName,
			})
			return nil
		})
		result, _ := t.InvokableRun(ctx, tc.Function.Arguments)
		return result

	default:
		return fmt.Sprintf("unknown tool: %s", tc.Function.Name)
	}
}

func loadHistory(conversationID string) ([]*schema.Message, error) {
	rows, err := storage.DB().Query(`
		SELECT role, content FROM messages
		WHERE conversation_id=? ORDER BY created_at ASC`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []*schema.Message
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			return nil, err
		}
		switch role {
		case "user":
			msgs = append(msgs, schema.UserMessage(content))
		case "assistant":
			msgs = append(msgs, schema.AssistantMessage(content, nil))
		case "system":
			msgs = append(msgs, schema.SystemMessage(content))
		}
	}
	return msgs, rows.Err()
}

func buildToolSummary(toolSvc *service.ToolService) string {
	tools, _ := toolSvc.List("", "")
	if len(tools) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("[用户已有工具]\n")
	for _, t := range tools {
		bi := ""
		if t.Builtin {
			bi = ", 内置"
		}
		sb.WriteString(fmt.Sprintf("- %s (%s, %s%s)\n", t.Name, t.Category, t.Status, bi))
	}
	sb.WriteString(fmt.Sprintf("共 %d 个工具。如果用户需要的功能已有工具可用，优先推荐使用。\n", len(tools)))
	return sb.String()
}

func classifyError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "401") || strings.Contains(msg, "invalid api key"):
		return "API Key 可能已失效，请前往设置检查"
	case strings.Contains(msg, "429") || strings.Contains(msg, "rate limit"):
		return "请求过于频繁，请稍后重试"
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return "连接超时，请检查网络后重试"
	case strings.Contains(msg, "no model configured"):
		return "尚未配置模型，请前往设置页面配置"
	case strings.Contains(msg, "no api key") || strings.Contains(msg, "no key for"):
		return "尚未配置该提供商的 API Key"
	default:
		return "AI 服务暂时不可用：" + err.Error()
	}
}
