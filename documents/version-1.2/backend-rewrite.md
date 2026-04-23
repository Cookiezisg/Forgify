# Backend 全新重写 — 契约优先 + 分层架构 + Agentic Workflow Platform

**创建于**：2026-04-22
**升级于**：2026-04-23（路线图升级为完整的 Agentic Workflow Platform）
**分支**：`backend-iteration`
**状态**：Phase 1 完成（地基），Phase 2 待启动（基础对话能力）
**预估总工时**：~67h（地基 10h ✅ + 产品能力 57h）

---

## 进度追踪

### 地基阶段（已完成）

| Phase | 内容 | 工时 | 状态 | 完成日期 |
|---|---|---|---|---|
| **Phase 0** | 骨架：go mod + main.go + 目录结构 + /health | 4h | ✅ 完成 | 2026-04-22 |
| **Phase 1** | Infra 基础：GORM / logger / crypto / events / middleware | 6h | ✅ 完成（7/7，72 个测试） | 2026-04-23 |

### 产品能力阶段（路线图）

| Phase | 主题 | 工时 | 状态 | 交付价值 |
|---|---|---|---|---|
| **Phase 2** | 基础对话能力：apikey + conversation + 简版 chat | ~10h | ⬜ 未开始 | 像 ChatGPT 客户端能用 |
| **Phase 3** | 工具锻造能力：forge + attachment + tool + chat 加 tool calling | ~12h | ⬜ 未开始 | Forgify V1.0 体验完整（聊天造工具）|
| **Phase 4** | 工作流能力：workflow + flowrun + 节点系统 + scheduler | ~20h | ⬜ 未开始 | 桌面版 Coze（拖拽编排 + 定时跑）|
| **Phase 5** | 智能化：knowledge + intent + mcp + skill + 完整智能 chat | ~15h | ⬜ 未开始 | 完整 Agent 平台（一句话生成工作流）|
| **Phase 6** | 切换：删 `backend/`、改名 `backend-new/`、Electron 配置 | ~2h | ⬜ 未开始 | 老后端下线，新后端上线 |

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
| 2026-04-23 | **路线图升级**：基于"Agentic Workflow Platform"愿景重新规划 Phase 2-6。原计划只是 V1.0 重写，新计划升级为完整产品（含 workflow / knowledge / MCP / 智能编排）。引入 6 个新 domain（workflow / flowrun / scheduler / knowledge / mcp / skill / intent），目标对标 Dify+Coze 的桌面版本。详见下方"产品愿景"和"Phase 2-6 详细路线图"章节 |
| 2026-04-23 | 文档目录重组：`Documents/` → `documents/`（小写），按版本分目录 `version-1.0` / `1.1` / `1.2`。`BACKEND_REWRITE.md` 落在 `version-1.2/` 下。文件名统一 kebab-case |
| 2026-04-23 | 加 auth middleware（`InjectUserID`）：硬编码 `DefaultLocalUserID = "local-user"`，Phase 2 多租户就绪。5 单测，累计 77 个 |
| 2026-04-23 | 加 locale middleware（`InjectLocale`）+ 跨层共享包 `internal/pkg/reqctx/`：解析 Accept-Language（zh-CN/en）注入 ctx，供 LLM 相关代码读。`reqctx` 包立 `pkg/` 约束（只 stdlib、无状态、单一职责）。UserID 逻辑从 middleware 迁到 reqctx 统一管理。新增 28 个测试 |
| 2026-04-23 | **全量注释瘦身**：15 个生产文件共砍 ~420 行冗余注释，保留双语 godoc 但移除架构哲学、重复说明、跑题猜测。S11 规范扩展为"双语 + 节制"完整规则 |

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

## 产品愿景（Phase 2 起）

Forgify 不只是"对话 + 造工具"——目标是 **Agentic Workflow Platform**：用户一句话能编排出工作流，工作流由多种节点构成，可挂知识库做 RAG，最终由调度器部署运行。

### 核心能力清单

1. **意图识别 / Intent Routing**：聊天时识别用户想干啥（创建工作流？改工具？更新知识库？纯问答？）
2. **工作流引擎**：节点 + 边的 DAG，能跑、有运行历史
3. **多种节点类型**：用户工具 / MCP 工具 / LLM 节点 / Skill / 知识库检索 / 触发器 / 审批
4. **知识库 / RAG**：上传文档 → 切分 → 向量化 → 检索，挂在 LLM 或工作流节点上
5. **MCP 集成**：接 Anthropic 的 MCP 服务器，第三方能力即插即用
6. **调度部署**：cron / 文件触发 / Webhook 触发
7. **Skill 系统**：预制 + 元数据完善的能力模板（V1 浅版即可）

