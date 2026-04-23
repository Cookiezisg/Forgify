# apikey domain — 综合设计文档

**所属 Phase**：Phase 2（基础对话能力，第一站）
**职责**：管理用户的 LLM provider API Key 凭证（存储、加密、测试连通性）
**依赖**：`domain/crypto`（加密）+ `domain/events`（可选，Key 失效时可推送）
**被依赖**：`chat`、`model`、工作流 LLM 节点、知识库 embedding 等所有需要调用 LLM 的地方

---

## 1. 为什么要这个 domain

所有 LLM 调用都需要凭证。用户使用 Forgify 前必须配一个或多个 API Key：
- OpenAI / Anthropic / DeepSeek / Ollama / ...

本 domain 负责：
1. **安全存储** Key（加密）
2. **列出** Key（给用户展示，带掩码）
3. **测试** Key 能不能用（连通性测试）
4. **提供** Key 给其他 domain 消费（LLM 调用时拿到明文 Key）

---

## 2. 核心决策（已敲定）

| 决策 | 选择 | 理由 |
|---|---|---|
| apikey vs model 是否分 domain | **分离** | apikey 管凭证，model 管"哪个场景用哪个模型"策略，职责清晰 |
| 多租户 user_id | **从 V1 就引入** | 每张表带 `user_id`，Phase 2 暂时硬编码 `local-user`，未来加 auth 时只需改 middleware |
| Provider 列表 | **硬编码白名单** | 11 个 + CHECK 约束，新 provider 需要代码改动（适配 base_url / 测试逻辑） |
| base_url 校验 | **service 层** | 不进 schema，灵活性高 |
| Key 失效反馈 | **标记 + 事件** | `test_status=error` + 流式响应推 `chat.error` |

---

## 3. 多租户准备（跨 domain 决策，apikey 首次落地）

> ⚠️ 这是 **项目级约定**，不只是 apikey 特有。其他 domain 照此办理。

### 设计
- **每张业务表**都有 `user_id TEXT NOT NULL` 列
- Phase 2 单用户阶段：全部填 `"local-user"` 字符串常量
- 未来加 auth 时：替换中间件让 ctx 携带真实 userID，业务代码零改

### 实现套路

```go
// infra/transport/httpapi/middleware/auth.go（Phase 2 写这个）
const DefaultLocalUserID = "local-user"

func InjectUserID(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Phase 2：硬编码 local-user
        // Phase N（加 auth 后）：从 JWT/session 读真实 userID
        ctx := context.WithValue(r.Context(), ctxKeyUserID, DefaultLocalUserID)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

// 业务代码拿 userID
userID, _ := r.Context().Value(ctxKeyUserID).(string)
```

### Repository / Service 方法签名
所有方法**必须接收 `ctx`**，从 ctx 里取 user_id 做过滤 / 写入：

```go
// domain/apikey/repository.go
type Repository interface {
    Get(ctx context.Context, id string) (*APIKey, error)
    // repo 内部 WHERE user_id = {ctx 里的 user_id}
}
```

---

## 4. Provider 清单（11 个）

| Provider | 分类 | base_url 必填 | 默认 base_url | 测试方式 |
|---|---|---|---|---|
| `openai` | 国际 | 否 | `https://api.openai.com/v1` | GET `/models` |
| `anthropic` | 国际 | 否 | `https://api.anthropic.com` | POST `/v1/messages`（1 token，约 $0.0001）|
| `google` | 国际 | 否 | `https://generativelanguage.googleapis.com` | GET `/v1beta/models` |
| `deepseek` | 国产/OpenAI 兼容 | 否 | `https://api.deepseek.com` | GET `/models` |
| `openrouter` | 国际聚合 | 否 | `https://openrouter.ai/api/v1` | GET `/models` |
| `qwen` | 国产（阿里）| 否 | `https://dashscope.aliyuncs.com/compatible-mode/v1` | GET `/models` |
| `zhipu` | 国产（智谱）| 否 | `https://open.bigmodel.cn/api/paas/v4` | GET `/models` |
| `moonshot` | 国产（Kimi）| 否 | `https://api.moonshot.cn/v1` | GET `/models` |
| `doubao` | 国产（字节）| 否 | `https://ark.cn-beijing.volces.com/api/v3` | GET `/models` |
| `ollama` | 本地 | **✅ 必填** | — | GET `{base_url}/api/tags` |
| `custom` | 兜底 | **✅ 必填** | — | 按 `api_format` 走（见下）|

