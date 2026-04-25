# Error Codes — V1.2 错误码一眼索引

**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- **配套实现**：`internal/transport/httpapi/response/errmap.go`

**定位**：**全仓所有错误码、HTTP 状态、sentinel 一眼索引**。每个 code 的详细触发条件、details 字段，**去对应 domain 的 `service-design-documents/<domain>.md` 看**。

---

## 全局约定

### 错误码命名
- 全部大写 + 下划线：`SCREAMING_SNAKE_CASE`
- 按 domain 加前缀（除非通用）：`TOOL_NOT_FOUND`、`API_KEY_INVALID`

### 三层错误模型

```
┌─────────────────────────────────────────────┐
│ domain/<name>/*.go                            │
│   var ErrNotFound = errors.New("...")         │  ← Sentinel
└──────────────────┬───────────────────────────┘
                   │ errors.Is 匹配
                   ↓
┌─────────────────────────────────────────────┐
│ transport/httpapi/response/errmap.go          │
│   errTable: sentinel → (HTTP, code)           │  ← 翻译表
└──────────────────┬───────────────────────────┘
                   │
                   ↓
            { "error": { "code", "message", "details" }}
```

### 添加新错误码的流程（三步缺一不可）

1. 在 `domain/<name>/*.go` 声明 sentinel：`var ErrNotFound = errors.New("<domain>: not found")`
2. 在 `response/errmap.go` 加映射行：`<domain>.ErrNotFound: {http.StatusNotFound, "<DOMAIN>_NOT_FOUND"}`
3. 在本文档对应 domain 段加一行

handler 侧调 `response.FromDomainError(w, log, err)` 自动翻译。

### 兜底
未注册到 `errTable` 的错误自动降级为 `500 INTERNAL_ERROR`，原始 message **不**暴露给客户端（防泄漏实现细节）。

---

## 错误码清单

> **状态**：⬜ 未定义 | ✅ 已实现

### 通用（Phase 1）

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `INVALID_REQUEST` | 400 | `derrors.ErrInvalidRequest` | JSON 坏 / 字段缺 / cursor 格式错 | ✅ |
| `INTERNAL_ERROR` | 500 | `derrors.ErrInternal` | 兜底；未映射错误降级到此 | ✅ |
| `NOT_FOUND` | 404 | (middleware 直接发，不走 errmap) | 路由未匹配 | ✅ |

---

### Phase 2：基础对话能力

#### apikey ✅
详见 [`../service-design-documents/apikey.md`](../service-design-documents/apikey.md) §13。

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `API_KEY_NOT_FOUND` | 404 | `apikey.ErrNotFound` | id 查不到 | ✅ |
| `API_KEY_PROVIDER_NOT_FOUND` | 404 | `apikey.ErrNotFoundForProvider` | 当前用户 该 provider 无活跃 key | ✅ |
| `INVALID_PROVIDER` | 400 | `apikey.ErrInvalidProvider` | provider 不在 11 白名单 | ✅ |
| `BASE_URL_REQUIRED` | 400 | `apikey.ErrBaseURLRequired` | ollama / custom 没填 baseURL | ✅ |
| `API_FORMAT_REQUIRED` | 400 | `apikey.ErrAPIFormatRequired` | custom 没填 apiFormat | ✅ |
| `KEY_REQUIRED` | 400 | `apikey.ErrKeyRequired` | 创建时 key 空 | ✅ |
| `API_KEY_TEST_FAILED` | 422 | `apikey.ErrTestFailed` | 连通性测试失败 | ✅ |
| `API_KEY_INVALID` | 401 | `apikey.ErrInvalid` | 使用时 provider 返回 401 | ✅ |

#### model 🔄
详见 [`../service-design-documents/model.md`](../service-design-documents/model.md)。

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `MODEL_NOT_CONFIGURED` | 422 | `model.ErrNotConfigured` | chat 调 PickForChat 时用户未配过 | ⬜ |
| `INVALID_SCENARIO` | 400 | `model.ErrInvalidScenario` | PUT path 的 scenario 不在白名单 | ⬜ |
| `PROVIDER_REQUIRED` | 400 | `model.ErrProviderRequired` | PUT body provider 空 | ⬜ |
| `MODEL_ID_REQUIRED` | 400 | `model.ErrModelIDRequired` | PUT body modelId 空 | ⬜ |

