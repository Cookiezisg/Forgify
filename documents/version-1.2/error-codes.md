# Error Codes — V1.2 业务错误码表

**关联**：[backend-rewrite.md](./backend-rewrite.md) — 总路线图
**配套实现**：`infra/transport/httpapi/response/errmap.go`

---

## 全局约定

### 错误码命名
- 全部大写 + 下划线：`SCREAMING_SNAKE_CASE`
- 按 domain 加前缀（除非通用）：`TOOL_NOT_FOUND`、`API_KEY_INVALID`

### 三层错误模型

```
┌─────────────────────────────────────────────┐
│ domain/<name>/errors.go                      │
│   var ErrNotFound = errors.New("...")        │  ← Sentinel 错误
└──────────────────┬───────────────────────────┘
                   │ errors.Is 判断
                   ↓
┌─────────────────────────────────────────────┐
│ transport/httpapi/response/errmap.go         │
│   var errTable = map[error]errMapping{       │  ← 翻译表
│     tool.ErrNotFound: {404, "TOOL_NOT_FOUND"} │
│   }                                           │
└──────────────────┬───────────────────────────┘
                   │ HTTP envelope
                   ↓
            { "error": {                       │
                "code": "TOOL_NOT_FOUND",      │
                "message": "...",              │
                "details": {...}               │
            }}
```

### 添加新错误码的流程

1. 在 `domain/<name>/errors.go` 声明 sentinel：
   ```go
   var ErrNotFound = errors.New("tool: not found")
   ```
2. 在 `infra/transport/httpapi/response/errmap.go` 加映射行：
   ```go
   tool.ErrNotFound: {http.StatusNotFound, "TOOL_NOT_FOUND"},
   ```
3. 在本文档下方对应 domain 段添加一行
4. handler 用 `response.FromDomainError(w, log, err)` 自动翻译

### 未注册错误的兜底
任何未注册到 `errTable` 的错误自动降级为 `500 INTERNAL_ERROR`，原始消息**不**暴露给客户端（防止泄漏实现细节）。

---

## 错误码清单

> 推进规则：每个 domain 设计阶段把该 domain 的错误码加到下方
> 状态：⬜ 未定义 | ✅ 已实现

### Phase 1（已实现）

| Code | HTTP | Sentinel | 触发场景 |
|---|---|---|---|
| `INVALID_REQUEST` | 400 | `derrors.ErrInvalidRequest` | 请求参数错（JSON 坏、字段缺失、cursor 格式错）✅ |
| `INTERNAL_ERROR` | 500 | `derrors.ErrInternal` | 兜底；未映射错误自动降级到这 ✅ |
| `NOT_FOUND` | 404 | （middleware 直接发，不走 errmap）| 路由未匹配 ✅ |

---

### Phase 2：基础对话能力

#### apikey

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `API_KEY_NOT_FOUND` | 404 | `apikey.ErrNotFound` | 通过 ID 查询不到 | ⬜ |
| `API_KEY_INVALID` | 401 | `apikey.ErrInvalid` | API Key 格式错或鉴权失败 | ⬜ |
| `API_KEY_TEST_FAILED` | 422 | `apikey.ErrTestFailed` | 测试连通性失败（网络、provider 拒绝）| ⬜ |
| `API_KEY_DUPLICATE_PROVIDER` | 409 | `apikey.ErrDuplicateProvider` | 同一 provider 已有 Key | ⬜（待定）|

#### conversation

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `CONVERSATION_NOT_FOUND` | 404 | `conversation.ErrNotFound` | 通过 ID 查询不到 | ⬜ |
| `CONVERSATION_DELETED` | 410 Gone | `conversation.ErrDeleted` | 已软删除（不带 includeDeleted=true 时）| ⬜ |
| `MESSAGE_NOT_FOUND` | 404 | `conversation.ErrMessageNotFound` | 消息查询不到 | ⬜ |

#### chat

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `STREAM_NOT_FOUND` | 404 | `chat.ErrStreamNotFound` | 取消不存在的流 | ⬜ |
| `STREAM_IN_PROGRESS` | 409 | `chat.ErrStreamInProgress` | 同一对话已有流在跑 | ⬜ |
| `MODEL_NOT_CONFIGURED` | 422 | `chat.ErrModelNotConfigured` | 没配置可用模型 | ⬜ |
| `LLM_PROVIDER_ERROR` | 502 | `chat.ErrProviderUnavailable` | 上游 LLM 故障 | ⬜ |