### `custom` provider 补充规则
`provider='custom'` 时还必须填：
- `base_url`（必填）
- `api_format`：`openai-compatible` / `anthropic-compatible`（默认 openai-compatible）

测试时按 api_format 走对应逻辑。

### Provider 元数据代码位置
```go
// domain/apikey/providers.go
type ProviderMeta struct {
    Name           string
    DisplayName    string    // UI 显示名
    DefaultBaseURL string
    BaseURLRequired bool
    TestMethod     string    // "models" | "messages" | "tags"
}

var providers = map[string]ProviderMeta{
    "openai":    {Name: "openai", DisplayName: "OpenAI", DefaultBaseURL: "https://api.openai.com/v1", TestMethod: "models"},
    // ...
}
```

---

## 5. 领域模型

### APIKey 结构

```go
// domain/apikey/types.go
package apikey

import (
    "time"
    "gorm.io/gorm"
)

type APIKey struct {
    ID           string         `gorm:"primaryKey;type:text" json:"id"`
    UserID       string         `gorm:"not null;index;type:text" json:"userId"`
    Provider     string         `gorm:"not null;type:text" json:"provider"`
    DisplayName  string         `gorm:"not null;type:text;default:''" json:"displayName"`
    KeyEncrypted string         `gorm:"not null;type:text" json:"-"`           // 永不返回
    KeyMasked    string         `gorm:"not null;type:text" json:"keyMasked"`   // 展示给前端
    BaseURL      string         `gorm:"type:text;default:''" json:"baseUrl"`
    APIFormat    string         `gorm:"type:text;default:''" json:"apiFormat"` // custom 专用
    TestStatus   string         `gorm:"type:text;default:'pending'" json:"testStatus"`
    TestError    string         `gorm:"type:text;default:''" json:"testError"` // 测试失败原因
    LastTestedAt *time.Time     `gorm:"" json:"lastTestedAt"`
    CreatedAt    time.Time      `gorm:"" json:"createdAt"`
    UpdatedAt    time.Time      `gorm:"" json:"updatedAt"`
    DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}
```

**字段说明**：
| 字段 | 说明 |
|---|---|
| `ID` | UUID，业务主键 |
| `UserID` | 用户 ID（Phase 2 固定 `"local-user"`）|
| `Provider` | provider 标识，来自白名单 |
| `DisplayName` | 用户自定义别名，如"我的 OpenAI 主号" |
| `KeyEncrypted` | 加密后的 Key（`v1:` 前缀 + AES-GCM）|
| `KeyMasked` | 掩码展示（如 `sk-proj...abc4`），冗余存储加速列表查询 |
| `BaseURL` | 自定义 endpoint（非官方用法），空则用 provider 默认 |
| `APIFormat` | `custom` provider 的协议类型 |
| `TestStatus` | `pending` / `ok` / `error` |
| `TestError` | 最近一次测试失败原因（成功时清空）|
| `LastTestedAt` | 最近测试时间 |

### 常量和枚举

```go
// domain/apikey/constants.go
const (
    TestStatusPending = "pending"
    TestStatusOK      = "ok"
    TestStatusError   = "error"
    
    APIFormatOpenAICompatible    = "openai-compatible"
    APIFormatAnthropicCompatible = "anthropic-compatible"
)
```

### 域错误

