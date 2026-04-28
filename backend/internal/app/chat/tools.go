// tools.go — Parallel tool call execution within the ReAct loop.
// runTools fans out to goroutines, one per tool call, and collects results
// in original-call order so block seq is deterministic.
//
// tools.go — ReAct loop 内的并行工具调用执行。
// runTools 为每个工具调用启动 goroutine，按原始调用顺序收集结果，
// 保证 block seq 确定。
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	agentapp "github.com/sunweilin/forgify/backend/internal/app/agent"
	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventsdomain "github.com/sunweilin/forgify/backend/internal/domain/events"
)

// runTools executes all tool calls in parallel, publishing SSE events for each,
// and returns tool_result blocks ordered by the original call index.
//
// runTools 并行执行所有工具调用，为每个推送 SSE 事件，
// 按原始调用 index 排序返回 tool_result blocks。
func (s *Service) runTools(
	ctx context.Context,
	calls []chatdomain.ToolCallData,
	convID, msgID string,
) []chatdomain.Block {
	type result struct {
		idx   int
		block chatdomain.Block
	}
	ch := make(chan result, len(calls))
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc chatdomain.ToolCallData) {
			defer wg.Done()
			ch <- result{idx: idx, block: s.runOneTool(ctx, tc, convID, msgID, idx)}
		}(i, call)
	}

	wg.Wait()
	close(ch)

	// Restore original order so block seq is deterministic.
	// 还原原始顺序，保证 block seq 确定。
	blocks := make([]chatdomain.Block, len(calls))
	for r := range ch {
		blocks[r.idx] = r.block
	}
	return blocks
}

// runOneTool executes a single tool call, publishes SSE, and returns the
// tool_result block. Never returns an error — failures become ok=false results.
//
// runOneTool 执行单个工具调用，推送 SSE，返回 tool_result block。
// 永不返回 error——失败以 ok=false 结果呈现。
func (s *Service) runOneTool(
	ctx context.Context,
	tc chatdomain.ToolCallData,
	convID, msgID string,
	seq int,
) chatdomain.Block {
	argsJSON, _ := json.Marshal(tc.Arguments)

	s.bridge.Publish(ctx, convID, eventsdomain.ChatToolCall{
		ConversationID: convID,
		MessageID:      msgID,
		ToolCallID:     tc.ID,
		ToolName:       tc.Name,
		ToolInput:      string(argsJSON),
		Summary:        tc.Summary,
	})

	toolCtx := agentapp.WithMessageID(ctx, msgID)
	toolCtx = agentapp.WithToolCallID(toolCtx, tc.ID)
	output, ok := s.executeTool(toolCtx, tc.Name, string(argsJSON))

	s.bridge.Publish(ctx, convID, eventsdomain.ChatToolResult{
		ConversationID: convID,
		ToolCallID:     tc.ID,
		Result:         output,
		OK:             ok,
	})

	d, _ := json.Marshal(chatdomain.ToolResultData{
		ToolCallID: tc.ID,
		OK:         ok,
		Result:     output,
	})
	return chatdomain.Block{
		ID:        newBlockID(),
		Seq:       seq,
		Type:      chatdomain.BlockTypeToolResult,
		Data:      string(d),
		CreatedAt: time.Now().UTC(),
	}
}

// executeTool finds the named tool and calls Execute.
// Returns (result, true) on success, (errorMessage, false) on failure.
//
// executeTool 查找指定工具并调用 Execute。
// 成功返回 (result, true)，失败返回 (errorMessage, false)。
func (s *Service) executeTool(ctx context.Context, name, argsJSON string) (string, bool) {
	for _, t := range s.tools {
		if t.Name() != name {
			continue
		}
		output, err := t.Execute(ctx, argsJSON)
		if err != nil {
			s.log.Warn("tool execute failed",
				zap.String("tool", name), zap.Error(err))
			if output != "" {
				return output, false
			}
			return err.Error(), false
		}
		return output, true
	}
	return fmt.Sprintf("tool %q not found", name), false
}
