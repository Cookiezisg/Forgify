# Backend 全新重写 — 契约优先 + 分层架构 + Agentic Workflow Platform

**创建于**：2026-04-22
**分支**：`backend-iteration`
**当前进度 / 开发日志**：[`progress-record.md`](./progress-record.md)

**本文档定位**：**稳定规范层** — 项目的 why / how / 全套规范 / 愿景 / 路线图全貌。这里的内容**很少改**；每天/每 Phase 变动的东西在 `progress-record.md`。

---

## Context — 为什么重构

经过对 Forgify 后端 + DB + SSE + 前端调用的全面审计，现有代码存在系统性架构债：

- **HTTP API**（45 端点）一致性 3.2/10：响应结构各异、0/45 端点有分页、REST 动词乱用、字段命名混用
- **DB schema**（10 表）健康度 5.8/10：软删除 3 种风格并存、关键 UNIQUE/FK 约束缺失、被引用的 `workflow` 表不存在
- **SSE 事件**（21 定义）一致性 3/10：14/21 是死事件、载荷多种形态、字段名混乱
- **架构**：handler 直接写 SQL、`ToolService` 是 29 方法 696 行的 god object、`routes_chat.go` 一个文件装 7 个责任

目标：**地基先打好**，再往上长。

---

## Strategy — 契约优先 + Green-field 重写 + 原子切换

1. 本轮（`backend-iteration` 分支）：新建 `backend-new/` 与旧 `backend/` 并存 → 按新架构 + 新契约 + 新 schema 全新写代码 + 配完整测试 → 验证通过后**原子切换**（删 `backend/`，改名 `backend-new/` → `backend/`）
2. 下轮（独立 iteration）：前端按本文档列出的"前端变更清单"统一跟进

**前端在本轮保持不动**。

---

## 产品愿景（Phase 2 起）

Forgify 不只是"对话 + 造工具"—目标是 **Agentic Workflow Platform**：用户一句话能编排出工作流，工作流由多种节点构成，可挂知识库做 RAG，最终由调度器部署运行。

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

定位：**桌面版 + 中文场景优化** — 在锻造工具 + 离线运行上做差异化。

### Eino 框架支撑度

| 能力 | Eino 原生支持？ | 推荐方案 |
|---|---|---|
| Intent 识别 | ✅ 原生 | `eino/agents/react` |
| Workflow Engine | ✅ 原生 | `eino/compose` Graph/Chain |
| LLM 节点 | ✅ 原生 | `eino/components/model` |
| 工具节点 | ✅ 原生 | `eino/components/tool` |
| RAG / 知识库 | ✅ 原生 | `eino/components/{embedding,retriever,indexer}` |
| MCP 集成 | ⚠️ 半 | `mark3labs/mcp-go` + Eino tool 适配 |
| Skill 系统 | ❌ 概念性 | 自定义 |
| Cron 调度 | ❌ | `robfig/cron` |
| 事件触发 | ❌ | `fsnotify` + HTTP |
| Python 沙箱 | ❌ | subprocess（已有）|

**结论**：Eino 覆盖 70% 核心能力，主要补 **MCP 适配 + 调度 + Skill 概念层**。

---

## Phase 路线图

**当前状态 / 任务细化** → [`progress-record.md`](./progress-record.md)

| Phase | 主题 | 工时 | 完成后产品形态 |
|---|---|---|---|
| 0-1 | 地基 | 10h | 基础设施全就位 |
| 2 | 基础对话 | 11h | ChatGPT 客户端 |
| 3 | 工具锻造 | 12h | Forgify V1.0 体验 |
| 4 | 工作流 | 20h | 桌面版 Coze |
| 5 | 智能 + 知识库 + MCP | 15h | 完整 Agent 平台 |
| 6 | 切换 | 2h | 老后端下线 |
| **合计** | | **~70h** | 完整愿景 |

### Phase 2 — 基础对话能力

4 个 domain：`apikey`（凭证）+ `model`（场景 → provider/model 策略）+ `conversation`（对话 CRUD）+ `chat` 极简版（流式，不带 tool calling）。

**关键调用链**：
```
handler.SendMessage
  → chat.Send
      → model.PickForChat                    → (provider, modelID)
      → apikey.ResolveCredentials(provider)  → (key, baseURL)
      → reqctx.GetLocale(ctx)                → "zh-CN" | "en"
      → eino.Stream(...)                     → SSE
```