```go
// domain/apikey/errors.go
var (
    ErrNotFound              = errors.New("apikey: not found")
    ErrInvalidProvider       = errors.New("apikey: invalid provider")
    ErrBaseURLRequired       = errors.New("apikey: base_url required for this provider")
    ErrAPIFormatRequired     = errors.New("apikey: api_format required for custom provider")
    ErrTestFailed            = errors.New("apikey: connectivity test failed")
    ErrKeyRequired           = errors.New("apikey: key value is required")
    ErrInvalid               = errors.New("apikey: key is invalid (401 from provider)")
)
```

---

## 6. 对外 API vs 对内函数（速查表）

本节**先给鸟瞰图**，让实现和消费方一眼看清边界。详细细节在后续章节。

### 6.1 对外 API（两类消费者，两套接口）

| 消费者 | 接口 | 位置 | 方法数 | 用途 |
|---|---|---|---|---|
| 🌐 **前端 / 外部客户端** | HTTP REST API | `/api/v1/api-keys/*` | **5 个端点** | 管理 Key：增删改查测 |
| 🧩 **其他 domain**（chat / workflow / knowledge）| `apikey.KeyProvider` 接口 | `domain/apikey/provider.go` | **2 个方法** | 拿凭证调 LLM + 回报失效 |

### 6.2 对外接口详情

#### 🌐 HTTP REST API（详见第 10 节）

```
POST   /api/v1/api-keys              创建
GET    /api/v1/api-keys              列表（分页、按 provider 过滤）
PATCH  /api/v1/api-keys/{id}         更新 display_name / base_url
DELETE /api/v1/api-keys/{id}         软删
POST   /api/v1/api-keys/{id}:test    测试连通性
```

**特点**：CRUD 风格，用户能看到的都在这 5 个里。**不**暴露解密后的明文 Key——这是给人看的。

#### 🧩 `apikey.KeyProvider` 接口（给其他 domain 的唯一入口）

```go
// domain/apikey/provider.go
package apikey

// KeyProvider is the cross-domain interface other services (chat, workflow,
// knowledge) use to fetch decrypted credentials for LLM calls. Consumers
// NEVER see the Repository, Service struct, or raw APIKey records — they
// only get ready-to-use Credentials.
//
// KeyProvider 是跨 domain 接口，其他服务（chat、workflow、knowledge）
// 用它获取调 LLM 的明文凭证。消费方**看不到** Repository、Service struct
// 或原始 APIKey 记录——它们只拿到现成可用的 Credentials。
type KeyProvider interface {
    // ResolveCredentials returns a ready-to-use (key, baseURL) pair for
    // the given provider under the current user (from ctx). Internally:
    // picks the best Key (prefer tested-OK, fall back to newest), decrypts,
    // merges baseURL with provider default.
    //
    // ResolveCredentials 为当前用户（从 ctx 取）的给定 provider 返回可用的
    // (key, baseURL) 二元组。内部：挑最佳 Key（优先 tested-OK，否则最新）、
    // 解密、合并 baseURL 与 provider 默认值。
    ResolveCredentials(ctx context.Context, provider string) (Credentials, error)

    // MarkInvalid is the feedback channel consumers call when the LLM
    // returns 401 with the credentials. Updates test_status=error and
    // records the reason so the UI can surface "key invalid" to users.
    //
    // MarkInvalid 是消费方的反馈通道：LLM 用该凭证返回 401 时调用。
    // 更新 test_status=error 并记录原因，UI 可向用户展示 "key 失效"。
    MarkInvalid(ctx context.Context, provider string, reason string) error
}

// Credentials is the bundle returned to LLM-calling consumers.
// Key is plaintext — callers MUST NOT log it, pass it to goroutines that
// outlive the current request, or store it in any persistent state.
//
// Credentials 是返回给 LLM 调用方的凭证包。Key 是明文——调用方**禁止**
// 写日志、传给跨请求存活的 goroutine、或落入任何持久化存储。
type Credentials struct {
    Key     string // plaintext, short-lived / 明文，短生命周期
    BaseURL string // resolved (default if user didn't override) / 已合并（用户未自定义则用默认）
}
```

