// Package tool (app layer) owns the Service that orchestrates the tool domain:
// CRUD, version/pending lifecycle, sandbox execution, test cases, and
// AI-powered test-case generation.
//
// All three tool packages (domain / app / store) declare `package tool`;
// external callers alias at import (e.g. toolapp "…/internal/app/tool").
//
// Package tool（app 层）负责 Service 编排 tool domain：CRUD、版本/pending
// 生命周期、沙箱执行、测试用例和 AI 辅助测试用例生成。
//
// 三个 tool 包均声明 `package tool`；外部调用方 import 时按角色起别名，
// 如 toolapp "…/internal/app/tool"。
package tool

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	tooldomain "github.com/sunweilin/forgify/backend/internal/domain/tool"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// ── Interfaces ────────────────────────────────────────────────────────────────

// Sandbox executes user Python tool code.
//
// Sandbox 执行用户 Python 工具代码。
type Sandbox interface {
	Run(ctx context.Context, code string, input map[string]any) (*tooldomain.ExecutionResult, error)
}

// LLMClient makes non-streaming LLM calls that return complete JSON responses.
// Used by GenerateTestCases. The implementation resolves model/key internally.
//
// LLMClient 进行非流式 LLM 调用，返回完整 JSON 响应。
// 供 GenerateTestCases 使用；实现层内部解析 model/key。
type LLMClient interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

// GenerateEvent is emitted by GenerateTestCases via the emit callback.
//
// GenerateEvent 是 GenerateTestCases 通过 emit callback 推送的事件。
type GenerateEvent struct {
	Type     string                   // "test_case" | "done" | "not_supported"
	TestCase *tooldomain.ToolTestCase // non-nil when Type="test_case"
	Count    int                      // non-zero when Type="done"
	Reason   string                   // non-empty when Type="not_supported"
}

// ── Input / Output types ──────────────────────────────────────────────────────

// CreateInput is the request shape for Service.Create.
//
// CreateInput 是 Service.Create 的请求形状。
type CreateInput struct {
	Name        string
	Description string
	Code        string
	Tags        []string
}

// UpdateInput is the request shape for Service.Update. Nil fields are unchanged.
//
// UpdateInput 是 Service.Update 的请求形状。nil 字段不更新。
type UpdateInput struct {
	Name        *string
	Description *string
	Tags        *[]string
	Code        *string
}

// PendingSnapshot is the proposed new state passed to Service.CreatePending.
//
// PendingSnapshot 是传给 Service.CreatePending 的提案新状态。
type PendingSnapshot struct {
	Name        string
	Description string
	Code        string
	Tags        string // JSON string
	Instruction string
}

// TestCaseInput is the request shape for Service.CreateTestCase.
//
// TestCaseInput 是 Service.CreateTestCase 的请求形状。
type TestCaseInput struct {
	Name           string
	InputData      string // JSON object string
	ExpectedOutput string // JSON string; empty = no assertion
}

// TestRunResult is the outcome of a single test case execution.
//
// TestRunResult 是单次测试用例执行的结果。
type TestRunResult struct {
	TestCaseID string
	Name       string
	Input      string
	Output     string
	OK         bool
	Pass       *bool
	ErrorMsg   string
	ElapsedMs  int64
}

// ToolDetail extends Tool with a pre-computed TestSummary for get_tool.
//
// ToolDetail 在 Tool 基础上追加预计算的 TestSummary，供 get_tool 使用。
type ToolDetail struct {
	*tooldomain.Tool
	TestSummary TestSummary
}

// TestSummary is a short digest of the most recent :test batch run.
//
// TestSummary 是最近一次 :test 批跑的简要摘要。
type TestSummary struct {
	Total        int    // current test case count
	LastPassRate string // "3/3" | "2/3" | "" (no record)
	LastRunAt    string // ISO 8601 or ""
}

// ── Service ───────────────────────────────────────────────────────────────────

// Service orchestrates the tool domain.
//
// Service 编排 tool domain。
type Service struct {
	repo    tooldomain.Repository
	sandbox Sandbox
	llm     LLMClient
	log     *zap.Logger
}