### Phase 3 — 工具锻造能力
`forge`（纯 AST）+ `attachment`（上传/解析）+ `tool`（最大 domain：版本/标签/测试/pending/沙箱执行/导入导出）+ `chat` 升级加 tool calling 循环。

### Phase 4 — 工作流能力（最大的一块）
`workflow`（DAG + 状态机）+ `flowrun`（执行实例）+ 5 类节点（LLM / Tool / Trigger / Approval / Variable）+ `scheduler` + `trigger`（cron / fsnotify / HTTP webhook）+ `chat` 再升级支持"对话创建工作流"。底层用 `eino/compose` Graph 构建执行引擎。

### Phase 5 — 智能化
`knowledge` + `document`（本地 sqlite-vec）+ `intent`（Eino ReAct Agent）+ `mcpserver`（`mark3labs/mcp-go`）+ `skill`（V1 浅版：打标签的工具）+ `chat` 终极版（意图识别 → 工作流推荐 → 自动建草稿）。

### Phase 6 — 原子切换
Electron 切路径 → 烟测 30 min → 删 `backend/` → 改名 `backend-new/` → commit。

### 跨 domain 协作图

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

       ┌─────────────────────────────────────────────────────┐
       │ 全程依赖：Phase 0-1 地基 + apikey / model / conversation│
       │ + crypto / events / db / logger / reqctx              │
       └─────────────────────────────────────────────────────┘
```

---

## 设计原则（7 条，**原则 #7 最高优先级**）

1. **每个 Phase 都能独立交付价值** — 不会出现"做了 80% 但啥都用不了"
2. **依赖严格自下而上** — 每个 Phase 只依赖前面已完成的 Phase
3. **复杂度阶梯式增长** — 基础 CRUD → 复杂 CRUD → 编排 → 智能
4. **前端暂不跟进** — 后端用 curl 测试为主，前端在所有 Phase 完成后统一适配
5. **端到端推演先行** — 每个 domain 开工前**必须**先走一遍"用户一个请求从 HTTP 到最终调用"的完整数据流，列出所有跨 domain 依赖。避免设计看起来完整、实现时才发现"缺一个 domain"
6. **反校验剧场** — Forgify 是**本地 Electron + 单用户 + 同人写前后端**；backend 只保留真正有价值的校验（JSON 畸形、必填字段非空、path 白名单、NotFound 404、DB CHECK/UNIQUE），跳过"前端 dropdown 已筛 + 下游自然报错"式的重复校验。加校验前问自己："前端能不能防住？下游会不会自然炸？"两个都是，就不加
7. **📌 文档与代码同步（最高优先级）** — 每个代码改动必须伴随对应文档的同步更新；每个 domain 完成/推进时必须回头更新**全部** 4 处文档：
   - `service-design-documents/<domain>.md`（详设计：方法签名、流程、端点形状、错误码、调用链）
   - `service-contract-documents/{api-design, database-design, error-codes, events-design}.md`（索引：1-2 行状态 + 端点表）
   - `progress-record.md`（dev log + 任务清单勾 ✅ + Phase 状态）
   - `backend-design.md`（如有新原则/规范变动；大部分时候不改）

   **文档落后于代码 = bug**。看 doc 的人（包括未来的你）做出错决策，后果和代码 bug 等价。
   文档**不是**"文档"—— 它是"让后续工作能继续往前走的接线图"。
   详细执行规则见 S14。

### 端到端推演模板

每个 domain 开工前必填一段"完整调用链"到 `service-design-documents/<domain>.md`：

```
触发源（HTTP/定时/事件）
  → transport 层：哪个 handler
    → app 层：哪个 service 方法
        → 调谁：model / apikey / 其他 domain，每一次 cross-domain 调用都要列
        → 用什么：从 ctx 读什么、从哪个 repo 读什么
      → infra 层：最终落到哪里（DB / 外部 API / 沙箱）
  → 响应路径：成功 / 失败分别怎么返
