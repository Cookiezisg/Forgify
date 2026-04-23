# Backend 全新重写 — 契约优先 + 分层架构

**创建于**：2026-04-22
**分支**：`backend-iteration`
**状态**：Phase 0 完成
**预估总工时**：~37h

---

## 进度追踪

| Phase | 内容 | 工时 | 状态 | 完成日期 |
|---|---|---|---|---|
| **Phase 0** | 骨架：go mod + main.go + 目录结构 + /health | 4h | ✅ 完成 | 2026-04-22 |
| **Phase 1** | Infra 基础：GORM / logger / crypto / events / middleware | 6h | ✅ 完成（7/7） | 2026-04-23 |
| **Phase 2** | Domain + Infra 实现（6 个域，按复杂度） | 15h | ⬜ 未开始 | — |
| **Phase 3** | 集成和数据迁移 | 4h | ⬜ 未开始 | — |
| **Phase 4** | 完整测试（契约、端到端、性能） | 6h | ⬜ 未开始 | — |
| **Phase 5** | 原子切换（删 `backend/`、改名 `backend-new/`） | 2h | ⬜ 未开始 | — |

---

## 开发日志

| 日期 | 内容 |
|---|---|
| 2026-04-22 | 全面契约审计（45 API 端点 + 10 DB 表 + 21 SSE 事件），一致性评分均低 |
| 2026-04-22 | 确定 12 条契约标准（N1-N5 API + D1-D5 DB + E1-E2 SSE） |
| 2026-04-22 | 确定 4 层架构：domain / app / infra / transport，GORM，单份结构带 tag |
| 2026-04-22 | Phase 0 完成：`backend-new/` 骨架，`/api/v1/health` 返回 envelope，优雅退出 |
| 2026-04-22 | 立 **S11 双语注释规范**（英文 + 中文），backend-new 全套代码/注释必须遵守 |
| 2026-04-22 | 日志框架定为 **zap**（dev 彩色 / prod JSON），`infra/logger/zap.go` 封装 |
| 2026-04-22 | transport 层结构升级：`http/` → `httpapi/`（避免包名冲突），拆出 `response/` / `middleware/` / `handlers/` 3 个子包，通用能力和业务 handler 分离 |
| 2026-04-22 | Phase 1 Step 2 完成：`response/envelope.go`（Success/Created/NoContent/Paged/Error）+ `response/errmap.go`（FromDomainError），N1 标准落地为强制 API |
| 2026-04-23 | Phase 1 Step 3 完成：`pagination/cursor.go`（Parse/EncodeCursor/DecodeCursor），cursor 分页 + 10 个单元测试 |
| 2026-04-23 | Phase 1 Step 4a 完成：`middleware/recover.go`，panic → 500 INTERNAL_ERROR，+ 6 个单测（含敏感信息不泄漏的安全守卫） |
| 2026-04-23 | Phase 1 Step 4b 完成：`middleware/logger.go`，访问日志（method/path/status/bytes/elapsed），+ 6 个单测 |
| 2026-04-23 | Phase 1 Step 4c 完成：`middleware/notfound.go`，envelope 格式 404 fallback，+ 4 个单测（含回归守卫） |
| 2026-04-23 | 模块名纠正：`github.com/sunweilin/forgify-new` → `github.com/sunweilin/forgify/backend`，采用 Go multi-module repo 标准命名，Phase 5 切换时目录和模块已对齐 |
| 2026-04-23 | Phase 1 Step 4d 完成：`middleware/cors.go`，白名单 CORS（拒绝 `*`），+ 7 单测 |
| 2026-04-23 | Phase 1 Step 4e 完成：`router/` 子包（router.go + deps.go + router_test.go）+ `handlers/health.go`（Register pattern 模版），4 个集成测试验证端到端中间件链 |
| 2026-04-23 | Phase 1 地基完成 4/7：所有中间件 + 路由总装 + Handler pattern 就位，37 个测试零失败。`/api/v1/health` 与 `/api/v1/nonexistent` 均走 envelope，CORS preflight 正确响应，访问日志按预期输出 |
| 2026-04-23 | Phase 1 Step 5 完成：crypto 接口化（`domain/crypto/Encryptor`）+ AES-GCM 实现（`infra/crypto/aesgcm.go`），老代码 4 个安全问题全部修复（fallback 密钥灾难、decrypt 返 nil nil bug、无版本标识、shell 命令脆弱），密文加 `v1:` 前缀为未来 KMS 信封加密留兼容位。14 个新测试，累计 51 个 |
| 2026-04-23 | Phase 1 Step 6 完成：`infra/gorm/`（db.go / migrate.go / schema_extras.go）。GORM 连接集中管理（WAL、FK 约束强制开、PrepareStmt 缓存、UTC 时间），`Migrate` 接 domain 类型（Phase 2 用）。决定 AutoMigrate + schema extras 模式（无数据迁移），4 个 schema 业务问题（asset polymorphism / pending 独立表 / version 语义 / 历史上限）推迟到 Phase 2 做 tool domain 时讨论。11 个新测试，累计 62 个 |
| 2026-04-23 | Phase 1 Step 7 完成：`domain/events/` 接口 + `infra/events/memory/` 内存实现。强类型事件（禁止 `map[string]any`）、扇出 pub-sub、buffer 满非阻塞丢弃、ctx 自动 cancel、sync.Once 幂等 cancel。10 个新测试含 race 并发测试，累计 **72 个测试**。**Phase 1 地基 7/7 全部完成** |