// NewService wires Service dependencies. Panics on nil logger.
//
// NewService 装配 Service 依赖。nil logger 会 panic。
func NewService(repo tooldomain.Repository, sandbox Sandbox, llm LLMClient, log *zap.Logger) *Service {
	if log == nil {
		panic("toolapp.NewService: logger is nil")
	}
	return &Service{repo: repo, sandbox: sandbox, llm: llm, log: log}
}

// ── CRUD ──────────────────────────────────────────────────────────────────────

// Create parses the code, persists the Tool, and saves v1 accepted version.
//
// Create 解析代码，持久化 Tool，保存 v1 已接受版本。
func (s *Service) Create(ctx context.Context, in CreateInput) (*tooldomain.Tool, error) {
	parsed, err := s.parse(in.Code)
	if err != nil {
		return nil, err
	}
	id := newID("t")
	now := time.Now().UTC()
	t := &tooldomain.Tool{
		ID:           id,
		Name:         in.Name,
		Description:  in.Description,
		Code:         in.Code,
		Parameters:   parsed.parametersJSON,
		ReturnSchema: parsed.returnSchemaJSON,
		Tags:         tagsJSON(in.Tags),
		VersionCount: 1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err = mustSetUserID(ctx, t); err != nil {
		return nil, err
	}
	if err = s.repo.SaveTool(ctx, t); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, tooldomain.ErrDuplicateName
		}
		return nil, fmt.Errorf("toolapp.Create: %w", err)
	}
	one := 1
	v := newVersion(t, tooldomain.VersionStatusAccepted, &one, "initial")
	if err = s.repo.SaveVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("toolapp.Create: save version: %w", err)
	}
	return t, nil
}

// Get fetches a single live Tool.
//
// Get 查询单条活跃 Tool。
func (s *Service) Get(ctx context.Context, id string) (*tooldomain.Tool, error) {
	return s.repo.GetTool(ctx, id)
}

// GetDetail returns the Tool plus a TestSummary for get_tool system tool.
//
// GetDetail 返回 Tool 及 TestSummary，供 get_tool system tool 使用。
func (s *Service) GetDetail(ctx context.Context, id string) (*ToolDetail, error) {
	t, err := s.repo.GetTool(ctx, id)
	if err != nil {
		return nil, err
	}
	cases, _ := s.repo.ListTestCases(ctx, id)
	summary := TestSummary{Total: len(cases)}

	// Last batch: find most recent batchID from test history.
	hist, _ := s.repo.ListTestHistory(ctx, id, 200)
	if len(hist) > 0 && hist[0].BatchID != "" {
		lastBatch, _ := s.repo.ListTestHistoryByBatch(ctx, hist[0].BatchID)
		if len(lastBatch) > 0 {
			passed := 0
			for _, h := range lastBatch {
				if h.Pass != nil && *h.Pass {
					passed++
				}
			}
			summary.LastPassRate = fmt.Sprintf("%d/%d", passed, len(lastBatch))
			summary.LastRunAt = lastBatch[len(lastBatch)-1].CreatedAt.UTC().Format(time.RFC3339)
		}
	}
	return &ToolDetail{Tool: t, TestSummary: summary}, nil
}

// List returns a cursor-paginated page of tools.
//
// List 返回 cursor 分页的工具列表。
func (s *Service) List(ctx context.Context, filter tooldomain.ListFilter) ([]*tooldomain.Tool, string, error) {
	return s.repo.ListTools(ctx, filter)
}

// ListAll returns all live tools without pagination (used by SearchTool).
//
// ListAll 返回所有活跃工具，不分页（供 SearchTool 使用）。
func (s *Service) ListAll(ctx context.Context) ([]*tooldomain.Tool, error) {
	return s.repo.ListAllTools(ctx)
}

