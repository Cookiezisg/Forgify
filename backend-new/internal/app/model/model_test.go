// service_test.go — unit tests for model.Service using a fake repository.
// No real DB or logger needed for most tests.
//
// service_test.go — model.Service 的单元测试，使用 fake repository。
// 大多数测试无需真实 DB 或 logger。
package model

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	modeldomain "github.com/sunweilin/forgify/backend/internal/domain/model"
	"github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// fakeRepo is an in-memory modeldomain.Repository for testing.
//
// fakeRepo 是用于测试的内存版 modeldomain.Repository。
type fakeRepo struct {
	rows      map[string]*modeldomain.ModelConfig // keyed by ID
	upsertErr error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{rows: make(map[string]*modeldomain.ModelConfig)}
}

func (r *fakeRepo) GetByScenario(ctx context.Context, scenario string) (*modeldomain.ModelConfig, error) {
	uid, _ := reqctx.GetUserID(ctx)
	for _, m := range r.rows {
		if m.UserID == uid && m.Scenario == scenario {
			cp := *m
			return &cp, nil
		}
	}
	return nil, modeldomain.ErrNotConfigured
}

func (r *fakeRepo) List(ctx context.Context) ([]*modeldomain.ModelConfig, error) {
	uid, _ := reqctx.GetUserID(ctx)
	var out []*modeldomain.ModelConfig
	for _, m := range r.rows {
		if m.UserID == uid {
			cp := *m
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *fakeRepo) Upsert(_ context.Context, m *modeldomain.ModelConfig) error {
	if r.upsertErr != nil {
		return r.upsertErr
	}
	cp := *m
	r.rows[m.ID] = &cp
	return nil
}

// helpers

func newSvc(t *testing.T, repo modeldomain.Repository) *Service {
	t.Helper()
	return NewService(repo, zap.NewNop())
}

func ctxAlice() context.Context {
	return reqctx.SetUserID(context.Background(), "u-alice")
}

// --- NewService ---

func TestNewService_NilLogger_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on nil logger, got none")
		}
	}()
	NewService(newFakeRepo(), nil)
}

// --- Upsert validation ---

func TestUpsert_InvalidScenario(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	_, err := svc.Upsert(ctxAlice(), "nonexistent", UpsertInput{Provider: "openai", ModelID: "gpt-4o"})
	if !errors.Is(err, modeldomain.ErrInvalidScenario) {
		t.Errorf("got %v, want ErrInvalidScenario", err)
	}
}

func TestUpsert_ProviderRequired(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	_, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioChat, UpsertInput{Provider: "  ", ModelID: "gpt-4o"})
	if !errors.Is(err, modeldomain.ErrProviderRequired) {
		t.Errorf("got %v, want ErrProviderRequired", err)
	}
}

func TestUpsert_ModelIDRequired(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	_, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioChat, UpsertInput{Provider: "openai", ModelID: ""})
	if !errors.Is(err, modeldomain.ErrModelIDRequired) {
		t.Errorf("got %v, want ErrModelIDRequired", err)
	}
}

// --- Upsert happy paths ---

func TestUpsert_NewScenario_CreatesRow(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	got, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioChat, UpsertInput{Provider: "openai", ModelID: "gpt-4o"})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got.Provider != "openai" || got.ModelID != "gpt-4o" {
		t.Errorf("wrong fields: %+v", got)
	}
	if got.ID == "" {
		t.Error("ID must not be empty")
	}
}

func TestUpsert_ExistingScenario_PreservesID(t *testing.T) {
	// Second Upsert on the same scenario must reuse the existing row's ID.
	// 同一 scenario 的第二次 Upsert 必须复用已有行的 ID。
	svc := newSvc(t, newFakeRepo())

	first, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioChat, UpsertInput{Provider: "openai", ModelID: "gpt-4o"})
	if err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	second, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioChat, UpsertInput{Provider: "anthropic", ModelID: "claude-3-5-sonnet-latest"})
	if err != nil {
		t.Fatalf("second Upsert: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("ID changed: first=%q second=%q", first.ID, second.ID)
	}
	if second.Provider != "anthropic" {
		t.Errorf("Provider not updated: got %q", second.Provider)
	}
}

func TestUpsert_TrimsWhitespace(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	got, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioChat, UpsertInput{Provider: "  openai  ", ModelID: " gpt-4o "})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if got.Provider != "openai" || got.ModelID != "gpt-4o" {
		t.Errorf("whitespace not trimmed: provider=%q modelID=%q", got.Provider, got.ModelID)
	}
}

// --- List ---

func TestList_Empty(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	rows, err := svc.List(ctxAlice())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}

func TestList_AfterUpsert(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	if _, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioChat, UpsertInput{Provider: "openai", ModelID: "gpt-4o"}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	rows, err := svc.List(ctxAlice())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("got %d rows, want 1", len(rows))
	}
}

// --- PickForChat ---

func TestPickForChat_NotConfigured(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	_, _, err := svc.PickForChat(ctxAlice())
	if !errors.Is(err, modeldomain.ErrNotConfigured) {
		t.Errorf("got %v, want ErrNotConfigured", err)
	}
}

func TestPickForChat_ReturnsConfigured(t *testing.T) {
	svc := newSvc(t, newFakeRepo())
	if _, err := svc.Upsert(ctxAlice(), modeldomain.ScenarioChat, UpsertInput{Provider: "anthropic", ModelID: "claude-3-5-sonnet-latest"}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	provider, modelID, err := svc.PickForChat(ctxAlice())
	if err != nil {
		t.Fatalf("PickForChat: %v", err)
	}
	if provider != "anthropic" || modelID != "claude-3-5-sonnet-latest" {
		t.Errorf("got (%q, %q), want (anthropic, claude-3-5-sonnet-latest)", provider, modelID)
	}
}
