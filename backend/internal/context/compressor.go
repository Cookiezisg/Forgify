package context

import (
	gocontext "context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/sunweilin/forgify/internal/events"
	"github.com/sunweilin/forgify/internal/model"
	"github.com/sunweilin/forgify/internal/storage"
)

const (
	thresholdMicro = 0.75
	thresholdAuto  = 0.88
	keepRecent     = 20
)

// CompressLevel indicates what compression was applied.
type CompressLevel string

const (
	LevelNone  CompressLevel = ""
	LevelMicro CompressLevel = "micro"
	LevelAuto  CompressLevel = "auto"
)

// Compressor handles context window management for conversations.
type Compressor struct {
	gateway *model.ModelGateway
	bridge  *events.Bridge
}

// NewCompressor creates a new context compressor.
func NewCompressor(gateway *model.ModelGateway, bridge *events.Bridge) *Compressor {
	return &Compressor{gateway: gateway, bridge: bridge}
}

// MaybeCompress checks the token usage ratio and applies compression if needed.
// Called before each LLM invocation.
func (c *Compressor) MaybeCompress(
	ctx gocontext.Context,
	convID string,
	messages []*schema.Message,
	modelLimit int,
) ([]*schema.Message, CompressLevel, error) {
	if modelLimit <= 0 {
		return messages, LevelNone, nil
	}

	usage := EstimateTokens(messages)
	ratio := float64(usage) / float64(modelLimit)

	switch {
	case ratio >= thresholdAuto:
		compressed, err := c.autoCompact(ctx, convID, messages)
		if err != nil {
			// Degrade gracefully — try micro, then return original
			return c.microCompact(messages), LevelMicro, nil
		}
		return compressed, LevelAuto, nil
	case ratio >= thresholdMicro:
		return c.microCompact(messages), LevelMicro, nil
	default:
		return messages, LevelNone, nil
	}
}

// microCompact performs code-level trimming without calling any LLM.
// - Removes duplicate canvas state injections (keeps only the latest)
// - Truncates messages longer than 4000 characters
func (c *Compressor) microCompact(messages []*schema.Message) []*schema.Message {
	result := make([]*schema.Message, 0, len(messages))
	seenCanvasUpdate := false

	// Walk backwards so we keep the latest canvas state
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]

		// Only keep the most recent canvas state injection
		if strings.HasPrefix(m.Content, "[当前工作流状态") {
			if seenCanvasUpdate {
				continue
			}
			seenCanvasUpdate = true
		}

		// Truncate very long messages
		runes := []rune(m.Content)
		if len(runes) > 4000 {
			truncated := &schema.Message{
				Role:    m.Role,
				Content: string(runes[:2000]) + "\n...[内容过长已截断]...\n" + string(runes[len(runes)-500:]),
			}
			result = append(result, truncated)
		} else {
			result = append(result, m)
		}
	}

	// Reverse to restore chronological order
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// autoCompact uses a cheap LLM to summarize early conversation history.
// Preserves the most recent `keepRecent` messages verbatim.
func (c *Compressor) autoCompact(
	ctx gocontext.Context, convID string, messages []*schema.Message,
) ([]*schema.Message, error) {
	if len(messages) <= keepRecent {
		return messages, nil
	}

	toSummarize := messages[:len(messages)-keepRecent]
	recent := messages[len(messages)-keepRecent:]

	// Build transcript for summary
	var sb strings.Builder
	for _, m := range toSummarize {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", m.Role, m.Content))
	}

	m, _, err := c.gateway.GetModel(ctx, model.PurposeCheap)
	if err != nil {
		return nil, err
	}

	compactCtx, cancel := gocontext.WithTimeout(gocontext.Background(), 2*time.Minute)
	defer cancel()

	resp, err := m.Generate(compactCtx, []*schema.Message{
		schema.SystemMessage(`你是一个对话摘要助手。请对以下对话历史做精炼摘要，必须包含：
1. 用户的核心目标
2. 已做出的关键决策
3. 当前进展状态
4. 重要约束条件

用"[对话摘要]"开头，500字以内。`),
		schema.UserMessage(sb.String()),
	})
	if err != nil {
		return nil, err
	}

	summaryMsg := &schema.Message{
		Role:    schema.System,
		Content: resp.Content,
	}

	// Persist the summary message
	saveSummary(convID, resp.Content)

	return append([]*schema.Message{summaryMsg}, recent...), nil
}

// FullCompact performs a user-triggered full conversation compression.
// Replaces all messages with a structured summary.
func (c *Compressor) FullCompact(ctx gocontext.Context, convID string) error {
	// Load all messages
	rows, err := storage.DB().Query(`
		SELECT role, content FROM messages
		WHERE conversation_id = ? ORDER BY created_at ASC`, convID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var sb strings.Builder
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			return fmt.Errorf("scan message: %w", err)
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, content))
	}

	if sb.Len() == 0 {
		return nil // nothing to compact
	}

	fullCtx, cancel := gocontext.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	llm, _, err := c.gateway.GetModel(fullCtx, model.PurposeConversation)
	if err != nil {
		return err
	}

	resp, err := llm.Generate(fullCtx, []*schema.Message{
		schema.SystemMessage(`请对以下完整对话做全面结构化摘要，包含：
- 用户目标
- 所有关键决策（含时间顺序）
- 已创建的工具和工作流
- 当前状态和下一步

用"[完整对话摘要]"开头，1000字以内。`),
		schema.UserMessage(sb.String()),
	})
	if err != nil {
		return err
	}

	return replaceWithSummary(convID, resp.Content)
}

func saveSummary(convID, content string) {
	storage.DB().Exec(`
		INSERT INTO messages (id, conversation_id, role, content, content_type)
		VALUES (?, ?, 'system', ?, 'summary')
	`, uuid.NewString(), convID, content)
}

func replaceWithSummary(convID, content string) error {
	tx, err := storage.DB().Begin()
	if err != nil {
		return err
	}
	tx.Exec("DELETE FROM messages WHERE conversation_id=?", convID)
	tx.Exec(`
		INSERT INTO messages (id, conversation_id, role, content, content_type)
		VALUES (?, ?, 'system', ?, 'summary')
	`, uuid.NewString(), convID, content)
	return tx.Commit()
}
