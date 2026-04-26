# V1.2 Backend 进展记录

**关联**：
- [`backend-design.md`](./backend-design.md) — 总体设计 + 规范（相对稳定，很少动）
- [`service-contract-documents/`](./service-contract-documents/) — 每个 domain 的服务契约索引（一眼清单）
- [`service-design-documents/`](./service-design-documents/) — 每个 domain 的详细设计

**本文档定位**：所有"正在发生"的状态都在这里。开发日志 / 完成快照 / 待办清单 / 原则演化。规范/架构/愿景这些"相对不变"的放 `backend-design.md`。

---

## 1. 当前快照（截止 2026-04-26）

### 1.1 Phase 完成度总览

| Phase | 主题 | 估工时 | 状态 | 里程碑 |
|---|---|---|---|---|
| **Phase 0** | 骨架（go mod + main + /health） | 4h | ✅ | 2026-04-22 |
| **Phase 1** | 基础 infra 7 件套（GORM / logger / crypto / events / middleware / response / pagination） | 6h | ✅ 72 测试 | 2026-04-23 |
| **Phase 2** | 基础对话能力（apikey / model / conversation / chat） | ~11h | ✅ ~170 测试 | 2026-04-25 |
| **Phase 3** | 工具锻造（forge / attachment / tool / chat 加 tool-calling） | ~12h | ⬜ | - |
| **Phase 4** | 工作流（workflow / flowrun / 节点 / scheduler / trigger） | ~20h | ⬜ | - |
| **Phase 5** | 智能化（knowledge / intent / mcp / skill / chat 终极版） | ~15h | ⬜ | - |

### 1.2 Phase 2 子任务明细

| Domain | 状态 | 说明 |
|---|---|---|
| **apikey** | ✅ 全部完成 | 7 步套路跑完：domain → store → tester → service → handler → 装配 → curl 冒烟 |
| **model** | ✅ 全部完成 | 7 步套路跑完，2026-04-25 |
| **conversation** | ✅ 全部完成 | 7 步套路跑完，2026-04-25 |
| **chat** | ✅ 全部完成 | 7 步套路跑完，2026-04-25 |

### 1.3 代码库统计（Phase 2 完成后）

- **测试总数**：~170 全绿
  - apikey 相关：61（app 38 + store 18 + domain 5）
  - model 相关：28（app 12 + store 9 + domain 3 + handler 7）
  - conversation 相关：28（app 11 + store 11 + handler 6）
  - 其他（infra / middleware / router / pagination / crypto / events）：~55
- **质量门**：`go vet ./...` 零警告、`go build ./...` 通过、`CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" go test -count=1 -race ./...` 全绿
- **2026-04-26 后置修复**：`apikey.ModelsFound` 持久化 + SSE buffer 64→2048

### 1.4 Phase 3 开工准备

Phase 2 全部完成，Phase 3 开工前需要：
1. 阅读 `backend-design.md` 端到端推演模板，对 `forge` / `attachment` / `tool` domain 各写一段完整调用链
2. 在 `service-design-documents/` 下新建 `tool.md` 详设计（含 4 个 schema 业务决策）
3. 确认 `service-contract-documents/` 各文件里 Phase 3 段落的 ⬜ 状态

---

## 2. 开发日志

按时间顺序（旧 → 新）。

### Phase 0-1：地基（2026-04-22 ~ 2026-04-23）

