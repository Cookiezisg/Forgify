// service.go — apikey.Service: input validation, encryption, persistence,
// connectivity testing. The HTTP handler's sole entry point.
//
// service.go — apikey.Service：校验输入、加密、持久化、连通性测试。
// HTTP handler 的唯一入口。

package apikey

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	"github.com/sunweilin/forgify/backend/internal/domain/crypto"
	"github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// Service orchestrates apikey CRUD + connectivity testing. It owns the
// encryption boundary — callers never see plaintext or ciphertext, only
// the APIKey entity (ciphertext hidden by `json:"-"`) and TestResult.
//
// Service 编排 apikey 的 CRUD + 连通性测试。持有加密边界——调用方既看
// 不到明文也看不到密文，只拿到 APIKey（密文由 json:"-" 隐藏）和 TestResult。
type Service struct {
	repo      apikeydomain.Repository
	encryptor crypto.Encryptor
	tester    ConnectivityTester
	log       *zap.Logger
}

// NewService wires Service dependencies. Panics on nil logger — a nil
// logger is a wiring bug, not a runtime condition.
//
// NewService 装配 Service 依赖。nil logger 会 panic——nil logger 是接线
// bug，不是运行时状态。
func NewService(repo apikeydomain.Repository, enc crypto.Encryptor, tester ConnectivityTester, log *zap.Logger) *Service {
	if log == nil {
		panic("apikey.NewService: logger is nil")
	}
	return &Service{repo: repo, encryptor: enc, tester: tester, log: log}
}

// CreateInput is the validated request for Service.Create.
//
// CreateInput 是 Service.Create 的已校验请求形状。
type CreateInput struct {
	Provider    string
	DisplayName string
	Key         string
	BaseURL     string
	APIFormat   string
}

// UpdateInput is the partial-update payload for Service.Update. nil
// fields are left unchanged; a non-nil pointer to "" clears the value.
// Key / Provider / APIFormat are intentionally absent — changing them
// means delete + recreate (cleaner audit, avoids stale test_status).
//
// UpdateInput 是 Service.Update 的部分更新载荷。nil 字段不改；指向 "" 的
// 非 nil 指针清空该值。故意不含 Key / Provider / APIFormat——改它们
// 意味着 delete + recreate（审计更清晰，避免过期的 test_status）。
type UpdateInput struct {
	DisplayName *string
	BaseURL     *string
}

// Create validates, encrypts, and persists a new APIKey. TestStatus is
// set to pending — a connectivity test is a separate explicit action.
//
// Create 校验、加密、持久化一条新 APIKey。TestStatus 初始为 pending——
// 连通性测试是独立的显式动作。
func (s *Service) Create(ctx context.Context, in CreateInput) (*apikeydomain.APIKey, error) {
	if err := validateCreate(in); err != nil {
		return nil, err
	}
	uid, ok := reqctx.GetUserID(ctx)
	if !ok {
		return nil, fmt.Errorf("apikey.Service.Create: missing user id in context")
	}

	ciphertext, err := s.encryptor.Encrypt(ctx, []byte(in.Key))
	if err != nil {
		return nil, fmt.Errorf("apikey.Service.Create: encrypt: %w", err)
	}

	now := time.Now().UTC()
	k := &apikeydomain.APIKey{
		ID:           newID(),
		UserID:       uid,
		Provider:     in.Provider,
		DisplayName:  strings.TrimSpace(in.DisplayName),
		KeyEncrypted: string(ciphertext),
		KeyMasked:    MaskKey(in.Key),
		BaseURL:      strings.TrimSpace(in.BaseURL),
		APIFormat:    in.APIFormat,
		TestStatus:   apikeydomain.TestStatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.Save(ctx, k); err != nil {
		return nil, err
	}
	s.log.Info("apikey created",
		zap.String("key_id", k.ID),
		zap.String("user_id", uid),
		zap.String("provider", k.Provider))
	return k, nil
}

// validateCreate applies the Phase 2 rules: provider in whitelist, key
// non-empty, baseURL required for providers that demand it, apiFormat
// required for custom. Key itself is not format-validated — provider
// decides shape at connectivity test time.
//
// validateCreate 应用 Phase 2 规则：provider 在白名单、key 非空、
// 特定 provider 必填 baseURL、custom 必填 apiFormat。key 本身不校验格式——
// 形状由 provider 在连通性测试时判断。
func validateCreate(in CreateInput) error {
	if !apikeydomain.IsValidProvider(in.Provider) {
		return fmt.Errorf("provider %q: %w", in.Provider, apikeydomain.ErrInvalidProvider)
	}
	if strings.TrimSpace(in.Key) == "" {
		return apikeydomain.ErrKeyRequired
	}
	meta, _ := apikeydomain.GetProviderMeta(in.Provider)
	if meta.BaseURLRequired && strings.TrimSpace(in.BaseURL) == "" {
		return apikeydomain.ErrBaseURLRequired
	}
	if in.Provider == "custom" && strings.TrimSpace(in.APIFormat) == "" {
		return apikeydomain.ErrAPIFormatRequired
	}
	return nil
}

// Update applies a partial update. Missing fields stay untouched.
//
// Update 做部分更新。未提供的字段保持不变。
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*apikeydomain.APIKey, error) {
	k, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if in.DisplayName != nil {
		k.DisplayName = strings.TrimSpace(*in.DisplayName)
	}
	if in.BaseURL != nil {
		k.BaseURL = strings.TrimSpace(*in.BaseURL)
	}
	k.UpdatedAt = time.Now().UTC()
	if err := s.repo.Save(ctx, k); err != nil {
		return nil, err
	}
	return k, nil
}

