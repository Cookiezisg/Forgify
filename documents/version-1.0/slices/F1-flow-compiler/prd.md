# F1 · Flow 编译器 — 产品需求文档

**切片**：F1  
**状态**：待 Review

---

## 1. 背景

`FlowDefinition`（JSON）是工作流的存储格式，定义了节点类型、配置和边。  
Flow 编译器负责将这份静态 JSON 翻译成一个可执行的 Eino Graph，是所有工作流执行能力的基础。

---

## 2. 编译目标

| 输入 | 输出 |
|---|---|
| `FlowDefinition` JSON | Eino `Graph`（可立即 `.Run()`） |

编译是**纯内存操作**，不写 DB，每次执行前实时编译。

---

## 3. 节点映射规则

每种节点类型对应一个 Eino 组件：

| NodeType | Eino 组件 | 说明 |
|---|---|---|
| `trigger_*` | 入口节点（StartNode）| 触发来源，作为 Graph 的起点 |
| `tool` | `ToolNode` | 调用 Python sandbox 执行 |
| `condition` | `BranchNode` | 多出线，按表达式路由 |
| `variable` | `TransformNode` | 设置/读取变量，无副作用 |
| `approval` | `BlockNode` | 暂停执行，等待人工确认 |
| `loop` | `LoopNode` | 子图反复执行 |
| `parallel` | `FanOutNode` | N 路并发，配合 `JoinNode` |
| `subworkflow` | 递归编译子 FlowDefinition | 嵌套 Graph |
| `llm` | `LLMNode` | 单次 Eino LLM 调用 |
| `agent` | `AgentNode` | Eino ReAct Agent 循环 |

---

## 4. 变量引用解析

节点配置中的 `{{node_id.result.field}}` 在运行时由执行上下文注入，编译器**不求值**，只做语法检查：

- 引用的 `node_id` 必须存在于当前 FlowDefinition
- 引用路径格式：`{{<node_id>.result}}` 或 `{{<node_id>.result.<field>}}`
- 非法引用：编译失败，返回错误列表

---

## 5. 编译错误类型

| 错误类型 | 含义 |
|---|---|
| `unknown_node_type` | 节点类型不在支持列表 |
| `missing_target_node` | 边的 target/source node_id 不存在 |
| `invalid_variable_ref` | `{{ref}}` 引用了不存在的节点 |
| `no_trigger_node` | 工作流没有触发节点 |
| `cycle_detected` | 图中存在非法环路（loop 节点除外） |
| `missing_required_config` | 必填配置项为空（如 tool 节点未选工具） |

编译失败时返回 `CompileResult{ Errors: [...] }`，不返回 Graph。

---

## 6. 画布上的反馈

编译错误在工作流画布上实时反映：
- 有错误的节点显示黄色边框 + ⚠️ 图标
- 悬停显示具体错误信息
- 工具栏的"运行"按钮在有编译错误时置灰

---

## 7. 验收测试

```
1. 完整合法的 FlowDefinition → 编译成功，返回可运行 Graph
2. 边引用了不存在的节点 → 编译失败，错误类型 missing_target_node
3. {{node_99.result}} 但 node_99 不存在 → 编译失败，错误类型 invalid_variable_ref
4. 没有触发节点 → 编译失败，错误类型 no_trigger_node
5. 循环节点（loop）的 loop_body 子图正确识别为子图，不报 cycle_detected
6. 编译错误节点在画布上显示黄色边框
```