| 日期 | 内容 |
|---|---|
| 2026-04-22 | 全面契约审计（45 API 端点 + 10 DB 表 + 21 SSE 事件），一致性评分均低 |
| 2026-04-22 | 确定 12 条契约标准（N1-N5 API + D1-D5 DB + E1-E2 SSE） |
| 2026-04-22 | 确定 4 层架构：domain / app / infra / transport，GORM，单份结构带 tag |
| 2026-04-22 | Phase 0 完成：`backend-new/` 骨架，`/api/v1/health` 返回 envelope，优雅退出 |
| 2026-04-22 | 立 **S11 双语注释规范**（英文 + 中文），backend-new 全套代码/注释必须遵守 |
| 2026-04-22 | 日志框架定为 **zap**（dev 彩色 / prod JSON），`infra/logger/zap.go` 封装 |
| 2026-04-22 | transport 层结构升级：`http/` → `httpapi/`（避免包名冲突），拆出 `response/` / `middleware/` / `handlers/` 3 子包 |
| 2026-04-22 | **Phase 1 Step 2** 完成：`response/envelope.go`（Success / Created / NoContent / Paged / Error）+ `response/errmap.go`（FromDomainError）。N1 标准落地为强制 API |
| 2026-04-23 | **Phase 1 Step 3** 完成：`pagination/cursor.go`（Parse / EncodeCursor / DecodeCursor），cursor 分页 + 10 单测 |
| 2026-04-23 | **Phase 1 Step 4a** 完成：`middleware/recover.go`，panic → 500 INTERNAL_ERROR + 6 单测（含敏感信息不泄漏守卫）|
| 2026-04-23 | **Phase 1 Step 4b** 完成：`middleware/logger.go`（method/path/status/bytes/elapsed）+ 6 单测 |
| 2026-04-23 | **Phase 1 Step 4c** 完成：`middleware/notfound.go`，envelope 格式 404 fallback + 4 单测 |
| 2026-04-23 | 模块名纠正：`github.com/sunweilin/forgify-new` → `github.com/sunweilin/forgify/backend`（Go multi-module repo 标准命名）|
| 2026-04-23 | **Phase 1 Step 4d** 完成：`middleware/cors.go`，白名单 CORS（拒绝 `*`）+ 7 单测 |
| 2026-04-23 | **Phase 1 Step 4e** 完成：`router/` 子包 + `handlers/health.go` Register pattern 模版，4 个集成测试验证端到端中间件链 |
| 2026-04-23 | Phase 1 地基 4/7，37 测试零失败；envelope、CORS、访问日志全链路通 |
| 2026-04-23 | **Phase 1 Step 5** 完成：crypto 接口化（`domain/crypto/Encryptor`）+ AES-GCM 实现。修 4 个老代码安全问题（fallback 密钥共享灾难 / decrypt 返 nil nil / 无版本标识 / shell 脆弱）。密文 `v1:` 前缀给 KMS 留兼容位。14 新测试，累计 51 |
| 2026-04-23 | **Phase 1 Step 6** 完成：`infra/gorm/` → 后来改名 `infra/db/`（db.go / migrate.go / schema_extras.go）。WAL / FK / PrepareStmt / UTC。AutoMigrate + schema_extras 模式，4 个 schema 业务问题推迟到 Phase 3 tool 阶段。11 新测试，累计 62 |
| 2026-04-23 | **Phase 1 Step 7** 完成：`domain/events/` 接口 + `infra/events/memory/` 内存实现。强类型事件（禁 `map[string]any`）、扇出 pub-sub、buffer 满非阻塞丢弃、ctx 自动 cancel、sync.Once 幂等。10 新测试含 race，累计 **72** |
| 2026-04-23 | **路线图升级**：定位从"V1.0 重写"→ Agentic Workflow Platform 完整愿景。引入 6 新 domain（workflow / flowrun / scheduler / knowledge / mcp / skill / intent），对标 Dify+Coze 桌面版 |
| 2026-04-23 | 文档目录重组：`Documents/` → `documents/`；按版本分 `version-1.0` / `1.1` / `1.2`；文件名 kebab-case |
| 2026-04-23 | 加 auth middleware `InjectUserID`（硬编码 `DefaultLocalUserID = "local-user"`），Phase 2 多租户字段就绪。5 单测，累计 77 |
| 2026-04-23 | 加 locale middleware `InjectLocale` + 跨层共享包 `internal/pkg/reqctx/`（只 stdlib、无状态、单一职责）。UserID 也迁到这。新增 28 测试 |
| 2026-04-23 | **全量注释瘦身**：15 个生产文件共砍 ~420 行冗余注释（架构哲学、重复说明、跑题猜测全删）。S11 规范扩展为"双语 + 节制" |
| 2026-04-23 | **Phase 2 路线图修正**：新增 `model` domain（"场景 → provider/model"策略层）。起因：chat 端到端推演时发现 provider 无归属——apikey 管凭证、chat 管编排、谁都不该决策 provider。立第 5 条设计原则 **"端到端推演先行"** |

### Phase 2：基础对话能力（2026-04-24 ~ 2026-04-26）