---

## Context

**为什么做这次重构**

经过对 Forgify 后端 + DB + SSE + 前端调用的全面审计，现有代码存在系统性架构债：

- **HTTP API**（45 端点）一致性 3.2/10：响应结构各异、0/45 端点有分页、REST 动词乱用、字段命名混用
- **DB schema**（10 表）健康度 5.8/10：软删除 3 种风格并存、关键 UNIQUE/FK 约束缺失、被引用的 `workflow` 表不存在
- **SSE 事件**（21 定义）一致性 3/10：14/21 是死事件、`ForgeCodeDetected` 载荷两种形态、字段名混乱
- **架构**：handler 直接写 SQL、`ToolService` 是 29 方法 696 行的 god object、`routes_chat.go` 一个文件装了 7 个责任

现有代码功能能跑，但要进入 Tier 5（工作流引擎）等复杂功能，地基不够扎实。

**用户明确要求**：在进入下一阶段前保证当前是"优秀的"——即**地基先打好**。

---

## Strategy

**Contract-first + Green-field 重写 + 原子切换**

1. 本轮（`backend-iteration` 分支）：
   - 新建 `backend-new/` 目录，与现有 `backend/` 并存
   - 在 `backend-new/` 里按新架构、新契约、新 schema 全新写代码
   - 配完整测试（单元 + 集成 + API 契约测试）
   - 验证通过后：删 `backend/`，将 `backend-new/` 重命名为 `backend/`
2. 下轮（独立 iteration）：前端按本文档列出的"前端变更清单"统一跟进

**前端在本轮保持不动**，本轮只产出"前端要改什么"的完整清单。

---

## Standards（12 条新契约宪法）

### HTTP API
1. **N1 统一 envelope**：成功 `{"data": ...}`；失败 `{"error": {"code", "message", "details"}}`
2. **N2 状态码严格语义**：200 读/更新 / 201 创建 / 204 删除 / 400 参数错 / 404 不存在 / 409 冲突 / 422 业务拒绝 / 500 内部错
3. **N3 字段 camelCase**：API 请求/响应一律 camelCase；DB 列 snake_case，repo 层转换
4. **N4 列表强制分页**：`?cursor=xxx&limit=50` → `{data, nextCursor, hasMore}`
5. **N5 RESTful 严格化**：资源用名词；状态改动走 `PATCH` + 状态字段；动词用 `:action` 后缀（`POST /tools/{id}:duplicate`）

### Database
6. **D1 软删除统一**：所有表用 `deleted_at DATETIME`（NULL = 未删除），废弃 `status='deleted'` 风格
7. **D2 时间戳统一**：每表必有 `created_at` / `updated_at`，类型 `DATETIME`，默认 `CURRENT_TIMESTAMP`
8. **D3 枚举必有 CHECK**：provider、category、role、content_type 等显式列出合法值
9. **D4 外键显式声明** + `PRAGMA foreign_keys=ON` 开启约束
10. **D5 业务唯一性用 UNIQUE 约束**：`tools.name`、`(tool_id, version)` 等

### SSE
11. **E1 死事件清理**：14 个从不发的事件全删；每个事件必有 Go struct 定义，禁止 `map[string]any`
12. **E2 事件名 snake_case 分层**：`chat.token`、`tool.code_updated`；所有事件必带 `conversationId` 或明确上下文

### 其他规则（S 系列延续）
- **S3 错误不吞**：`_` 忽略必须带注释说明原因
- **S5 单文件 ≤ 250 行**，单函数 ≤ 60 行
- **S6 handler ≤ 20 行**：只解析/调用/序列化
- **S8 SQL 只在 `infra/gorm/`**：其他层出现 SQL 都是违规
- **S9 context 传播**：每个跨层调用传 `ctx`
- **S10 结构化日志**：用 **zap**，生产 JSON / 开发带彩色
- **S11 双语注释**：从 `backend-new/` 开始，所有注释（包/函数/类型/内联）必须**英文 + 中文**双语。格式：英文块在前，空行，中文块在后。示例见下方

**S11 注释格式范例**：