#### conversation ⬜

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `CONVERSATION_NOT_FOUND` | 404 | `conversation.ErrNotFound` | id 查不到 | ⬜ |
| `MESSAGE_NOT_FOUND` | 404 | `conversation.ErrMessageNotFound` | 消息 id 查不到 | ⬜ |

#### chat ⬜

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `STREAM_NOT_FOUND` | 404 | `chat.ErrStreamNotFound` | 取消不存在的流 | ⬜ |
| `STREAM_IN_PROGRESS` | 409 | `chat.ErrStreamInProgress` | 同一对话已有流在跑 | ⬜ |
| `LLM_PROVIDER_ERROR` | 502 | `chat.ErrProviderUnavailable` | 上游 LLM 故障 | ⬜ |

---

### Phase 3：工具锻造能力

#### attachment ⬜

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `ATTACHMENT_TOO_LARGE` | 413 | `attachment.ErrTooLarge` | 超过大小上限 | ⬜ |
| `ATTACHMENT_TYPE_UNSUPPORTED` | 415 | `attachment.ErrTypeUnsupported` | 不支持的格式 | ⬜ |
| `ATTACHMENT_PARSE_FAILED` | 422 | `attachment.ErrParseFailed` | 文件损坏 / 解析失败 | ⬜ |

#### tool ⬜

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `TOOL_NOT_FOUND` | 404 | `tool.ErrNotFound` | | ⬜ |
| `TOOL_NAME_DUPLICATE` | 409 | `tool.ErrDuplicateName` | 创建/重命名时撞名 | ⬜ |
| `TOOL_IS_BUILTIN` | 403 | `tool.ErrBuiltinProtected` | 修改/删除内置工具 | ⬜ |
| `TOOL_RUN_FAILED` | 422 | `tool.ErrRunFailed` | 沙箱执行失败 | ⬜ |
| `TOOL_VERSION_NOT_FOUND` | 404 | `tool.ErrVersionNotFound` | 恢复不存在的版本 | ⬜ |
| `TOOL_PENDING_NOT_FOUND` | 404 | `tool.ErrPendingNotFound` | accept 不存在的 pending | ⬜ |
| `TOOL_TEST_CASE_NOT_FOUND` | 404 | `tool.ErrTestCaseNotFound` | | ⬜ |
| `TOOL_IMPORT_INVALID` | 400 | `tool.ErrImportInvalid` | 导入文件格式错 | ⬜ |
| `TOOL_IMPORT_CONFLICT` | 409 | `tool.ErrImportConflict` | 导入名字冲突需用户决策 | ⬜ |

---

### Phase 4：工作流能力

#### workflow ⬜

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `WORKFLOW_NOT_FOUND` | 404 | `workflow.ErrNotFound` | | ⬜ |
| `WORKFLOW_INVALID_DEFINITION` | 400 | `workflow.ErrInvalidDefinition` | DAG 校验失败（环 / 孤儿节点）| ⬜ |
| `WORKFLOW_NODE_NOT_FOUND` | 404 | `workflow.ErrNodeNotFound` | | ⬜ |

#### flowrun ⬜

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `FLOWRUN_NOT_FOUND` | 404 | `flowrun.ErrNotFound` | | ⬜ |
| `FLOWRUN_ALREADY_FINISHED` | 409 | `flowrun.ErrAlreadyFinished` | 取消已结束的运行 | ⬜ |

#### scheduler / trigger ⬜

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `TRIGGER_INVALID_CRON` | 400 | `scheduler.ErrInvalidCron` | cron 表达式错 | ⬜ |
| `TRIGGER_DUPLICATE` | 409 | `scheduler.ErrDuplicate` | 同一触发器重复注册 | ⬜ |

---

### Phase 5：智能化能力

#### knowledge ⬜

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `KNOWLEDGE_NOT_FOUND` | 404 | `knowledge.ErrNotFound` | | ⬜ |
| `DOCUMENT_NOT_FOUND` | 404 | `knowledge.ErrDocumentNotFound` | | ⬜ |
| `EMBEDDING_FAILED` | 502 | `knowledge.ErrEmbeddingFailed` | 向量化失败 | ⬜ |

#### mcp ⬜

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `MCP_SERVER_NOT_FOUND` | 404 | `mcp.ErrNotFound` | | ⬜ |
| `MCP_CONNECTION_FAILED` | 502 | `mcp.ErrConnectionFailed` | 连不上 MCP server | ⬜ |

#### intent ⬜

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `INTENT_AMBIGUOUS` | 422 | `intent.ErrAmbiguous` | 意图无法明确识别 | ⬜（待定）|