**设计原则**：
- 消费方**只 import `domain/apikey` 接口**，看不到 Service struct、Repository、encryption 细节
- 实现由 `app/apikey/Service` 满足（`var _ apikey.KeyProvider = (*Service)(nil)` 编译期守护）
- 未来 apikey 换 KMS 实现，chat/workflow 代码**零改动**

### 6.3 对内函数 / 类型

| 类别 | 名字 | 位置 | 谁调用它 |
|---|---|---|---|
| **Repository 接口** | `Repository` | `domain/apikey/repository.go` | Service 层调（从 DB 存取）|
| **Repository 实现** | `APIKeyRepo` | `infra/gorm/apikey_repo.go` | main.go DI 时注入 Service |
| **Service struct（完整实现）** | `Service` | `app/apikey/service.go` | HTTP handler 调（CRUD）+ 实现 KeyProvider（供其他 domain）|
| **ConnectivityTester 接口** | `ConnectivityTester` | `app/apikey/tester.go` | Service 内部调（测连通性时）|
| **ConnectivityTester 实现** | `HTTPTester` | `app/apikey/tester_impl.go` | main.go DI 时注入 Service |
| **Provider 元数据** | `providers map` | `domain/apikey/providers.go` | Service 校验 provider / 拿默认 base_url |
| **掩码辅助函数** | `maskKey` | `domain/apikey/mask.go` | Service 创建 Key 时生成展示值 |
| **ProviderMeta struct** | `ProviderMeta` | `domain/apikey/providers.go` | 内部读默认 base_url / test method |

**关键界线**：
- `Repository` 接口是 **apikey domain 内部**的抽象（Service ↔ DB）——**其他 domain 不该 import 它**
- `KeyProvider` 接口是 **apikey 对外**的抽象（给 chat / workflow）——这才是跨 domain 调用的入口
- 这两个接口**都在 `domain/apikey/` 包里**，但用途和可见范围不同

---

## 7. Repository 接口

```go
// domain/apikey/repository.go
type Repository interface {
    // Get 按 id 查。repo 内部过滤 user_id（从 ctx 取）。
    Get(ctx context.Context, id string) (*APIKey, error)
    
    // List 列出当前 user 的所有 Key（不含软删的）。
    List(ctx context.Context, filter ListFilter) ([]*APIKey, string, error)
    // 返回 (keys, nextCursor, error)
    
    // GetByProvider 为 model 策略层提供："当前 user 这个 provider 的首个活跃 Key"
    // 有多个 Key 时返回 LastTestedAt 最近成功的。
    GetByProvider(ctx context.Context, provider string) (*APIKey, error)
    
    // Save 创建或更新（由 ID 是否存在决定）。
    Save(ctx context.Context, k *APIKey) error
    
    // Delete 软删除。
    Delete(ctx context.Context, id string) error
    
    // UpdateTestResult 只更新测试相关字段（TestStatus, TestError, LastTestedAt）
    UpdateTestResult(ctx context.Context, id string, status, errMsg string) error
}

type ListFilter struct {
    Cursor    string
    Limit     int
    Provider  string  // 可选，按 provider 过滤
}
```

---

## 8. Service 层