---

### Phase 3：工具锻造能力

#### attachment

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `ATTACHMENT_TOO_LARGE` | 413 | `attachment.ErrTooLarge` | 超过大小上限 | ⬜ |
| `ATTACHMENT_TYPE_UNSUPPORTED` | 415 | `attachment.ErrTypeUnsupported` | 不支持的格式 | ⬜ |
| `ATTACHMENT_PARSE_FAILED` | 422 | `attachment.ErrParseFailed` | 文件损坏 / 解析失败 | ⬜ |

#### tool

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `TOOL_NOT_FOUND` | 404 | `tool.ErrNotFound` | | ⬜ |
| `TOOL_NAME_DUPLICATE` | 409 | `tool.ErrDuplicateName` | 创建/重命名时撞名 | ⬜ |
| `TOOL_IS_BUILTIN` | 403 | `tool.ErrBuiltinProtected` | 试图修改/删除内置工具 | ⬜ |
| `TOOL_RUN_FAILED` | 422 | `tool.ErrRunFailed` | 沙箱执行失败 | ⬜ |
| `TOOL_VERSION_NOT_FOUND` | 404 | `tool.ErrVersionNotFound` | 恢复不存在的版本 | ⬜ |
| `TOOL_PENDING_NOT_FOUND` | 404 | `tool.ErrPendingNotFound` | accept 不存在的 pending change | ⬜ |
| `TOOL_TEST_CASE_NOT_FOUND` | 404 | `tool.ErrTestCaseNotFound` | | ⬜ |
| `TOOL_IMPORT_INVALID` | 400 | `tool.ErrImportInvalid` | 导入文件格式错 | ⬜ |
| `TOOL_IMPORT_CONFLICT` | 409 | `tool.ErrImportConflict` | 导入时名字冲突需用户决策 | ⬜ |

---

### Phase 4：工作流能力

#### workflow

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `WORKFLOW_NOT_FOUND` | 404 | `workflow.ErrNotFound` | | ⬜ |
| `WORKFLOW_INVALID_DEFINITION` | 400 | `workflow.ErrInvalidDefinition` | DAG 校验失败（环、孤儿节点等）| ⬜ |
| `WORKFLOW_NODE_NOT_FOUND` | 404 | `workflow.ErrNodeNotFound` | | ⬜ |

#### flowrun

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `FLOWRUN_NOT_FOUND` | 404 | `flowrun.ErrNotFound` | | ⬜ |
| `FLOWRUN_ALREADY_FINISHED` | 409 | `flowrun.ErrAlreadyFinished` | 取消已结束的运行 | ⬜ |

#### scheduler / trigger

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `TRIGGER_INVALID_CRON` | 400 | `scheduler.ErrInvalidCron` | cron 表达式错 | ⬜ |
| `TRIGGER_DUPLICATE` | 409 | `scheduler.ErrDuplicate` | 同一触发器重复注册 | ⬜ |

---

### Phase 5：智能化能力

#### knowledge

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `KNOWLEDGE_NOT_FOUND` | 404 | `knowledge.ErrNotFound` | | ⬜ |
| `DOCUMENT_NOT_FOUND` | 404 | `knowledge.ErrDocumentNotFound` | | ⬜ |
| `EMBEDDING_FAILED` | 502 | `knowledge.ErrEmbeddingFailed` | 向量化失败 | ⬜ |

#### mcp

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `MCP_SERVER_NOT_FOUND` | 404 | `mcp.ErrNotFound` | | ⬜ |
| `MCP_CONNECTION_FAILED` | 502 | `mcp.ErrConnectionFailed` | 连不上 MCP 服务器 | ⬜ |

#### intent

| Code | HTTP | Sentinel | 场景 | 状态 |
|---|---|---|---|---|
| `INTENT_AMBIGUOUS` | 422 | `intent.ErrAmbiguous` | 意图无法明确识别 | ⬜（待定）|
