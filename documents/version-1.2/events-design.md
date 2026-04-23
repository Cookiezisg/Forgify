# Events Design — V1.2 SSE 事件设计

**关联**：[backend-rewrite.md](./backend-rewrite.md) — 总路线图
**遵守标准**：E1（强类型，禁止 `map[string]any`）/ E2（snake_case 分层，必带上下文）

---

## 全局约定

### 事件传输
- 客户端通过 `GET /api/v1/events?conversationId=xxx` 订阅 SSE 流
- 后端 `domain/events.Bridge` 接口 + `infra/events/memory` 内存实现（已就绪）
- 未来 SaaS：`infra/events/redis` 实现，业务代码零改

### 事件命名（E2）
- 全部 snake_case，按 domain 加点号前缀：`chat.token`、`tool.code_updated`
- 死事件**禁止**：每个事件必须有真实发布点

### 事件 struct（E1）
所有事件必须有具体 Go struct 定义在 `domain/events/types.go` 或按 domain 分文件。
禁止在发布或订阅时使用 `map[string]any`。

### 字段规范
- 字段命名：`camelCase` JSON tag（前端友好）
- 每个事件必带 `conversationId` 或其他过滤上下文（subscribeFilter key 用得上）

---

## 事件清单

> 推进规则：每个 Phase 涉及流式或异步通知时，把该 Phase 新增的事件填到下方
> 状态：⬜ 未设计 | 🔄 设计中 | ✅ 已实现

### Phase 1（已实现）

| 事件名 | struct | 触发位置 | 状态 |
|---|---|---|---|
| `chat.token` | `events.ChatToken` | （Phase 2 chat 实现时发布）| ✅ struct 已定义 |

---

### Phase 2：基础对话能力

| 事件名 | 用途 | 状态 |
|---|---|---|
| `chat.token` | 流式 token 增量 | ⬜ 待补 struct |
| `chat.done` | 流式完成 | ⬜ |
| `chat.error` | 流式错误 | ⬜ |
| `conversation.title_updated` | 自动命名后通知 | ⬜ |

---

### Phase 3：工具锻造能力

| 事件名 | 用途 | 状态 |
|---|---|---|
| `tool.code_detected` | AI 在回复中生成了代码块 | ⬜ |
| `tool.code_updated` | AI 改了已绑定的工具代码 | ⬜ |
| `tool.run_started` | 工具开始运行 | ⬜ |
| `tool.run_completed` | 工具运行完成 | ⬜ |

---

### Phase 4：工作流能力

| 事件名 | 用途 | 状态 |
|---|---|---|
| `workflow.run_started` | 工作流开始运行 | ⬜ |
| `workflow.node_started` | 某节点开始执行 | ⬜ |
| `workflow.node_completed` | 某节点完成 | ⬜ |
| `workflow.run_completed` | 工作流运行完成 | ⬜ |
| `workflow.run_failed` | 工作流运行失败 | ⬜ |

---

### Phase 5：智能化能力

| 事件名 | 用途 | 状态 |
|---|---|---|
| `intent.identified` | 意图识别结果 | ⬜ |
| `knowledge.indexing_progress` | 知识库索引进度 | ⬜ |
| `mcp.server_connected` | MCP 服务器连接成功 | ⬜ |

---

## 事件订阅规则

### 过滤 key 约定
- 大部分事件用 `conversationId` 作为过滤 key（订阅特定对话）
- 工作流执行用 `flowrunId`
- 全局事件（如 MCP 连接状态）用空 key 或 `global`

### Bridge 行为（已实现）
- 慢订阅者 buffer 满 → **丢弃 + 日志**，不阻塞 publisher
- ctx 结束 → 自动 unsubscribe
- 多次 cancel → sync.Once 保证幂等
