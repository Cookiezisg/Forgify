# Integration Dev Console — 设计文档

**创建于**：2026-04-26
**关联**：[`backend-design.md`](./backend-design.md) / [`progress-record.md`](./progress-record.md)

---

## 概述

`integration/` 是面向开发者的本地调试面板。`make dev` 一键启动，浏览器打开即可：配置凭证、真实聊天、观察流式响应、查后端日志、查数据库、测 API、跑测试集合。

**不是前端应用，不进生产——纯 dev 工具。**

---

## 目录结构

```
integration/
├── index.html              Dev Console UI（单文件，Alpine.js CDN，无构建）
├── Makefile                make dev / make stop
└── collections/            测试集合配置文件（YAML）
    ├── phase2-smoke.yaml   Phase 2 冒烟测试
    └── ...                 用户自定义
```

后端新增（仅 `--dev` 模式挂载）：

```
backend/internal/
├── infra/logger/
│   └── broadcast.go        LogBroadcaster（Zap Core + SSE 扇出）
└── transport/httpapi/handlers/
    └── dev.go              /dev/logs  /dev/sql  /dev/collections
```

---

## UI 布局

```
┌─────────────────────────────────────────────────────────────┐
│  Forgify Dev Console                              ⚙ Settings │
├───────────────┬────────────────────────┬────────────────────┤
│               │                        │  SSE │ Logs │ SQL │ Tests │
│  对话列表     │      聊天区            │                    │
│  sidebar      │   (流式渲染)           │  右侧工具面板      │
│               │                        │  (4 个 Tab)        │
│  + 新对话     │  输入框 + Send / Cancel│                    │
└───────────────┴────────────────────────┴────────────────────┘
```

---

## 后端变更

### 修改文件

| 文件 | 变更 |
|---|---|
| `router/deps.go` | 加 `Dev bool`、`DB *gorm.DB`、`LogBroadcaster *logger.LogBroadcaster` |
| `router/router.go` | `if deps.Dev { devHandler.Register(mux) }` |
| `cmd/server/main.go` | dev 模式构建 LogBroadcaster，接入 Zap tee core，传入 Deps |
| `infra/logger/zap.go` | `New()` 接受可选 extra `zapcore.Core`（dev 时传 broadcaster） |

### LogBroadcaster（infra/logger/broadcast.go）

实现 `zapcore.Core`，作为 tee core 的第二路，仅 dev 模式启用。

```
LogBroadcaster
├── ring buffer（最近 500 条，供新订阅者连接时回放）
├── subs  []*logSub
└── Write(entry) → 追加 buffer + 非阻塞扇出
```

LogEntry 结构（SSE data payload）：
```json
{
  "time": "2026-04-26T10:00:00Z",
  "level": "info",
  "msg": "chat task done",
  "fields": { "conversation_id": "cv_xxx", "stop_reason": "end_turn" }
}
```

设计与 `infra/events/memory/bridge.go` 对称：RLock 快照 subs → 释放锁 → 发送，慢订阅者非阻塞丢弃。

### Dev Endpoints（handlers/dev.go）

仅 `--dev` 时注册，不走 errmap，直接返回 JSON。

#### `GET /dev/logs` — SSE 日志流
1. 先把 ring buffer 历史全量推一遍（replay）
2. 订阅新条目，持续推送
3. 每 15s 发 `: keep-alive`

#### `POST /dev/sql` — SQL 查询
```
Request:  { "sql": "SELECT ..." }
Response: { "columns": ["id", "role", ...], "rows": [["msg_x", "user"], ...] }
         | { "error": "只允许 SELECT 语句" }
```
- 前缀必须是 `SELECT`（`strings.ToUpper(TrimSpace(sql))` 检查）
- `db.Raw(sql).Rows()` + `(*sql.Rows).Columns()` 动态扫描返回

#### `GET /dev/collections` — 读取集合列表
扫描 `integration/collections/*.yaml`，返回每个集合的 name / description / step 数量。
路径通过启动参数 `--collections-dir` 传入（main.go → Deps）。

#### `GET /dev/tools` — 列出 system tools
返回所有注册到 agent 的 system tool 名称和描述。
```
Response: [{"name": "web_search", "desc": "..."}, ...]
```

#### `POST /dev/invoke` — 直接调用 system tool
绕过 LLM agent 直接执行指定 system tool，供调试使用。
```
Request:  { "tool": "web_search", "args": "{\"query\": \"go generics\"}" }
Response: { "output": "...", "ok": true, "elapsedMs": 342 }
         | { "output": "", "ok": false, "elapsedMs": 12, "error": "tool not found: xxx" }
```
- `args` 为 JSON 字符串，传给 tool 的 `InvokableRun`；省略时默认 `{}`
- `tool` 名称不存在或 tool 未实现 `InvokableTool` 接口时返回 404

#### `POST /dev/collections/{name}/run` — 运行测试集合
- 按 YAML step 顺序执行，每步发 HTTP 请求到本机后端
- 支持上下文变量捕获（`capture.convId: "$.data.id"`）和替换（`{{convId}}`）
- 支持环境变量（`{{env.TEST_API_KEY}}`）
- 返回每步结果：`{ name, method, path, status, pass, latencyMs, response }`
- 结果存内存，刷新后清空

---

## 右侧工具面板（6 个 Tab）