// GetToolsByIDs fetches multiple live tools by ID slice, preserving order.
//
// GetToolsByIDs 按 ID 切片批量查活跃工具，保持顺序。
func (s *Service) GetToolsByIDs(ctx context.Context, ids []string) ([]*tooldomain.Tool, error) {
	return s.repo.GetToolsByIDs(ctx, ids)
}

// ListRunHistoryForTool returns recent run history for a tool.
//
// ListRunHistoryForTool 返回工具最近的运行历史。
func (s *Service) ListRunHistoryForTool(ctx context.Context, toolID string, limit int) ([]*tooldomain.ToolRunHistory, error) {
	return s.repo.ListRunHistory(ctx, toolID, limit)
}

// ListTestHistoryForTool returns recent test history for a tool.
//
// ListTestHistoryForTool 返回工具最近的测试历史。
func (s *Service) ListTestHistoryForTool(ctx context.Context, toolID string, limit int) ([]*tooldomain.ToolTestHistory, error) {
	return s.repo.ListTestHistory(ctx, toolID, limit)
}

// ListTestHistoryByBatch returns test history records for a batch run.
//
// ListTestHistoryByBatch 返回指定批次的测试历史记录。
func (s *Service) ListTestHistoryByBatch(ctx context.Context, batchID string) ([]*tooldomain.ToolTestHistory, error) {
	return s.repo.ListTestHistoryByBatch(ctx, batchID)
}

// Update applies partial changes to a Tool. Code changes trigger an AST
// re-parse and auto-reject any active pending.
//
// Update 对 Tool 做局部更新。代码变更触发 AST 重解析并自动 reject 现有 pending。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*tooldomain.Tool, error) {
	t, err := s.repo.GetTool(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.Name != nil {
		t.Name = *in.Name
	}
	if in.Description != nil {
		t.Description = *in.Description
	}
	if in.Tags != nil {
		t.Tags = tagsJSON(*in.Tags)
	}
	if in.Code != nil {
		if err = s.autoRejectPending(ctx, id); err != nil {
			return nil, err
		}
		parsed, err := s.parse(*in.Code)
		if err != nil {
			return nil, err
		}
		t.Code = *in.Code
		t.Parameters = parsed.parametersJSON
		t.ReturnSchema = parsed.returnSchemaJSON
		t.VersionCount++
		v := newVersion(t, tooldomain.VersionStatusAccepted, &t.VersionCount, "manual edit")
		if err = s.repo.SaveVersion(ctx, v); err != nil {
			return nil, fmt.Errorf("toolapp.Update: save version: %w", err)
		}
		if err = s.trimVersions(ctx, id); err != nil {
			return nil, err
		}
	}
	t.UpdatedAt = time.Now().UTC()
	if err = s.repo.SaveTool(ctx, t); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, tooldomain.ErrDuplicateName
		}
		return nil, fmt.Errorf("toolapp.Update: %w", err)
	}
	return t, nil
}

// Delete soft-deletes a Tool.
//
// Delete 软删除 Tool。
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.DeleteTool(ctx, id)
}

// ── Version management ────────────────────────────────────────────────────────

// ListVersions returns accepted versions newest-first.
//
// ListVersions 返回已接受版本，最新在前。
func (s *Service) ListVersions(ctx context.Context, toolID string) ([]*tooldomain.ToolVersion, error) {
	return s.repo.ListAcceptedVersions(ctx, toolID)
}

// GetVersion returns a specific accepted version.
//
// GetVersion 返回指定已接受版本。
func (s *Service) GetVersion(ctx context.Context, toolID string, version int) (*tooldomain.ToolVersion, error) {
	return s.repo.GetVersion(ctx, toolID, version)
}