```go
// app/apikey/service.go
type Service struct {
    repo      apikey.Repository
    encryptor crypto.Encryptor
    tester    ConnectivityTester  // 见第 9 节
    log       *zap.Logger
}

// Create 添加新的 Key。
// 1. 校验 provider 白名单
// 2. 校验 base_url（ollama/custom 必填）
// 3. 校验 api_format（custom 必填）
// 4. 加密 key
// 5. 生成 masked 展示值
// 6. 存 repo
func (s *Service) Create(ctx context.Context, in CreateInput) (*apikey.APIKey, error)

// Update 更新 display_name / base_url（不支持改 key 值，要改就删了重建）
func (s *Service) Update(ctx context.Context, id string, in UpdateInput) (*apikey.APIKey, error)

// Delete 软删除
func (s *Service) Delete(ctx context.Context, id string) error

// List 列表
func (s *Service) List(ctx context.Context, p pagination.Params) ([]*apikey.APIKey, string, error)

// Get 单条
func (s *Service) Get(ctx context.Context, id string) (*apikey.APIKey, error)

// Test 测试连通性
// 1. 从 repo 拿到 Key（含 key_encrypted）
// 2. 解密
// 3. 通过 tester 调 provider 端点
// 4. 更新 test_status / last_tested_at
// 5. 返回结果
func (s *Service) Test(ctx context.Context, id string) (*TestResult, error)

// GetDecryptedKey 内部方法：给其他 domain 用（chat 调 LLM 时）
// 返回明文 Key，调用方需妥善使用（不要日志、不要错误信息里暴露）
func (s *Service) GetDecryptedKey(ctx context.Context, id string) (string, string, error)
// 返回 (key, baseURL, error)

type CreateInput struct {
    Provider    string
    DisplayName string
    Key         string
    BaseURL     string
    APIFormat   string
}

type UpdateInput struct {
    DisplayName *string  // 指针，nil 不更新
    BaseURL     *string
}

type TestResult struct {
    OK           bool
    Message      string
    LatencyMs    int64
    ModelsFound  []string // 可选，如果 provider 返回模型列表
}
```

### 掩码规则

```go
// domain/apikey/mask.go
// maskKey 把 Key 转成掩码展示形式。
// 
// 规则：
//   - 长度 < 8：全部用 * 替代
//   - 长度 8-20：前 3 + "..." + 后 4
//   - 长度 > 20：前 7 + "..." + 后 4
//
// 例：
//   "sk-proj-abcdefg1234567890xyz" → "sk-proj...0xyz"
//   "AKIA1234567890ABCDEF"         → "AKI...CDEF"
func maskKey(key string) string
```

---

## 9. Provider 适配层（ConnectivityTester）

```go
// app/apikey/tester.go
type ConnectivityTester interface {
    Test(ctx context.Context, provider, key, baseURL, apiFormat string) (*TestResult, error)
}

// app/apikey/tester_impl.go
type HTTPTester struct {
    client *http.Client
}

func (t *HTTPTester) Test(ctx context.Context, provider, key, baseURL, apiFormat string) (*TestResult, error) {
    meta := apikey.GetProviderMeta(provider)
    effectiveBaseURL := baseURL
    if effectiveBaseURL == "" {
        effectiveBaseURL = meta.DefaultBaseURL
    }
    
    switch meta.TestMethod {
    case "models":
        return t.testGetModels(ctx, effectiveBaseURL, key)
    case "messages":
        return t.testAnthropicMessage(ctx, effectiveBaseURL, key)
    case "tags":
        return t.testOllamaTags(ctx, effectiveBaseURL)
    }
    // custom 按 apiFormat 走
    if provider == "custom" {
        if apiFormat == apikey.APIFormatAnthropicCompatible {
            return t.testAnthropicMessage(ctx, effectiveBaseURL, key)
        }
        return t.testGetModels(ctx, effectiveBaseURL, key)  // 默认 openai-compatible
    }
    return nil, fmt.Errorf("unknown test method")
}

// testGetModels: GET {baseURL}/models
//   - 带 Authorization: Bearer {key}
//   - 200 → OK，解析 JSON 拿模型列表
//   - 401 → key 无效
//   - 其他 → 网络错误
func (t *HTTPTester) testGetModels(ctx, baseURL, key)

// testAnthropicMessage: POST {baseURL}/v1/messages
//   - header x-api-key: {key}
//   - body: { "model": "claude-3-5-haiku-latest", "max_tokens": 1, "messages": [{"role":"user","content":"hi"}] }
//   - 200 → OK
//   - 401 → 无效
//   - 注意：会产生极小额费用（约 $0.0001）
func (t *HTTPTester) testAnthropicMessage(ctx, baseURL, key)

// testOllamaTags: GET {baseURL}/api/tags
//   - Ollama 本地服务，无需 auth
//   - 200 → OK，返回已安装模型列表
func (t *HTTPTester) testOllamaTags(ctx, baseURL)
```