### Tab 1 — SSE
- **Stream 视图**（默认）：按 messageId 聚合，每轮一个蓝左边框块，展示工具调用摘要 + 拼装文本
- **Raw 视图**：逐条 pretty-print JSON，[Stream][Raw] 切换按钮
- 支持 `chat.tool_call` / `chat.tool_result` 事件展示工具 input/output（可展开 details）
- 切换对话时自动重新订阅，Clear 按钮同时清空两个视图

### Tab 2 — Logs
- 连接 `GET /dev/logs` EventSource
- 每行：`[时间] LEVEL  msg  {fields JSON}`
- level 颜色：`INFO`=绿 / `WARN`=黄 / `ERROR`=红 / `DEBUG`=灰
- 启动时回放历史最近 500 条
- Auto-scroll + Clear + 关键词过滤输入框

### Tab 3 — SQL
- textarea 输入 SQL → Run → `POST /dev/sql`
- 结果渲染为可横向滚动的 HTML table，含行数统计
- 快捷按钮：messages / conversations / api_keys / model_configs / attachments /
  tools / tool_versions / tool_test_cases / tool_run_history / tool_test_history

### Tab 4 — Tests（测试集合）
- 左侧：集合列表（`GET /dev/collections`）
- 右侧：选中集合的 step 列表 + Run 按钮
- 运行时每步实时更新状态：⏳ running / ✅ pass / ❌ fail
- 每步可展开：请求详情 + 响应体 + assert 结果 + latencyMs
- 顶部环境变量输入框（`KEY=VALUE` 格式，注入 `{{env.KEY}}`）

### Tab 5 — Config
- API Keys CRUD + test connectivity
- Chat Model 选择

### Tab 6 — Tools
两个子面板通过 [System][User Tools] 切换：

**System 子面板**（`/dev/invoke`）
- 下拉选择 tool（`GET /dev/tools` 填充）
- JSON textarea 填入参数（Ctrl+Enter 运行）
- 展示 output、ok/error、elapsedMs

**User Tools 子面板**（`/api/v1/tools`）
- 可搜索的工具列表（最多 200 条）
- 点击工具 → 展示代码预览 + JSON input 表单
- Run 按钮 → `POST /api/v1/tools/:id:run` → 展示 ExecutionResult

---

## 测试集合格式（collections/*.yaml）

```yaml
name: "Phase 2 冒烟测试"
description: "apikey / model / conversation / chat 基础流程"
steps:
  - name: "创建 API Key"
    method: POST
    path: /api/v1/api-keys
    body:
      provider: "openai"
      key: "{{env.TEST_API_KEY}}"
      displayName: "smoke-test"
    expect:
      status: 201

  - name: "设置模型"
    method: PUT
    path: /api/v1/model-configs/chat
    body:
      provider: "openai"
      modelId: "gpt-4o-mini"
    expect:
      status: 200

  - name: "创建对话"
    method: POST
    path: /api/v1/conversations
    body:
      title: "smoke test conv"
    expect:
      status: 201
    capture:
      convId: "$.data.id"

  - name: "发消息"
    method: POST
    path: /api/v1/conversations/{{convId}}/messages
    body:
      content: "Hello"
    expect:
      status: 202
```

**字段说明**：

| 字段 | 必填 | 说明 |
|---|---|---|
| `name` | ✅ | 步骤名 |
| `method` | ✅ | HTTP 方法 |
| `path` | ✅ | 路径，可含 `{{变量}}` |
| `body` | ❌ | JSON 对象，可含 `{{变量}}` |
| `expect.status` | ❌ | 期望状态码，不填则不断言 |
| `capture` | ❌ | 从响应 JSON 捕获变量，支持 JSONPath（`$.data.id`）|

---

## Makefile（integration/Makefile）

```makefile
BACKEND_DATA_DIR  ?= /tmp/forgify-dev
COLLECTIONS_DIR   ?= $(shell pwd)/collections

dev:
	@echo "→ Starting Forgify backend (dev mode)..."
	@cd ../backend && \
	  CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" \
	  go run ./cmd/server \
	    --dev \
	    --data-dir $(BACKEND_DATA_DIR) \
	    --collections-dir $(COLLECTIONS_DIR) &
	@sleep 1
	@echo "→ Opening dev console..."
	@open index.html

stop:
	@pkill -f "forgify/backend" 2>/dev/null || true
	@echo "→ Backend stopped"

.PHONY: dev stop
```

---

## 验证步骤

```bash
cd integration && make dev
# 浏览器自动打开 index.html

# 1. ⚙ Settings → 填 API Key + 模型 → 保存
#    Logs Tab：看到 "apikey created" + "model config upserted"
# 2. 左侧新建对话 → 中间输入框发一条消息
#    聊天区：流式 token 逐字出现，标题自动生成后 sidebar 更新
# 3. SSE Tab：看到 chat.token * N 条 + chat.done
# 4. Logs Tab：chat task enqueued → chat task done（含 stop_reason）
# 5. SQL Tab → 快捷按钮 messages：出现 user + assistant 两行
# 6. API Tab：GET /api/v1/conversations → 返回对话列表 JSON
# 7. Tests Tab → 选 phase2-smoke.yaml → 填 TEST_API_KEY → Run → 全绿

# 验证非 dev 模式下路由不暴露：
# go run ./cmd/server（不加 --dev）→ curl PORT/dev/logs → 404
```

```bash
# 回归测试
cd backend && CGO_CFLAGS="-DSQLITE_ENABLE_FTS5" go test -count=1 -race ./...
```