| 日期 | 内容 |
|---|---|
| 2026-04-24 | **Phase 2 Task #1** 完成：apikey domain 层。试过扁平 / 按角色子包（types/ports/registry/tools）/ Go 社区味子包多种结构，最终定**平铺**：`apikey.go`（entity + 常量 + errors + Credentials + ListFilter + Repository + KeyProvider）+ `providers.go`（11 provider 白名单）。`mask` 搬到 app 层（只 Service 用）。立 **S12 包结构**（domain 平铺按概念拆，禁子目录）。14 新测试 |
| 2026-04-24 | **Phase 2 Task #2** 完成：apikey Repository 实现 + 18 集成测试（CRUD / 跨用户隔离 / 分页 / GetByProvider 排序）。3 相关重构：(1) `infra/gorm/` → `infra/db/`；(2) Repository 实现最终落 `infra/store/<domain>/`（Clean Architecture 正统）；(3) 立 **S13 包命名**（三层同名 + `<name><role>` 别名：apikeydomain / apikeyapp / apikeystore）|
| 2026-04-24 | **Phase 2 Task #3** 完成：`app/apikey/tester.go` ConnectivityTester + HTTPTester + 21 httptest 用例。4 种 HTTP 模式分派（openai-compatible `/models` / anthropic `/v1/messages` 1-token / google `/v1beta/models?key=` / ollama `/api/tags`）+ custom 按 APIFormat 二选一。约定：网络/401/5xx/ctx 取消 → `TestResult{OK:false}`；未知 provider / 必填 baseURL 缺 → Go error（程序 bug 才上抛）。审计发现 S13 别名两处违规一并修。立 **"spec 优先于邻居文件"** 审计纪律 |
| 2026-04-24 | **Phase 2 Task #4** 完成：`app/apikey/service.go` + `keyprovider.go` + 18 单测。Service 拥有加密边界（repo 见密文、tester 见明文，二者互不相识）。Test 编排：`repo.Get → decrypt → tester.Test → repo.UpdateTestResult → log`。实现 `apikeydomain.KeyProvider`（ResolveCredentials 合并默认 baseURL；MarkInvalid 给消费方回报 401）。用真 AES-GCM + fake repo + fake tester 端到端跑通加解密。ID `aki_` + 8 字节 crypto/rand；nil logger 启动期 panic（守卫单测）|
| 2026-04-24 | **Phase 2 Task #5** 完成：`handlers/apikey.go`（5 端点）+ `errmap.go` 加 7 apikey sentinel 映射 + `error-codes.md` 同步 + 15 个 E2E 契约测试。`:action` URL 规范通过 `POST /{idAction}` 通配符 + `strings.Cut(":")` 拆分实现。`POST /{id}:test` 失败 → 422 `API_KEY_TEST_FAILED` + `details.latencyMs`（N2 的 422 = 业务拒绝）|
| 2026-04-24 | **Phase 2 Task #6** 完成：apikey 装配。`router/deps.go` 加 `APIKeyService` 字段（nil-tolerant）；`router.New` 条件注册 handler；`main.go` 串起 `MachineFingerprint → DeriveKey → AES-GCM → Store → HTTPTester → Service`。curl 实机冒烟：5 端点中 4 个无需真 key 的全通 |
| 2026-04-24 | **立设计原则 #6 "反校验剧场"**：Forgify 是本地 Electron + 单用户 + 同人写前后端。跳过"前端下拉已筛 + 下游自然报错"式的 backend 校验。保留的才是有价值的：JSON 畸形、必填字段非空、path 白名单、NotFound 404、DB 层 CHECK/UNIQUE |
| 2026-04-24 | **model domain 设计定档**：Q1 `/model-configs/{scenario}` 复数 path + path param；Q2 不校验 provider 白名单；Q3 不校验 hasKey。4 sentinel：`ErrNotConfigured` / `ErrInvalidScenario` / `ErrProviderRequired` / `ErrModelIDRequired` |
| 2026-04-24 | **文档结构重组**：`backend-rewrite.md` → `backend-design.md`；顶层分册迁入 `service-contract-documents/`；详设计迁入 `service-design-documents/`；新增 `progress-record.md` |
| 2026-04-24 | **文档大审计 + 重写**：apikey.md 与实际代码对齐（14 处失真），按代码重写完整 790 行。写 `service-design-documents/model.md` 完整详设计（19 节 + 完整调用链 + 7 步 checklist）|
| 2026-04-24 | **立设计原则 #7 + S14 "文档同步纪律"（最高优先级）**：因审计发现 14 处失真，意识到文档滞后 = 集体性 bug。规则：每次代码改动必须联动三处文档；发现不符立刻修 |
| 2026-04-25 | **[arch-fix] providers.go 归属修正**：`providers.go` 从 `domain/apikey/` 迁移到 `app/apikey/`。理由：所有消费者（Service.validateCreate + HTTPTester）都在 app 层；符合 Go "接口在消费方" 原则。`domain/apikey/apikey.go` 现在只剩 entity + sentinel + Repository + KeyProvider，职责纯净 |
| 2026-04-25 | **[arch] S12 文件命名规范扩展**：主文件用包名的规则从 domain 层扩展到 app / infra/store 全部三层。`app/apikey/service.go` → `apikey.go`；`app/model/service.go + modelpicker.go` → `model.go`（合并）；`infra/store/*/store.go` → `*/apikey.go` / `*/model.go` |
| 2026-04-25 | **[arch] app/apikey 文件整合**：`keyprovider.go` + `mask.go` 合并入 `apikey.go`；`service_test.go` + `mask_test.go` 合并入 `apikey_test.go` |
| 2026-04-25 | **Phase 2 model domain 完成**：7 步套路全跑完。domain（ModelConfig + 4 sentinel + Repository + ModelPicker）→ store（9 集成测试）→ app（Service + PickForChat，12 单测）→ handler（GET + PUT，7 E2E 测试）→ errmap 4 条 → 装配 → curl 冒烟 6 场景全通 |
| 2026-04-25 | **Phase 2 conversation domain 完成**：7 步套路全跑完。domain（Conversation + ErrNotFound + Repository）→ store（11 集成测试）→ app（Create/List/Rename/Delete，11 单测）→ handler（POST/GET/PATCH/DELETE，6 E2E 测试）→ errmap 1 条 → 装配 |
| 2026-04-25 | **Phase 2 chat domain 完成**：7 步套路全跑完。Eino ReAct Agent 驱动，per-conversation 队列化并发控制（buffered channel 5）；SSE 15s keep-alive ping；ContentExtractor 可插拔（PlainText / PDF / DOCX / Excel / PPTX / HTML / Image）；auto-titling 异步 goroutine；FTS5 全文索引（`CGO_CFLAGS="-DSQLITE_ENABLE_FTS5"`）；8 sentinel + errmap 全覆盖 |
| 2026-04-25 | **目录重组**：`backend-new/` → `backend/`；旧 Electron 代码移入 `legacy/`；`.gitignore` 按标准 Go 项目重写。Phase 6（原子切换）已内嵌完成，从路线图移除 |
| 2026-04-25 | **[doc-fix] 文档补全**：model.md §17 实现清单全勾 ✅；新建 conversation.md 完整详设计（13 节）；api-design.md / database-design.md / error-codes.md 同步 model ✅ + conversation ✅ |
| 2026-04-26 | **[feat] apikey.ModelsFound 持久化**：`APIKey` entity 新增 `ModelsFound []string`（GORM `serializer:json`，DB 列 `models_found TEXT`）。`UpdateTestResult` 接口 + store 实现加第 4 个参数 `models []string`；`Service.Test` 成功时把 `result.ModelsFound` 写入。`GET /api/v1/api-keys` 响应每条 key 带 `modelsFound` 数组，前端配模型时可直接用作下拉选项 |
| 2026-04-26 | **[fix] SSE buffer 扩容**：`infra/events/memory/bridge.go` `defaultBufferSize` 64 → 2048，解决 DeepSeek 等快速 LLM 大量 token 事件被丢弃（"subscriber buffer full"）导致聊天回复不完整的问题 |
| 2026-04-26 | **[Phase 3 完成] 冒烟测试通过**：curl 验证 create / list / :run / versions / run-history / delete 全通。修复 `ExecutionResult` 缺少 json 标签（N3 规范）。service-contract-documents 全量同步（api-design ✅ / database-design ✅ / error-codes ✅ / events-design ✅）。tool.md 实现清单全勾 ✅。|
| 2026-04-26 | **[Phase 3] 装配完成**：handlers/tool.go（22 端点）+ errmap 9 条 tool 错误码 + router/deps.go（ToolService 字段）+ router/router.go（注册 ToolHandler）+ main.go（Migrate 5 个 tool 表、创建 sandbox/toolLLMClientAdapter/toolService、ForgeTools 注入 chatService.SetTools）。history 端点改用 pagination.Parse。全量 19 包测试全绿。|
| 2026-04-26 | **[Phase 3] `app/agent/forge.go`**：5 个 System Tool（SearchTool/GetTool/CreateTool/EditTool/RunTool）+ ForgeTools 工厂 + resolveAttachments。SearchTool 用 LLM 排序；Create/EditTool 流式推 ToolCodeStreaming SSE 事件；RunTool att_id 解析。新增 `agentpkg.WithConversationID`/`GetConversationID` 供 system tool 读取 conversationID。chat.Service 加 SetTools + 注入 agentCtx + ToolsNodeConfig.Tools 激活。全量编译通过。|
| 2026-04-26 | **[Phase 3] `app/tool/tool.go`**：Service 完整实现，含 CRUD / 版本管理 / pending 生命周期 / sandbox 执行 / 测试用例 / LLM 生成测试用例（emit callback 解耦 HTTP）/ 导入导出。AcceptPending/RejectPending 改为 toolID 参数（HTTP 路径一致）。全量 19 包测试全绿。|
| 2026-04-26 | **[Phase 3] `app/tool/ast.go`**：Python subprocess AST 解析，提取函数名/参数（含 required/description/default）/返回值（type+description）。Google-style docstring 解析，无 docstring 不报错。6 测试全绿。|
| 2026-04-26 | **[Phase 3] `infra/store/tool/tool.go`**：完整 Repository 实现，30 个方法，覆盖 Tool CRUD / Version+Pending 生命周期 / TestCase / RunHistory / TestHistory。compile-time 接口检查。11 集成测试全绿。|
| 2026-04-26 | **[Phase 3] `infra/db/schema_extras.go` 重构 + tools partial UNIQUE**：把单个 schemaExtras 列表改为按 table 分组的 extraGroup 结构，每组独立 guard 自身所需的表，新 domain 只需追加一个 extraGroup。追加 tools 的部分唯一索引 `UNIQUE(user_id, name) WHERE deleted_at IS NULL`。db 测试全绿。|
| 2026-04-26 | **[arch] 工具搜索方案切换**：chromem-go 向量搜索 → LLM 排序（SearchTool 把全部工具发给 LLM 选出最相关 N 个）。准确率更高，无需 embedding API。删除 `infra/vectordb/`，移除 chromem-go 依赖，domain/tool 移除 VectorHit，Repository 加 ListAllTools，Service 移除 VectorDB 依赖。全量编译通过。|
| 2026-04-26 | **[Phase 3] `infra/sandbox/python.go`**：PythonSandbox 实现，Python subprocess + 30s 超时；extractFuncName 从代码提取函数名；driver 模板追加 __main__ 桥接；Python 异常返回 ok=false 不上升为 Go error。8 测试全绿。|
| 2026-04-26 | **[Phase 3] `domain/events/types.go` 追加 6 个 tool SSE 事件**：`tool.code_streaming`（加 `MessageID`+`ToolCallID` 绑定对话轮次）/ `tool.created` / `tool.pending_created` / `tool.test_case_generated` / `tool.test_cases_done` / `tool.test_cases_not_supported`。events-design.md + tool.md §13 同步更新。|
| 2026-04-26 | **[Phase 3 开工] tool domain layer**：完成 `domain/tool/tool.go`。5 个 entity（Tool / ToolVersion / ToolTestCase / ToolRunHistory / ToolTestHistory）+ ExecutionResult（定义在 domain 层避免 infra/sandbox ↔ app/tool 循环依赖）+ 9 个 sentinel + Repository 接口（30 个方法）+ 常量。ToolVersion 合并 pending 职责（status 字段区分 pending/accepted/rejected）。编译通过，database-design.md + error-codes.md 同步更新。|