---

## 10. HTTP API 设计

### 通用约定
- 前缀：`/api/v1/api-keys`
- 所有端点走 auth middleware 注入 user_id
- 响应走 envelope（N1）

### 端点清单（5 个）

#### 9.1 `POST /api/v1/api-keys` — 创建

**Request**：
```json
{
  "provider": "openai",
  "displayName": "My OpenAI Main",
  "key": "sk-proj-xxxxxxxxxxxxxxxx",
  "baseUrl": "",
  "apiFormat": ""
}
```

**校验**：
- `provider` 必填，在白名单内 → 否则 400 `INVALID_PROVIDER`
- `key` 必填 → 否则 400 `KEY_REQUIRED`
- `provider=ollama` 或 `custom` 时 `baseUrl` 必填 → 否则 400 `BASE_URL_REQUIRED`
- `provider=custom` 时 `apiFormat` 必填 → 否则 400 `API_FORMAT_REQUIRED`

**Response 201**：
```json
{
  "data": {
    "id": "aki_abc123",
    "userId": "local-user",
    "provider": "openai",
    "displayName": "My OpenAI Main",
    "keyMasked": "sk-proj...wxyz",
    "baseUrl": "",
    "apiFormat": "",
    "testStatus": "pending",
    "testError": "",
    "lastTestedAt": null,
    "createdAt": "2026-04-23T14:00:00Z",
    "updatedAt": "2026-04-23T14:00:00Z"
  }
}
```

#### 9.2 `GET /api/v1/api-keys` — 列表

**Query**：`?cursor=&limit=50&provider=openai`

**Response 200**：
```json
{
  "data": [ {...}, {...} ],
  "nextCursor": "cursor_xyz",
  "hasMore": true
}
```

#### 9.3 `PATCH /api/v1/api-keys/{id}` — 更新

**Request**（字段可选，只更新传了的）：
```json
{
  "displayName": "My OpenAI Team",
  "baseUrl": "https://custom-proxy.example.com"
}
```

**注意**：**不支持改 key 值**。要改就删了重建（避免误操作 + 审计更清晰）。

**Response 200**：更新后的完整对象。

#### 9.4 `DELETE /api/v1/api-keys/{id}` — 软删除

**Response 204**（无 body）。

#### 9.5 `POST /api/v1/api-keys/{id}:test` — 测试连通性

**Request**：无（从 DB 读 Key）。

**Response 200**（成功）：
```json
{
  "data": {
    "ok": true,
    "message": "Connected, found 45 models",
    "latencyMs": 1203,
    "modelsFound": ["gpt-4o", "gpt-4-turbo", ...]
  }
}
```

**Response 422**（连通性失败，但端点本身调用成功）：
```json
{
  "error": {
    "code": "API_KEY_TEST_FAILED",
    "message": "Connection failed",
    "details": {
      "providerStatusCode": 401,
      "providerMessage": "Invalid API key"
    }
  }
}
```

**副作用**：
- 成功 → 更新 `test_status=ok`, `last_tested_at=now`, `test_error=''`
- 失败 → 更新 `test_status=error`, `test_error={原因}`

---

## 11. 数据库表设计

```sql
CREATE TABLE api_keys (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL,
    provider         TEXT NOT NULL CHECK (provider IN (
        'openai', 'anthropic', 'google', 'deepseek', 'openrouter',
        'qwen', 'zhipu', 'moonshot', 'doubao', 'ollama', 'custom'
    )),
    display_name     TEXT NOT NULL DEFAULT '',
    key_encrypted    TEXT NOT NULL,
    key_masked       TEXT NOT NULL,
    base_url         TEXT NOT NULL DEFAULT '',
    api_format       TEXT NOT NULL DEFAULT '',
    test_status      TEXT NOT NULL DEFAULT 'pending' CHECK (test_status IN ('pending', 'ok', 'error')),
    test_error       TEXT NOT NULL DEFAULT '',
    last_tested_at   DATETIME,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at       DATETIME
);

-- 索引
CREATE INDEX idx_api_keys_user_id ON api_keys(user_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_api_keys_user_provider ON api_keys(user_id, provider) WHERE deleted_at IS NULL;
CREATE INDEX idx_api_keys_deleted_at ON api_keys(deleted_at);
```