```go
// Package logger provides the project-wide zap logger factory.
// Logger is injected via DI from cmd/server/main.go.
//
// Package logger 提供项目级 zap logger 工厂。
// Logger 通过 DI 从 cmd/server/main.go 注入。
package logger

// New builds a zap logger. dev=true selects the colored console encoder;
// dev=false selects production JSON.
//
// New 构建 zap logger。dev=true 使用彩色控制台编码器；dev=false 使用生产 JSON。
func New(dev bool) (*zap.Logger, error) {
    // WriteTimeout intentionally 0: SSE streams may run for minutes.
    // WriteTimeout 特意设为 0：SSE 流可能持续几分钟。
    ...
}
```

**为什么 S11**：团队读写效率最大化——英文保持代码专业性和搜索友好，中文降低理解成本，尤其对架构决策/业务规则注释。

---

## Target Architecture

```
backend-new/
├── cmd/server/main.go              ← 入口，DI 组装
├── go.mod / go.sum
└── internal/
    ├── domain/                     ← 纯业务（仅 import 标准库）
    │   ├── conversation/
    │   │   ├── types.go            ← Conversation、Message
    │   │   ├── errors.go           ← ErrNotFound 等 sentinel
    │   │   ├── repository.go       ← ConversationRepository 接口
    │   │   └── rules.go            ← 纯校验函数（无副作用）
    │   ├── tool/
    │   │   ├── types.go            ← Tool、Parameter、Version、TestCase
    │   │   ├── errors.go
    │   │   ├── repository.go
    │   │   └── rules.go            ← 参数校验、代码合法性规则
    │   ├── chat/
    │   │   └── types.go            ← Stream、ToolCall、Message
    │   ├── forge/
    │   │   └── types.go            ← ParsedCode、DetectedBlock
    │   ├── apikey/
    │   │   ├── types.go
    │   │   ├── errors.go
    │   │   └── repository.go
    │   └── attachment/
    │       └── types.go
    │
    ├── app/                        ← service 层（协调 domain + infra）
    │   ├── conversation/
    │   │   └── service.go          ← ConversationService（Create/List/Archive/...）
    │   ├── tool/
    │   │   ├── service.go          ← ToolService（CRUD + 运行）
    │   │   ├── version.go          ← 版本管理子服务
    │   │   └── import_export.go    ← 导入导出
    │   ├── chat/
    │   │   ├── service.go          ← ChatService.Send（入口）
    │   │   ├── stream.go           ← 流式循环（原 doStream）
    │   │   └── tool_calling.go     ← 工具调用（原 executeToolCall）
    │   ├── forge/
    │   │   ├── parser.go           ← AST 解析（搬迁）
    │   │   ├── detector.go         ← 代码块检测
    │   │   └── service.go          ← 锻造流程编排
    │   ├── apikey/
    │   │   └── service.go
    │   └── attachment/
    │       └── service.go
    │
    ├── infra/                      ← 技术实现
    │   ├── gorm/                   ← 唯一碰 SQL 的地方
    │   │   ├── db.go               ← GORM 初始化，读现有 migrations
    │   │   ├── migrations/         ← SQL 迁移文件（新的 schema）
    │   │   ├── conversation_repo.go
    │   │   ├── tool_repo.go
    │   │   ├── apikey_repo.go
    │   │   └── ...
    │   ├── eino/                   ← Eino LLM gateway 适配
    │   │   └── chat_model.go
    │   ├── sandbox/                ← Python 执行
    │   │   ├── executor.go
    │   │   ├── installer.go
    │   │   └── process.go
    │   ├── events/                 ← SSE broker
    │   │   ├── bridge.go
    │   │   └── types.go            ← 所有事件的 Go struct
    │   ├── crypto/                 ← 加密
    │   │   ├── encrypt.go
    │   │   └── fingerprint.go
    │   └── logger/                 ← slog 配置
    │       └── slog.go
    │
    └── transport/
        └── httpapi/                ← 包名避开 net/http 冲突
            ├── server.go           ← HTTP 服务器生命周期（启动、优雅关闭）
            ├── router.go           ← 路由注册集中管理
            ├── deps.go             ← DI 结构体（持有所有 service）
            │
            ├── response/           ← 📦 通用能力：响应包装（独立包）
            │   ├── envelope.go     ← Success / Created / NoContent / Paged / Error
            │   └── errmap.go       ← FromDomainError + 错误映射表
            │
            ├── middleware/         ← 📦 通用能力：中间件（独立包）
            │   ├── recover.go      ← panic 恢复
            │   ├── logger.go       ← 请求日志
            │   ├── cors.go         ← 跨域
            │   └── notfound.go     ← 404 envelope（覆盖默认裸文本）
            │
            └── handlers/           ← 📦 业务 handler（独立包）
                ├── health.go
                ├── chat.go
                ├── tool.go
                ├── conversation.go
                ├── apikey.go
                ├── attachment.go
                ├── model.go
                └── sse.go
```

