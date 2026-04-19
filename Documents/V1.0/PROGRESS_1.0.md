# Forgify 开发进度

**更新于**：2026-04-19  
**当前状态**：文档阶段完成，准备开始写代码

---

## 状态标记说明

```
⬜ 未开始
🔄 进行中
✅ 已完成
⏸ 暂缓（有依赖未完成）
```

---

## Tier 1 — 地基（必须最先做）

这三个切片是整个项目的骨架，后续所有切片都依赖它们。

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **A1** App Shell | Electron 项目初始化，侧边栏四 Tab，SplitView | ⬜ | 先跑起来一个空壳 |
| **A2** Data Layer | SQLite 连接，001-012 迁移自动执行 | ⬜ | 所有表一次性建好 |
| **A3** Event System | EventBridge，events 常量定义 | ⬜ | 先定义好所有 Event 名 |

---

## Tier 2 — AI 核心（对话能跑起来）

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **B1** API Key Management | API Key 存储，UI 管理 | ⬜ | |
| **K1** Model Settings | 模型配置，连接测试 | ⬜ | 和 B1 一起做，先能选模型 |
| **B2** Streaming Core | Eino ConversationAgent，流式 token | ⬜ | 核心 AI 调用 |
| **B3** Model Strategy | 多模型切换策略 | ⬜ | 可以最后再做 |

---

## Tier 3 — 对话界面

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **C1** Conversation Management | 对话列表，1:1 资产绑定 | ⬜ | |
| **C2** Message UI | 消息气泡，Markdown 渲染，流式 | ⬜ | |
| **C4** Context Compression | 三层压缩 | ⬜ | 可以晚一点做 |
| **C3** File Attachment | 文件拖拽上传 | ⬜ | 可以晚一点做 |

---

## Tier 4 — 工具基础（工具能创建和运行）

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **D3** Python Sandbox | uv venv，subprocess 执行 | ⬜ | 先做执行引擎 |
| **D6** Built-in Tools | 内置工具注册，go:embed | ⬜ | 需要 D3 |
| **D1** Tool Library | 工具列表，搜索 | ⬜ | |
| **D4** Tool Detail | Monaco 代码查看/编辑 | ⬜ | |
| **D2** Tool Forge | AI 对话创建工具 | ⬜ | 需要 B2 + D3 + D4 |
| **D5** Tool Sharing | 导入导出 | ⬜ | 最后做 |

---

## Tier 5 — 工作流画布（节点能渲染和编辑）

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **E1** Workflow Canvas | ReactFlow，BaseNode，DB 迁移 | ⬜ | 先渲染一个空画布 |
| **E2** Workflow Creation | AI 输出 flow-definition，状态注入 | ⬜ | 需要 B2 + E1 |
| **E3** Basic Nodes | Trigger/Tool/Condition/Variable/Approval | ⬜ | 需要 E1 |
| **E4** Advanced Nodes | Loop/Parallel/Subworkflow | ⬜ | 需要 E3 |
| **E5** AI Nodes | LLM/Agent | ⬜ | 需要 E3 |

---

## Tier 6 — 执行引擎（工作流能跑起来）

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **F1** Flow Compiler | FlowDefinition → Eino Graph | ⬜ | 核心，其他 F* 依赖它 |
| **F2** Manual Run | 执行引擎，节点状态推送 | ⬜ | 需要 F1 |
| **F4** Error Handling | 重试，错误展示 | ⬜ | 需要 F2 |
| **F5** Run History | 历史记录，历史回放 | ⬜ | 需要 F2 |
| **F3** Mailbox Approval | Approval 节点阻塞等待 | ⬜ | 需要 F2 |

---

## Tier 7 — 自动化触发

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **G2** Deploy Config | 状态机，部署前检查 | ⬜ | 需要 F1 |
| **G1** Cron Scheduler | robfig/cron，定时触发 | ⬜ | 需要 G2 |
| **G3** Event Triggers | 文件监听 + Webhook | ⬜ | 需要 G2 |
| **G4** Error Fix Conversation | 失败后 AI 诊断 | ⬜ | 需要 F4 + B2 |

---

## Tier 8 — 权限 & Inbox

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **H2** Mailbox Queue | 统一消息队列 DB | ⬜ | 其他 Inbox 依赖它 |
| **H1** Permission Gate | 工具权限门控 | ⬜ | 需要 H2 |
| **I1** Inbox Core | 消息列表，未读徽章 | ⬜ | 需要 H2 |
| **I2** Approval Workflow | 审批操作 UI | ⬜ | 需要 I1 + F3 |
| **I3** Inbox Context View | 只读 Canvas + 代码 | ⬜ | 需要 I1 + E1 |

---

## Tier 9 — 收尾

| 切片 | 描述 | 状态 | 备注 |
|---|---|---|---|
| **J1** Home Page | 状态摘要 + 最近活动 | ⬜ | |
| **K2** General Settings | 超时配置，数据导出，权限管理 | ⬜ | |

---

## 依赖关系速查

```
A1 ← 所有切片
A2 ← 所有切片（数据库）
A3 ← 所有有事件推送的切片

B1, K1 → B2 → C1, C2, D2, E2
D3 → D6, D2
E1 → E2, E3, E4, E5
F1 → F2 → F3, F4, F5
F1 → G2 → G1, G3
H2 → H1, I1, I2, I3
```

---

## 各 Tier 完成后能跑的东西

| Tier 完成后 | 可以体验的功能 |
|---|---|
| Tier 1 | 空应用跑起来，四个 Tab 可以切换 |
| Tier 2 | 和 AI 对话，流式输出 |
| Tier 3 | 完整对话体验，对话列表 |
| Tier 4 | 创建工具，运行工具，内置工具可用 |
| Tier 5 | 通过对话创建工作流，画布渲染节点 |
| Tier 6 | 工作流可以手动运行，节点状态实时显示 |
| Tier 7 | 工作流自动运行（定时/文件/Webhook）|
| Tier 8 | 权限确认，Inbox 消息，审批操作 |
| Tier 9 | Home 页、设置页，全功能完整 |

---

## 开发日志

| 日期 | 内容 |
|---|---|
| 2026-04-19 | PRD v0.3 + TDD v0.3 + 38 个切片文档全部完成 |
| | |
