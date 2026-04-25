// Package model is the domain layer for LLM model strategy management.
// It records which (provider, modelID) the user has chosen for each scenario
// and exposes two contracts:
//
//   - Repository  — storage port (implemented by infra/store/model)
//   - ModelPicker — cross-domain consumer port (implemented by app/model)
//
// Naming convention: all three model packages (domain / app / store) declare
// `package model`. External callers alias by role at import:
//
//	modeldomain "…/internal/domain/model"
//	modelapp    "…/internal/app/model"
//	modelstore  "…/internal/infra/store/model"
//
// Package model 是 LLM 模型策略管理的 domain 层。记录用户为每个 scenario
// 选定的 (provider, modelID)，对外暴露两个契约：
//
//   - Repository   — 存储 port（由 infra/store/model 实现）
//   - ModelPicker  — 跨 domain 消费 port（由 app/model 实现）
package model

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ModelConfig records the user's chosen (provider, modelID) for one scenario.
// At most one active row exists per (user_id, scenario) pair — enforced by a
// partial UNIQUE index in schema_extras.go.
//
// ModelConfig 记录用户为某一 scenario 选定的 (provider, modelID)。
// 每个 (user_id, scenario) 对最多存在一条活跃行——由 schema_extras.go
// 的 partial UNIQUE 索引保证。
type ModelConfig struct {
	ID        string         `gorm:"primaryKey;type:text" json:"id"`
	UserID    string         `gorm:"not null;type:text;uniqueIndex:idx_mc_user_scenario,priority:1" json:"-"`
	Scenario  string         `gorm:"not null;type:text;uniqueIndex:idx_mc_user_scenario,priority:2" json:"scenario"`
	Provider  string         `gorm:"not null;type:text" json:"provider"`
	ModelID   string         `gorm:"not null;type:text" json:"modelId"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName locks the DB table to "model_configs".
//
// TableName 把表名锁定为 "model_configs"。
func (ModelConfig) TableName() string { return "model_configs" }

// Scenario constants. Each constant maps to one row in model_configs.
// New scenarios are added here as later Phases introduce them.
//
// Scenario 常量。每个常量对应 model_configs 里的一类行。
// 新 scenario 随后续 Phase 推进在此处追加。
const (
	ScenarioChat = "chat" // main user conversation / 用户主对话
	// Phase 4+: ScenarioWorkflowLLM = "workflow_llm"
	// Phase 5+: ScenarioEmbedding   = "embedding"
	// Phase 5+: ScenarioIntent      = "intent"
)

// IsValidScenario reports whether s is a recognised scenario name.
// Validation lives in the app layer (not DB CHECK) so new scenarios can be
// added without a schema migration.
//
// IsValidScenario 报告 s 是否是已知的 scenario 名称。
// 校验放在 app 层而非 DB CHECK，便于新增 scenario 时不做 schema 迁移。
func IsValidScenario(s string) bool {
	switch s {
	case ScenarioChat:
		return true
	default:
		return false
	}
}

// ListScenarios returns all currently recognised scenario names.
//
// ListScenarios 返回当前所有已知 scenario 名称。
func ListScenarios() []string {
	return []string{ScenarioChat}
}

// Sentinel errors. Mapped to HTTP responses by
// transport/httpapi/response/errmap.go.
//
// Sentinel 错误。由 transport/httpapi/response/errmap.go 映射到 HTTP 响应。
var (
	// ErrNotConfigured: scenario has no active config (user never set it).
	// ErrNotConfigured：该 scenario 无活跃配置（用户从未设置过）。
	ErrNotConfigured = errors.New("model: not configured for scenario")

	// ErrInvalidScenario: scenario name not in the supported whitelist.
	// ErrInvalidScenario：scenario 名称不在支持的白名单内。
	ErrInvalidScenario = errors.New("model: invalid scenario")

	// ErrProviderRequired: PUT body is missing a non-empty provider.
	// ErrProviderRequired：PUT body 缺少非空的 provider。
	ErrProviderRequired = errors.New("model: provider is required")

	// ErrModelIDRequired: PUT body is missing a non-empty modelId.
	// ErrModelIDRequired：PUT body 缺少非空的 modelId。
	ErrModelIDRequired = errors.New("model: model id is required")
)

// Repository is the storage contract for ModelConfig. Implementations filter
// by the userID in ctx — callers MUST ensure InjectUserID middleware has run.
//
// Implemented by: infra/store/model.Store
// Consumer:       app/model.Service (only)
//
// Repository 是 ModelConfig 的存储契约。实现按 ctx 中的 userID 过滤——
// 调用方必须保证 InjectUserID 中间件已运行。
//
// 实现：infra/store/model.Store
// 消费：仅 app/model.Service
type Repository interface {
	// GetByScenario fetches the active config for (current user, scenario).
	// Returns ErrNotConfigured if none exists.
	//
	// GetByScenario 返回 (当前用户, scenario) 的活跃配置；无则返 ErrNotConfigured。
	GetByScenario(ctx context.Context, scenario string) (*ModelConfig, error)

	// List returns all active configs for the current user, ordered by scenario.
	// No pagination — Phase 2 has at most 1 entry; future phases ≤ ~6.
	//
	// List 返回当前用户所有活跃配置，按 scenario 排序。
	// 不分页——Phase 2 最多 1 条；未来各 Phase 加起来 ≤ ~6 条。
	List(ctx context.Context) ([]*ModelConfig, error)

	// Upsert creates or updates the config for (user_id, scenario). Caller must
	// have set m.UserID and m.Scenario before calling.
	//
	// Upsert 按 (user_id, scenario) 创建或更新配置。
	// 调用方须先填 m.UserID 和 m.Scenario。
	Upsert(ctx context.Context, m *ModelConfig) error
}

// ModelPicker is the cross-domain interface. Services that need to call an
// LLM (chat, workflow nodes, embedding) use this to obtain the user's chosen
// (provider, modelID) for their scenario — they never touch Repository or
// ModelConfig directly.
//
// Implemented by: app/model.Service
//
// ModelPicker 是跨 domain 接口。需要调 LLM 的 service（chat、workflow 节点、
// embedding）通过本接口获取用户为其 scenario 选定的 (provider, modelID)，
// 不直接接触 Repository 或 ModelConfig。
//
// 由 app/model.Service 实现。
type ModelPicker interface {
	// PickForChat returns the (provider, modelID) for the user's main chat
	// scenario. Returns ErrNotConfigured if the user has never set it.
	//
	// PickForChat 返回当前用户主对话 scenario 的 (provider, modelID)。
	// 用户从未配置过则返回 ErrNotConfigured。
	PickForChat(ctx context.Context) (provider, modelID string, err error)
}
