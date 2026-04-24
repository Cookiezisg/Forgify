// service_test.go — unit tests for Service using a fake Repository +
// fake ConnectivityTester + real AES-GCM Encryptor. Real crypto is used
// (not a mock) so encrypt/decrypt wiring is exercised end-to-end.
//
// service_test.go — Service 单测：fake Repository + fake ConnectivityTester +
// 真 AES-GCM Encryptor。用真 crypto 不是为了慢，而是端到端跑通加解密接线。

package apikey

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
	infracrypto "github.com/sunweilin/forgify/backend/internal/infra/crypto"
	"github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// fakeRepo is an in-memory apikeydomain.Repository for unit tests.
//
// fakeRepo 是单测用的内存版 apikeydomain.Repository。
type fakeRepo struct {
	items map[string]*apikeydomain.APIKey
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{items: map[string]*apikeydomain.APIKey{}}
}

func (r *fakeRepo) Get(ctx context.Context, id string) (*apikeydomain.APIKey, error) {
	uid, _ := reqctx.GetUserID(ctx)
	k, ok := r.items[id]
	if !ok || k.UserID != uid {
		return nil, apikeydomain.ErrNotFound
	}
	return k, nil
}

func (r *fakeRepo) List(ctx context.Context, filter apikeydomain.ListFilter) ([]*apikeydomain.APIKey, string, error) {
	uid, _ := reqctx.GetUserID(ctx)
	out := []*apikeydomain.APIKey{}
	for _, k := range r.items {
		if k.UserID != uid {
			continue
		}
		if filter.Provider != "" && k.Provider != filter.Provider {
			continue
		}
		out = append(out, k)
	}
	return out, "", nil
}

func (r *fakeRepo) GetByProvider(ctx context.Context, provider string) (*apikeydomain.APIKey, error) {
	uid, _ := reqctx.GetUserID(ctx)
	// Prefer test_status=ok; otherwise most recent created_at.
	// 优先 test_status=ok，否则 created_at 最新。
	var best *apikeydomain.APIKey
	for _, k := range r.items {
		if k.UserID != uid || k.Provider != provider {
			continue
		}
		if best == nil ||
			(k.TestStatus == apikeydomain.TestStatusOK && best.TestStatus != apikeydomain.TestStatusOK) ||
			(k.TestStatus == best.TestStatus && k.CreatedAt.After(best.CreatedAt)) {
			best = k
		}
	}
	if best == nil {
		return nil, apikeydomain.ErrNotFoundForProvider
	}
	return best, nil
}

func (r *fakeRepo) Save(ctx context.Context, k *apikeydomain.APIKey) error {
	r.items[k.ID] = k
	return nil
}