**依赖方向**：`transport → app → domain`、`infra → domain`（实现接口）、`domain` 不依赖任何人。

**类型策略**：domain 类型直接带 GORM tag（一份到底）。

**transport/httpapi 内部分层原则**：**稳定的（通用能力）和频繁变的（业务 handler）分开放**。
- `response/` `middleware/` 属于框架级通用能力，写一次用很久
- `handlers/` 属于业务级代码，每加一个 feature 就新增/修改

---

## Optimized API Contracts

### 通用 envelope

```typescript
// 成功
type SuccessResponse<T> = { data: T }

// 列表成功
type PagedResponse<T> = {
  data: T[]
  nextCursor: string | null
  hasMore: boolean
}

// 失败
type ErrorResponse = {
  error: {
    code: string        // 如 "TOOL_NOT_FOUND"
    message: string     // 人类可读
    details?: object    // 可选上下文
  }
}
```

### 资源 1：API Keys

| 旧 | 新 | 说明 |
|---|---|---|
| `GET /api/api-keys` → `[{...}]` | `GET /api/v1/api-keys?cursor=&limit=50` → `{data, nextCursor, hasMore}` | 分页、envelope |
| `POST /api/api-keys`（混合创建+更新） | `POST /api/v1/api-keys` → 201 `{data}` | 只创建 |
| （同上）| `PATCH /api/v1/api-keys/{id}` → 200 `{data}` | 更新分离 |
| `DELETE /api/api-keys/{id}` | `DELETE /api/v1/api-keys/{id}` → 204 | 保持 |
| `POST /api/api-keys/test` → 200 `{ok, message}` | `POST /api/v1/api-keys/{id}:test` → 200 `{data: {ok, latencyMs}}` 失败 422 `{error}` | 业务错误用状态码 |

### 资源 2：Conversations

| 旧 | 新 |
|---|---|
| `GET /api/conversations`（LIMIT 200 硬编码） | `GET /api/v1/conversations?cursor=&limit=50&status=active` |
| `GET /api/conversations/archived` | `GET /api/v1/conversations?status=archived` |
| `GET /api/conversations/search?q=` | `GET /api/v1/conversations:search?q=&cursor=` |
| `POST /api/conversations` → 200 | `POST /api/v1/conversations` → **201** `{data}` |
| `DELETE /api/conversations/{id}` | `DELETE /api/v1/conversations/{id}` → 204（软删） |
| `PATCH /api/conversations/{id}/rename` | `PATCH /api/v1/conversations/{id}` body `{title}` → 200 |
| `PATCH /api/conversations/{id}/archive` | `PATCH /api/v1/conversations/{id}` body `{status: "archived"}` |
| `PATCH /api/conversations/{id}/restore` | `PATCH /api/v1/conversations/{id}` body `{status: "active"}` |
| `PATCH /api/conversations/{id}/bind` | `PUT /api/v1/conversations/{id}/binding` body `{assetId, assetType}` |
| `PATCH /api/conversations/{id}/unbind` | `DELETE /api/v1/conversations/{id}/binding` |
| `POST /api/conversations/batch-archive` | `PATCH /api/v1/conversations` body `{ids, patch: {status: "archived"}}` → `{data: {updated, failed}}` |
| `POST /api/conversations/batch-delete` | `DELETE /api/v1/conversations?ids=a,b,c` → `{data: {deleted, failed}}` |
| `GET /api/conversations/{id}/messages` | `GET /api/v1/conversations/{id}/messages?cursor=&limit=100` |
| `POST /api/conversations/{id}/compact` | 保留 `POST /api/v1/conversations/{id}:compact` → 200 `{data: {compactedCount, summaryId}}` |
| `GET /api/asset-conversations/{id}` | `GET /api/v1/conversations?assetId={id}` |

### 资源 3：Chat

| 旧 | 新 |
|---|---|
| `POST /api/chat/send` → 204 | `POST /api/v1/chat/messages` → 202 `{data: {messageId, streamId}}` |
| `POST /api/chat/stop` | `DELETE /api/v1/chat/streams/{streamId}` → 204 |

新增 `streamId` 让前端能关联 HTTP 请求和 SSE 事件流。

### 资源 4：Tools

主要变化：
- 列表端点**不返回完整 `code`**，摘要版只有 `{id, name, displayName, description, category, status, ...}`
- `code` 仅在 `GET /api/v1/tools/{id}` 完整返回