// RevertToVersion restores a tool to the complete snapshot of a prior version.
//
// RevertToVersion 将工具恢复到指定历史版本的完整快照。
func (s *Service) RevertToVersion(ctx context.Context, toolID string, version int) (*tooldomain.Tool, error) {
	v, err := s.repo.GetVersion(ctx, toolID, version)
	if err != nil {
		return nil, err
	}
	t, err := s.repo.GetTool(ctx, toolID)
	if err != nil {
		return nil, err
	}
	if err = s.autoRejectPending(ctx, toolID); err != nil {
		return nil, err
	}
	t.Name = v.Name
	t.Description = v.Description
	t.Code = v.Code
	t.Parameters = v.Parameters
	t.ReturnSchema = v.ReturnSchema
	t.Tags = v.Tags
	t.VersionCount++
	t.UpdatedAt = time.Now().UTC()
	msg := fmt.Sprintf("reverted to v%d", version)
	newV := newVersion(t, tooldomain.VersionStatusAccepted, &t.VersionCount, msg)
	if err = s.repo.SaveVersion(ctx, newV); err != nil {
		return nil, fmt.Errorf("toolapp.RevertToVersion: %w", err)
	}
	if err = s.repo.SaveTool(ctx, t); err != nil {
		return nil, fmt.Errorf("toolapp.RevertToVersion: %w", err)
	}
	if err = s.trimVersions(ctx, toolID); err != nil {
		return nil, err
	}
	return t, nil
}

// ── Pending management ────────────────────────────────────────────────────────

// GetActivePending returns the pending ToolVersion or ErrPendingNotFound.
//
// GetActivePending 返回 pending ToolVersion，不存在时返回 ErrPendingNotFound。
func (s *Service) GetActivePending(ctx context.Context, toolID string) (*tooldomain.ToolVersion, error) {
	return s.repo.GetActivePending(ctx, toolID)
}

