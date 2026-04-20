package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	einomodel "github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/sunweilin/forgify/internal/attachment"
	ctxcompress "github.com/sunweilin/forgify/internal/context"
	"github.com/sunweilin/forgify/internal/events"
	"github.com/sunweilin/forgify/internal/forge"
	"github.com/sunweilin/forgify/internal/model"
	"github.com/sunweilin/forgify/internal/storage"
)

type ChatService struct {
	gateway    *model.ModelGateway
	bridge     *events.Bridge
	convSvc    *ConversationService
	toolSvc    *ToolService
	compressor *ctxcompress.Compressor
	mu         sync.Mutex
	cancels    map[string]context.CancelFunc
}

func NewChatService(gateway *model.ModelGateway, bridge *events.Bridge, convSvc *ConversationService, toolSvc *ToolService) *ChatService {
	return &ChatService{
		gateway:    gateway,
		bridge:     bridge,
		convSvc:    convSvc,
		toolSvc:    toolSvc,
		compressor: ctxcompress.NewCompressor(gateway, bridge),
		cancels:    make(map[string]context.CancelFunc),
	}
}

func (s *ChatService) FullCompact(ctx context.Context, conversationID string) error {
	return s.compressor.FullCompact(ctx, conversationID)
}

type chatTokenPayload struct {
	ConversationID string `json:"conversationId"`
	Token          string `json:"token"`
}
type chatDonePayload struct {
	ConversationID string `json:"conversationId"`
	ModelID        string `json:"modelId,omitempty"`
}
type chatErrorPayload struct {
	ConversationID string `json:"conversationId"`
	Error          string `json:"error"`
}
type notificationPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Level string `json:"level"`
}

func (s *ChatService) SendMessageWithAttachments(ctx context.Context, conversationID, userMessage string, files []*attachment.FileInfo) error {
	var attachSummary string
	if len(files) > 0 {
		var names []string
		for _, f := range files {
			names = append(names, fmt.Sprintf("%s (%s)", f.Name, formatSize(f.Size)))
		}
		attachSummary = "\n📎 " + strings.Join(names, ", ")
	}

	userMsgID := uuid.NewString()
	if _, err := storage.DB().Exec(
		`INSERT INTO messages (id, conversation_id, role, content, content_type) VALUES (?, ?, 'user', ?, 'text')`,
		userMsgID, conversationID, userMessage+attachSummary,
	); err != nil {
		return fmt.Errorf("save user message: %w", err)
	}
	s.convSvc.TouchUpdatedAt(conversationID)

	history, err := s.loadHistory(conversationID)
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}

	if len(files) > 0 {
		history, err = attachment.InjectIntoMessages(history, files)
		if err != nil {
			return fmt.Errorf("inject attachments: %w", err)
		}
	}

	const defaultModelLimit = 128000
	history, compressLevel, _ := s.compressor.MaybeCompress(ctx, conversationID, history, defaultModelLimit)
	if compressLevel == ctxcompress.LevelAuto {
		s.bridge.Emit(events.ChatCompacted, map[string]any{
			"conversationId": conversationID,
			"level":          string(compressLevel),
		})
	}

	llm, modelID, getErr := s.gateway.GetModel(ctx, model.PurposeConversation)
	if getErr != nil {
		var fallbackErr model.ErrUsedFallback
		if errors.As(getErr, &fallbackErr) {
			modelID = fallbackErr.Fallback
			s.bridge.Emit(events.Notification, notificationPayload{
				Title: "已自动切换模型",
				Body:  fmt.Sprintf("主模型不可用，已切换到 %s", fallbackErr.Fallback),
				Level: "info",
			})
		} else {
			s.bridge.Emit(events.ChatError, chatErrorPayload{
				ConversationID: conversationID,
				Error:          classifyError(getErr),
			})
			return getErr
		}
	}

	// Bind forge tools based on conversation state
	boundLLM := s.bindForgeTools(ctx, conversationID, llm)

	streamCtx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[conversationID] = cancel
	s.mu.Unlock()

	// Pass both: boundLLM (with tools, for Generate) and llm (without tools, for Stream)
	go s.doStream(streamCtx, cancel, conversationID, boundLLM, llm, history, modelID, userMessage)
	return nil
}