### 业界对标

| 产品 | 对标的能力 |
|---|---|
| **Dify** | 工作流 + 知识库 + Agent |
| **Coze**（字节）| Bot + 工作流 + 插件 / Skill |
| **n8n + AI 节点** | 通用工作流 + AI |
| **Langflow / Flowise** | 可视化 LLM pipeline |

定位：**桌面版 + 中文场景优化** —— 在锻造工具 + 离线运行上做差异化。

### Eino 框架支撑度

| 能力 | Eino 原生支持？ | 推荐方案 |
|---|---|---|
| Intent 识别 | ✅ 原生 | `eino/agents/react` |
| Workflow Engine | ✅ 原生 | `eino/compose` Graph/Chain |
| LLM 节点 | ✅ 原生 | `eino/components/model` |
| 工具节点 | ✅ 原生 | `eino/components/tool` |
| RAG / 知识库 | ✅ 原生 | `eino/components/{embedding,retriever,indexer}` |
| MCP 集成 | ⚠️ 半 | `mark3labs/mcp-go` + Eino tool 适配 |
| Skill 系统 | ❌ 概念性 | 我们自定义 |
| Cron 调度 | ❌ | `robfig/cron` |
| 事件触发 | ❌ | `fsnotify` + HTTP |
| Python 沙箱 | ❌ | subprocess（已有）|

**结论**：Eino 覆盖 70% 核心能力，主要补 **MCP 适配 + 调度 + Skill 概念层**。

---

## Phase 2-6 详细路线图

### 设计原则

1. **每个 Phase 都能独立交付价值** —— 不会出现"做了 80% 但啥都用不了"
2. **依赖严格自下而上** —— 每个 Phase 只依赖前面已完成的 Phase
3. **复杂度阶梯式增长** —— 难度从基础 CRUD → 复杂 CRUD → 编排 → 智能
4. **前端暂不跟进** —— 后端用 curl 测试为主，前端在所有 Phase 完成后统一适配

---

### 🥚 Phase 2：基础对话能力（~10h）

**目标**：用户能保存 API Key、新建对话、发消息、看 LLM 流式返回

**做哪些 domain**：
1. **apikey**（~2h）—— 最简单，过整条 Pattern：domain interface → infra GORM 实现 → app service → handler → router 注册
2. **conversation**（~3h）—— 对话 + 消息 CRUD（含分页）
3. **chat 极简版**（~5h）—— 纯流式 LLM，**不带工具调用**（保留给 Phase 3）

**新增 domain 目录**：
```
domain/apikey/        ← 已有目录，填内容
domain/conversation/  ← 已有目录，填内容
domain/chat/          ← 已有目录，填内容
```

**完成后能用 curl 做什么**：
- `POST /api/v1/api-keys` 加 Key
- `POST /api/v1/conversations` 新建对话
- `POST /api/v1/chat/messages` + `GET /api/v1/events?conversationId=xxx` 流式聊天

**Phase 2 完成 = 一个最简 ChatGPT 客户端的后端**

---

### 🐣 Phase 3：工具锻造能力（~12h）

**目标**：在对话里 AI 能写代码、保存为工具、运行工具、自动修错

**做哪些 domain**：
1. **forge**（~2h）—— 无 DB 纯逻辑：AST 解析、代码块检测、`# @` 注释规范化
2. **attachment**（~2h）—— 文件上传 + 解析（Excel/PDF/Word/图片）
3. **tool**（~6h）—— 最复杂的 CRUD：版本/标签/测试用例/pending change/Python 沙箱执行/导入导出
4. **chat 升级**（~2h）—— 加 tool calling 循环：LLM → 决定调工具 → 执行 → 喂回 LLM

**4 个 schema 业务问题需要在做 tool domain 时讨论**：
- `conversations.asset_id/asset_type` polymorphism vs 拆两列
- `tools.pending_code` 字段对 vs 独立 `tool_pending_changes` 表
- `tools.version` (TEXT) vs `tool_versions.version` (INTEGER) 语义不一致
- `tool_test_history` 20 条上限的强制机制

**Phase 3 完成 = Forgify V1.0 的核心能力全部可用**

---

### 🐤 Phase 4：工作流能力（~20h，最大的一块）

**目标**：能可视化编排工作流（后端能力，前端 Phase N 再做），工作流能跑，能定时

