# Database Design — V1.2 表一眼索引

**关联**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-design-documents/`](../service-design-documents/) — 每个 domain 的详设计（含完整 struct + 业务规则）

**定位**：**一眼看到全仓有哪些表 + 关键约束**。struct / 索引细节 / CHECK 约束原文、schema_extras 补丁，**去 service-design-documents 看**。

**遵守标准**：D1（软删 deleted_at）/ D2（时间戳）/ D3（枚举 CHECK）/ D4（FK + `PRAGMA foreign_keys=on`）/ D5（业务 UNIQUE）

---

## 全局约定

### 数据库
- **SQLite**（本地）+ GORM
- WAL、FK、PrepareStmt、UTC 全开（见 `infra/db/db.go`）

### 类型策略
- **一份到底**：domain 类型直接带 GORM tag，不分两套
- **DB 列名**：`snake_case`
- **主键**：文本 ID（带 domain 前缀，如 `aki_<16hex>`、`mc_<16hex>`）

### 时间戳 + 软删除（D1 + D2）
每表标配：
```go
CreatedAt time.Time      // GORM 自动
UpdatedAt time.Time      // GORM 自动
DeletedAt gorm.DeletedAt // 软删（写入 deleted_at 列）
```
废弃 `status='deleted'` / `archived_at` 等变体。

### 枚举（D3）
- **稳定白名单**（`role`、`content_type`、`test_status` 等）在 DB 层 CHECK
- **会随 Phase 扩张的白名单**（如 `scenario`）在 app 层校验，DB 不 CHECK

### 高级 schema（`infra/db/schema_extras.go`）
GORM tag 表达不了的都在这里：
- 部分 UNIQUE 索引（`WHERE deleted_at IS NULL`）
- FTS5 虚拟表
- 触发器

---

## 表清单

> **状态**：⬜ 未设计 | 🔄 讨论中 | ✅ 已实现

### Phase 2

#### `api_keys` ✅
详见 [`../service-design-documents/apikey.md`](../service-design-documents/apikey.md) §11。
主键 `aki_<16hex>`；软删（`DeletedAt`）；全索引 `(user_id)` + `(user_id, provider)` + `(deleted_at)`（目前未走部分索引 `WHERE deleted_at IS NULL`，见 backlog）。敏感字段 `key_encrypted`（AES-GCM `v1:` 前缀，`json:"-"` 守护永不上线）+ `key_masked` 冗余展示。不加 `UNIQUE(user_id, provider)`，允许同 provider 多 key。Provider / TestStatus 的 DB 层 CHECK 约束**未加**，由 app 层校验。

#### `model_configs` ✅
详见 [`../service-design-documents/model.md`](../service-design-documents/model.md) §11。
主键 `mc_<16hex>`；软删（`deleted_at`）；GORM 全唯一索引 `UNIQUE(user_id, scenario)`（partial UNIQUE 暂缓，见 §17 决定）。Scenario 白名单 app 层校验，DB 不 CHECK。

#### `conversations` ✅
详见 [`../service-design-documents/conversation.md`](../service-design-documents/conversation.md) §8。
主键 `cv_<16hex>`；软删（`deleted_at`）；`user_id` 索引。新增字段：`system_prompt TEXT`（对话级自定义系统提示词，可为空）/ `auto_titled BOOLEAN`（标记标题是 AI 自动生成的还是用户手动改的）。Title 允许空字符串，首轮完成后 auto-titling goroutine 回写。

#### `messages` ✅
chat domain 所有；主键 `msg_<16hex>`；字段：`conversation_id`（索引）/ `user_id` / `role`（user\|assistant\|tool）/ `content` / `status`（pending\|streaming\|completed\|error\|cancelled）/ `stop_reason` / `token_usage`（JSON）/ `tool_calls`（JSON）/ `tool_call_id` / `attachment_ids`（JSON 数组）/ 软删 `deleted_at`。FTS5 虚拟表 `messages_fts` 已在 `schema_extras.go` 实现，`messages` 表存在后自动建索引 + 3 个触发器（insert/update/delete）。构建时需 `CGO_CFLAGS="-DSQLITE_ENABLE_FTS5"`。详见 `service-design-documents/chat.md` §5。

#### `chat_attachments` ✅
chat domain 所有；主键 `att_<16hex>`；字段：`user_id` / `file_name` / `mime_type` / `size_bytes` / `storage_path`（相对 dataDir，json:"-" 不对外暴露）。文件实体存 `{dataDir}/attachments/{att_id}/original.{ext}`，50MB 限制。无软删（附件随对话消亡）。

---

### Phase 3

#### `tools` ⬜
> 4 个 schema 业务问题，做 tool domain 时讨论（见 progress-record.md）：
> 1. `pending_code` + `pending_summary` 字段对 → 独立 `tool_pending_changes` 表？
> 2. `tools.version` (TEXT) vs `tool_versions.version` (INTEGER) 语义不一致
> 3. `tool_test_history` 20 条上限（app 层注释 vs DB 触发器）
> 4. `conversations.asset_id/asset_type` polymorphism vs 拆 `bound_tool_id` + `bound_workflow_id` 两列

#### `tool_versions` ⬜
#### `tool_tags` ⬜
#### `tool_test_history` ⬜
#### `tool_test_cases` ⬜
#### `tool_pending_changes` ⬜（待定）
#### `attachments` ⬜（如果决定持久化）

---

### Phase 4

#### `workflows` ⬜
#### `flowruns` ⬜
#### `nodes` ⬜（如节点独立成表）
#### `schedulers` ⬜
#### `triggers` ⬜

---

### Phase 5

#### `knowledge_bases` ⬜
#### `documents` ⬜
#### `document_chunks` ⬜
#### `embeddings` ⬜（向量存储，本地 sqlite-vec）
#### `mcp_servers` ⬜
#### `skills` ⬜

---

## 跨表关系图

> 每完成一个 Phase 更新一次。

**当前（Phase 2 完成）**：
```
api_keys        model_configs     conversations
   │                 │                  │
   │  (user_id)      │  (user_id)       │  (user_id)
   │                 │                  │
   └──────────────── local-user ────────┘
                                        │ conversation_id (索引)
                                    messages
                                        │ att_id (attachment_ids JSON)
                               chat_attachments
```