| 旧 | 新 |
|---|---|
| `GET /api/tools?category=&q=` | `GET /api/v1/tools?cursor=&limit=50&category=&q=&status=active&tag=` |
| `POST /api/tools` → 200 | `POST /api/v1/tools` → **201** |
| `GET /api/tools/{id}` | `GET /api/v1/tools/{id}` |
| `PUT /api/tools/{id}` | `PATCH /api/v1/tools/{id}` body `{code?, displayName?, description?, category?}` |
| `PATCH /api/tools/{id}/meta` | 合并进上一条 |
| `DELETE /api/tools/{id}` | `DELETE /api/v1/tools/{id}` → 204（软删） |
| `POST /api/tools/{id}/restore` | `PATCH /api/v1/tools/{id}` body `{deletedAt: null}` |
| `DELETE /api/tools/{id}/permanent` | `DELETE /api/v1/tools/{id}?permanent=true` |
| `GET /api/tools/deleted` | `GET /api/v1/tools?status=deleted` |
| `POST /api/tools/{id}/run` → `{Output, Error, DurationMs}` | `POST /api/v1/tools/{id}:run` → 200 `{data: {output, durationMs}}` / 422 `{error}` |
| `GET /api/tools/{id}/test-history` | `GET /api/v1/tools/{id}/test-runs?cursor=&limit=20` |
| `GET /api/tools/{id}/pending` | `GET /api/v1/tools/{id}/pending-change` |
| `POST /api/tools/{id}/accept` | `POST /api/v1/tools/{id}/pending-change:accept` → 200 `{data: <tool>}` |
| `POST /api/tools/{id}/reject` | `DELETE /api/v1/tools/{id}/pending-change` → 204 |
| `GET /api/tools/{id}/tags` | `GET /api/v1/tools/{id}/tags` |
| `POST /api/tools/{id}/tags` → 204 | `POST /api/v1/tools/{id}/tags` → **201** `{data: {tag}}` |
| `DELETE /api/tools/{id}/tags/{tag}` | 保持 |
| `GET /api/tools/{id}/versions` | `GET /api/v1/tools/{id}/versions?cursor=&limit=20` |
| `POST /api/tools/{id}/versions/{v}/restore` | `POST /api/v1/tools/{id}/versions/{v}:restore` |
| `GET /api/tools/{id}/test-cases` | 保持 |
| `POST /api/tools/{id}/test-cases` → 204 | → **201** `{data}` |
| `DELETE /api/test-cases/{id}` | `DELETE /api/v1/tools/{toolId}/test-cases/{id}` 恢复资源层级 |
| `GET /api/tools/{id}/export` | `GET /api/v1/tools/{id}:export` |
| `POST /api/tools/import/parse` | `POST /api/v1/tools:parse-import` |
| `POST /api/tools/import/confirm` | `POST /api/v1/tools:confirm-import` |

### 资源 5：Models

| 旧 | 新 |
|---|---|
| `GET /api/models` | `GET /api/v1/models` → `{data: [...]}` |
| `GET /api/model-config` | `GET /api/v1/model-config` → `{data}` |
| `POST /api/model-config` → 204 | `PUT /api/v1/model-config` → 200 `{data}` |

### 资源 6：Attachments

| 旧 | 新 |
|---|---|
| `POST /api/attachments/upload`（未注册！） | `POST /api/v1/attachments` → 201 `{data: {id, url, name, size, kind, preview}}` |

### 资源 7：系统

| 旧 | 新 |
|---|---|
| `GET /health` | `GET /api/v1/health` → `{data: {status: "ok", version, uptime}}` |
| `GET /events` | `GET /api/v1/events?conversationId=xxx` 支持按对话过滤 |

---

## Optimized Database Schema

### 核心变化
- **软删除统一**：所有表加 `deleted_at DATETIME NULL`，删除 `status='deleted'` 风格
- **时间戳统一**：`created_at`、`updated_at` 默认 `CURRENT_TIMESTAMP`
- **外键全部显式**，开启 `PRAGMA foreign_keys=ON`
- **`workflow` 表补齐**（即使 Tier 5 才用，schema 先有）
- **UNIQUE 约束补齐**
- **FTS5 虚拟表**用于搜索

### 表清单（新版）

