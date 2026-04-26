// Package tool is the domain layer for the user's Python tool library.
// It owns five entities (Tool, ToolVersion, ToolTestCase, ToolRunHistory,
// ToolTestHistory), the shared ExecutionResult value object, enumeration
// constants, sentinel errors, and the storage contract (Repository).
//
// Design notes:
//
//   - ToolVersion doubles as pending-change storage: status='pending' means
//     awaiting user confirmation; status='accepted' is a committed version.
//
//   - ExecutionResult lives here (not in app/tool) so that infra/sandbox can
//     return it without importing app/tool, avoiding a circular dependency.
//
//   - All three tool packages (domain / app / store) declare `package tool`.
//     External callers alias by role at import time:
//
//     tooldomain "…/internal/domain/tool"
//     toolapp    "…/internal/app/tool"
//     toolstore  "…/internal/infra/store/tool"
//
// Package tool 是用户 Python 工具库的 domain 层。拥有 5 个实体
// （Tool / ToolVersion / ToolTestCase / ToolRunHistory / ToolTestHistory）、
// 共享值对象 ExecutionResult、枚举常量、sentinel 错误及存储契约（Repository）。
//
// 设计说明：
//   - ToolVersion 同时承担 pending 变更存储：status='pending' 表示待用户确认；
//     status='accepted' 是已提交版本。
//   - ExecutionResult 定义在本层（而非 app/tool），使 infra/sandbox 可直接
//     返回它，不必 import app/tool，避免循环依赖。
//   - 三个 tool 包均声明 `package tool`，调用方 import 时按角色起别名（见上）。
package tool

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ── Tool ──────────────────────────────────────────────────────────────────────