// Delete soft-deletes by id (scoped to caller).
//
// Delete 按 id 软删除（按调用者过滤）。
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// Get fetches one APIKey.
//
// Get 取一条 APIKey。
func (s *Service) Get(ctx context.Context, id string) (*apikeydomain.APIKey, error) {
	return s.repo.Get(ctx, id)
}

// List returns a paginated page of the caller's APIKeys.
//
// List 返回调用者 APIKey 的一页（分页）。
func (s *Service) List(ctx context.Context, filter apikeydomain.ListFilter) ([]*apikeydomain.APIKey, string, error) {
	return s.repo.List(ctx, filter)
}

// Test fetches the APIKey, decrypts, probes the upstream, writes the
// outcome back, and returns the TestResult. The DB write is Service's
// job, not Tester's — Tester is a stateless probe.
//
// Test 取回 APIKey、解密、探测上游、写回结果、返回 TestResult。
// 写表是 Service 的职责，不是 Tester 的——Tester 是无状态探针。
func (s *Service) Test(ctx context.Context, id string) (*TestResult, error) {
	k, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	plain, err := s.encryptor.Decrypt(ctx, []byte(k.KeyEncrypted))
	if err != nil {
		return nil, fmt.Errorf("apikey.Service.Test: decrypt: %w", err)
	}
	result, err := s.tester.Test(ctx, k.Provider, string(plain), k.BaseURL, k.APIFormat)
	if err != nil {
		// Programmer-bug path (unknown provider, missing required baseURL).
		// Record as test error so the UI shows something useful.
		//
		// 程序 bug 路径（未知 provider、必填 baseURL 缺失）。记为测试失败，
		// UI 才有东西可展示。
		_ = s.repo.UpdateTestResult(ctx, id, apikeydomain.TestStatusError, err.Error())
		return nil, fmt.Errorf("apikey.Service.Test: tester: %w", err)
	}

	status := apikeydomain.TestStatusError
	errMsg := result.Message
	if result.OK {
		status = apikeydomain.TestStatusOK
		errMsg = ""
	}
	if upErr := s.repo.UpdateTestResult(ctx, id, status, errMsg); upErr != nil {
		return nil, upErr
	}
	s.log.Info("apikey tested",
		zap.String("key_id", id),
		zap.String("provider", k.Provider),
		zap.Bool("ok", result.OK),
		zap.Int64("latency_ms", result.LatencyMs))
	return result, nil
}

// newID mints "aki_" + 16 hex chars (64 bits of entropy). Collision risk
// at 100M keys per user is negligible (~10^-9).
//
// newID 生成 "aki_" + 16 hex（64 位熵）。单用户 1 亿条 key 的碰撞概率
// 可忽略（约 10^-9）。
func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure means the OS RNG is broken — a panic is
		// the correct response; the server cannot safely mint IDs.
		//
		// crypto/rand 失败意味着 OS RNG 故障——panic 是正解，服务端已无法
		// 安全地生成 ID。
		panic(fmt.Sprintf("apikey: crypto/rand failed: %v", err))
	}
	return "aki_" + hex.EncodeToString(b[:])
}