```sql
-- 001_init.sql (新的初始迁移，覆盖现有 001+002)

PRAGMA foreign_keys = ON;

CREATE TABLE conversations (
    id          TEXT PRIMARY KEY,
    title       TEXT NOT NULL DEFAULT '',
    asset_id    TEXT,
    asset_type  TEXT CHECK(asset_type IN ('tool', 'workflow') OR asset_type IS NULL),
    status      TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'archived')),
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at  DATETIME
);
CREATE INDEX idx_conv_status_updated ON conversations(status, deleted_at, updated_at DESC);
CREATE INDEX idx_conv_asset ON conversations(asset_id) WHERE asset_id IS NOT NULL;

CREATE TABLE messages (
    id               TEXT PRIMARY KEY,
    conversation_id  TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role             TEXT NOT NULL CHECK(role IN ('user', 'assistant', 'system', 'tool')),
    content          TEXT NOT NULL DEFAULT '',
    content_type     TEXT NOT NULL DEFAULT 'text' CHECK(content_type IN ('text', 'image')),
    metadata         TEXT,  -- JSON
    model_id         TEXT,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_msg_conv_created ON messages(conversation_id, created_at);

-- 全文搜索（新增）
CREATE VIRTUAL TABLE messages_fts USING fts5(content, content_rowid=rowid);

CREATE TABLE tools (
    id               TEXT PRIMARY KEY,
    name             TEXT NOT NULL UNIQUE,   -- 修复：加 UNIQUE
    display_name     TEXT NOT NULL DEFAULT '',
    description      TEXT NOT NULL DEFAULT '',
    code             TEXT NOT NULL DEFAULT '',
    requirements     TEXT NOT NULL DEFAULT '[]',  -- JSON
    parameters       TEXT NOT NULL DEFAULT '[]',  -- JSON
    category         TEXT NOT NULL DEFAULT 'other'
                     CHECK(category IN ('email', 'data', 'web', 'file', 'system', 'other')),
    status           TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft', 'tested', 'failed')),
    builtin          BOOLEAN NOT NULL DEFAULT FALSE,
    version          TEXT NOT NULL DEFAULT '1.0',
    requires_key     TEXT,
    pending_code     TEXT,
    pending_summary  TEXT,
    last_test_at     DATETIME,
    last_test_passed BOOLEAN,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at       DATETIME
);
CREATE INDEX idx_tools_list ON tools(deleted_at, builtin DESC, updated_at DESC);
CREATE INDEX idx_tools_category ON tools(category) WHERE deleted_at IS NULL;

-- 全文搜索
CREATE VIRTUAL TABLE tools_fts USING fts5(name, display_name, description, content_rowid=rowid);

CREATE TABLE tool_versions (
    id              TEXT PRIMARY KEY,
    tool_id         TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    version         INTEGER NOT NULL CHECK(version > 0),
    code            TEXT NOT NULL,
    change_summary  TEXT NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tool_id, version)  -- 修复：加 UNIQUE
);
CREATE INDEX idx_tool_versions ON tool_versions(tool_id, version DESC);

CREATE TABLE tool_tags (
    tool_id  TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    tag      TEXT NOT NULL CHECK(length(tag) > 0),
    PRIMARY KEY (tool_id, tag)
);
CREATE INDEX idx_tag_reverse ON tool_tags(tag);  -- 反向查找

CREATE TABLE tool_test_history (
    id           TEXT PRIMARY KEY,
    tool_id      TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    passed       BOOLEAN NOT NULL,
    duration_ms  INTEGER NOT NULL DEFAULT 0,
    input_json   TEXT,  -- JSON
    output_json  TEXT,  -- JSON
    error_msg    TEXT,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_test_history ON tool_test_history(tool_id, created_at DESC);

CREATE TABLE tool_test_cases (
    id           TEXT PRIMARY KEY,
    tool_id      TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    name         TEXT NOT NULL DEFAULT 'Default',
    params_json  TEXT NOT NULL DEFAULT '{}',  -- JSON
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_test_cases_tool ON tool_test_cases(tool_id);

CREATE TABLE api_keys (
    id            TEXT PRIMARY KEY,
    provider      TEXT NOT NULL CHECK(provider IN ('openai', 'anthropic', 'deepseek', 'ollama', 'openrouter')),
    display_name  TEXT NOT NULL DEFAULT '',
    key_encrypted TEXT NOT NULL,
    base_url      TEXT,
    test_status   TEXT CHECK(test_status IN ('ok', 'error', 'pending') OR test_status IS NULL),
    last_tested   DATETIME,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at    DATETIME
);
CREATE INDEX idx_apikeys_provider ON api_keys(provider) WHERE deleted_at IS NULL;

-- 新增：workflow 表（即使 Tier 5 才用，先建表占位）
CREATE TABLE workflows (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    definition   TEXT NOT NULL DEFAULT '{}',  -- JSON flow definition
    status       TEXT NOT NULL DEFAULT 'draft' CHECK(status IN ('draft', 'deployed', 'paused')),
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at   DATETIME
);
CREATE INDEX idx_workflows_status ON workflows(status, deleted_at);

CREATE TABLE app_config (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL DEFAULT '',
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

### 数据迁移策略

由于 schema 改动较大（软删除字段重做、FK 补齐、命名调整），采用**一次性迁移脚本**：

```sql
-- 001_migrate_from_old.sql (在 backend-new 启动时自动执行一次)
-- 1. 读旧表数据
-- 2. 转换 status='deleted' → deleted_at
-- 3. 转换 status='archived' → status='archived' + updated_at 不变
-- 4. 写入新表
-- 5. 验证 count 一致
-- 6. 备份旧表为 {name}_backup_<date>
```

---

## Optimized SSE Events

### 保留的 7 个活跃事件（载荷强类型）

```go
// infra/events/types.go
package events