**索引设计解释**：
- `user_id`：列表查询最常用（"当前用户的所有 Key"）
- `(user_id, provider)`：按 provider 查 Key（chat 用）
- 都是**部分索引**（`WHERE deleted_at IS NULL`），软删后不占索引空间

**通过 GORM AutoMigrate 生成**：
AutoMigrate 会根据 struct tag 建表和索引。CHECK 约束 GORM tag 不支持，通过 `schema_extras.go` 补上。

---

## 12. 事件

**Phase 2 不推送事件**。

未来可能的事件（Phase 3+）：
- `apikey.test_failed` — 测试失败时通知前端（主动推，不用用户刷新）
- `apikey.invalidated` — chat 调用 Key 返回 401 时，通知前端该 Key 失效

**暂留空，用到再加。**

---

## 13. 错误码

| Code | HTTP | 触发场景 |
|---|---|---|
| `API_KEY_NOT_FOUND` | 404 | Get/Delete/Test/Update 时 ID 不存在 |
| `INVALID_PROVIDER` | 400 | 创建时 provider 不在白名单 |
| `BASE_URL_REQUIRED` | 400 | ollama/custom 缺 base_url |
| `API_FORMAT_REQUIRED` | 400 | custom 缺 api_format |
| `KEY_REQUIRED` | 400 | 创建时 key 为空 |
| `API_KEY_TEST_FAILED` | 422 | 测试连通性失败 |
| `API_KEY_INVALID` | 401 | 使用时 provider 返回 401（由 chat 等消费方触发）|

---

## 14. 消费方如何用

### chat domain 调 LLM 时

```go
// app/chat/service.go（伪代码）
func (s *ChatService) Send(ctx context.Context, in SendInput) {
    // 1. 通过 model domain 决定"这次用哪个 provider"
    provider, modelID := s.modelSvc.PickForChat(ctx)
    
    // 2. 从 apikey 拿 Key
    apiKey, baseURL, err := s.apiKeySvc.GetKeyForProvider(ctx, provider)
    if err != nil { ... }
    
    // 3. 调 LLM（Eino）
    resp, err := s.llmGateway.Stream(ctx, apiKey, baseURL, modelID, messages)
    
    // 4. 如果 LLM 返回 401：
    if isAuthError(err) {
        s.apiKeySvc.MarkInvalid(ctx, apiKey.ID)  // 更新 test_status=error
        bridge.Publish(events.ChatError{Code: "API_KEY_INVALID", ...})
        return
    }
}
```

### model domain 如何挑 Key

（此为 model domain 的职责，在此处仅示意）：
```go
// app/model/service.go
func (s *ModelService) PickForChat(ctx context.Context) (provider string, modelID string) {
    // 读用户配置："主对话模型用哪个 provider 的哪个 model"
    // 返回 provider 名
}
```

apikey 提供的是 `GetByProvider(provider)` — **给定 provider，返回一个能用的 Key**。具体"哪个 provider"由 model domain 决定。

---

## 15. 安全考虑

| 点 | 设计 |
|---|---|
| 加密 | AES-GCM + 机器指纹派生密钥（Phase 1 已实现 `v1:` 格式）|
| 明文 Key 不落日志 | service 层绝不 `log.Info("key: %s", key)` |
| 明文 Key 不落响应 | `KeyEncrypted` 带 `json:"-"` tag，不会被序列化 |
| 明文 Key 生命周期短 | 只在调 LLM 瞬间存在于内存，请求结束即 GC |
| Key 在 DB 丢失 | 有 `key_masked` 列冗余保底（恢复 UI 展示），Key 本身要求用户重填 |
| 删除保留审计 | 软删除（deleted_at），保留 30 天后可物理删 |

