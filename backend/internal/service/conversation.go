package service

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"
	"github.com/sunweilin/forgify/internal/events"
	"github.com/sunweilin/forgify/internal/model"
	"github.com/sunweilin/forgify/internal/storage"
)

type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	AssetID   *string   `json:"assetId"`
	AssetType *string   `json:"assetType"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ConversationService struct {
	gateway *model.ModelGateway
	bridge  *events.Bridge
}

func NewConversationService(gateway *model.ModelGateway, bridge *events.Bridge) *ConversationService {
	return &ConversationService{gateway: gateway, bridge: bridge}
}

func (s *ConversationService) Create(title string) (*Conversation, error) {
	if title == "" {
		title = "新对话"
	}
	id := uuid.NewString()
	if _, err := storage.DB().Exec(
		`INSERT INTO conversations (id, title) VALUES (?, ?)`, id, title,
	); err != nil {
		return nil, err
	}
	return s.Get(id)
}

func (s *ConversationService) Get(id string) (*Conversation, error) {
	convs, err := s.scan(
		`SELECT id, title, asset_id, asset_type, status, created_at, updated_at
		 FROM conversations WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(convs) == 0 {
		return nil, nil
	}
	return convs[0], nil
}

func (s *ConversationService) List() ([]*Conversation, error) {
	return s.scan(`
		SELECT id, title, asset_id, asset_type, status, created_at, updated_at
		FROM conversations
		WHERE status = 'active'
		ORDER BY updated_at DESC
		LIMIT 200
	`)
}

func (s *ConversationService) ListArchived() ([]*Conversation, error) {
	return s.scan(`
		SELECT id, title, asset_id, asset_type, status, created_at, updated_at
		FROM conversations
		WHERE status = 'archived'
		ORDER BY updated_at DESC
		LIMIT 100
	`)
}

func (s *ConversationService) Rename(id, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	_, err := storage.DB().Exec(
		`UPDATE conversations SET title=?, updated_at=datetime('now') WHERE id=?`,
		title, id)
	return err
}

func (s *ConversationService) Archive(id string) error {
	_, err := storage.DB().Exec(
		`UPDATE conversations SET status='archived', updated_at=datetime('now') WHERE id=?`, id)
	return err
}

func (s *ConversationService) Restore(id string) error {
	_, err := storage.DB().Exec(
		`UPDATE conversations SET status='active', updated_at=datetime('now') WHERE id=?`, id)
	return err
}

func (s *ConversationService) Delete(id string) error {
	_, err := storage.DB().Exec(`DELETE FROM conversations WHERE id=?`, id)
	return err
}

func (s *ConversationService) Search(query string) ([]*Conversation, error) {
	q := "%" + query + "%"
	return s.scan(`
		SELECT DISTINCT c.id, c.title, c.asset_id, c.asset_type, c.status, c.created_at, c.updated_at
		FROM conversations c
		LEFT JOIN messages m ON m.conversation_id = c.id
		WHERE c.status = 'active'
		  AND (c.title LIKE ? OR m.content LIKE ?)
		ORDER BY c.updated_at DESC
		LIMIT 100
	`, q, q)
}

func (s *ConversationService) Bind(id, assetID, assetType string) error {
	var aID, aType any
	if assetID != "" {
		aID = assetID
	}
	if assetType != "" {
		aType = assetType
	}
	_, err := storage.DB().Exec(
		`UPDATE conversations SET asset_id=?, asset_type=?, updated_at=datetime('now') WHERE id=?`,
		aID, aType, id)
	if err != nil {
		return err
	}
	s.bridge.Emit(events.ChatBound, map[string]any{
		"conversationId": id,
		"assetId":        assetID,
		"assetType":      assetType,
	})
	return nil
}

func (s *ConversationService) Unbind(id string) error {
	return s.Bind(id, "", "")
}

func (s *ConversationService) TouchUpdatedAt(id string) {
	storage.DB().Exec(`UPDATE conversations SET updated_at=datetime('now') WHERE id=?`, id)
}

// AutoTitle generates a conversation title asynchronously after the first AI response.
// It checks if the conversation still has the default title before proceeding.
func (s *ConversationService) AutoTitle(ctx context.Context, convID, userMsg, assistantMsg string) {
	// Check current title — only auto-title if still default
	var currentTitle string
	err := storage.DB().QueryRow(
		`SELECT title FROM conversations WHERE id=?`, convID).Scan(&currentTitle)
	if err != nil || currentTitle != "新对话" {
		return
	}

	go func() {
		m, _, err := s.gateway.GetModel(ctx, model.PurposeCheap)
		if err != nil || m == nil {
			return
		}

		firstExchange := "用户：" + userMsg + "\nAI：" + assistantMsg
		if len([]rune(firstExchange)) > 2000 {
			firstExchange = string([]rune(firstExchange)[:2000])
		}

		resp, err := m.Generate(context.Background(), []*schema.Message{
			schema.UserMessage("根据以下对话内容，生成一个简洁的标题（最多15个字，不加引号，不加标点）：\n\n" + firstExchange),
		})
		if err != nil {
			return
		}
		title := strings.TrimSpace(resp.Content)
		// Remove surrounding quotes if present
		title = strings.Trim(title, "\"'\u201c\u201d\u2018\u2019\u300c\u300d")
		if len([]rune(title)) > 15 {
			title = string([]rune(title)[:15])
		}
		if title == "" {
			return
		}
		if err := s.Rename(convID, title); err != nil {
			return
		}
		s.bridge.Emit(events.ChatTitleUpdated, map[string]any{
			"conversationId": convID,
			"title":          title,
		})
	}()
}

func (s *ConversationService) scan(query string, args ...any) ([]*Conversation, error) {
	rows, err := storage.DB().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convs []*Conversation
	for rows.Next() {
		c := &Conversation{}
		var assetID, assetType sql.NullString
		var created, updated string
		if err := rows.Scan(&c.ID, &c.Title, &assetID, &assetType,
			&c.Status, &created, &updated); err != nil {
			return nil, err
		}
		if assetID.Valid {
			c.AssetID = &assetID.String
		}
		if assetType.Valid {
			c.AssetType = &assetType.String
		}
		c.CreatedAt, _ = time.Parse(time.DateTime, created)
		c.UpdatedAt, _ = time.Parse(time.DateTime, updated)
		convs = append(convs, c)
	}
	if convs == nil {
		convs = []*Conversation{}
	}
	return convs, rows.Err()
}