type ChatToken struct {
    ConversationID string `json:"conversationId"`
    StreamID       string `json:"streamId"`
    Token          string `json:"token"`
}

type ChatDone struct {
    ConversationID string `json:"conversationId"`
    StreamID       string `json:"streamId"`
    ModelID        string `json:"modelId"`
    MessageID      string `json:"messageId"`
}

type ChatError struct {
    ConversationID string `json:"conversationId"`
    StreamID       string `json:"streamId"`
    Code           string `json:"code"`
    Message        string `json:"message"`
}

type ToolCodeDetected struct {
    ConversationID string  `json:"conversationId"`
    MessageID      string  `json:"messageId"`
    ToolID         *string `json:"toolId,omitempty"`   // 已绑定工具时有
    FuncName       string  `json:"funcName"`
    Code           string  `json:"code"`
    DisplayName    string  `json:"displayName"`
    Description    string  `json:"description"`
    Category       string  `json:"category"`
}

type ToolCodeUpdated struct {
    ConversationID string `json:"conversationId"`
    ToolID         string `json:"toolId"`
    Summary        string `json:"summary"`
    HasPending     bool   `json:"hasPending"`
}

type ToolCodeStreaming struct {
    ConversationID string `json:"conversationId"`
    Status         string `json:"status"`  // "analyzing" | "generating"
}

type ConversationBound struct {
    ConversationID string `json:"conversationId"`
    AssetID        string `json:"assetId"`
    AssetType      string `json:"assetType"`
}

