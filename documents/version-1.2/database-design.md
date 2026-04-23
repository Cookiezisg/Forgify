# Database Design — V1.2 表结构

**关联**：[backend-rewrite.md](./backend-rewrite.md) — 总路线图
**遵守标准**：D1（软删 deleted_at）/ D2（时间戳 created_at + updated_at）/ D3（CHECK 约束）/ D4（外键 PRAGMA on）/ D5（业务唯一 UNIQUE）

---

## 全局约定

### 数据库
- **SQLite**（本地）+ GORM
- WAL 模式、FK 约束开启、PrepareStmt 缓存（已在 `infra/gorm/db.go` 实现）

### 类型策略
- **一份到底**：domain 类型直接带 GORM tag，不分两套
- **DB 列名**：`snake_case`（GORM 默认）
- **主键**：UUID（TEXT 类型）

### 时间戳（D2）
每张表必有：
```go
CreatedAt time.Time      // GORM 自动维护
UpdatedAt time.Time      // GORM 自动维护
DeletedAt gorm.DeletedAt // 软删除（GORM 内置类型）
```

### 软删除（D1）
统一用 `gorm.DeletedAt` 字段（写入 `deleted_at` 列），不再用 `status='deleted'`、`status='archived'` 等。

### 外键（D4）
- 所有 FK 必须显式声明（GORM tag 或 `references` 子句）
- `PRAGMA foreign_keys=on` 强制约束（已开）

### 索引
- 列出每张表的常用查询路径，对应建索引
- 复合索引顺序按"等值过滤 → 范围过滤 → 排序"原则

### 高级 schema（schema_extras.go）
- FTS5 虚拟表
- CHECK 约束（GORM tag 不全支持）
- 触发器
都在 `infra/gorm/schema_extras.go` 维护

---

## 表设计清单（按 domain）

> 推进规则：每个 domain 开干前，先在下方填该 domain 的表设计 + 业务规则讨论
> 状态：⬜ 未设计 | 🔄 讨论中 | ✅ 已实现

### Phase 2

#### api_keys ⬜
> apikey domain 开干时填

#### conversations ⬜
#### messages ⬜

---

### Phase 3

#### attachments ⬜
（如果决定持久化附件，否则不需要）

#### tools ⬜
> 4 个待讨论的 schema 业务问题：
> 1. `pending_code` + `pending_summary` 字段对 → 拆成独立 `tool_pending_changes` 表？
> 2. `version` 字段：`tools.version TEXT` vs `tool_versions.version INTEGER` 语义不一致 → 删 `tools.version`，靠 `tool_versions` 取最大值？
> 3. `tool_test_history` 20 条上限：app 层注释 vs DB 触发器？
> 4. `conversations.asset_id/asset_type` polymorphism vs 拆 `bound_tool_id` + `bound_workflow_id` 两列？

#### tool_versions ⬜
#### tool_tags ⬜
#### tool_test_history ⬜
#### tool_test_cases ⬜
#### tool_pending_changes ⬜（待定）

---

### Phase 4

#### workflows ⬜
#### flowruns ⬜
#### nodes ⬜（如果节点独立成表）
#### schedulers ⬜
#### triggers ⬜

---

### Phase 5

#### knowledge_bases ⬜
#### documents ⬜
#### document_chunks ⬜
#### embeddings ⬜（向量存储）
#### mcp_servers ⬜
#### skills ⬜

---

## 跨表关系图

> 每完成一个 Phase 后更新这张图，便于看清 domain 间关系

```
（Phase 2 完成后填）
```