---

## 16. 实现清单（撸代码时的 checklist）

### domain 层
- [ ] `domain/apikey/types.go` — APIKey struct + 常量
- [ ] `domain/apikey/errors.go` — sentinel 错误
- [ ] `domain/apikey/repository.go` — Repository 接口
- [ ] `domain/apikey/providers.go` — Provider 元数据 + 白名单
- [ ] `domain/apikey/mask.go` — key 掩码逻辑（纯函数，可单测）
- [ ] `domain/apikey/providers_test.go` — Provider 列表完整性
- [ ] `domain/apikey/mask_test.go` — 掩码规则

### infra 层
- [ ] `infra/gorm/apikey_repo.go` — Repository 实现
- [ ] `infra/gorm/apikey_repo_test.go` — 集成测试（含 user_id 过滤）

### app 层
- [ ] `app/apikey/service.go` — Service 业务逻辑
- [ ] `app/apikey/tester.go` — ConnectivityTester 接口
- [ ] `app/apikey/tester_impl.go` — HTTP 测试实现
- [ ] `app/apikey/service_test.go` — 单元测试（mock repo + mock tester）
- [ ] `app/apikey/tester_test.go` — httptest mock server 测试各 provider 逻辑

### transport 层
- [ ] `transport/httpapi/handlers/apikey.go` — HTTP handler
- [ ] `transport/httpapi/handlers/apikey_test.go` — E2E 契约测试
- [ ] `transport/httpapi/middleware/auth.go` — user_id 注入中间件
- [ ] `transport/httpapi/router/router.go` — 注册 apikey routes

### 配套基础设施
- [ ] `infra/gorm/schema_extras.go` — 补 CHECK 约束
- [ ] `transport/httpapi/response/errmap.go` — 加 apikey 错误码行
- [ ] `cmd/server/main.go` — 装配：`gormdb.Migrate(db, &apikey.APIKey{})`

### 验收
- [ ] 全量 `go test ./...` 零失败
- [ ] `go vet` 零警告
- [ ] 5 个 curl 端到端测试通过
- [ ] 至少 3 个 provider 测试连通性真实跑通（OpenAI / DeepSeek / Ollama）

---

## 17. 待确认 / 可能遗漏

1. **Provider 的默认 base_url 是否全对？** 11 个中我对 `doubao`、`zhipu`、`qwen` 的 endpoint 有把握但没真跑过，实现时需要真查官方文档。
2. **`anthropic` 测试会真实发请求产生费用**（约 $0.0001），这个接受吗？还是给一个选项"跳过 anthropic 测试"？
3. **软删除的 30 天后物理清理**是否需要？现在不做，记录到未来 backlog。
4. **`GetByProvider` 有多个候选时的排序**：按 `last_tested_at DESC` + `test_status='ok'` 优先？还是按 `created_at DESC`？
5. **API Key 数量上限**：单用户最多存多少个 Key？防止用户误操作 / 疯狂创建。建议 100 个上限。

---

## 18. 与其他 domain 的协作

```
     ┌─────────────────┐
     │   chat / model  │
     │   workflow LLM  │   ← 消费方
     │   knowledge     │
     └────────┬────────┘
              │ GetKeyForProvider(provider)
              │ MarkInvalid(id) on 401
              ↓
      ┌───────────────┐
      │  apikey domain│   ← 本 domain
      │  (凭证管理)    │
      └───────┬───────┘
              │ Encrypt / Decrypt
              ↓
      ┌───────────────┐
      │  crypto domain│   ← 基础设施（已就绪）
      └───────────────┘
```

---

**Review 重点**：
- 11 个 provider 清单是否合理？
- 5 个端点是否正确？特别是 `:test` 的行为
- user_id 引入方式是否 OK？（Phase 2 硬编码 `"local-user"`）
- APIKey struct 字段是否全？
- 错误码是否合理？
- 上方第 17 节的 5 个待确认点

请对照每一节给 feedback。🙏