// CreatePending checks for conflict, parses code if present, and saves a
// pending ToolVersion. Called by edit_tool system tool.
//
// CreatePending 检查冲突，解析代码（如有），保存 pending ToolVersion。
// 由 edit_tool system tool 调用。
func (s *Service) CreatePending(ctx context.Context, toolID string, snap PendingSnapshot) (*tooldomain.ToolVersion, error) {
	t, err := s.repo.GetTool(ctx, toolID)
	if err != nil {
		return nil, err
	}
	_, err = s.repo.GetActivePending(ctx, toolID)
	if err == nil {
		return nil, tooldomain.ErrPendingConflict
	}
	if !errors.Is(err, tooldomain.ErrPendingNotFound) {
		return nil, fmt.Errorf("toolapp.CreatePending: %w", err)
	}

	// Use snapshot fields if provided; fall back to current tool state.
	name := t.Name
	if snap.Name != "" {
		name = snap.Name
	}
	description := t.Description
	if snap.Description != "" {
		description = snap.Description
	}
	tags := t.Tags
	if snap.Tags != "" {
		tags = snap.Tags
	}
	code := t.Code
	params := t.Parameters
	returnSchema := t.ReturnSchema
	if snap.Code != "" {
		code = snap.Code
		parsed, err := s.parse(code)
		if err != nil {
			return nil, err
		}
		params = parsed.parametersJSON
		returnSchema = parsed.returnSchemaJSON
	}

	uid, _ := uidFromTool(t)
	v := &tooldomain.ToolVersion{
		ID:           newID("tv"),
		ToolID:       toolID,
		UserID:       uid,
		Status:       tooldomain.VersionStatusPending,
		Name:         name,
		Description:  description,
		Code:         code,
		Parameters:   params,
		ReturnSchema: returnSchema,
		Tags:         tags,
		Message:      snap.Instruction,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err = s.repo.SaveVersion(ctx, v); err != nil {
		return nil, fmt.Errorf("toolapp.CreatePending: %w", err)
	}
	return v, nil
}

// AcceptPending promotes the active pending for toolID to accepted and updates the tool.
//
// AcceptPending 将 toolID 的 active pending 提升为 accepted，并更新工具主表。
func (s *Service) AcceptPending(ctx context.Context, toolID string) (*tooldomain.Tool, error) {
	pv, err := s.repo.GetActivePending(ctx, toolID)
	if err != nil {
		return nil, err
	}
	t, err := s.repo.GetTool(ctx, toolID)
	if err != nil {
		return nil, err
	}
	t.Name = pv.Name
	t.Description = pv.Description
	t.Code = pv.Code
	t.Parameters = pv.Parameters
	t.ReturnSchema = pv.ReturnSchema
	t.Tags = pv.Tags
	t.VersionCount++
	t.UpdatedAt = time.Now().UTC()

	if err = s.repo.UpdateVersionStatus(ctx, pv.ID, tooldomain.VersionStatusAccepted, &t.VersionCount); err != nil {
		return nil, fmt.Errorf("toolapp.AcceptPending: %w", err)
	}
	if err = s.repo.SaveTool(ctx, t); err != nil {
		return nil, fmt.Errorf("toolapp.AcceptPending: %w", err)
	}
	if err = s.trimVersions(ctx, toolID); err != nil {
		return nil, err
	}
	return t, nil
}

// RejectPending marks the active pending for toolID as rejected.
//
// RejectPending 将 toolID 的 active pending 标记为 rejected。
func (s *Service) RejectPending(ctx context.Context, toolID string) error {
	pv, err := s.repo.GetActivePending(ctx, toolID)
	if err != nil {
		return err
	}
	if err = s.repo.UpdateVersionStatus(ctx, pv.ID, tooldomain.VersionStatusRejected, nil); err != nil {
		return fmt.Errorf("toolapp.RejectPending: %w", err)
	}
	return nil
}

// ── Execution ─────────────────────────────────────────────────────────────────

// RunTool executes the tool's current code in the sandbox and records history.
// input must already have att_ids resolved to file paths by the caller.
//
// RunTool 在沙箱中执行工具当前代码并记录历史。
// input 中的 att_id 必须由调用方预先解析为真实路径。
func (s *Service) RunTool(ctx context.Context, toolID string, input map[string]any) (*tooldomain.ExecutionResult, error) {
	t, err := s.repo.GetTool(ctx, toolID)
	if err != nil {
		return nil, err
	}
	result, err := s.sandbox.Run(ctx, t.Code, input)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", tooldomain.ErrRunFailed, err)
	}
	inputJSON, _ := json.Marshal(input)
	outputJSON := ""
	if result.Output != nil {
		if b, e := json.Marshal(result.Output); e == nil {
			outputJSON = string(b)
		}
	}
	uid, _ := uidFromTool(t)
	h := &tooldomain.ToolRunHistory{
		ID:          newID("trh"),
		ToolID:      toolID,
		UserID:      uid,
		ToolVersion: t.VersionCount,
		Input:       string(inputJSON),
		Output:      outputJSON,
		OK:          result.OK,
		ErrorMsg:    result.ErrorMsg,
		ElapsedMs:   result.ElapsedMs,
		CreatedAt:   time.Now().UTC(),
	}
	_ = s.repo.SaveRunHistory(ctx, h)
	if n, _ := s.repo.CountRunHistory(ctx, toolID); n > tooldomain.MaxRunHistoryPerTool {
		_ = s.repo.DeleteOldestRunHistory(ctx, toolID)
	}
	return result, nil
}

// ── Test cases ────────────────────────────────────────────────────────────────

// CreateTestCase adds a test case to a tool.
//
// CreateTestCase 为工具添加测试用例。
func (s *Service) CreateTestCase(ctx context.Context, toolID string, in TestCaseInput) (*tooldomain.ToolTestCase, error) {
	t, err := s.repo.GetTool(ctx, toolID)
	if err != nil {
		return nil, err
	}
	uid, _ := uidFromTool(t)
	tc := &tooldomain.ToolTestCase{
		ID:             newID("tc"),
		ToolID:         toolID,
		UserID:         uid,
		Name:           in.Name,
		InputData:      in.InputData,
		ExpectedOutput: in.ExpectedOutput,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}
	if err = s.repo.SaveTestCase(ctx, tc); err != nil {
		return nil, fmt.Errorf("toolapp.CreateTestCase: %w", err)
	}
	return tc, nil
}

