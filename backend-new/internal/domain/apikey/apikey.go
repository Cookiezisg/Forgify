// Package apikey is the domain layer for LLM credential management.
// It defines the APIKey entity, its value objects (Credentials, ListFilter),
// enumeration constants, sentinel errors, and the two contracts this domain
// exposes to the outside world:
//
//   - Repository   — storage port (implemented by infra/store/apikey)
//   - KeyProvider  — cross-domain consumer port (implemented by app/apikey)
//
// Naming convention: all three apikey packages (domain / app / store)
// declare `package apikey`. External callers alias by role at import:
//
//	apikeydomain "…/internal/domain/apikey"
//	apikeyapp    "…/internal/app/apikey"
//	apikeystore  "…/internal/infra/store/apikey"
//
// Package apikey 是 LLM 凭证管理的 domain 层。定义 APIKey 实体、配套
// 值对象（Credentials、ListFilter）、枚举常量、sentinel 错误，以及
// 本 domain 向外暴露的两个契约：
//
//   - Repository     ——存储 port（由 infra/store/apikey 实现）
//   - KeyProvider    ——跨 domain 消费 port（由 app/apikey 实现）
//
// 命名约定：三个 apikey 包（domain / app / store）都声明 `package apikey`。
// 外部调用方 import 时按角色起别名（见上）。
package apikey

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// APIKey is a user's credential for one LLM provider. KeyEncrypted carries
// the ciphertext (v1:...); KeyMasked is a display string like
// "sk-proj...abc4".
//
// APIKey 是一个用户在某 provider 下的凭证。KeyEncrypted 存密文（v1:...）；
// KeyMasked 是展示字符串如 "sk-proj...abc4"。
type APIKey struct {
	ID           string         `gorm:"primaryKey;type:text" json:"id"`
	UserID       string         `gorm:"not null;index:idx_api_keys_user_id;type:text" json:"userId"`
	Provider     string         `gorm:"not null;index:idx_api_keys_user_provider,priority:2;type:text" json:"provider"`
	DisplayName  string         `gorm:"not null;type:text;default:''" json:"displayName"`
	KeyEncrypted string         `gorm:"not null;type:text" json:"-"`
	KeyMasked    string         `gorm:"not null;type:text" json:"keyMasked"`
	BaseURL      string         `gorm:"type:text;default:''" json:"baseUrl"`
	APIFormat    string         `gorm:"type:text;default:''" json:"apiFormat"`
	TestStatus   string         `gorm:"type:text;default:'pending'" json:"testStatus"`
	TestError    string         `gorm:"type:text;default:''" json:"testError"`
	LastTestedAt *time.Time     `json:"lastTestedAt"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName locks the DB table to "api_keys" regardless of GORM's default
// pluralization.
//
// TableName 把表名锁定为 "api_keys"，不随 GORM 默认复数化。
func (APIKey) TableName() string { return "api_keys" }

// TestStatus records the OUTCOME of the most recent connectivity test —
// this is a snapshot field, NOT a streaming state machine. Tests are
// synchronous blocking calls that write the outcome once.
//
// TestStatus 记录**最近一次**连通性测试的**结果**——这是快照字段，
// **不是**流式状态机。测试是同步阻塞调用，完成后一次性写入结果。
const (
	TestStatusPending = "pending" // never tested yet / 从未测试过
	TestStatusOK      = "ok"      // last test succeeded / 最近一次成功
	TestStatusError   = "error"   // last test failed / 最近一次失败
)

// APIFormat values for APIKey.APIFormat (used by custom provider only).
//
// APIKey.APIFormat 的取值（仅 custom provider 使用）。
const (
	APIFormatOpenAICompatible    = "openai-compatible"
	APIFormatAnthropicCompatible = "anthropic-compatible"
)

// Credentials is the bundle returned to consumers (chat, workflow,
// embedding) for LLM calls. Key is plaintext — callers must treat it as
// ephemeral, never log or persist it.
//
// Credentials 是返回给调用方（chat、workflow、embedding）用于调 LLM 的
// 凭证包。Key 是明文——调用方必须当短生命周期对待，禁止日志或持久化。
type Credentials struct {
	Key     string
	BaseURL string
}

// ListFilter is the query shape accepted by Repository.List.
//
// ListFilter 是 Repository.List 接受的查询形状。
type ListFilter struct {
	Cursor   string
	Limit    int
	Provider string // optional filter / 可选按 provider 过滤
}

// Sentinel errors. Mapped to HTTP responses by
// transport/httpapi/response/errmap.go.
//
// Sentinel 错误。由 transport/httpapi/response/errmap.go 映射到 HTTP 响应。
var (
	// ErrNotFound: lookup by id did not match a live record.
	// ErrNotFound：按 id 查询未命中活跃记录。
	ErrNotFound = errors.New("apikey: not found")

	// ErrNotFoundForProvider: current user has no live Key for the given provider.
	// ErrNotFoundForProvider：当前用户在给定 provider 下没有活跃 Key。
	ErrNotFoundForProvider = errors.New("apikey: no key for provider")

	// ErrInvalidProvider: provider name not in the supported whitelist.
	// ErrInvalidProvider：provider 名称不在支持的白名单内。
	ErrInvalidProvider = errors.New("apikey: invalid provider")

	// ErrBaseURLRequired: provider requires a base_url (ollama / custom) but none given.
	// ErrBaseURLRequired：provider 要求 base_url（ollama / custom），但未提供。
	ErrBaseURLRequired = errors.New("apikey: base_url required for this provider")

	// ErrAPIFormatRequired: custom provider needs an api_format.
	// ErrAPIFormatRequired：custom provider 必须指定 api_format。
	ErrAPIFormatRequired = errors.New("apikey: api_format required for custom provider")

	// ErrKeyRequired: create request missing the key value.
	// ErrKeyRequired：创建请求缺少 key 值。
	ErrKeyRequired = errors.New("apikey: key value is required")

	// ErrTestFailed: connectivity test failed (request reached provider, provider rejected).
	// ErrTestFailed：连通性测试失败（请求已达 provider，但被拒或出错）。
	ErrTestFailed = errors.New("apikey: connectivity test failed")

	// ErrInvalid: provider returned 401/403 at actual LLM call time.
	// ErrInvalid：provider 在真实 LLM 调用时返回 401/403。
	ErrInvalid = errors.New("apikey: key rejected by provider")
)

// Repository is the storage contract for APIKey. Implementations filter
// by the userID in ctx — callers MUST ensure InjectUserID middleware has
// run before invoking any method here.
//
// Implemented by: infra/store/apikey.Store
// Consumer:       app/apikey.Service (only)
//
// Repository 是 APIKey 的存储契约。实现按 ctx 中的 userID 过滤——调用方
// 必须保证 InjectUserID 中间件已在链中运行。
//
// 实现：infra/store/apikey.Store
// 消费：仅 app/apikey.Service
type Repository interface {
	// Get fetches a single APIKey by id, scoped to the user in ctx.
	// Returns ErrNotFound if no live record matches.
	//
	// Get 按 id 查询单条 APIKey，按 ctx 中的用户过滤。
	// 未命中活跃记录返回 ErrNotFound。
	Get(ctx context.Context, id string) (*APIKey, error)

	// List returns a page of keys for the current user, with optional
	// provider filter. Returns (rows, nextCursor, err).
	//
	// List 返回当前用户的一页 Key，可选按 provider 过滤。
	// 返回 (rows, nextCursor, err)。
	List(ctx context.Context, filter ListFilter) ([]*APIKey, string, error)

	// GetByProvider picks the most suitable live APIKey for the given
	// provider under the current user. Selection order:
	//   1. test_status = 'ok' preferred
	//   2. last_tested_at DESC (most recently validated)
	//   3. created_at DESC (most recently created)
	// Returns ErrNotFoundForProvider if none exists.
	//
	// GetByProvider 为当前用户在指定 provider 下挑选**最适合**的活跃 Key。
	// 挑选顺序：
	//   1. test_status = 'ok' 优先
	//   2. last_tested_at DESC（最近验证过）
	//   3. created_at DESC（最近创建）
	// 未命中返回 ErrNotFoundForProvider。
	GetByProvider(ctx context.Context, provider string) (*APIKey, error)

	// Save inserts or updates based on whether k.ID already exists. The
	// caller must have set UserID on k (typically from ctx).
	//
	// Save 按 k.ID 决定插入或更新。调用方需确保已设置 UserID（通常从 ctx 取）。
	Save(ctx context.Context, k *APIKey) error

	// Delete soft-deletes by id, scoped to current user.
	//
	// Delete 软删除（按 ctx 中用户过滤）。
	Delete(ctx context.Context, id string) error

	// UpdateTestResult writes only test_status / test_error / last_tested_at.
	// Used by Service.Test and MarkInvalid to avoid a full-record round-trip.
	//
	// UpdateTestResult 只写 test_status / test_error / last_tested_at。
	// Service.Test 和 MarkInvalid 使用，避免读写整条记录。
	UpdateTestResult(ctx context.Context, id, status, errMsg string) error
}

// KeyProvider is the cross-domain interface. Other services (chat,
// workflow, knowledge/embedding, etc.) import this to obtain ready-to-use
// decrypted credentials for an LLM call — they never see Repository or
// raw APIKey records.
//
// Implemented by: app/apikey.Service
//
// KeyProvider 是跨 domain 接口。其他 service（chat、workflow、知识库
// embedding 等）通过本接口拿到调 LLM 用的明文凭证——它们看不到
// Repository 或原始 APIKey 记录。
//
// 由 app/apikey.Service 实现。
type KeyProvider interface {
	// ResolveCredentials returns a usable (key, baseURL) pair for the
	// given provider under the current user (from ctx). Internally: picks
	// the best APIKey (tested-OK preferred), decrypts, merges baseURL
	// with the provider default.
	//
	// Returns ErrNotFoundForProvider if no live key exists.
	//
	// ResolveCredentials 为当前用户（从 ctx 取）在给定 provider 下返回可用的
	// (key, baseURL)。内部：挑最佳 APIKey（tested-OK 优先）、解密、合并
	// baseURL 与 provider 默认值。
	//
	// 用户在该 provider 无活跃 Key 时返回 ErrNotFoundForProvider。
	ResolveCredentials(ctx context.Context, provider string) (Credentials, error)

	// MarkInvalid is the feedback channel: call when an LLM call with the
	// returned credentials got 401/403. Updates test_status to error and
	// records the reason so the UI can surface it.
	//
	// MarkInvalid 是反馈通道：用返回的凭证调 LLM 遇到 401/403 时调用。
	// 把 test_status 更新为 error 并记录原因，UI 可向用户展示。
	MarkInvalid(ctx context.Context, provider string, reason string) error
}