```

**不走一遍这个推演，不开工**。

---

## Standards — 12 条契约宪法 + S 系列

### HTTP API
1. **N1 统一 envelope**：成功 `{"data": ...}`；失败 `{"error": {"code", "message", "details"}}`
2. **N2 状态码严格语义**：200 读/更新 / 201 创建 / 204 删除 / 400 参数错 / 404 不存在 / 409 冲突 / 422 业务拒绝 / 500 内部错
3. **N3 字段 camelCase**：API 请求/响应一律 camelCase；DB 列 snake_case，repo 层转换
4. **N4 列表强制分页**：`?cursor=xxx&limit=50` → `{data, nextCursor, hasMore}`
5. **N5 RESTful 严格化**：资源用名词；状态改动走 `PATCH` + 状态字段；动词用 `:action` 后缀（`POST /tools/{id}:duplicate`）

### Database
6. **D1 软删除统一**：所有表用 `deleted_at DATETIME`（NULL = 未删除），废弃 `status='deleted'` 风格
7. **D2 时间戳统一**：每表必有 `created_at` / `updated_at`，GORM 自动维护
8. **D3 枚举 CHECK 约束**：稳定白名单（如 `role`、`content_type`）在 DB 层做 CHECK；会随 Phase 扩张的白名单（如 `scenario`）在 app 层校验
9. **D4 外键显式声明** + `PRAGMA foreign_keys=ON` 开启约束
10. **D5 业务唯一性用 UNIQUE 约束**：`tools.name`、`(tool_id, version)`、`(user_id, scenario)` 等

### SSE
11. **E1 死事件清理**：每个事件必须有真实发布点 + Go struct 定义，禁止 `map[string]any`
12. **E2 事件名 snake_case 分层**：`chat.token`、`tool.code_updated`；所有事件必带 `conversationId` 或明确上下文

### 其他规则（S 系列）

- **S3 错误不吞**：`_` 忽略必须带注释说明原因
- **S5 单文件 ≤ 250 行 soft target**（概念内聚可放宽到 500），单函数 ≤ 60 行
- **S6 handler ≤ 20 行**：只解析 / 调用 / 序列化
- **S8 SQL 只在 `infra/store/` 和 `infra/db/`**：其他层出现 SQL 都是违规
- **S9 context 传播**：每个跨层调用传 `ctx`
- **S10 结构化日志**：用 **zap**（dev 彩色 / prod JSON）。**同步原语不自己打 log**（store / tester 等由调用者决定），**异步或 fire-and-forget 必须打**（events bridge、recover middleware）
- **S11 注释规范** — 见 §S11
- **S12 包结构** — 见 §S12
- **S13 包命名** — 见 §S13
- **S14 📌 文档同步纪律** — 见 §S14（**最高优先级**，对应设计原则 #7）

---

### S11 注释规范（双语 + 节制）

所有 `backend-new/` 代码注释必须遵守。

#### 1. 双语格式
- **包/类型/函数**的 godoc 注释必须**英文在前、空行、中文在后**
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

#### 3. 什么禁止写（MUST NOT）
- ❌ **架构哲学**：如"为什么放这里而不放那里"——搬到本文档
- ❌ **团队约定/规范解释**：如"S11 要求我们..."——搬到本文档
- ❌ **历史决策过程**：如"早期我们用 X，后来改用 Y"——放 git log / PR 描述
- ❌ **对代码的机械复述**：如 `// Set name sets the name`
- ❌ **跑题猜测**：如"未来可能会..."（除非是真的 TODO）
- ❌ **冗余重复**：同一段英文再写一遍中文相同意思——说明内容本身可以砍

#### 4. 长度指南
- Package doc：**2–5 行**，一个包只在主文件有 package doc（其他文件用普通文件头注释，需要空行和 `package X` 分隔）
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

### S12 包结构（domain 平铺，按概念拆文件）

每个 domain 的代码**平铺到包根目录**，**禁止子目录**。文件按"概念 / feature"拆分，**禁止**按"种类"拆分。

#### 1. 拆错（DDD/Java 味）vs 拆对（Go 味）

```
❌ 错误：按 "kind of thing" 拆
domain/chat/
├── types.go        (全部 struct)
├── errors.go       (全部错误)
├── constants.go    (全部常量)
└── interfaces.go   (全部接口)

✅ 正确：按 "concept / feature" 拆
domain/chat/
├── chat.go         Conversation 核心 + godoc
├── message.go      Message struct + 相关常量/错误
├── stream.go       流式输出契约
└── repository.go   存储接口
```