// ListTestCases returns all test cases for a tool.
//
// ListTestCases 返回工具所有测试用例。
func (s *Service) ListTestCases(ctx context.Context, toolID string) ([]*tooldomain.ToolTestCase, error) {
	return s.repo.ListTestCases(ctx, toolID)
}

// DeleteTestCase hard-deletes a test case.
//
// DeleteTestCase 硬删除测试用例。
func (s *Service) DeleteTestCase(ctx context.Context, id string) error {
	return s.repo.DeleteTestCase(ctx, id)
}

// RunTestCase executes a single test case and records history.
// batchID is empty for individual runs.
//
// RunTestCase 执行单条测试用例并记录历史。单跑时 batchID 为空。
func (s *Service) RunTestCase(ctx context.Context, testCaseID, batchID string) (*TestRunResult, error) {
	tc, err := s.repo.GetTestCase(ctx, testCaseID)
	if err != nil {
		return nil, err
	}
	t, err := s.repo.GetTool(ctx, tc.ToolID)
	if err != nil {
		return nil, err
	}
	var input map[string]any
	_ = json.Unmarshal([]byte(tc.InputData), &input)

	result, sandboxErr := s.sandbox.Run(ctx, t.Code, input)
	if sandboxErr != nil {
		return nil, fmt.Errorf("%w: %v", tooldomain.ErrRunFailed, sandboxErr)
	}

	var pass *bool
	if tc.ExpectedOutput != "" && result.OK {
		actual, _ := json.Marshal(result.Output)
		p := strings.TrimSpace(string(actual)) == strings.TrimSpace(tc.ExpectedOutput)
		pass = &p
	}

	outputJSON := ""
	if b, e := json.Marshal(result.Output); e == nil {
		outputJSON = string(b)
	}

	uid, _ := uidFromTool(t)
	h := &tooldomain.ToolTestHistory{
		ID:          newID("tth"),
		ToolID:      t.ID,
		UserID:      uid,
		ToolVersion: t.VersionCount,
		TestCaseID:  testCaseID,
		BatchID:     batchID,
		Input:       tc.InputData,
		Output:      outputJSON,
		OK:          result.OK,
		Pass:        pass,
		ErrorMsg:    result.ErrorMsg,
		ElapsedMs:   result.ElapsedMs,
		CreatedAt:   time.Now().UTC(),
	}
	_ = s.repo.SaveTestHistory(ctx, h)
	if n, _ := s.repo.CountTestHistory(ctx, t.ID); n > tooldomain.MaxTestHistoryPerTool {
		_ = s.repo.DeleteOldestTestHistory(ctx, t.ID)
	}

	return &TestRunResult{
		TestCaseID: testCaseID,
		Name:       tc.Name,
		Input:      tc.InputData,
		Output:     outputJSON,
		OK:         result.OK,
		Pass:       pass,
		ErrorMsg:   result.ErrorMsg,
		ElapsedMs:  result.ElapsedMs,
	}, nil
}