---

## 3. Phase 2 完成交付清单 ✅

### 3.1 apikey domain ✅

- [x] domain / store / tester / service / handler 全套实现
- [x] AES-GCM 加密边界，机器指纹派生密钥
- [x] 5 个 HTTP 端点（CRUD + `:test`）+ 61 测试
- [x] `ModelsFound` 持久化（2026-04-26 后置补充）

### 3.2 model domain ✅

- [x] domain / store / app / handler 全套实现
- [x] `GET /model-configs` + `PUT /model-configs/{scenario}`
- [x] `ModelPicker` 跨 domain 接口，chat 消费
- [x] 28 测试

### 3.3 conversation domain ✅

- [x] domain / store / app / handler 全套实现（CRUD 4 端点）
- [x] cursor 分页，软删除，auto_titled + system_prompt 字段
- [x] 28 测试

### 3.4 chat domain ✅

- [x] Eino ReAct Agent 驱动，per-conversation 队列（buffered channel 5）
- [x] SSE 5 个端点（上传附件 / 发消息 / 取消 / 消息列表 / 事件流）
- [x] ContentExtractor 7 种格式 + Vision 路径
- [x] FTS5 全文索引 + auto-titling
- [x] 8 sentinel + errmap，SSE buffer 2048（2026-04-26 修复）