每个文件还是混合 types + 常量 + errors + 小 interface——只要它们围绕**同一个子概念**。
对照 stdlib：`net/http/request.go` 同时定义 `Request` 类型、它的方法、相关常量、相关错误。

#### 2. 主文件命名

主文件用**包名**（如 `apikey.go`、`chat.go`）。包级 godoc **只写在主文件顶部**；其他文件的文件头注释要和 `package X` 之间留空行，免得 godoc 当成二次包 doc 拼接。**禁止**单独建 `doc.go`。

**三层统一**：这条规则适用于 domain / app / infra/store 全部三层——不只是 domain 层。

```
domain/apikey/apikey.go       ← 主文件
app/apikey/apikey.go          ← 主文件（不叫 service.go）
infra/store/apikey/apikey.go  ← 主文件（不叫 store.go）
```

例外：有独立接口 + 独立具体类型 + 独立测试的子组件可以单独一个文件（如 `tester.go`）。
仅"Service 实现某接口"或"小工具函数"这类情况，合并进主文件，不单独建文件。

#### 6. 辅助注册表文件的归属

`providers.go`（provider 注册表）这类"纯配置 + 查询函数"的文件，放在**消费它的层**，而非 domain 层。判断标准：

> domain 层自身使用 → 放 domain；仅 app 层消费 → 放 app。

`apikey/providers.go` 的所有消费者（`Service.validateCreate`、`HTTPTester.Test`）都在 app 层，domain 层的 entity / interface 不使用它，故放 `app/apikey/providers.go`。

#### 3. 文件长度

- < 500 行 舒服
- 500-1000 行 可接受（只要概念内聚）
- 1000+ 行 该拆，但拆**文件**不拆包，按子概念（`message.go` / `stream.go`），**不**按种类

#### 4. 何时拆子包

两个硬条件**同时满足**才拆：
1. 有独立的**词汇体系**（开始给内部概念起专门的名字）
2. 至少 **10+ 个文件**围绕这个子词汇

stdlib 例子：`net/http/cookiejar`（cookie 自有概念）、`database/sql/driver`（driver vs user 两套 API）。但 `net/http` 本体 60+ 文件就是平铺。

#### 5. 共享纯工具

跨 domain 用的纯函数（无业务、无 infra 依赖）放 `internal/pkg/<name>/`（如 `pkg/reqctx/`）。

---

### S13 包命名（三层同名 + 调用方别名）

#### 1. 包内统一名

每个 domain 在 **domain / app / infra/store** 三层的包名都用 domain 单名（如 `apikey`）：

| 目录 | 包声明 |
|---|---|
| `internal/domain/apikey/` | `package apikey` |
| `internal/app/apikey/` | `package apikey` |
| `internal/infra/store/apikey/` | `package apikey` |

#### 2. 调用方按角色起别名

外部 import 时按 `<name><role>` 区分：

```go
import (
    apikeydomain "github.com/sunweilin/forgify/backend/internal/domain/apikey"
    apikeyapp    "github.com/sunweilin/forgify/backend/internal/app/apikey"
    apikeystore  "github.com/sunweilin/forgify/backend/internal/infra/store/apikey"
)

repo := apikeystore.New(gdb)
svc  := apikeyapp.NewService(repo, ...)
var _ apikeydomain.Repository  = repo
var _ apikeydomain.KeyProvider = svc
```

#### 3. 互相 import 的别名规则

层间互引时，被引方按角色别名（**即使在自己包里**）：

```go
// internal/infra/store/apikey/store.go
package apikey                                              // 自己叫 apikey

import (
    apikeydomain "…/internal/domain/apikey"                 // 引 domain 起别名
)

func (s *Store) Get(ctx context.Context, id string) (*apikeydomain.APIKey, error) {
    if errors.Is(err, gorm.ErrRecordNotFound) {
        return nil, apikeydomain.ErrNotFound
    }
}
```

> 当前包的 `package apikey` 声明本身**不占用** identifier slot，技术上不别名也能编过；但**统一别名**是项目规范，便于人眼一眼分辨"这是哪一层的 apikey"。

#### 4. 接口定义位置