func (r *fakeRepo) Delete(ctx context.Context, id string) error {
	if _, ok := r.items[id]; !ok {
		return apikeydomain.ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *fakeRepo) UpdateTestResult(ctx context.Context, id, status, errMsg string) error {
	k, ok := r.items[id]
	if !ok {
		return apikeydomain.ErrNotFound
	}
	k.TestStatus = status
	k.TestError = errMsg
	now := time.Now().UTC()
	k.LastTestedAt = &now
	return nil
}

// fakeTester returns pre-canned results and records call count.
//
// fakeTester 返回预设结果并记录调用次数。
type fakeTester struct {
	result *TestResult
	err    error
	calls  int
}

func (t *fakeTester) Test(ctx context.Context, provider, key, baseURL, apiFormat string) (*TestResult, error) {
	t.calls++
	return t.result, t.err
}

// newTestService builds a Service with fake repo + fake tester + real AES-GCM encryptor.
//
// newTestService 构造带 fake repo + fake tester + 真 AES-GCM encryptor 的 Service。
func newTestService(t *testing.T, tester ConnectivityTester) (*Service, *fakeRepo) {
	t.Helper()
	enc, err := infracrypto.NewAESGCMEncryptor(infracrypto.DeriveKey("service-test-fixture"))
	if err != nil {
		t.Fatalf("NewAESGCMEncryptor: %v", err)
	}
	repo := newFakeRepo()
	svc := NewService(repo, enc, tester, zaptest.NewLogger(t))
	return svc, repo
}

func ctxFor(userID string) context.Context {
	return reqctx.SetUserID(context.Background(), userID)
}

// ---- Create ----

func TestService_Create_Success(t *testing.T) {
	svc, repo := newTestService(t, &fakeTester{})
	ctx := ctxFor("u-alice")

	k, err := svc.Create(ctx, CreateInput{
		Provider:    "openai",
		DisplayName: "Main OpenAI",
		Key:         "sk-proj-abcdefg1234567890xyz",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if !strings.HasPrefix(k.ID, "aki_") {
		t.Errorf("ID = %q, want prefix aki_", k.ID)
	}
	if k.UserID != "u-alice" {
		t.Errorf("UserID = %q, want u-alice", k.UserID)
	}
	if k.KeyMasked != "sk-proj...0xyz" {
		t.Errorf("KeyMasked = %q, want sk-proj...0xyz", k.KeyMasked)
	}
	if k.KeyEncrypted == "" || k.KeyEncrypted == "sk-proj-abcdefg1234567890xyz" {
		t.Errorf("KeyEncrypted = %q, want non-empty ciphertext different from plaintext", k.KeyEncrypted)
	}
	if !strings.HasPrefix(k.KeyEncrypted, "v1:") {
		t.Errorf("KeyEncrypted = %q, want v1: prefix (AES-GCM format)", k.KeyEncrypted)
	}
	if k.TestStatus != apikeydomain.TestStatusPending {
		t.Errorf("TestStatus = %q, want pending", k.TestStatus)
	}
	if _, ok := repo.items[k.ID]; !ok {
		t.Error("repo did not store the new key")
	}
}

func TestService_Create_ValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		in   CreateInput
		want error
	}{
		{"unknown provider", CreateInput{Provider: "notreal", Key: "k"}, apikeydomain.ErrInvalidProvider},
		{"empty key", CreateInput{Provider: "openai", Key: "  "}, apikeydomain.ErrKeyRequired},
		{"ollama missing baseURL", CreateInput{Provider: "ollama", Key: "k"}, apikeydomain.ErrBaseURLRequired},
		{"custom missing apiFormat", CreateInput{Provider: "custom", Key: "k", BaseURL: "http://x"}, apikeydomain.ErrAPIFormatRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc, repo := newTestService(t, &fakeTester{})
			_, err := svc.Create(ctxFor("u"), c.in)
			if !errors.Is(err, c.want) {
				t.Errorf("err = %v, want to wrap %v", err, c.want)
			}
			if len(repo.items) != 0 {
				t.Errorf("repo got %d items on validation error, want 0", len(repo.items))
			}
		})
	}
}

func TestService_Create_MissingUserID_Errors(t *testing.T) {
	// Hitting Create without InjectUserID middleware is a server wiring bug.
	// 没经过 InjectUserID 就调 Create 是服务端接线 bug。
	svc, _ := newTestService(t, &fakeTester{})
	_, err := svc.Create(context.Background(), CreateInput{Provider: "openai", Key: "sk-x"})
	if err == nil {
		t.Fatal("want error when userID missing")
	}
	if !strings.Contains(err.Error(), "missing user id") {
		t.Errorf("err = %v, want message about missing user id", err)
	}
}