func formatSize(bytes int64) string {
	const kb = 1024
	const mb = kb * 1024
	switch {
	case bytes >= mb:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.0fKB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func (s *ChatService) StopGeneration(conversationID string) {
	s.mu.Lock()
	cancel, ok := s.cancels[conversationID]
	s.mu.Unlock()
	if ok {
		cancel()
	}
}

// bindForgeTools attaches create_tool or update_tool_code to the LLM.
func (s *ChatService) bindForgeTools(ctx context.Context, conversationID string, llm einomodel.BaseChatModel) einomodel.BaseChatModel {
	toolLLM, ok := llm.(einomodel.ToolCallingChatModel)
	if !ok {
		return llm
	}

	conv, _ := s.convSvc.Get(conversationID)
	isBound := conv != nil && conv.AssetID != nil && conv.AssetType != nil && *conv.AssetType == "tool"

	var forgeTools []einotool.InvokableTool
	if isBound {
		forgeTools = append(forgeTools, forge.NewUpdateToolCodeTool(
			func(ctx context.Context, code, explanation string) error {
				boundToolID := *conv.AssetID
				existingTool, _ := s.toolSvc.Get(boundToolID)
				if existingTool == nil {
					return fmt.Errorf("tool not found")
				}
				summary := explanation
				s.toolSvc.SetPendingChange(boundToolID, code, summary)
				s.bridge.Emit(events.ForgeCodeUpdated, map[string]any{
					"conversationId": conversationID, "toolId": boundToolID, "summary": summary,
				})
				return nil
			},
		))
	} else {
		forgeTools = append(forgeTools, forge.NewCreateToolTool(
			func(ctx context.Context, code, explanation string) error {
				parsed := forge.ParseFunction(code)
				if parsed.FuncName == "" {
					return fmt.Errorf("invalid code: no function found")
				}
				params := make([]ToolParameter, len(parsed.Params))
				for i, p := range parsed.Params {
					params[i] = ToolParameter{Name: p.Name, Type: p.Type, Required: p.Required, Default: p.Default}
				}
				displayName := parsed.FuncName
				if parsed.Docstring != "" {
					displayName = parsed.Docstring
				}
				tool := &Tool{
					Name: parsed.FuncName, DisplayName: displayName, Description: parsed.Docstring,
					Code: code, Requirements: parsed.Requirements, Parameters: params,
					Category: "other", Status: "draft",
				}
				existing, _ := s.toolSvc.GetByName(parsed.FuncName)
				if existing != nil {
					tool.ID = existing.ID
				}
				if err := s.toolSvc.Save(tool); err != nil {
					return err
				}
				s.convSvc.Bind(conversationID, tool.ID, "tool")
				s.bridge.Emit(events.ForgeCodeDetected, map[string]any{
					"conversationId": conversationID, "toolId": tool.ID, "funcName": parsed.FuncName,
				})
				return nil
			},
		))
	}

	var toolInfos []*schema.ToolInfo
	for _, ft := range forgeTools {
		info, err := ft.Info(ctx)
		if err != nil {
			continue
		}
		toolInfos = append(toolInfos, info)
	}
	withTools, err := toolLLM.WithTools(toolInfos)
	if err != nil {
		return llm
	}
	return withTools
}

func (s *ChatService) doStream(
	ctx context.Context,
	cancel context.CancelFunc,
	conversationID string,
	toolLLM einomodel.BaseChatModel, // LLM with forge tools bound (for Generate phase)
	plainLLM einomodel.BaseChatModel, // LLM without tools (for Stream phase)
	history []*schema.Message,
	modelID string,
	userMessage string,
) {
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.cancels, conversationID)
		s.mu.Unlock()
	}()

	// Phase 1: Generate (non-streaming) to handle tool calls.
	// Emit a "generating" event so frontend shows progress indicator.
	currentHistory := history
	for iteration := 0; iteration < 3; iteration++ {
		// Tell frontend that AI is working (tool call phase)
		s.bridge.Emit(events.ForgeCodeStreaming, map[string]any{
			"conversationId": conversationID,
			"event":          "generating",
		})

		resp, err := toolLLM.Generate(ctx, currentHistory)
		if err != nil {
			s.bridge.Emit(events.ChatError, chatErrorPayload{
				ConversationID: conversationID,
				Error:          classifyError(err),
			})
			return
		}

		if len(resp.ToolCalls) > 0 {
			assistantMsg := &schema.Message{
				Role: schema.Assistant, Content: resp.Content, ToolCalls: resp.ToolCalls,
			}
			currentHistory = append(currentHistory, assistantMsg)

			for _, tc := range resp.ToolCalls {
				result := s.executeForgeToolCall(ctx, conversationID, tc)
				currentHistory = append(currentHistory, schema.ToolMessage(result, tc.ID))
			}
			continue
		}

		// No tool calls — break to streaming phase
		break
	}

	// Phase 2: Stream the final text response using plain LLM (no tools).
	sr, err := plainLLM.Stream(ctx, currentHistory)
	if err != nil {
		s.bridge.Emit(events.ChatError, chatErrorPayload{
			ConversationID: conversationID,
			Error:          classifyError(err),
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
			s.convSvc.TouchUpdatedAt(conversationID)
			s.bridge.Emit(events.ChatDone, chatDonePayload{ConversationID: conversationID, ModelID: modelID})
			s.convSvc.AutoTitle(ctx, conversationID, userMessage, content)
			return
		}
		if err != nil {
			s.bridge.Emit(events.ChatError, chatErrorPayload{
				ConversationID: conversationID, Error: classifyError(err),
			})
			return
		}
		if chunk != nil && chunk.Content != "" {
			buf.WriteString(chunk.Content)
			s.bridge.Emit(events.ChatToken, chatTokenPayload{
				ConversationID: conversationID, Token: chunk.Content,
			})
		}
	}
}

// executeForgeToolCall executes a tool call and returns the result.
func (s *ChatService) executeForgeToolCall(ctx context.Context, conversationID string, tc schema.ToolCall) string {
	conv, _ := s.convSvc.Get(conversationID)
	isBound := conv != nil && conv.AssetID != nil && conv.AssetType != nil && *conv.AssetType == "tool"

	switch tc.Function.Name {
	case "update_tool_code":
		if !isBound {
			return "当前对话未绑定工具"
		}
		tool := forge.NewUpdateToolCodeTool(func(ctx context.Context, code, explanation string) error {
			boundToolID := *conv.AssetID
			existingTool, _ := s.toolSvc.Get(boundToolID)
			if existingTool == nil {
				return fmt.Errorf("tool not found")
			}
			s.toolSvc.SetPendingChange(boundToolID, code, explanation)
			s.bridge.Emit(events.ForgeCodeUpdated, map[string]any{
				"conversationId": conversationID, "toolId": boundToolID, "summary": explanation,
			})
			return nil
		})
		result, _ := tool.InvokableRun(ctx, tc.Function.Arguments)
		return result

	case "create_tool":
		if isBound {
			return "当前对话已绑定工具，如需创建新工具请开新对话"
		}
		tool := forge.NewCreateToolTool(func(ctx context.Context, code, explanation string) error {
			parsed := forge.ParseFunction(code)
			if parsed.FuncName == "" {
				return fmt.Errorf("invalid code")
			}
			params := make([]ToolParameter, len(parsed.Params))
			for i, p := range parsed.Params {
				params[i] = ToolParameter{Name: p.Name, Type: p.Type, Required: p.Required, Default: p.Default}
			}
			displayName := parsed.FuncName
			if parsed.Docstring != "" {
				displayName = parsed.Docstring
			}
			t := &Tool{
				Name: parsed.FuncName, DisplayName: displayName, Description: parsed.Docstring,
				Code: code, Requirements: parsed.Requirements, Parameters: params,
				Category: "other", Status: "draft",
			}
			existing, _ := s.toolSvc.GetByName(parsed.FuncName)
			if existing != nil {
				t.ID = existing.ID
			}
			if err := s.toolSvc.Save(t); err != nil {
				return err
			}
			s.convSvc.Bind(conversationID, t.ID, "tool")
			s.bridge.Emit(events.ForgeCodeDetected, map[string]any{
				"conversationId": conversationID, "toolId": t.ID, "funcName": parsed.FuncName,
			})
			return nil
		})
		result, _ := tool.InvokableRun(ctx, tc.Function.Arguments)
		return result

	default:
		return fmt.Sprintf("unknown tool: %s", tc.Function.Name)
	}
}

func (s *ChatService) loadHistory(conversationID string) ([]*schema.Message, error) {
	toolSummary := s.buildToolSummary()

	rows, err := storage.DB().Query(`
		SELECT role, content FROM messages
		WHERE conversation_id=? ORDER BY created_at ASC`, conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	msgs := []*schema.Message{
		schema.SystemMessage(forge.ForgeSystemPrompt),
	}
	if toolSummary != "" {
		msgs = append(msgs, schema.SystemMessage(toolSummary))
	}

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

func (s *ChatService) buildToolSummary() string {
	tools, _ := s.toolSvc.List("", "")
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
	sb.WriteString(fmt.Sprintf("共 %d 个工具。如果用户需要的功能已有工具可用，优先推荐使用已有工具。\n", len(tools)))
	return sb.String()
}

func classifyError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "401") || strings.Contains(msg, "invalid api key") || strings.Contains(msg, "authentication"):
		return "API Key 可能已失效，请前往设置检查"
	case strings.Contains(msg, "429") || strings.Contains(msg, "rate limit"):
		return "请求过于频繁，请稍后重试"
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return "连接超时，请检查网络后重试"
	case strings.Contains(msg, "no model configured"):
		return "尚未配置模型，请前往设置页面配置"
	case strings.Contains(msg, "no api key") || strings.Contains(msg, "no key for"):
		return "尚未配置该提供商的 API Key，请前往设置检查"
	default:
		return "AI 服务暂时不可用：" + err.Error()
	}
}