// Tool is the main entity representing a user-forged Python tool.
// Code holds the currently active version; VersionCount is the highest
// accepted version number (0 before first save).
//
// Tool 是用户锻造的 Python 工具主实体。
// Code 存当前活跃代码；VersionCount 是最大已接受版本号（首次保存前为 0）。
type Tool struct {
	ID           string         `gorm:"primaryKey;type:text"           json:"id"`
	UserID       string         `gorm:"not null;index;type:text"       json:"-"`
	Name         string         `gorm:"not null;type:text"             json:"name"`
	Description  string         `gorm:"not null;type:text;default:''"  json:"description"`
	Code         string         `gorm:"not null;type:text"             json:"code"`
	Parameters   string         `gorm:"type:text;default:'[]'"         json:"parameters"`   // JSON: [{name,type,required,description,default?}]
	ReturnSchema string         `gorm:"type:text;default:'{}'"         json:"returnSchema"` // JSON: {type,description}
	Tags         string         `gorm:"type:text;default:'[]'"         json:"tags"`         // JSON: ["tag1","tag2"]
	VersionCount int            `gorm:"not null;default:0"             json:"versionCount"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
	DeletedAt    gorm.DeletedAt `gorm:"index"                          json:"-"`
}

// TableName locks the DB table to "tools".
//
// TableName 把表名锁定为 "tools"。
func (Tool) TableName() string { return "tools" }

// ── ToolVersion ───────────────────────────────────────────────────────────────

// ToolVersion is a complete snapshot of a Tool at a point in time.
// It serves dual purpose: status='accepted' records committed history;
// status='pending' is an unconfirmed LLM proposal waiting for user review.
// Version is nil for pending/rejected rows; assigned on acceptance.
//
// ToolVersion 是工具在某一时刻的完整快照。双重职责：
// status='accepted' 记录已提交历史；status='pending' 是待用户审核的 LLM 提案。
// Version 在 pending/rejected 时为 nil；接受时分配版本号。
type ToolVersion struct {
	ID      string `gorm:"primaryKey;type:text"           json:"id"`
	ToolID  string `gorm:"not null;index;type:text"       json:"toolId"`
	UserID  string `gorm:"not null;type:text"             json:"-"`
	Version *int   `gorm:"type:integer"                   json:"version"`
	Status  string `gorm:"not null;type:text"             json:"status"` // "pending"|"accepted"|"rejected"

	// Complete tool snapshot at this point in time.
	// 该时刻工具的完整快照。
	Name         string `gorm:"not null;type:text"             json:"name"`
	Description  string `gorm:"type:text;default:''"           json:"description"`
	Code         string `gorm:"not null;type:text"             json:"code"`
	Parameters   string `gorm:"type:text;default:'[]'"         json:"parameters"`
	ReturnSchema string `gorm:"type:text;default:'{}'"         json:"returnSchema"`
	Tags         string `gorm:"type:text;default:'[]'"         json:"tags"`

	// Message records the intent: LLM instruction, "manual edit", "reverted to v{N}", or "initial".
	// Message 记录变更意图：LLM 指令、"manual edit"、"reverted to v{N}" 或 "initial"。
	Message   string    `gorm:"type:text;default:''" json:"message"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName locks the DB table to "tool_versions".
//
// TableName 把表名锁定为 "tool_versions"。
func (ToolVersion) TableName() string { return "tool_versions" }

// ── ToolTestCase ──────────────────────────────────────────────────────────────

// ToolTestCase is a named test case for a tool. ExpectedOutput is optional;
// an empty string means no assertion — the run is judged by sandbox success only.
//
// ToolTestCase 是工具的命名测试用例。ExpectedOutput 可选；
// 空字符串表示不断言——仅由 sandbox 执行成功与否判断。
type ToolTestCase struct {
	ID             string    `gorm:"primaryKey;type:text"        json:"id"`
	ToolID         string    `gorm:"not null;index;type:text"    json:"toolId"`
	UserID         string    `gorm:"not null;type:text"          json:"-"`
	Name           string    `gorm:"not null;type:text"          json:"name"`
	InputData      string    `gorm:"type:text;default:'{}'"      json:"inputData"`      // JSON object
	ExpectedOutput string    `gorm:"type:text;default:''"        json:"expectedOutput"` // JSON; empty = no assertion
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// TableName locks the DB table to "tool_test_cases".
//
// TableName 把表名锁定为 "tool_test_cases"。
func (ToolTestCase) TableName() string { return "tool_test_cases" }

// ── ToolRunHistory ────────────────────────────────────────────────────────────

// ToolRunHistory records every ad-hoc :run execution, success or failure.
// ToolVersion captures which accepted version was running at the time.
//
// ToolRunHistory 记录每次临时 :run 执行，无论成功或失败。
// ToolVersion 记录执行时处于第几个已接受版本。
type ToolRunHistory struct {
	ID          string    `gorm:"primaryKey;type:text"     json:"id"`
	ToolID      string    `gorm:"not null;index;type:text" json:"toolId"`
	UserID      string    `gorm:"not null;type:text"       json:"-"`
	ToolVersion int       `gorm:"not null"                 json:"toolVersion"`
	Input       string    `gorm:"type:text;default:'{}'"   json:"input"`
	Output      string    `gorm:"type:text;default:''"     json:"output"`
	OK          bool      `gorm:"not null"                 json:"ok"`
	ErrorMsg    string    `gorm:"type:text;default:''"     json:"errorMsg"`
	ElapsedMs   int64     `gorm:"not null;default:0"       json:"elapsedMs"`
	CreatedAt   time.Time `json:"createdAt"`
}

// TableName locks the DB table to "tool_run_history".
//
// TableName 把表名锁定为 "tool_run_history"。
func (ToolRunHistory) TableName() string { return "tool_run_history" }

// ── ToolTestHistory ───────────────────────────────────────────────────────────

// ToolTestHistory records every test-case execution, whether triggered
// individually or as part of a batch (:test). BatchID ties all records from
// a single :test run together; it is empty for individual runs.
// Pass is nil when ExpectedOutput was empty (no assertion).
//
// ToolTestHistory 记录每次测试用例执行，无论是单跑还是批跑（:test）。
// BatchID 把同一次 :test 的所有记录串起来；单跑时为空。
// ExpectedOutput 为空时（无断言）Pass 为 nil。
type ToolTestHistory struct {
	ID          string    `gorm:"primaryKey;type:text"       json:"id"`
	ToolID      string    `gorm:"not null;index;type:text"   json:"toolId"`
	UserID      string    `gorm:"not null;type:text"         json:"-"`
	ToolVersion int       `gorm:"not null"                   json:"toolVersion"`
	TestCaseID  string    `gorm:"not null;index;type:text"   json:"testCaseId"`
	BatchID     string    `gorm:"type:text;default:'';index" json:"batchId"`
	Input       string    `gorm:"type:text;default:'{}'"     json:"input"`
	Output      string    `gorm:"type:text;default:''"       json:"output"`
	OK          bool      `gorm:"not null"                   json:"ok"`
	Pass        *bool     `gorm:"type:integer"               json:"pass"` // nil = no assertion
	ErrorMsg    string    `gorm:"type:text;default:''"       json:"errorMsg"`
	ElapsedMs   int64     `gorm:"not null;default:0"         json:"elapsedMs"`
	CreatedAt   time.Time `json:"createdAt"`
}

// TableName locks the DB table to "tool_test_history".
//
// TableName 把表名锁定为 "tool_test_history"。
func (ToolTestHistory) TableName() string { return "tool_test_history" }

// ── ExecutionResult ───────────────────────────────────────────────────────────

// ExecutionResult is the outcome of a single sandbox Run call. It lives in
// the domain layer so that infra/sandbox can return it without depending on
// app/tool (which would create a circular import).
//
// ExecutionResult 是单次 sandbox Run 的执行结果。定义在 domain 层，
// 使 infra/sandbox 可直接返回它而不必 import app/tool（否则循环依赖）。
type ExecutionResult struct {
	OK        bool
	Output    any
	ErrorMsg  string
	ElapsedMs int64
}

// ── Constants ─────────────────────────────────────────────────────────────────

// VersionStatus values for ToolVersion.Status.
//
// ToolVersion.Status 的取值。
const (
	VersionStatusPending  = "pending"  // LLM proposal awaiting user review / LLM 提案，等待用户审核
	VersionStatusAccepted = "accepted" // committed version / 已提交版本
	VersionStatusRejected = "rejected" // user-rejected proposal / 用户已拒绝的提案
)

// Retention limits. Enforced at write time by app/tool.Service.
//
// 保留上限。由 app/tool.Service 在写入时强制执行。
const (
	MaxAcceptedVersions   = 50  // per tool / 每工具
	MaxRunHistoryPerTool  = 100 // per tool / 每工具
	MaxTestHistoryPerTool = 200 // per tool / 每工具
)

// SandboxTimeout is the hard limit for a single Python sandbox execution.
//
// SandboxTimeout 是单次 Python 沙箱执行的硬超时限制。
const SandboxTimeout = 30 * time.Second

// ── Sentinel errors ───────────────────────────────────────────────────────────

// Sentinel errors. Mapped to HTTP responses by
// transport/httpapi/response/errmap.go.
//
// Sentinel 错误。由 transport/httpapi/response/errmap.go 映射到 HTTP 响应。
var (
	// ErrNotFound: tool id does not match any live record.
	// ErrNotFound：tool id 未命中任何活跃记录。
	ErrNotFound = errors.New("tool: not found")

	// ErrDuplicateName: name already taken by another live tool for this user.
	// ErrDuplicateName：该用户下已有同名活跃工具。
	ErrDuplicateName = errors.New("tool: name already exists")

	// ErrVersionNotFound: requested version number does not exist for the tool.
	// ErrVersionNotFound：工具下不存在该版本号。
	ErrVersionNotFound = errors.New("tool: version not found")

	// ErrPendingNotFound: accept/reject called but no pending change exists.
	// ErrPendingNotFound：调用 accept/reject 但工具没有待审核的变更。
	ErrPendingNotFound = errors.New("tool: no pending change found")

	// ErrPendingConflict: edit_tool called while an unresolved pending exists.
	// ErrPendingConflict：edit_tool 调用时已有未处理的 pending 变更。
	ErrPendingConflict = errors.New("tool: already has a pending change")

	// ErrTestCaseNotFound: test case id does not match any record for the tool.
	// ErrTestCaseNotFound：test case id 在工具下未命中任何记录。
	ErrTestCaseNotFound = errors.New("tool: test case not found")

	// ErrRunFailed: sandbox internal error (distinct from ok=false execution failure).
	// ErrRunFailed：sandbox 内部错误（与 ok=false 的执行失败不同）。
	ErrRunFailed = errors.New("tool: execution failed")

	// ErrASTParseError: Python AST parsing of the submitted code failed.
	// ErrASTParseError：提交代码的 Python AST 解析失败。
	ErrASTParseError = errors.New("tool: code AST parse failed")

	// ErrImportInvalid: import payload is malformed or missing required fields.
	// ErrImportInvalid：导入数据格式错误或缺少必填字段。
	ErrImportInvalid = errors.New("tool: import data invalid")
)

// ── Repository ────────────────────────────────────────────────────────────────

// Repository is the storage contract for all tool-related entities.
// Every method scopes queries to the userID carried in ctx — callers must
// ensure the InjectUserID middleware has run.
//
// Implemented by: infra/store/tool.Store
// Consumer:       app/tool.Service (only)
//
// Repository 是所有工具相关实体的存储契约。
// 每个方法都按 ctx 中的 userID 过滤——调用方必须保证 InjectUserID 中间件已运行。
//
// 实现：infra/store/tool.Store
// 消费：仅 app/tool.Service
type Repository interface {

	// ── Tool CRUD ─────────────────────────────────────────────────────────

	// SaveTool inserts or updates a Tool by primary key.
	//
	// SaveTool 按主键插入或更新 Tool。
	SaveTool(ctx context.Context, t *Tool) error

	// GetTool fetches a single Tool by id, scoped to the current user.
	// Returns ErrNotFound if no live record matches.
	//
	// GetTool 按 id 查单条，按当前用户过滤。未命中活跃记录返回 ErrNotFound。
	GetTool(ctx context.Context, id string) (*Tool, error)

	// GetToolsByIDs fetches multiple Tools by id slice, preserving order.
	// Used by the semantic search path to hydrate vector search results.
	//
	// GetToolsByIDs 按 id 切片批量查询 Tool，保持顺序。
	// 供语义搜索路径从向量结果中还原完整对象使用。
	GetToolsByIDs(ctx context.Context, ids []string) ([]*Tool, error)

	// ListTools returns a cursor-paginated page of live tools for the current user.
	// Returns (rows, nextCursor, err).
	//
	// ListTools 返回当前用户活跃工具的 cursor 分页结果。
	// 返回 (rows, nextCursor, err)。
	ListTools(ctx context.Context, filter ListFilter) ([]*Tool, string, error)

	// DeleteTool soft-deletes a tool by id, scoped to the current user.
	//
	// DeleteTool 软删除（按当前用户过滤）。
	DeleteTool(ctx context.Context, id string) error

	// ── Versions (including pending) ──────────────────────────────────────

	// SaveVersion inserts a ToolVersion record.
	//
	// SaveVersion 插入一条 ToolVersion 记录。
	SaveVersion(ctx context.Context, v *ToolVersion) error

	// GetVersion fetches the accepted ToolVersion with the given version number.
	// Returns ErrVersionNotFound if it does not exist.
	//
	// GetVersion 查询指定版本号的已接受版本记录。
	// 不存在时返回 ErrVersionNotFound。
	GetVersion(ctx context.Context, toolID string, version int) (*ToolVersion, error)

	// GetActivePending returns the current pending ToolVersion for the tool,
	// or ErrPendingNotFound if none exists.
	//
	// GetActivePending 返回工具当前的 pending ToolVersion。
	// 不存在时返回 ErrPendingNotFound。
	GetActivePending(ctx context.Context, toolID string) (*ToolVersion, error)

	// ListAcceptedVersions returns all accepted versions for a tool,
	// ordered by version DESC (newest first).
	//
	// ListAcceptedVersions 返回工具所有已接受版本，按版本号降序（最新在前）。
	ListAcceptedVersions(ctx context.Context, toolID string) ([]*ToolVersion, error)

	// UpdateVersionStatus updates the status field and optionally assigns a
	// version number (pass nil to leave it NULL, e.g. for rejection).
	//
	// UpdateVersionStatus 更新 status 字段，可选分配版本号
	// （拒绝时传 nil 保持 NULL）。
	UpdateVersionStatus(ctx context.Context, id, status string, version *int) error

	// CountAcceptedVersions returns the number of accepted versions for a tool.
	//
	// CountAcceptedVersions 返回工具已接受版本数。
	CountAcceptedVersions(ctx context.Context, toolID string) (int64, error)

	// DeleteOldestAcceptedVersion hard-deletes the accepted version with the
	// lowest version number for the given tool.
	//
	// DeleteOldestAcceptedVersion 硬删除指定工具版本号最小的已接受版本。
	DeleteOldestAcceptedVersion(ctx context.Context, toolID string) error

	// ── Test cases ────────────────────────────────────────────────────────

	// SaveTestCase inserts a ToolTestCase.
	//
	// SaveTestCase 插入 ToolTestCase。
	SaveTestCase(ctx context.Context, tc *ToolTestCase) error

	// GetTestCase fetches a test case by id.
	// Returns ErrTestCaseNotFound if no record matches.
	//
	// GetTestCase 按 id 查测试用例。未命中返回 ErrTestCaseNotFound。
	GetTestCase(ctx context.Context, id string) (*ToolTestCase, error)

	// ListTestCases returns all test cases for the given tool, ordered by
	// created_at ASC.
	//
	// ListTestCases 返回指定工具所有测试用例，按 created_at ASC 排序。
	ListTestCases(ctx context.Context, toolID string) ([]*ToolTestCase, error)

	// DeleteTestCase hard-deletes a test case by id.
	//
	// DeleteTestCase 硬删除测试用例。
	DeleteTestCase(ctx context.Context, id string) error

	// ── Run history ───────────────────────────────────────────────────────

	// SaveRunHistory inserts a ToolRunHistory record.
	//
	// SaveRunHistory 插入 ToolRunHistory 记录。
	SaveRunHistory(ctx context.Context, h *ToolRunHistory) error

	// ListRunHistory returns the most recent limit run history records for
	// the given tool, ordered by created_at DESC.
	//
	// ListRunHistory 返回指定工具最近 limit 条运行历史，按 created_at DESC。
	ListRunHistory(ctx context.Context, toolID string, limit int) ([]*ToolRunHistory, error)

	// CountRunHistory returns the total number of run history records for a tool.
	//
	// CountRunHistory 返回工具运行历史总条数。
	CountRunHistory(ctx context.Context, toolID string) (int64, error)

	// DeleteOldestRunHistory hard-deletes the oldest run history record for
	// the given tool.
	//
	// DeleteOldestRunHistory 硬删除指定工具最早的运行历史记录。
	DeleteOldestRunHistory(ctx context.Context, toolID string) error

	// ── Test history ──────────────────────────────────────────────────────

	// SaveTestHistory inserts a ToolTestHistory record.
	//
	// SaveTestHistory 插入 ToolTestHistory 记录。
	SaveTestHistory(ctx context.Context, h *ToolTestHistory) error

	// ListTestHistory returns the most recent limit test history records for
	// the given tool, ordered by created_at DESC.
	//
	// ListTestHistory 返回指定工具最近 limit 条测试历史，按 created_at DESC。
	ListTestHistory(ctx context.Context, toolID string, limit int) ([]*ToolTestHistory, error)

	// ListTestHistoryByBatch returns all test history records sharing the
	// given batchID, ordered by created_at ASC.
	//
	// ListTestHistoryByBatch 返回指定 batchID 的所有测试历史记录，
	// 按 created_at ASC 排序。
	ListTestHistoryByBatch(ctx context.Context, batchID string) ([]*ToolTestHistory, error)

	// CountTestHistory returns the total number of test history records for a tool.
	//
	// CountTestHistory 返回工具测试历史总条数。
	CountTestHistory(ctx context.Context, toolID string) (int64, error)

	// DeleteOldestTestHistory hard-deletes the oldest test history record for
	// the given tool.
	//
	// DeleteOldestTestHistory 硬删除指定工具最早的测试历史记录。
	DeleteOldestTestHistory(ctx context.Context, toolID string) error
}

// ListFilter is the query shape accepted by Repository.ListTools.
//
// ListFilter 是 Repository.ListTools 接受的查询形状。
type ListFilter struct {
	Cursor string
	Limit  int
}
