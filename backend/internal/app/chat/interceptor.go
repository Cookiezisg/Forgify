package chat

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	"github.com/sunweilin/forgify/backend/internal/domain/events"
)

// invokable is the execution subset of tool.InvokableTool needed by
// toolInterceptor. Defined locally to avoid importing the full interface.
//
// invokable 是 toolInterceptor 需要的 tool.InvokableTool 执行子集，
// 本地定义避免引入完整接口。
type invokable interface {
	InvokableRun(ctx context.Context, argsJSON string, opts ...tool.Option) (string, error)
}

// Summarizable is an optional interface that a tool can implement to declare
// its own human-readable core information for the SSE summary field.
// Tools that don't implement it fall back to extractFallbackSummary.
//
// Summarizable 是可选接口，tool 实现后可声明自己的核心信息，
// 用于 SSE 事件的 summary 字段。未实现时退回 extractFallbackSummary。
type Summarizable interface {
	CoreInfo(argsJSON string) string
}

// toolInterceptor wraps a tool.BaseTool to publish ChatToolCall /
// ChatToolResult SSE events and append messages to a shared buffer before
// and after each tool execution. It never writes to the DB directly —
// the buffer is flushed by processTask after the full turn completes,
// which guarantees correct chronological order in the messages table.
//
// toolInterceptor 包装 tool.BaseTool，在每次 tool 执行前后发布 SSE 事件
// 并追加消息到共享缓冲区。不直接写 DB——缓冲区由 processTask 在整个 turn
// 完成后统一刷入，确保 messages 表的时序正确。
type toolInterceptor struct {
	inner  tool.BaseTool
	convID string
	msgID  string // assistant message ID for this turn; used for SSE correlation
	uid    string
	bridge events.Bridge
	buf    *[]*chatdomain.Message
	log    *zap.Logger
}

// Info delegates to the wrapped tool.
//
// Info 委托给被包装的 tool。
func (t *toolInterceptor) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return t.inner.Info(ctx)
}

// InvokableRun intercepts the tool call to (1) publish ChatToolCall SSE,
// (2) buffer the assistant tool-call message, (3) execute the real tool,
// (4) publish ChatToolResult SSE, and (5) buffer the tool-result message.
// The actual output is returned unchanged so the agent loop is unaffected.
//
// InvokableRun 拦截 tool 调用，依次：(1) 发 ChatToolCall SSE，
// (2) 缓冲 assistant tool-call 消息，(3) 执行真实 tool，
// (4) 发 ChatToolResult SSE，(5) 缓冲 tool-result 消息。
// 实际输出原样返回，不影响 agent 循环。
func (t *toolInterceptor) InvokableRun(ctx context.Context, argsJSON string, opts ...tool.Option) (string, error) {
	info, _ := t.inner.Info(ctx)
	toolName := ""
	if info != nil {
		toolName = info.Name
	}

	summary := ""
	if s, ok := t.inner.(Summarizable); ok {
		summary = s.CoreInfo(argsJSON)
	} else {
		summary = extractFallbackSummary(argsJSON)
	}

	toolCallID := newToolCallID()

	// Publish before execution so the frontend shows the step as "running".
	// 先发 SSE，前端能立即渲染 running 状态。
	t.bridge.Publish(ctx, t.convID, events.ChatToolCall{
		ConversationID: t.convID,
		MessageID:      t.msgID,
		ToolCallID:     toolCallID,
		ToolName:       toolName,
		ToolInput:      argsJSON,
		Summary:        summary,
	})

	// Buffer assistant tool-call message; no DB write yet.
	// 缓冲 assistant tool-call 消息，暂不写 DB。
	tcJSON, _ := json.Marshal([]schema.ToolCall{{
		ID:       toolCallID,
		Function: schema.FunctionCall{Name: toolName, Arguments: argsJSON},
	}})
	*t.buf = append(*t.buf, &chatdomain.Message{
		ID:             newMsgID(),
		ConversationID: t.convID,
		UserID:         t.uid,
		Role:           chatdomain.RoleAssistant,
		Content:        "",
		ToolCalls:      string(tcJSON),
		Status:         chatdomain.StatusCompleted,
	})

	inv, ok := t.inner.(invokable)
	if !ok {
		return "", fmt.Errorf("tool %s does not implement InvokableRun", toolName)
	}
	output, err := inv.InvokableRun(ctx, argsJSON, opts...)

	result := output
	execOK := err == nil
	if !execOK && output == "" {
		result = err.Error()
	}

	// Publish after execution so the frontend marks the step done.
	// 执行后发 SSE，前端将步骤标为完成。
	t.bridge.Publish(ctx, t.convID, events.ChatToolResult{
		ConversationID: t.convID,
		ToolCallID:     toolCallID,
		Result:         result,
		OK:             execOK,
	})

	// Buffer tool-result message; no DB write yet.
	// 缓冲 tool-result 消息，暂不写 DB。
	*t.buf = append(*t.buf, &chatdomain.Message{
		ID:             newMsgID(),
		ConversationID: t.convID,
		UserID:         t.uid,
		Role:           chatdomain.RoleTool,
		Content:        result,
		ToolCallID:     toolCallID,
		Status:         chatdomain.StatusCompleted,
	})

	return output, err
}

// wrapTools wraps every tool with a toolInterceptor that shares the same
// message buffer. liveTools are passed to the ReAct agent in processTask.
//
// wrapTools 为每个 tool 套上共享同一消息缓冲区的 toolInterceptor，
// 结果传给 processTask 中的 ReAct agent。
func (s *Service) wrapTools(tools []tool.BaseTool, convID, msgID, uid string, buf *[]*chatdomain.Message) []tool.BaseTool {
	if len(tools) == 0 {
		return nil
	}
	wrapped := make([]tool.BaseTool, len(tools))
	for i, t := range tools {
		wrapped[i] = &toolInterceptor{
			inner:  t,
			convID: convID,
			msgID:  msgID,
			uid:    uid,
			bridge: s.bridge,
			buf:    buf,
			log:    s.log,
		}
	}
	return wrapped
}

// extractFallbackSummary extracts a human-readable value by scanning for
// well-known field names. Used for third-party tools (e.g. DuckDuckGo)
// that don't implement Summarizable.
//
// extractFallbackSummary 扫描常见字段名提取可读值，用于未实现 Summarizable
// 的第三方 tool（如 DuckDuckGo）。
func extractFallbackSummary(argsJSON string) string {
	var args map[string]any
	if json.Unmarshal([]byte(argsJSON), &args) != nil {
		return ""
	}
	for _, key := range []string{"query", "url", "path", "command", "name", "tool_id"} {
		if v, ok := args[key]; ok {
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}
