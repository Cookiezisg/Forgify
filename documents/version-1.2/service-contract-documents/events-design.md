# Events Design — V1.2 SSE 事件一眼索引

**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- **配套实现**：`domain/events.Bridge` 接口 + `infra/events/memory.Bridge`（已就绪，72 测试）
- **SSE 订阅端点**：`GET /api/v1/events?conversationId=xxx`（Phase 2 chat 落地时接）

**定位**：**全仓所有 SSE 事件一眼索引**。每个事件的完整 struct / 触发时机 / 详细载荷，**去对应 domain 的 `service-design-documents/<domain>.md` 看**。

**遵守标准**：E1（强类型，禁止 `map[string]any`）/ E2（snake_case 分层，必带过滤上下文）

---

## 全局约定

### 事件传输
- 客户端通过 `GET /api/v1/events?conversationId=xxx` 订阅 SSE 流
- 后端 `domain/events.Bridge` 接口 + `infra/events/memory.Bridge` 内存实现
- 未来 SaaS 可换 `infra/events/redis` 实现，业务代码零改

### 事件命名（E2）
- 全部 snake_case，按 domain 加点号前缀：`chat.token`、`tool.code_updated`
- 每个事件必带 `conversationId` 或其他过滤上下文（subscriber 的 filter key）
- **死事件禁止**：每个事件必须有真实发布点

### 事件 struct（E1）
所有事件必须有具体 Go struct，定义在 `domain/events/types.go` 或按 domain 分文件。
**禁止** 发布或订阅时使用 `map[string]any`。

### 字段规范
- 字段命名：`camelCase` JSON tag（前端友好）
- 每个事件必带 `conversationId` 或其他明确过滤上下文

### Bridge 行为（已实现）
- 慢订阅者 buffer 满 → **丢弃 + warn log**，不阻塞 publisher
- ctx 结束 → 自动 unsubscribe
- 多次 cancel → sync.Once 保证幂等

---

## 事件清单

> **状态**：⬜ 未设计 | 🔄 struct 定义中 | ✅ 已实现（struct + 真实发布点）

### Phase 2：基础对话能力

| 事件名 | 用途 | 过滤 key | 状态 |
|---|---|---|---|
| `chat.reasoning_token` | 推理模型 thinking 内容增量（`messageId` + `delta`），仅 DeepSeek-R1 等推理型模型产生 | `conversationId` | ✅ |
| `chat.token` | 流式 token 增量（`messageId` + `delta`）| `conversationId` | ✅ |
| `chat.tool_call` | Agent 调用 tool（`toolCallId` + `toolName` + `toolInput` + `summary`）| `conversationId` | ✅ |
| `chat.tool_result` | Tool 执行完成（`toolCallId` + `result` + `ok`）| `conversationId` | ✅ |
| `chat.done` | 流式完成（`messageId` + `stopReason` + `tokenUsage`）| `conversationId` | ✅ |
| `chat.error` | 流式错误（`code` + `message`，code 匹配 SCREAMING_SNAKE_CASE）| `conversationId` | ✅ |
| `conversation.title_updated` | Auto-titling 回写标题（`title` + `autoTitled`）| `conversationId` | ✅ |

---

### Phase 3：工具锻造能力

| 事件名 | 用途 | 过滤 key | 状态 |
|---|---|---|---|
| `tool.code_streaming` | create_tool / edit_tool 代码生成逐 token（`messageId` + `toolCallId` + `toolId` + `actionType` + `delta`）| `conversationId` | ✅ |
| `tool.created` | create_tool 成功保存新工具（`messageId` + `toolCallId` + `toolId` + `toolName`）| `conversationId` | ✅ |
| `tool.pending_created` | edit_tool 保存 pending 变更（`messageId` + `toolCallId` + `toolId` + `pendingId` + `instruction`）| `conversationId` | ✅ |
| `tool.test_case_generated` | generate-test-cases 生成一条完整测试用例（`toolId` + `testCaseId` + `name` + `inputData` + `expectedOutput`）| `toolId` | ✅ |
| `tool.test_cases_done` | generate-test-cases 全部完成（`toolId` + `count`）| `toolId` | ✅ |
| `tool.test_cases_not_supported` | LLM 判断工具不可自动测试（`toolId` + `reason`）| `toolId` | ✅ |

---

### Phase 4：工作流能力

| 事件名 | 用途 | 过滤 key | 状态 |
|---|---|---|---|
| `workflow.run_started` | 工作流开始运行 | `flowrunId` | ⬜ |
| `workflow.node_started` | 某节点开始执行 | `flowrunId` | ⬜ |
| `workflow.node_completed` | 某节点完成 | `flowrunId` | ⬜ |
| `workflow.run_completed` | 工作流运行完成 | `flowrunId` | ⬜ |
| `workflow.run_failed` | 工作流运行失败 | `flowrunId` | ⬜ |

---

### Phase 5：智能化能力

| 事件名 | 用途 | 过滤 key | 状态 |
|---|---|---|---|
| `intent.identified` | 意图识别结果 | `conversationId` | ⬜ |
| `knowledge.indexing_progress` | 知识库索引进度 | `knowledgeBaseId` | ⬜ |
| `mcp.server_connected` | MCP server 连接成功 | `global` | ⬜ |