type ConversationTitleUpdated struct {
    ConversationID string `json:"conversationId"`
    Title          string `json:"title"`
}
```

### 删除的死事件（14 个）

`ForgeNameGenerated`、`MailboxUpdated`、`Notification`、以及 11 个其他未发的事件全部从代码移除。

### 事件名规范

旧：`chat.token`、`forge.code_detected`（不一致）
新：`chat.token`、`tool.code_detected`（`forge.*` 前缀合并到 `tool.*`，因为锻造本质是工具生命周期的一部分）

---

## Error Code Catalog

统一的业务错误码（`transport/http/response.go` 负责 HTTP status 翻译）：

```go
// domain/errors/codes.go
const (
    // 通用
    ErrInvalidRequest   = "INVALID_REQUEST"     // 400
    ErrInternal         = "INTERNAL_ERROR"      // 500

    // 对话
    ErrConvNotFound     = "CONVERSATION_NOT_FOUND"  // 404
    ErrConvArchived     = "CONVERSATION_ARCHIVED"   // 409

    // 工具
    ErrToolNotFound      = "TOOL_NOT_FOUND"          // 404
    ErrToolNameDuplicate = "TOOL_NAME_DUPLICATE"     // 409
    ErrToolBuiltin       = "TOOL_IS_BUILTIN"         // 403
    ErrToolRunFailed     = "TOOL_RUN_FAILED"         // 422

    // API Key
    ErrAPIKeyNotFound    = "API_KEY_NOT_FOUND"       // 404
    ErrAPIKeyInvalid     = "API_KEY_INVALID"         // 401
    ErrAPIKeyTestFailed  = "API_KEY_TEST_FAILED"     // 422

    // 流式
    ErrStreamNotFound    = "STREAM_NOT_FOUND"        // 404
    ErrStreamInProgress  = "STREAM_IN_PROGRESS"      // 409
    ErrModelNotConfigured = "MODEL_NOT_CONFIGURED"   // 422
)
```

---

## Frontend 变更清单（下轮迭代）

本轮后端完成后，前端需要做的变更（**不在本轮做**，列清楚供下轮参考）：

### 1. `frontend/src/lib/api.ts` 改造
- 解包 envelope：`response.data` 或抛 `response.error`
- 所有端点路径加 `/v1/` 前缀
- 新增 `ApiError` 类型，基于 error code 做判断

### 2. 分页支持
- `ConversationList`、`ToolList`、`MessageList` 组件加无限滚动
- `useChat` 的历史加载改成 cursor-based

### 3. 错误处理
- 替换所有 `.catch(() => {})` 为真实处理
- 按 error code 显示 i18n 文案
- 弃用 `classifyError` 的字符串匹配逻辑

### 4. SSE 订阅
- 订阅 URL 加 `?conversationId=xxx` 过滤
- 所有 payload 用生成的 TypeScript type，不再 `any`
- 删除对死事件的订阅代码

### 5. 类型生成
- 引入 `tygo` 或类似工具
- 从 Go struct 生成 TypeScript types
- 进入前端 build 流程

### 6. 状态变更 API
- `bind/unbind/archive/restore/rename` 全部改走 `PATCH` + 状态字段
- 响应处理改成读 `{data: <resource>}`

**工作量预估**：10-15h（前端简单跟进，不重写逻辑）

---

## Migration Plan（本轮执行顺序）

### Phase 0：骨架（~4h）
- 在 `backend-iteration` 分支上创建 `backend-new/` 目录
- `go mod init`，加依赖（gorm、slog、现有 eino）
- 建立空的 domain/app/infra/transport 目录树
- 配 `cmd/server/main.go` 骨架（空 handler，能 run）
- 配 Makefile：`build-new`、`test-new` 目标

### Phase 1：Infra 基础（~6h）
- `infra/gorm/`：GORM 初始化、连接配置、新版 migrations
- `infra/logger/`：slog 配置
- `infra/crypto/`：搬现有 crypto
- `infra/events/`：新版 Bridge + 事件类型定义
- `transport/http/`：middleware（recover/logger/cors/error）+ response/pagination 工具

### Phase 2：Domain + Infra 实现（~15h，按复杂度）
按以下顺序做（每个 domain 走完才进入下一个）：
1. **apikey**（最简单，试水） ~2h
2. **attachment**（只读文件）~1h
3. **conversation**（中等）~3h
4. **tool**（复杂，最多子概念）~5h
5. **forge**（无 DB，纯逻辑）~1h
6. **chat**（最复杂，要拆 doStream）~3h

每个 domain 包括：
- domain/xxx/ 类型和接口
- infra/gorm/xxx_repo.go 实现
- transport/http/xxx.go handlers
- domain/xxx/service_test.go 单元测试
- transport/http/xxx_test.go API 集成测试

### Phase 3：集成和数据迁移（~4h）
- `cmd/server/main.go` DI 组装
- 数据迁移脚本（旧表 → 新表 schema）
- 启动自检：migration 应用、表数据校验

### Phase 4：完整测试（~6h）
- 所有 API 端点的 curl 测试（golden file 比对）
- 核心流程端到端：发消息 → AI 回复 → 创建工具 → 测试工具 → 保存版本
- 性能基准：对比旧 API 响应时间

### Phase 5：切换（~2h）
- Electron 配置切换：从跑 `backend/` 改为跑 `backend-new/`
- 烟测 15 min
- 删除 `backend/`，重命名 `backend-new/` → `backend/`
- commit: "feat: full backend rewrite — clean architecture, GORM, unified contracts"

### 总工时：~37h

---

## Verification

### 单元测试
- `go test ./...` 零失败
- domain/ 层覆盖率 > 80%（纯逻辑好测）
- app/ 层核心 service 必测

### 契约测试
每个端点一个 curl 脚本，验证：
- 状态码正确
- envelope 格式正确
- 错误码符合约定
- 分页参数生效

### 端到端场景（手动）
1. 新建对话 → AI 回复 → 创建工具 → 分屏 → 测试工具 → 失败 → 让 AI 修复 → 成功
2. 归档对话 → 查看归档列表 → 恢复
3. 导出工具 → 删除 → 重新导入
4. 批量归档 / 批量删除

### 性能基准
- 流式对话 token latency < 旧版 110%
- 工具列表加载 < 500ms
- 搜索响应 < 300ms（FTS5 加持）

### Schema 完整性
- `PRAGMA foreign_key_check` 零返回
- `PRAGMA integrity_check` 返回 `ok`
- 数据迁移后 `SELECT COUNT(*)` 每表一致

---

## 关键文件清单

### 新增（backend-new/）
- `backend-new/cmd/server/main.go` — 入口
- `backend-new/internal/domain/{conversation,tool,chat,forge,apikey,attachment}/` — 6 个 domain
- `backend-new/internal/infra/{gorm,eino,sandbox,events,crypto,logger}/` — 6 个 infra
- `backend-new/internal/transport/http/` — HTTP 层
- `backend-new/internal/infra/gorm/migrations/001_init.sql` — 新 schema
- `backend-new/Makefile`

### 删除（切换后）
- `backend/` 整个目录（被 `backend-new/` 替换）

### 保持不变（本轮）
- `frontend/` — 完全不动
- `electron/` — 只改配置文件，指向新后端二进制
- `Documents/` — 更新 PROGRESS_1.0.md

---

## 非目标（本轮不做）

- ❌ 多租户（user_id 列）—— 下轮如果要 SaaS 再加
- ❌ auth middleware —— 同上
- ❌ Docker 沙箱 —— 保持 subprocess
- ❌ 前端类型生成工具链 —— 下轮前端 iteration 再接
- ❌ 前端代码改动 —— 下轮独立做

---

## 进度追踪

本计划完成后，回写 `Documents/V1.1/OPTIMIZATION_PLAN.md`：
- 宣布 Stage 2-5 的原计划**作废**（因为做了更彻底的重写）
- 新增 `Documents/BACKEND_REWRITE.md` 记录本次重写（从本 plan 文件抽取）
