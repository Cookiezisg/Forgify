# API Design — V1.2 完整 REST API 设计

**关联**：[backend-rewrite.md](./backend-rewrite.md) — 总路线图
**适用版本**：V1.2 后端重写
**遵守标准**：N1（envelope）/ N2（状态码）/ N3（camelCase）/ N4（分页）/ N5（RESTful）

---

## 全局约定

### 路径前缀
所有 API 统一前缀 `/api/v1/`。版本号留出未来升级空间。

### 响应 envelope

```typescript
// 成功
type Success<T> = { data: T }

// 列表（分页）
type Paged<T> = {
  data: T[]
  nextCursor: string | null
  hasMore: boolean
}

// 失败
type Error = {
  error: {
    code: string        // 如 "TOOL_NOT_FOUND"
    message: string     // 人类可读
    details?: object    // 可选上下文
  }
}
```

### 状态码语义（N2）

| 码 | 场景 |
|---|---|
| 200 | 读取成功 / 更新成功（有响应体） |
| 201 | 创建成功（返回新资源）|
| 202 | 异步任务已接受（如启动流式响应）|
| 204 | 删除成功 / 操作成功（无响应体） |
| 400 | 请求参数错误 |
| 401 | 未认证（Phase N 引入 auth 后）|
| 403 | 已认证但无权限 |
| 404 | 资源不存在 |
| 409 | 业务冲突（如重名）|
| 422 | 参数合法但业务拒绝（如 API Key 测试失败）|
| 500 | 内部错误（bug）|

### 字段命名（N3）
- 请求/响应字段：`camelCase`
- DB 列名：`snake_case`（repo 层负责转换）
- 错误码：`SCREAMING_SNAKE_CASE`

### 分页（N4）
所有列表端点支持 `?cursor=xxx&limit=50`，默认 50，最大 200。

### 业务动作命名（N5）
- 状态变更：用 `PATCH` + 状态字段（不要用 `/archive`、`/restore` 子路径）
- 不能用 RESTful 表达的业务动作：用 `:action` 后缀（如 `POST /tools/{id}:run`）

---

## API 设计清单（按 domain）

> 推进规则：每个 Phase 开干前，把该 Phase 涉及 domain 的 API 段填到下方对应位置。
> 状态：⬜ 未设计 | 🔄 设计中 | ✅ 已实现

### Phase 2：基础对话能力

#### apikey ⬜
> Phase 2 开干 apikey 时填这里

#### conversation ⬜
> Phase 2 开干 conversation 时填这里

#### chat（极简版）⬜
> Phase 2 开干 chat 时填这里

---

### Phase 3：工具锻造能力

#### attachment ⬜
#### tool ⬜
#### chat（升级带 tool calling）⬜

---

### Phase 4：工作流能力

#### workflow ⬜
#### flowrun ⬜
#### scheduler ⬜
#### trigger ⬜

---

### Phase 5：智能化能力

#### knowledge ⬜
#### document ⬜
#### intent ⬜
#### mcpserver ⬜
#### skill ⬜
#### chat（终极智能版）⬜

---

## 通用端点

### Health
- `GET /api/v1/health` ✅ 已实现 — `{"data":{"status":"ok"}}`

### SSE 事件流
- `GET /api/v1/events?conversationId=xxx` ⬜ Phase 2 实现 — 订阅指定对话的事件流