- **接口定义**在 domain 层（如 `apikeydomain.Repository`、`apikeydomain.KeyProvider`）
- **store 实现** Repository（被 service 消费）
- **app/Service 实现** KeyProvider（被其他 domain 消费）
- **跨 domain 消费**只通过 port 接口，禁止暴露 entity

#### 5. 为什么这样定

- **包内统一名**：每个 layer 内部代码读起来"就是 apikey"，不用 `apikeydomain.` 这种长前缀
- **调用方别名**：组装时（main.go、service 互相依赖）一眼分辨层
- **接口在 domain**：实现可换（GORM → ent → mock），domain 不动

---

---

### S14 📌 文档同步纪律（最高优先级）

对应设计原则 #7。这一条是**全项目最硬的纪律**，比代码风格更严。理由：文档是多人（含未来的你）在做新决策时唯一的参考；**滞后的文档 = 集体性质的 bug，损害下一个 domain 的设计和实现**。

#### 1. 三处联动（每次代码改动都问自己"另外两处动没？"）

当代码改动涉及以下任何一条，**三处都要动**：

| 代码变动类型 | 联动位置 |
|---|---|
| 新 entity / 表 / struct 字段改动 / 约束改动 | ① `service-design-documents/<domain>.md` 的 §领域模型 + §数据库表<br>② `service-contract-documents/database-design.md` 的索引行<br>③ `progress-record.md` 的 dev log |
| 新 sentinel 错误 / 新 errmap 行 | ① `service-design-documents/<domain>.md` 的 §错误码<br>② `service-contract-documents/error-codes.md` 的表格行<br>③ `progress-record.md` dev log |
| 新 endpoint / request/response 形状改 / path 变 | ① `service-design-documents/<domain>.md` 的 §HTTP API 详细<br>② `service-contract-documents/api-design.md` 的端点表<br>③ `progress-record.md` dev log |
| 新事件 / struct 改 / 过滤 key 改 | ① `service-design-documents/<domain>.md` 的 §事件<br>② `service-contract-documents/events-design.md` 的表格行<br>③ `progress-record.md` dev log |
| 方法签名改 / 新方法 / 接口变 | ① `service-design-documents/<domain>.md` 的对应章节<br>②（仅当影响对外入口才动索引级别的 contract 文档） |
| 新/变跨 domain 依赖 | ① `service-design-documents/<domain>.md` 的 §对外 API + §消费方 + §协作图<br>② 受影响的其他 domain 的 `<other-domain>.md` 也要改 |

#### 2. 每个 domain 推进时的标准 checklist

每当一个子任务做完（如 Task #3 tester 写完、Task #5 handler 写完）：

- [ ] `service-design-documents/<domain>.md` 的"实现清单" 勾 ✅ 对应条目
- [ ] 如改了 API/schema/error，`service-contract-documents/*.md` 对应表格行更新 ✅
- [ ] `progress-record.md` 加一行 dev log（**含具体做了什么 + 测试数 + 新规范/决策**，不是空泛的 "完成了 X"）
- [ ] 如立了新原则/规范，加到 `backend-design.md` § 设计原则 / Standards 章节

#### 3. 每个 domain 完工时的总体 checklist

- [ ] `service-design-documents/<domain>.md` 整体过一遍是否与代码**逐字段匹配**（entity gorm tag、方法签名、endpoint 形状、错误码、调用链）
- [ ] `service-contract-documents/*.md` 该 domain 行从 ⬜ 改成 ✅ / 🔄 并补端点 / 字段清单
- [ ] `progress-record.md` 更新 Phase 2 子任务表状态 + 加完工日志条目
- [ ] `backend-design.md` 如 domain 引入新的跨域模式（如 apikey 引入 KeyProvider + HTTPTester 这种层），更新 § Target Architecture 的说明

#### 4. 发现文档与代码不符时

- **立刻停下手里的事修文档**（哪怕正在写新 domain）
- 修完记一条 dev log，类别标 `[doc-fix]`
- 反思：为什么当时没联动？是不是缺了 checklist 入口？

#### 5. 审查文档的典型套路

在开始做一个新 domain 前，以"我要实现一个完全新的 domain，我要从文档里找指南"的视角读一遍：

- 读 `backend-design.md` 找规范
- 读对应 `<domain>.md` 详设计找具体做什么
- 读 `service-contract-documents/*` 确认索引层信息一致
- 读 `progress-record.md` 找"刚刚别的 domain 用了什么套路"