**做哪些 domain**：
1. **workflow**（~5h）—— 工作流定义、状态机、版本
2. **flowrun**（~3h）—— 工作流的运行实例 / 执行历史 / 节点状态推送
3. **节点系统**（~5h）—— 在 `domain/node/` 或 `domain/workflow/nodes/` 下，5 类节点适配器：
   - LLM Node（封装 Eino model）
   - Tool Node（调用 user tool）
   - Trigger Node
   - Approval Node（人工确认，阻塞等待）
   - Variable Node（中间结果存储）
4. **scheduler + trigger**（~4h）—— `domain/scheduler/` + `domain/trigger/`
   - cron 定时触发
   - 文件变化触发（fsnotify）
   - HTTP webhook 触发
5. **chat 再升级**（~3h）—— 能"通过对话创建工作流"基础版（V0.5 版本的智能编排）

**新增 domain 目录**：
```
domain/workflow/   ← 新增
domain/flowrun/    ← 新增
domain/scheduler/  ← 新增
domain/trigger/    ← 新增
（节点系统看实现复杂度决定是单独 domain/node/ 还是 workflow 的子包）
```

**核心实现**：底层用 `eino/compose` Graph 构建执行引擎

**Phase 4 完成 = 桌面版 Coze 的能力**

---

### 🐓 Phase 5：智能化和高级能力（~15h）

**目标**：完成"一句话生成工作流" + 知识库 RAG + MCP 集成 + Skill

**做哪些 domain**：
1. **knowledge + document**（~5h）—— 知识库 + 文档：
   - 向量库：本地 SQLite + sqlite-vec 扩展（轻量）
   - Embedding：Eino embedding 组件
   - 文档切分 + 索引 + 检索
2. **intent**（~3h）—— 意图识别系统、Eino ReAct Agent、系统 prompt 管理
3. **mcpserver + 适配**（~3h）—— `mark3labs/mcp-go` 客户端 + Eino tool adapter
4. **skill**（~2h）—— 预制 skill 概念（V1 浅版：打了标签的工具）
5. **chat 终极版**（~2h）—— 完整智能编排：意图识别 → 工作流推荐 → 自动建草稿

**新增 domain 目录**：
```
domain/knowledge/    ← 新增
domain/document/     ← 新增
domain/intent/       ← 新增
domain/mcpserver/    ← 新增
domain/skill/        ← 新增
```

**Phase 5 完成 = 完整 Agent 平台**

---

### 🚀 Phase 6：原子切换（~2h）

**目标**：删老 backend，新 backend 上线

**步骤**：
1. Electron 配置切换：从跑 `backend/` 改为跑 `backend-new/`
2. 烟测 30 min（核心场景手测一遍）
3. 删除 `backend/`，重命名 `backend-new/` → `backend/`
4. 更新 `go.mod` module 名（如有需要）
5. commit: "feat: full backend rewrite — clean architecture, agentic workflow platform"

**前置条件**：Phase 2 完成后即可考虑切换（旧后端能力被覆盖）；Phase 5 完成后是最佳切换时机（彻底替换）

---

### 总览

| Phase | 主题 | 工时 | 完成后产品形态 |
|---|---|---|---|
| 0-1 | 地基（已完成）| 10h | 基础设施全就位 |
| 2 | 基础对话 | 10h | ChatGPT 客户端 |
| 3 | 工具锻造 | 12h | Forgify V1.0 体验 |
| 4 | 工作流 | 20h | 桌面版 Coze |
| 5 | 智能 + 知识库 + MCP | 15h | 完整 Agent 平台 |
| 6 | 切换 | 2h | 老后端下线 |
| **合计** | | **~69h** | 完整愿景 |

---

### 跨 domain 协作的关键点

```
                    ┌──────────────────┐
                    │ chat (智能编排)   │ ← Phase 5 终极
                    └────────┬─────────┘
              ┌──────────────┼──────────────┐
              ↓              ↓              ↓
        ┌──────────┐  ┌──────────┐  ┌──────────┐
        │ workflow │  │   tool   │  │knowledge │  ← 中层"能力载体"
        └────┬─────┘  └────┬─────┘  └────┬─────┘
             ↓             ↓             ↓
        flowrun       forge         document
        scheduler     attachment    (向量库)
        trigger
                                    ┌──────────┐
                                    │   mcp    │
                                    └──────────┘
                                    ┌──────────┐
                                    │  skill   │
                                    └──────────┘

       ┌─────────────────────────────────────────────┐
       │ 全程依赖：Phase 0-1 已建好的基础设施          │
       │ apikey / conversation / crypto / events / gorm│
       └─────────────────────────────────────────────┘
```

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
- **S11 注释规范**：详见下方「S11 注释规范（双语 + 节制）」章节