func TestService_Create_UsesCustomBaseURLAndAPIFormat(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	k, err := svc.Create(ctxFor("u"), CreateInput{
		Provider:  "custom",
		Key:       "sk-x",
		BaseURL:   "https://proxy.example.com/v1",
		APIFormat: apikeydomain.APIFormatAnthropicCompatible,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if k.BaseURL != "https://proxy.example.com/v1" {
		t.Errorf("BaseURL = %q", k.BaseURL)
	}
	if k.APIFormat != apikeydomain.APIFormatAnthropicCompatible {
		t.Errorf("APIFormat = %q", k.APIFormat)
	}
}

// ---- Update ----

func TestService_Update_PartialFields(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	ctx := ctxFor("u")
	created, err := svc.Create(ctx, CreateInput{Provider: "openai", DisplayName: "Old", Key: "sk-x"})
	if err != nil {
		t.Fatalf("seed Create: %v", err)
	}

	newName := "New Display"
	updated, err := svc.Update(ctx, created.ID, UpdateInput{DisplayName: &newName})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.DisplayName != "New Display" {
		t.Errorf("DisplayName = %q, want New Display", updated.DisplayName)
	}
	if updated.BaseURL != created.BaseURL {
		t.Errorf("BaseURL changed from %q to %q, want unchanged", created.BaseURL, updated.BaseURL)
	}
	if updated.UpdatedAt.Before(created.UpdatedAt) {
		t.Errorf("UpdatedAt did not advance")
	}
}

func TestService_Update_NotFound(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	name := "x"
	_, err := svc.Update(ctxFor("u"), "nonexistent", UpdateInput{DisplayName: &name})
	if !errors.Is(err, apikeydomain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// ---- Delete ----

func TestService_Delete_RemovesEntry(t *testing.T) {
	svc, repo := newTestService(t, &fakeTester{})
	ctx := ctxFor("u")
	k, _ := svc.Create(ctx, CreateInput{Provider: "openai", Key: "sk-x"})

	if err := svc.Delete(ctx, k.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok := repo.items[k.ID]; ok {
		t.Error("repo still has entry after Delete")
	}
}

func TestService_Delete_NotFound(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	err := svc.Delete(ctxFor("u"), "nope")
	if !errors.Is(err, apikeydomain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// ---- Test (connectivity) ----

func TestService_Test_Success_WritesOKStatus(t *testing.T) {
	tester := &fakeTester{result: &TestResult{OK: true, Message: "ok", LatencyMs: 42}}
	svc, repo := newTestService(t, tester)
	ctx := ctxFor("u")
	k, _ := svc.Create(ctx, CreateInput{Provider: "openai", Key: "sk-x"})

	res, err := svc.Test(ctx, k.ID)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	if !res.OK || res.LatencyMs != 42 {
		t.Errorf("result = %+v, want OK=true LatencyMs=42", res)
	}
	if tester.calls != 1 {
		t.Errorf("tester calls = %d, want 1", tester.calls)
	}
	stored := repo.items[k.ID]
	if stored.TestStatus != apikeydomain.TestStatusOK {
		t.Errorf("TestStatus = %q, want ok", stored.TestStatus)
	}
	if stored.TestError != "" {
		t.Errorf("TestError = %q, want empty on success", stored.TestError)
	}
	if stored.LastTestedAt == nil {
		t.Error("LastTestedAt still nil after successful test")
	}
}

func TestService_Test_Failure_WritesErrorStatus(t *testing.T) {
	tester := &fakeTester{result: &TestResult{OK: false, Message: "HTTP 401: invalid"}}
	svc, repo := newTestService(t, tester)
	ctx := ctxFor("u")
	k, _ := svc.Create(ctx, CreateInput{Provider: "openai", Key: "sk-x"})

	res, err := svc.Test(ctx, k.ID)
	if err != nil {
		t.Fatalf("Test: %v (non-OK result must not be a Go error)", err)
	}
	if res.OK {
		t.Error("result.OK = true, want false")
	}
	stored := repo.items[k.ID]
	if stored.TestStatus != apikeydomain.TestStatusError {
		t.Errorf("TestStatus = %q, want error", stored.TestStatus)
	}
	if stored.TestError != "HTTP 401: invalid" {
		t.Errorf("TestError = %q, want 'HTTP 401: invalid'", stored.TestError)
	}
}

func TestService_Test_TesterProgrammerBug_RecordedAndPropagated(t *testing.T) {
	// Tester returning a real error is a programmer bug (unknown provider etc).
	// Service should record it and propagate the error.
	// Tester 返回真 error 是程序 bug（未知 provider 等）。Service 应记录并向上传。
	tester := &fakeTester{err: errors.New("unknown provider")}
	svc, repo := newTestService(t, tester)
	ctx := ctxFor("u")
	k, _ := svc.Create(ctx, CreateInput{Provider: "openai", Key: "sk-x"})

	_, err := svc.Test(ctx, k.ID)
	if err == nil {
		t.Fatal("want error when tester returns error")
	}
	if repo.items[k.ID].TestStatus != apikeydomain.TestStatusError {
		t.Error("test_status not recorded as error on programmer-bug path")
	}
}

func TestService_Test_NotFound(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	_, err := svc.Test(ctxFor("u"), "nope")
	if !errors.Is(err, apikeydomain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// ---- Get / List ----

func TestService_List_FiltersByProvider(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	ctx := ctxFor("u")
	_, _ = svc.Create(ctx, CreateInput{Provider: "openai", Key: "sk-x"})
	_, _ = svc.Create(ctx, CreateInput{Provider: "anthropic", Key: "sk-ant-y"})

	got, _, err := svc.List(ctx, apikeydomain.ListFilter{Provider: "openai"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Provider != "openai" {
		t.Errorf("got %d items (provider %v), want exactly 1 openai", len(got), providersOf(got))
	}
}

func providersOf(ks []*apikeydomain.APIKey) []string {
	out := make([]string, len(ks))
	for i, k := range ks {
		out[i] = k.Provider
	}
	return out
}

// ---- KeyProvider interface (ResolveCredentials / MarkInvalid) ----

func TestService_ResolveCredentials_DecryptsAndMergesDefaultBaseURL(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	ctx := ctxFor("u")
	_, _ = svc.Create(ctx, CreateInput{Provider: "openai", Key: "sk-plaintext"})

	creds, err := svc.ResolveCredentials(ctx, "openai")
	if err != nil {
		t.Fatalf("ResolveCredentials: %v", err)
	}
	if creds.Key != "sk-plaintext" {
		t.Errorf("Key = %q, want sk-plaintext (decryption failed)", creds.Key)
	}
	want := "https://api.openai.com/v1"
	if creds.BaseURL != want {
		t.Errorf("BaseURL = %q, want %q (provider default)", creds.BaseURL, want)
	}
}

func TestService_ResolveCredentials_UserBaseURLOverridesDefault(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	ctx := ctxFor("u")
	_, _ = svc.Create(ctx, CreateInput{
		Provider: "openai",
		Key:      "sk-x",
		BaseURL:  "https://custom.proxy/v1",
	})

	creds, err := svc.ResolveCredentials(ctx, "openai")
	if err != nil {
		t.Fatalf("ResolveCredentials: %v", err)
	}
	if creds.BaseURL != "https://custom.proxy/v1" {
		t.Errorf("BaseURL = %q, want user-supplied override", creds.BaseURL)
	}
}

func TestService_ResolveCredentials_NoKeyForProvider(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	_, err := svc.ResolveCredentials(ctxFor("u"), "openai")
	if !errors.Is(err, apikeydomain.ErrNotFoundForProvider) {
		t.Errorf("err = %v, want ErrNotFoundForProvider", err)
	}
}

func TestService_MarkInvalid_UpdatesTestResult(t *testing.T) {
	svc, repo := newTestService(t, &fakeTester{})
	ctx := ctxFor("u")
	k, _ := svc.Create(ctx, CreateInput{Provider: "openai", Key: "sk-x"})

	if err := svc.MarkInvalid(ctx, "openai", "chat returned 401"); err != nil {
		t.Fatalf("MarkInvalid: %v", err)
	}
	stored := repo.items[k.ID]
	if stored.TestStatus != apikeydomain.TestStatusError {
		t.Errorf("TestStatus = %q, want error", stored.TestStatus)
	}
	if stored.TestError != "chat returned 401" {
		t.Errorf("TestError = %q, want 'chat returned 401'", stored.TestError)
	}
}

func TestService_MarkInvalid_NoKeyForProvider(t *testing.T) {
	svc, _ := newTestService(t, &fakeTester{})
	err := svc.MarkInvalid(ctxFor("u"), "openai", "nope")
	if !errors.Is(err, apikeydomain.ErrNotFoundForProvider) {
		t.Errorf("err = %v, want ErrNotFoundForProvider", err)
	}
}

// ---- DI guard ----

func TestNewService_NilLogger_Panics(t *testing.T) {
	// A nil logger is a wiring bug — fail loud at boot, not in prod log sites.
	// nil logger 是接线 bug——启动时响亮失败，而不是在生产 log 处炸。
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewService did not panic on nil logger")
		}
	}()
	_ = NewService(newFakeRepo(), nil, &fakeTester{}, nil)
}