**如果你在读的过程中发现"这里描述的和我脑子里的不一致"或"这里少了一块"，立刻修文档，然后再继续 domain 实现**。

#### 6. 为什么把它升到最高优先级

- 单次漏改文档成本小（几行字），积累下来的成本巨大（后续 domain 的设计决策建立在错误信息上）
- 本项目是边做边讨论（"我们需要边讨论边做"），规范会随项目演化；文档是唯一**持久保存演化结果**的地方
- 代码能告诉你"是什么"，文档告诉你"为什么 / 怎么连 / 其他地方还会用到什么" —— 后者失真，整个协作就失血
- 本人既是作者、审阅者、未来的维护者；**对未来的自己诚实 = 给未来的自己减负**

---

## Target Architecture

> 以 apikey 为参照样板。其他 domain 按同样套路开。

```
backend-new/
├── cmd/server/main.go              ← 入口，DI 组装
├── go.mod / go.sum
└── internal/
    ├── domain/                     ← 纯业务（仅 import 标准库 + GORM tag）
    │   ├── apikey/                 ← 平铺，按概念拆文件（S12）
    │   │   ├── apikey.go           ← entity + 常量 + errors + Credentials +
    │   │   │                          ListFilter + Repository + KeyProvider 接口
    │   │   ├── providers.go        ← 11 provider 白名单
    │   │   └── providers_test.go
    │   ├── model/                  ← Phase 2
    │   ├── conversation/           ← Phase 2
    │   ├── chat/                   ← Phase 2
    │   ├── tool/                   ← Phase 3
    │   ├── forge/                  ← Phase 3
    │   ├── attachment/             ← Phase 3
    │   ├── workflow/               ← Phase 4
    │   ├── flowrun/                ← Phase 4
    │   ├── scheduler/              ← Phase 4
    │   ├── trigger/                ← Phase 4
    │   ├── knowledge/              ← Phase 5
    │   ├── document/               ← Phase 5
    │   ├── intent/                 ← Phase 5
    │   ├── mcpserver/              ← Phase 5
    │   ├── skill/                  ← Phase 5
    │   ├── crypto/                 ← 接口（已）
    │   ├── events/                 ← 接口（已）
    │   └── errors/                 ← 跨 domain 通用 sentinel（已）
    │
    ├── app/                        ← service 层（协调 domain + infra）
    │   ├── apikey/                 ← 包名 apikey，调用方别名 apikeyapp（S13）
    │   │   ├── service.go          ← Service（CRUD + Test 编排）
    │   │   ├── keyprovider.go      ← 实现 apikeydomain.KeyProvider
    │   │   ├── tester.go           ← ConnectivityTester + HTTPTester
    │   │   ├── mask.go             ← MaskKey
    │   │   └── *_test.go
    │   └── <其他 domain>/          ← 按 Phase 2-5 逐个落
    │
    ├── infra/                      ← 技术实现
    │   ├── db/                     ← 通用 DB 底层（domain 无关）
    │   │   ├── db.go               ← GORM 初始化（WAL / FK / PrepareStmt / UTC）
    │   │   ├── migrate.go          ← AutoMigrate + schema_extras 入口
    │   │   └── schema_extras.go    ← FTS5 / 部分索引等 GORM tag 表达不了的 SQL
    │   ├── store/                  ← domain-aware 的 Repository 实现
    │   │   └── apikey/             ← 包名 apikey，调用方别名 apikeystore（S13）
    │   ├── eino/                   ← Eino LLM gateway（Phase 2 chat 时填）
    │   ├── sandbox/                ← Python 执行（Phase 3）
    │   ├── events/                 ← in-memory event bridge（已）
    │   ├── crypto/                 ← AES-256-GCM 加解密（已）
    │   └── logger/                 ← Zap 配置（已）
    │
    ├── pkg/                        ← 跨层共享纯工具（无业务、无 infra 依赖）
    │   └── reqctx/                 ← user_id / locale 注入与读取（已）
    │
    └── transport/
        └── httpapi/                ← 包名避开 net/http 冲突
            ├── router/             ← 路由注册 + Deps DI
            ├── response/           ← 📦 envelope + errmap（框架级通用能力）
            ├── middleware/         ← 📦 recover / logger / cors / locale / userid / notfound
            ├── pagination/         ← 📦 cursor 分页解析与编码
            └── handlers/           ← 📦 业务 handler（每 domain 一个文件）
                ├── health.go       ← 已
                ├── apikey.go       ← 已（Phase 2 #5）
                └── <其他>.go       ← 各 Phase 逐个落
```