---

### S11 注释规范（双语 + 节制）

从 `backend-new/` 开始，所有代码注释必须遵守以下规则。

#### 1. 双语格式
- **包/类型/函数** 的 godoc 注释必须**英文在前、空行、中文在后**
- **英文块**优先简洁，面向国际/AI 搜索友好
- **中文块**不是机械翻译，可以更贴业务上下文

**格式示例**：

```go
// InjectUserID is the Phase 2 simplified auth middleware: stamps
// DefaultLocalUserID into ctx. Will be rewritten to parse real auth
// credentials (JWT / session) later.
//
// InjectUserID 是 Phase 2 的简化 auth 中间件：把 DefaultLocalUserID
// 塞入 ctx。未来重写为解析真实凭证（JWT / session）。
func InjectUserID(next http.Handler) http.Handler { ... }
```

#### 2. 什么必须写（SHOULD have）
- ✅ **Package doc**（2–5 行）：包的职责，一句话能讲清
- ✅ **导出符号的 godoc**：类型、函数、常量、变量（Go 惯例 + 工具链要求）
- ✅ **Non-obvious 的 WHY**：代码"做什么"显而易见时，只有"为什么这么做"值得写
- ✅ **陷阱/安全警告**：如 "不得返回 fallback key，否则全用户共享"
- ✅ **行为契约**：如 "best-effort delivery，slow subscribers 丢事件"
- ✅ **回归守卫测试**的意图注释（可选，但推荐）

#### 3. 什么禁止写（MUST NOT）
- ❌ **架构哲学**：如"为什么放这里而不放那里"——搬到本文档
- ❌ **团队约定/规范解释**：如"S11 要求我们..."——搬到本文档
- ❌ **历史决策过程**：如"早期我们用 X，后来改用 Y"——放 git log / PR 描述
- ❌ **对代码的机械复述**：如 `// Set name sets the name`
- ❌ **跑题猜测**：如"未来可能会..."（除非是真的 TODO）
- ❌ **冗余重复**：同一段英文再写一遍中文相同意思——说明内容本身可以砍

#### 4. 长度指南
- Package doc：**2–5 行**
- 函数/类型 godoc：**1–5 行**，超过 10 行要怀疑
- 内联注释：**单行优先**，非平凡的业务规则/陷阱可以 2–3 行

#### 5. 测试文件放宽要求
测试文件里"为什么测这个"往往需要解释，不限长度。但也要双语。

#### 6. 内联注释的双语写法
**非平凡**内联注释才双语：

```go
// WriteTimeout intentionally 0: SSE streams may run for minutes.
// WriteTimeout 特意设为 0：SSE 流可能持续几分钟。
IdleTimeout: 60 * time.Second,
```

**平凡**的（如 `// loop over items`）可以单英文或省略。

#### 7. 为什么这样规定
- **英文保持专业性**：grep 友好、AI-assist 友好、行业惯例
- **中文降低理解门槛**：团队中文母语，业务术语中文更准
- **节制防止注释腐烂**：过度注释会过时、会误导、会淹没真正重要的信息
- **架构决策归档**：Why-level 的决策放文档，不是代码注释——文档能持续更新，注释会被遗忘

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

## 详细设计文档（一段段填）

本文档是**总路线图 + 总体原则**。具体到每个 domain 的 API、表结构、事件、错误码等**细节**，
按 Phase 推进时填进下面的**专门分册**。这样总文档保持精简，分册按 Phase 演化。

| 文档 | 内容 | 推进节奏 |
|---|---|---|
| [`api-design.md`](./api-design.md) | 完整 REST API 设计：路径、方法、请求/响应 schema、状态码 | 每个 Phase 开干前先在这里定该 Phase 的 API 段 |
| [`database-design.md`](./database-design.md) | 完整 GORM 表结构：Go struct + tag + 索引 + 业务规则 | 每个 domain 开干前在这里定该表 schema |
| [`events-design.md`](./events-design.md) | SSE 事件类型定义：每个 event 的 struct + 触发时机 + 载荷 | 涉及流式或异步通知的 Phase 需要 |
| [`error-codes.md`](./error-codes.md) | 业务错误码表：domain sentinel + HTTP 状态映射 | 每个 domain 设计阶段同步补充 |

**原则**：先在分册写设计 → review 通过 → 再撸代码 → 写完更新分册的"实现状态"列。

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