---

## 4. Phase 3-5 粗粒度路线

各 Phase 开工前在此段展开细节。当前状态均为 ⬜。

### Phase 3：工具锻造能力（~12h）

forge（纯 AST）+ attachment（上传/解析）+ tool（最大最复杂 domain）+ chat 升级加 tool calling。4 个 schema 业务问题（asset polymorphism / pending 独立表 / version 语义 / 历史上限）在做 tool domain 时讨论。

### Phase 4：工作流能力（~20h，最大一块）

workflow（DAG + 状态机）+ flowrun（执行实例）+ 节点系统（LLM / Tool / Trigger / Approval / Variable 5 类）+ scheduler（cron / fsnotify / HTTP webhook）+ chat 再升级支持"对话创建工作流"。底层用 `eino/compose` Graph 构建执行引擎。

### Phase 5：智能化（~15h）

knowledge + document（本地 sqlite-vec）+ intent（Eino ReAct Agent）+ mcpserver（`mark3labs/mcp-go`）+ skill（V1 浅版：打标签的工具）+ chat 终极版（意图识别 → 工作流推荐 → 自动建草稿）。

---

## 5. 规范/原则演化

按时间倒序，查最近变化用。

| 日期 | 变化 |
|---|---|
| 2026-04-26 | **S14 hook 落地**：在 `.claude/settings.local.json` 配 PostToolUse hook，编辑 `backend/internal/` 下文件时自动注入文档同步提醒 |
| 2026-04-25 | **S3 扩展"严禁藏错误"**：`_ = err` 静默跳过是严禁行为——隐藏的错误会在意想不到的地方爆发（教训：FTS5 虚拟表建失败后触发器仍建成，INSERT 时才炸）。功能不可用必须让调用方看到错误或在文档/日志明确说明 |
| 2026-04-25 | **S12 扩展**：主文件用包名规则推广至 app / infra/store 全层；明确"仅 Service 实现接口 / 小工具函数"合并入主文件，不单独建文件 |
| 2026-04-25 | **providers.go 归属原则**：辅助注册表放在消费它的层（非 domain）；domain 层只放 entity + sentinel + 接口 |
| 2026-04-24 | 立 **设计原则 #7 + S14 "文档同步纪律"（最高优先级）**：每次改代码联动三处文档；发现不符立刻修 |
| 2026-04-24 | 立 **设计原则 #6 "反校验剧场"**（单开发者 + 本地 Electron 不搞前端已覆盖的校验）|
| 2026-04-24 | 立 **"spec 优先于邻居文件"** 审计纪律（避免复制 pre-existing 违规）|
| 2026-04-24 | 文档结构三层分工：`backend-design.md`（规范） / `service-contract-documents/`（索引） / `service-design-documents/`（详设计） / `progress-record.md`（进展） |
| 2026-04-24 | 立 **S13 包命名**（三层同名 + `<name><role>` 调用方别名）|
| 2026-04-24 | 立 **S12 包结构**（domain 平铺按概念拆，禁子目录）|
| 2026-04-23 | 立 **设计原则 #5 "端到端推演先行"**（每 domain 开工前走完整数据流）|
| 2026-04-23 | 路线图升级：V1.0 重写 → Agentic Workflow Platform 完整愿景 |
| 2026-04-23 | S11 扩展为 **"双语 + 节制"** 完整规则；全量瘦身 ~420 行冗余注释 |
| 2026-04-22 | 立 **S11 双语注释规范** |
| 2026-04-22 | 定 **12 条契约标准**（N1-N5 / D1-D5 / E1-E2）|