**依赖方向**：`transport → app → (domain ∪ infra/store)`、`infra/store → domain`（实现接口）、`infra/db → 标准库`、`domain` 不依赖任何人。

**`infra/db/` vs `infra/store/<domain>/` 的拆分**：
- `infra/db/` —通用 DB 底层（连接、迁移、schema_extras），与任何具体表无关
- `infra/store/<domain>/` —表相关的 CRUD（业务 aware），实现 `domain/<domain>.Repository`
- 同一个 domain 在 store 层的包名也叫 `<domain>`（如 `apikey`），调用方 import 时按 `<name><role>` 起别名（见 S13）

**类型策略**：domain 类型直接带 GORM tag（一份到底）；store 层不再做 entity↔row 转换。

**transport/httpapi 内部分层原则**：**稳定的（通用能力）和频繁变的（业务 handler）分开放**。
- `response/` `middleware/` `pagination/` 属于框架级通用能力，写一次用很久
- `handlers/` 属于业务级代码，每加一个 feature 就新增/修改

---

## 文档分册结构

本文档是**稳定规范层**。其余按角色分三组：

| 文档 | 用途 | 推进节奏 |
|---|---|---|
| [`service-contract-documents/api-design.md`](./service-contract-documents/api-design.md) | **全部 REST API 一眼索引**（路径 / 方法 / 用途 + 指向详设计）| 每 domain 开工时加一段 |
| [`service-contract-documents/database-design.md`](./service-contract-documents/database-design.md) | **全部表一眼索引**（要点 + 指向详设计）| 同上 |
| [`service-contract-documents/error-codes.md`](./service-contract-documents/error-codes.md) | **全部错误码一眼索引**（code / HTTP / sentinel / 场景）| 同上 |
| [`service-contract-documents/events-design.md`](./service-contract-documents/events-design.md) | **全部 SSE 事件一眼索引** | 涉及流式时加 |
| [`service-design-documents/<domain>.md`](./service-design-documents/) | **每个 domain 的详设计**（调用链 / entity / Service / 端点 / 错误码 / schema / 端到端推演 / 实现清单）| 每 domain 开工前写 |
| [`progress-record.md`](./progress-record.md) | 开发日志 + 当前完成快照 + 任务清单 + 原则演化 | 实时更新 |

**工作流**：
1. **开工前** → 填 `service-design-documents/<domain>.md` 详设计（含端到端推演 + 实现清单）
2. **实现中** → 同步更新 `service-contract-documents/*.md` 里该 domain 的索引段
3. **完成后** → 在 `progress-record.md` 加一行 dev log + 勾任务清单

---

## Verification

### 单元测试
- `go test -count=1 -race ./...` 零失败
- domain/ 层覆盖率 > 80%（纯逻辑好测）
- app/ 层核心 service 必测

### 契约测试
每个端点一个 curl 脚本，验证：
- 状态码正确
- envelope 格式正确
- 错误码符合约定
- 分页参数生效

### 端到端场景（Phase 3 可测起）
1. 新建对话 → AI 回复 → 创建工具 → 分屏 → 测试工具 → 失败 → 让 AI 修复 → 成功
2. 归档对话 → 查看归档列表 → 恢复
3. 导出工具 → 删除 → 重新导入

### 性能基准
- 流式对话 token latency < 旧版 110%
- 工具列表加载 < 500ms
- 搜索响应 < 300ms（FTS5 加持）

### Schema 完整性
- `PRAGMA foreign_key_check` 零返回
- `PRAGMA integrity_check` 返回 `ok`

---

## 非目标（本轮不做）

- ❌ 多租户真实 user_id 来源 —— 先硬编码 `local-user`；未来 SaaS 时加 auth middleware
- ❌ Docker 沙箱 —— 保持 subprocess
- ❌ 前端类型生成工具链 —— 下轮前端 iteration 再接
- ❌ 前端代码改动 —— 下轮独立做