// RunAllTests runs all test cases for a tool under a shared batch ID.
//
// RunAllTests 使用共享 batchID 运行工具的全部测试用例。
func (s *Service) RunAllTests(ctx context.Context, toolID string) ([]*TestRunResult, error) {
	cases, err := s.repo.ListTestCases(ctx, toolID)
	if err != nil {
		return nil, err
	}
	batchID := newID("b")
	results := make([]*TestRunResult, 0, len(cases))
	for _, tc := range cases {
		r, err := s.RunTestCase(ctx, tc.ID, batchID)
		if err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// GenerateTestCases asks the LLM to generate test cases and emits them one
// by one via the emit callback. The HTTP handler injects emit to write SSE.
//
// GenerateTestCases 请求 LLM 生成测试用例，通过 emit callback 逐条推送。
// HTTP handler 注入 emit 以写入 SSE。
func (s *Service) GenerateTestCases(ctx context.Context, toolID string, count int, emit func(GenerateEvent)) error {
	t, err := s.repo.GetTool(ctx, toolID)
	if err != nil {
		return err
	}
	prompt := buildGeneratePrompt(t, count)
	raw, err := s.llm.Generate(ctx, prompt)
	if err != nil {
		return fmt.Errorf("toolapp.GenerateTestCases: llm: %w", err)
	}
	jsonRaw := extractJSONFromLLM(raw)
	var resp struct {
		NotSupported bool   `json:"not_supported"`
		Reason       string `json:"reason"`
		TestCases    []struct {
			Name           string          `json:"name"`
			Input          json.RawMessage `json:"input"`
			ExpectedOutput json.RawMessage `json:"expected_output"`
		} `json:"test_cases"`
	}
	if err = json.Unmarshal([]byte(jsonRaw), &resp); err != nil {
		return fmt.Errorf("toolapp.GenerateTestCases: parse response: %w", err)
	}
	if resp.NotSupported {
		emit(GenerateEvent{Type: "not_supported", Reason: resp.Reason})
		return nil
	}
	uid, _ := uidFromTool(t)
	for _, tc := range resp.TestCases {
		saved := &tooldomain.ToolTestCase{
			ID:             newID("tc"),
			ToolID:         toolID,
			UserID:         uid,
			Name:           tc.Name,
			InputData:      string(tc.Input),
			ExpectedOutput: string(tc.ExpectedOutput),
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
		}
		if err = s.repo.SaveTestCase(ctx, saved); err != nil {
			return fmt.Errorf("toolapp.GenerateTestCases: save: %w", err)
		}
		emit(GenerateEvent{Type: "test_case", TestCase: saved})
	}
	emit(GenerateEvent{Type: "done", Count: len(resp.TestCases)})
	return nil
}

// ── Import / Export ───────────────────────────────────────────────────────────

// exportShape is the JSON shape for tool export/import.
//
// exportShape 是工具导入/导出的 JSON 形状。
type exportShape struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Code        string          `json:"code"`
	Tags        []string        `json:"tags"`
	TestCases   []TestCaseInput `json:"testCases"`
}

// Export serialises a tool and its test cases to JSON.
//
// Export 把工具及测试用例序列化为 JSON。
func (s *Service) Export(ctx context.Context, toolID string) ([]byte, error) {
	t, err := s.repo.GetTool(ctx, toolID)
	if err != nil {
		return nil, err
	}
	cases, _ := s.repo.ListTestCases(ctx, toolID)
	var tags []string
	_ = json.Unmarshal([]byte(t.Tags), &tags)
	tcInputs := make([]TestCaseInput, len(cases))
	for i, tc := range cases {
		tcInputs[i] = TestCaseInput{Name: tc.Name, InputData: tc.InputData, ExpectedOutput: tc.ExpectedOutput}
	}
	return json.Marshal(exportShape{
		Name: t.Name, Description: t.Description, Code: t.Code,
		Tags: tags, TestCases: tcInputs,
	})
}

// Import creates a new tool from exported JSON, including test cases.
//
// Import 从导出的 JSON 新建工具，包含测试用例。
func (s *Service) Import(ctx context.Context, data []byte) (*tooldomain.Tool, error) {
	var shape exportShape
	if err := json.Unmarshal(data, &shape); err != nil || shape.Name == "" || shape.Code == "" {
		return nil, tooldomain.ErrImportInvalid
	}
	t, err := s.Create(ctx, CreateInput{
		Name: shape.Name, Description: shape.Description,
		Code: shape.Code, Tags: shape.Tags,
	})
	if err != nil {
		return nil, err
	}
	for _, tc := range shape.TestCases {
		_, _ = s.CreateTestCase(ctx, t.ID, tc)
	}
	return t, nil
}

// ── Internal helpers ──────────────────────────────────────────────────────────

type parsedFields struct {
	parametersJSON   string
	returnSchemaJSON string
}

func (s *Service) parse(code string) (parsedFields, error) {
	p, err := parseToolCode(code)
	if err != nil {
		return parsedFields{}, tooldomain.ErrASTParseError
	}
	params := make([]map[string]any, len(p.Parameters))
	for i, pp := range p.Parameters {
		m := map[string]any{
			"name": pp.Name, "type": pp.Type,
			"required": pp.Required, "description": pp.Description,
		}
		if pp.Default != nil {
			m["default"] = *pp.Default
		} else {
			m["default"] = nil
		}
		params[i] = m
	}
	pb, _ := json.Marshal(params)
	rb, _ := json.Marshal(map[string]string{"type": p.Return.Type, "description": p.Return.Description})
	return parsedFields{parametersJSON: string(pb), returnSchemaJSON: string(rb)}, nil
}

func (s *Service) autoRejectPending(ctx context.Context, toolID string) error {
	v, err := s.repo.GetActivePending(ctx, toolID)
	if errors.Is(err, tooldomain.ErrPendingNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("toolapp.autoRejectPending: %w", err)
	}
	return s.repo.UpdateVersionStatus(ctx, v.ID, tooldomain.VersionStatusRejected, nil)
}

func (s *Service) trimVersions(ctx context.Context, toolID string) error {
	n, err := s.repo.CountAcceptedVersions(ctx, toolID)
	if err != nil {
		return fmt.Errorf("toolapp.trimVersions: %w", err)
	}
	if n > tooldomain.MaxAcceptedVersions {
		return s.repo.DeleteOldestAcceptedVersion(ctx, toolID)
	}
	return nil
}

func newVersion(t *tooldomain.Tool, status string, version *int, message string) *tooldomain.ToolVersion {
	now := time.Now().UTC()
	return &tooldomain.ToolVersion{
		ID:           newID("tv"),
		ToolID:       t.ID,
		UserID:       t.UserID,
		Version:      version,
		Status:       status,
		Name:         t.Name,
		Description:  t.Description,
		Code:         t.Code,
		Parameters:   t.Parameters,
		ReturnSchema: t.ReturnSchema,
		Tags:         t.Tags,
		Message:      message,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func tagsJSON(tags []string) string {
	if tags == nil {
		tags = []string{}
	}
	b, _ := json.Marshal(tags)
	return string(b)
}

func mustSetUserID(ctx context.Context, t *tooldomain.Tool) error {
	uid, ok := reqctxpkg.GetUserID(ctx)
	if !ok {
		return fmt.Errorf("toolapp: missing user id in context")
	}
	t.UserID = uid
	return nil
}

// extractJSONFromLLM strips markdown code fences that LLMs often wrap around
// JSON responses, then finds the outermost JSON object or array.
// Returns the original string unchanged if no JSON delimiter is found.
func extractJSONFromLLM(s string) string {
	s = strings.TrimSpace(s)
	// Strip ```json ... ``` or ``` ... ``` fences.
	for _, fence := range []string{"```json\n", "```\n", "```json", "```"} {
		if after, ok := strings.CutPrefix(s, fence); ok {
			s = after
			if idx := strings.LastIndex(s, "```"); idx >= 0 {
				s = s[:idx]
			}
			s = strings.TrimSpace(s)
			break
		}
	}
	// Find outermost { } or [ ].
	for _, pair := range [][2]byte{{'{', '}'}, {'[', ']'}} {
		start := strings.IndexByte(s, pair[0])
		end := strings.LastIndexByte(s, pair[1])
		if start >= 0 && end > start {
			return s[start : end+1]
		}
	}
	return s
}

func uidFromTool(t *tooldomain.Tool) (string, bool) {
	return t.UserID, t.UserID != ""
}

func buildGeneratePrompt(t *tooldomain.Tool, count int) string {
	return fmt.Sprintf(`Analyze this Python function and generate test cases.

Function name: %s
Description: %s
Code:
%s

If the function depends on external state (file paths, network, randomness, side effects),
respond with: {"not_supported": true, "reason": "<explanation>"}

Otherwise, generate %d diverse test cases and respond with:
{"test_cases": [{"name": "<name>", "input": <json_object>, "expected_output": <json_value>}, ...]}

Respond with valid JSON only, no explanation.`,
		t.Name, t.Description, t.Code, count)
}
