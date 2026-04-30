# Claude Code 调研

**用途**：每个 `---Research Start---` 到 `---Research End---` 之间是一份调研需求。

**调研报告输出目录**：`项目目录/documents/version-1.2/claude-code-research-documents/`
创建一个分支，最后pull request发过来

---

## 共同背景

Forgify 是一个本地优先的 AI 工具工作台（桌面 Electron app），后端用 Go 实现了自有 ReAct Agent 管线。2026 年 4 月，Claude Code 的 npm source map 意外泄漏了 512K 行 TypeScript 源码（1906 文件，2026-03-31 快照），网上有多篇深度技术分析文章。

---

## 提示词列表

| # | 方向 | 输出文件 |
|---|---|---|
| 01 | Agent 核心循环 / ReAct Pipeline | `01-agent-loop.md` |
| 02 | 工具完整度（Tool 系统） | `02-tools.md` |
| 03 | Context 管理 / Compaction | `03-context.md` |
| 04 | 记忆系统 | `04-memory.md` |
| 05 | Subagent 系统 | `05-subagent.md` |
| 06 | Hooks 系统 | `06-hooks.md` |
| 07 | 用户体验工具（AskUser / Task） | `07-ux-tools.md` |
| 08 | 权限与安全系统 | `08-permissions.md` |
| 09 | MCP 集成 | `09-mcp.md` |

---

---Research Start--- 01 / Agent 核心循环 / ReAct Pipeline

## 背景

我在做一个项目叫 **Forgify**，是一个本地优先的 AI 工具工作台（桌面 Electron app）。后端用 Go 实现了一套自有的 ReAct Agent 管线。整个 agent 核心循环的实现在：

```
backend/internal/app/chat/
  runner.go   — agentRun（ReAct loop 主体）：for step := range maxSteps { streamLLM → runTools → extendHistory }
  stream.go   — streamLLM：消费 iter.Seq[StreamEvent]，组装 Block，提取 tool calls
  tools.go    — runTools：并行执行所有 tool call，收集 tool_result block
  history.go  — buildHistory + extendHistory：DB 历史 → LLM messages
```

当前实现特点：
- 单层 for 循环，最多 20 步（maxSteps=20）
- 每步：streamLLM → 有 tool call → runTools（并行）→ extendHistory → 下一步
- 无 tool call → final writeDB → 结束
- 取消靠 context cancel
- 无 Stop Hook，无 steer 机制

**2026 年 4 月，Claude Code 的 npm source map 意外泄漏了 512K 行 TypeScript 源码（1906 文件，2026-03-31 快照）。** 网上有多篇深度技术分析文章。

## 任务

请做一份**超级认真、全面、细致到具体实现方式和代码级别**的研究，聚焦在 **Claude Code 的 Agent 核心循环 / ReAct Pipeline 实现**。

具体要研究并写清楚：

### 1. 主循环结构
- Claude Code 的 agent loop 整体代码结构是什么样的（函数名、文件、调用链）
- 终止条件怎么判断（纯文本 = 停？还是有其他条件？）
- 最大步骤数怎么控制（有没有 maxSteps，还是靠其他机制）
- 循环内状态怎么管理（每步的上下文是如何传递和积累的）

### 2. Streaming 与执行的配合
- LLM response 是完整收完再执行，还是流式过程中就触发执行
- 具体触发时机：是等 EventFinish，还是 tool arguments 完整时就执行
- 多个 tool call 在同一 response 里时，是串行还是并行执行

### 3. 取消与 Steer 机制
- 用户取消（Ctrl+C）时 Claude Code 怎么处理——是立刻停，还是等当前 tool 执行完
- **h2A 异步队列**：用户是否能在 agent 执行中途注入新指令，Claude 如何在当前 tool result 处理后"转向"
- Stop Hook：agent 打算结束时是否有回调可以阻止它（如"测试没过，不许停"）

### 4. 错误处理与恢复
- tool 执行失败时 agent 怎么做（直接报错？还是把错误作为结果继续）
- LLM API 错误时怎么处理（重试？还是终止循环）
- 是否有 checkpoint 机制，loop 中途崩溃能否恢复

### 5. 并发与队列
- 多个对话并发时怎么管理（每个 conversation 独立 loop？还是全局队列）
- 是否有 per-conversation 的排队机制

### 6. 对 Forgify 的改进建议
对比 Forgify 现有 agentRun 实现，列出**具体的、可落地的改进点**。每个改进点说清楚：
- 现在 Forgify 怎么做的 → Claude Code 怎么做的 → 建议改成什么 → 影响哪些文件

## 输出要求
- 语言：**中文**
- 保存到：`项目目录/documents/version-1.2/claude-code-research-documents/01-agent-loop.md`
- 细致到代码级别，引用具体函数名、类型定义、逻辑流程
- 找不到的细节明确标注"未在泄漏源码分析中找到"，不要猜测

## 信息来源建议
搜索关键词：
- "Claude Code source code leak agent loop"
- "Claude Code ReAct implementation internals"
- "Claude Code npm source map agentic pipeline"
- "Claude Code main loop tool execution flow"

---Research End---


---Research Start--- 02 / 工具完整度（Tool 系统）

## 背景

我在做一个项目叫 **Forgify**，是一个本地优先的 AI 工具工作台（桌面 Electron app）。后端用 Go 实现了自有 ReAct Agent 管线。现有 Tool 系统：

```
backend/internal/app/agent/tool.go    — Tool 接口（4 方法）+ summary 注入/剥除
backend/internal/app/agent/system.go  — 6 个 system tools：read_file / write_file / list_dir / run_shell / run_python / datetime
backend/internal/app/agent/web.go     — 2 个 web tools：web_search / fetch_url
backend/internal/app/agent/forge.go   — 5 个 forge tools：search_tools / get_tool / create_tool / edit_tool / run_tool
```

Forgify Tool 接口：
```go
type Tool interface {
    Name() string
    Description() string
    Parameters() json.RawMessage   // JSON Schema，不含 summary（框架注入）
    Execute(ctx context.Context, argsJSON string) (string, error)
}
```

共 13 个工具。没有 Edit（精确替换）、Grep、Glob、AskUserQuestion 等。

**2026 年 4 月，Claude Code 的 npm source map 意外泄漏了 512K 行 TypeScript 源码（1906 文件，2026-03-31 快照）。** 网上有多篇深度技术分析文章。

## 任务

请做一份**超级认真、全面、细致到具体实现方式和代码级别**的研究，聚焦在 **Claude Code 的 Tool 接口设计和完整工具清单**。

具体要研究并写清楚：

### 1. Tool 接口设计
- Claude Code 的 Tool 是怎么定义的（TypeScript 类型/接口/基类）
- 必须字段（name / description / parameters / execute）的具体类型签名
- 额外元数据字段（readOnly / requiresPermission / cacheable / isReadOnly 等）
- Tool 是静态注册还是动态发现，有没有 registry 模式，注册代码在哪

### 2. 完整工具清单（重点）
对 Claude Code 所有 40+ 个工具逐一研究：
- **文件操作**：Read / Write / Edit / MultiEdit / Glob / Grep / LS
  - **Edit 的精确替换逻辑**：old_string → new_string 是怎么匹配的（精确匹配？正则？），diff 怎么生成，唯一性怎么保证，失败怎么报错
  - Glob 的 modification time 排序是怎么实现的
  - Grep 底层是调 ripgrep 还是自己实现
- **执行类**：Bash（工作目录持久化怎么做，env var 为什么不持久）
- **Web 类**：WebFetch（独立 context window 怎么实现）/ WebSearch / WebBrowser
- **LSP**：跳定义/查引用/call hierarchy 怎么接语言服务器
- **Agent 编排**：Task / TaskCreate / TaskUpdate / SendMessage / EnterWorktree
- **用户交互**：AskUserQuestion / TodoWrite
- **MCP**：MCPTool 怎么映射到统一接口
每个工具要写：用途 + Parameters Schema 设计思路 + 关键实现细节

### 3. 工具的输出格式设计
- tool result 返回给 LLM 的格式是什么（纯文本？JSON？结构化？）
- 有没有统一的 output formatter
- 大输出（如大文件内容）是怎么截断或压缩的

### 4. 对 Forgify 的改进建议
对比 Forgify 现有 13 个工具，列出**具体的、可落地的改进点**：
- 优先级排序（哪个先做收益最大）
- 每个新工具的 Go 实现思路（特别是 Edit tool 的精确替换算法）
- Forgify 的 Tool 接口是否需要扩展字段

## 输出要求
- 语言：**中文**
- 保存到：`项目目录/documents/version-1.2/claude-code-research-documents/02-tools.md`
- 细致到代码级别
- 找不到的细节明确标注"未在泄漏源码分析中找到"

## 信息来源建议
搜索关键词：
- "Claude Code source code leak tool implementation"
- "Claude Code Edit tool string replacement implementation"
- "Claude Code Bash tool working directory persistent"
- "Claude Code tool interface TypeScript definition"
- "Claude Code 40 tools complete list internals"

---Research End---


---Research Start--- 03 / Context 管理 / Compaction

## 背景

我在做一个项目叫 **Forgify**，是一个本地优先的 AI 工具工作台（桌面 Electron app）。后端用 Go 实现了自有 ReAct Agent 管线。现有 Context 管理：

```go
// history.go
const maxHistoryMessages = 200
// buildHistory 硬截取最新 200 条，超出的旧消息静默丢弃
```

没有 token 计数，没有 compaction，没有缓存分割，没有 checkpoint。

**2026 年 4 月，Claude Code 的 npm source map 意外泄漏了 512K 行 TypeScript 源码（1906 文件，2026-03-31 快照）。** 网上有多篇深度技术分析文章。

## 任务

请做一份**超级认真、全面、细致到具体实现方式和代码级别**的研究，聚焦在 **Claude Code 的 Context 管理 / Compaction 系统**。

具体要研究并写清楚：

### 1. Token 计数与触发机制
- Claude Code 如何实时追踪 context 使用量（哪个字段，哪个函数）
- 触发 compaction 的阈值是多少（92%？95%？如何计算百分比）
- 触发时是同步阻塞还是异步，触发后 loop 怎么继续

### 2. 三层 Compaction Pipeline
- **Tier 1 MicroCompact**：哪些内容被移除（原始 tool output 的判断标准是什么），哪些保留，代码怎么实现
- **Tier 2 AutoCompact**：
  - 把历史发给 LLM 做摘要时的具体 prompt 是什么
  - LLM 输出的摘要格式是怎么定义的
  - "保留架构决策/未解决 bug"是怎么指导 LLM 的
  - 60~80% 的压缩率是怎么验证的
  - compaction 后 CLAUDE.md 重新读盘的时机和方式
- **Tier 3 /compact [focus]**：用户指定 focus 怎么传给 LLM，实现在哪

### 3. System Prompt 缓存分割
- `SYSTEM_PROMPT_DYNAMIC_BOUNDARY` 的具体位置（在 system prompt 的第几行/哪个位置）
- 静态部分包含什么，动态部分包含什么，具体示例
- Anthropic API 的 prompt cache 是怎么用的（cache_control 字段怎么打）
- 跨 session 缓存复用的实现机制

### 4. Checkpoint 系统
- Edit/Write 工具执行前文件快照怎么存（存在哪里，文件格式）
- 跨 session 持久化怎么实现
- 回滚的触发方式和实现

### 5. Context 可见性
- `/context` 命令返回的信息结构是什么（token 用量分布）
- 用户如何知道自己快到上限了

### 6. 对 Forgify 的改进建议
对比 Forgify 现有 200 条硬截断，列出**具体可落地的改进路径**：
- 哪一层最先做（最小实现是什么）
- Tier 2 的摘要 prompt 怎么设计
- Go 实现的关键代码结构（service 层怎么改，history.go 怎么改）
- token 计数用 Anthropic API 返回的 usage 字段还是自己估算

## 输出要求
- 语言：**中文**
- 保存到：`项目目录/documents/version-1.2/claude-code-research-documents/03-context.md`
- 细致到代码级别
- 找不到的细节明确标注"未在泄漏源码分析中找到"

## 信息来源建议
搜索关键词：
- "Claude Code context compaction implementation internals"
- "Claude Code AutoCompact MicroCompact source code"
- "Claude Code SYSTEM_PROMPT_DYNAMIC_BOUNDARY"
- "Claude Code prompt caching implementation"
- "Claude Code checkpoint system file snapshot"


---Research End---


---Research Start--- 04 / 记忆系统

## 背景

我在做一个项目叫 **Forgify**，是一个本地优先的 AI 工具工作台（桌面 Electron app）。后端用 Go 实现了自有 ReAct Agent 管线。现有记忆能力：
- `conversation.systemPrompt`：用户手动写，不是 agent 学到的
- 没有跨会话记忆
- 没有 CLAUDE.md 等价物
- 没有任何自动记忆机制

**2026 年 4 月，Claude Code 的 npm source map 意外泄漏了 512K 行 TypeScript 源码（1906 文件，2026-03-31 快照）。** 网上有多篇深度技术分析文章。

## 任务

请做一份**超级认真、全面、细致到具体实现方式和代码级别**的研究，聚焦在 **Claude Code 的记忆系统**。

具体要研究并写清楚：

### 1. CLAUDE.md 机制
- CLAUDE.md 文件的加载时机（session 启动时？每次请求前？）
- 目录遍历逻辑：从项目目录向上遍历，遍历几层，如何合并多个 CLAUDE.md
- 层级覆盖规则：项目 > 用户 > 组织，具体怎么 merge
- 注入方式：作为 user message 注入还是 system prompt，注入在历史的哪个位置
- compaction 后重新读盘的实现

### 2. MEMORY.md 三层架构
- MEMORY.md（index 文件）：格式规范，≤200 行限制怎么 enforce
- Topic 文件（user.md / feedback.md / project.md / reference.md）：每种文件的 schema
- Session 日志：append-only 格式，靶向检索怎么实现
- on-demand 加载：agent 判断"相关"的逻辑是什么（关键词匹配？LLM 判断？）

### 3. autoDream 后台整合
- 触发条件（24小时 + 5 session + lock）的具体实现
- 四阶段（Orient / Gather / Consolidate / Prune）每个阶段做什么
- 作为 subagent 运行的细节（独立 context，返回什么）
- 锁机制怎么实现（防止并发）

### 4. Rules files
- `.claude/rules/*.md` 的加载规则
- path-scoped 按需加载怎么实现（路径前缀匹配）
- 与 CLAUDE.md 的区别

### 5. Skills 系统
- `~/.claude/skills/` 的文件格式（Markdown 结构）
- skill 如何变成 slash command
- `Skill` tool 的实现

### 6. 对 Forgify 的改进建议
Forgify 目前记忆从零开始，列出**具体可落地的最小实现路径**：
- 第一步最小可行版本是什么
- FORGIFY.md 的等价实现在 Go 里怎么做（哪个文件读，什么时候注入，注入到哪）
- agent 写记忆文件的 tool 怎么设计（read_memory / write_memory tool 的参数设计）

## 输出要求
- 语言：**中文**
- 保存到：`项目目录/documents/version-1.2/claude-code-research-documents/04-memory.md`
- 细致到代码级别
- 找不到的细节明确标注"未在泄漏源码分析中找到"

## 信息来源建议
搜索关键词：
- "Claude Code CLAUDE.md memory system implementation"
- "Claude Code MEMORY.md autoDream background consolidation"
- "Claude Code memory files loading mechanism"
- "Claude Code rules files skills system"
- "Claude Code cross session memory internals"

---Research End---


---Research Start--- 05 / Subagent 系统

## 背景

我在做一个项目叫 **Forgify**，是一个本地优先的 AI 工具工作台（桌面 Electron app）。后端用 Go 实现了自有 ReAct Agent 管线。现有状态：无 subagent，所有工具调用在同一 ReAct 循环同一 context 内完成。

Forgify 的 agent 服务入口：`backend/internal/app/chat/chat.go` → `Service.Send`

**2026 年 4 月，Claude Code 的 npm source map 意外泄漏了 512K 行 TypeScript 源码（1906 文件，2026-03-31 快照）。** 网上有多篇深度技术分析文章。

## 任务

请做一份**超级认真、全面、细致到具体实现方式和代码级别**的研究，聚焦在 **Claude Code 的 Subagent 系统**。

具体要研究并写清楚：

### 1. Agent Tool 实现
- `Agent` / `Task` tool 的 TypeScript 实现（参数 schema、execute 函数）
- spawn 一个 subagent 的完整流程（创建独立 context、传递 system prompt、注入 tools）
- subagent 完成后如何返回 summary（1000~2000 token 的 summary 是怎么生成的）
- 父 agent 如何等待 subagent 完成（async/await？Promise？callback？）

### 2. 深度限制
- 深度限制 1 层的实现方式（在 ctx 里打标记？全局计数？）
- subagent 尝试再 spawn 时怎么报错

### 3. 内置 Subagent 类型
- Explore 类型：如何限制只有 Read/Grep/Glob 工具，如何指定 Haiku 模型
- Plan 类型：只读限制怎么实现
- general-purpose 类型：继承父 agent 的工具列表怎么实现
- 自定义类型：system prompt / tool allowlist+denylist / model 怎么配置

### 4. Worktree 隔离
- `isolation: worktree` 模式的完整实现（`git worktree add` 在哪里调用）
- subagent 的工作目录如何切换到 worktree
- 完成后自动清理的实现（`git worktree remove`）
- 对主库的隔离是怎么保证的

### 5. Teams 模式（并行）
- 多个 agent 如何真正并行（tmux pane 是怎么用的）
- Unix Domain Socket mailbox 的实现（SendMessage tool）
- 角色分配（researcher / implementer / verifier）是硬编码还是动态的
- 并行 agent 之间如何同步（等待条件是什么）

### 6. 对 Forgify 的改进建议
列出**具体可落地的 subagent 实现方案**：
- Go 里实现 Agent Tool 的最小方案（启动独立 Service 实例还是复用）
- context 隔离怎么做（独立 goroutine + 独立 message history）
- 深度限制在 Go 里用 context value 传递的实现
- Explore 类型（只读+快模型）在 Forgify 里怎么配置

## 输出要求
- 语言：**中文**
- 保存到：`项目目录/documents/version-1.2/claude-code-research-documents/05-subagent.md`
- 细致到代码级别
- 找不到的细节明确标注"未在泄漏源码分析中找到"

## 信息来源建议
搜索关键词：
- "Claude Code subagent implementation Agent tool"
- "Claude Code Task tool spawn agent context isolation"
- "Claude Code worktree isolation subagent"
- "Claude Code Teams mode parallel agents mailbox"
- "Claude Code depth limit subagent recursion"

---Research End---


---Research Start--- 06 / Hooks 系统

## 背景

我在做一个项目叫 **Forgify**，是一个本地优先的 AI 工具工作台（桌面 Electron app）。后端用 Go 实现了自有 ReAct Agent 管线。现有 Hook 状态：
- `settings.local.json` 里有一个 PostToolUse hook（编辑 backend/ 文件时注入文档同步提醒）
- 没有正式的 Hook 接口
- 没有 PreToolUse、Stop Hook 等

**2026 年 4 月，Claude Code 的 npm source map 意外泄漏了 512K 行 TypeScript 源码（1906 文件，2026-03-31 快照）。** 网上有多篇深度技术分析文章。

## 任务

请做一份**超级认真、全面、细致到具体实现方式和代码级别**的研究，聚焦在 **Claude Code 的 Hooks 系统**。

具体要研究并写清楚：

### 1. Hook 类型完整清单
对每种 Hook 类型详细研究：
- **PreToolUse**：触发时机（在 checkPermissions 之前还是之后），能返回什么（allow/deny/modify），modify 怎么修改 input
- **PermissionRequest**：在权限对话框出现前触发，如何替代用户决策
- **PostToolUse**：触发时机，如何把反馈注入到下一轮 LLM 消息
- **Stop**：Claude 打算结束时怎么触发，返回"继续"的信号格式是什么，agent 如何处理"不许停"
- **SessionStart / SessionEnd**：能做什么，常见用途
- **ConfigChange**：什么算配置变更，如何追踪

### 2. Hook 实现方式
- **shell 命令**：怎么执行，stdin/stdout 格式是什么，超时怎么处理
- **HTTP endpoint**：请求格式（POST JSON？），响应格式，超时
- **LLM prompt**：如何让 LLM 做 yes/no 评估，prompt 格式
- **subagent**（Read+Grep+Glob）：专门用于代码审查的 hook 怎么配置

### 3. Hook 配置格式
- `settings.json` 里 hook 的完整配置 schema（TypeScript 类型）
- glob 模式匹配工具名的语法（`Bash(npm:*)` 是什么格式）
- 多个 hook 的执行顺序

### 4. MCP 工具走 Hook 系统
- `mcp__<server>__<tool>` 命名是怎么映射到 Hook 匹配规则的
- MCP tool 的 PreToolUse 和普通 tool 有什么区别

### 5. Hook 与权限系统的关系
- PreToolUse hook 和 settings allowlist/denylist 的优先级
- hook 返回 allow 能否绕过 denylist

### 6. 对 Forgify 的改进建议
列出**具体可落地的 Hook 系统实现**：
- Go 里设计正式 Hook 接口的方案（interface 设计）
- PreToolUse 在 executeTool 里的插入点
- Stop Hook 在 agentRun 里的插入点（writeDB 之前）
- 配置文件格式设计（YAML/JSON）

## 输出要求
- 语言：**中文**
- 保存到：`项目目录/documents/version-1.2/claude-code-research-documents/06-hooks.md`
- 细致到代码级别
- 找不到的细节明确标注"未在泄漏源码分析中找到"

## 信息来源建议
搜索关键词：
- "Claude Code hooks system PreToolUse PostToolUse implementation"
- "Claude Code Stop hook agent termination"
- "Claude Code hook configuration settings.json schema"
- "Claude Code hook shell command HTTP endpoint"
- "Claude Code hook MCP tool permission"


---Research End---


---Research Start--- 07 / 用户体验工具（AskUserQuestion / Task 系统）

## 背景

我在做一个项目叫 **Forgify**，是一个本地优先的 AI 工具工作台（桌面 Electron app）。后端用 Go 实现了自有 ReAct Agent 管线。现有状态：
- Reasoning token 可见性：`chat.reasoning_token` SSE 事件 ✅
- 无 Task/Todo 系统
- 无 AskUserQuestion（agent 只能等用户主动输入，不能主动暂停问）
- SSE 事件系统已有：`chat.token` / `chat.tool_call` / `chat.tool_result` / `chat.done` / `chat.error`

**2026 年 4 月，Claude Code 的 npm source map 意外泄漏了 512K 行 TypeScript 源码（1906 文件，2026-03-31 快照）。** 网上有多篇深度技术分析文章。

## 任务

请做一份**超级认真、全面、细致到具体实现方式和代码级别**的研究，聚焦在 **Claude Code 的用户体验工具：AskUserQuestion 和 Task 系统**。

具体要研究并写清楚：

### 1. AskUserQuestion Tool
- TypeScript 实现（参数 schema：question 字段，choices 字段？）
- `execute()` 函数的实现：如何暂停 agent loop（Promise resolve 挂起？）
- 用户回答如何传回来（WebSocket？轮询？callback？）
- 用户回答后 agent 如何继续（result 格式是什么，注入到 LLM context 的位置）
- 超时处理：用户长时间不回答怎么办
- 前端 UI：如何展示问题，如何接收回答

### 2. TodoWrite / TaskCreate 系统
- TodoWrite 的参数 schema（todos 数组的结构：id / content / status / priority）
- Task 对象的完整数据结构
- UI 渲染成 checklist 的方式（SSE 推送还是轮询）
- agent 如何更新任务状态（in_progress → completed），SSE 事件格式
- 任务列表是存在内存还是持久化
- TaskList / TaskUpdate / TaskStop 的实现

### 3. Extended Thinking 可见性
- thinking block 在 UI 里的展示方式（默认折叠？）
- effort level（low / medium / high / max）如何控制（API 参数？前端设置？）
- reasoning token 如何从 API 响应里提取（Anthropic thinking block 格式）

### 4. Slash Commands
- 约 85 个 slash command 的注册机制（代码在哪，怎么定义新的）
- `/compact` / `/context` / `/usage` 的具体实现
- 用户自定义 slash command 的格式（skills 文件）

### 5. 对 Forgify 的改进建议
列出**具体可落地的实现方案**：
- **AskUserQuestion**：Go 实现方案（agent loop 怎么暂停，新 SSE 事件 `chat.question` 的格式，前端 input 触发 resume API）
- **Task 系统**：新 SSE 事件 `chat.task_update` 的格式，tool 的参数 schema 设计，前端 checklist 渲染
- 两个功能的实现复杂度评估和先后顺序建议

## 输出要求
- 语言：**中文**
- 保存到：`项目目录/documents/version-1.2/claude-code-research-documents/07-ux-tools.md`
- 细致到代码级别
- 找不到的细节明确标注"未在泄漏源码分析中找到"

## 信息来源建议
搜索关键词：
- "Claude Code AskUserQuestion tool implementation pause agent"
- "Claude Code TodoWrite Task system checklist"
- "Claude Code slash commands implementation registration"
- "Claude Code thinking block extended thinking visibility"
- "Claude Code user interaction tools internals"

---Research End---


---Research Start--- 08 / 权限与安全系统

## 背景

我在做一个项目叫 **Forgify**，是一个本地优先的 AI 工具工作台（桌面 Electron app）。后端用 Go 实现了自有 ReAct Agent 管线。现有安全状态：
- 无权限系统
- Bash tool 有 30s 超时（subprocess）
- 无敏感路径保护
- 无 allow/deny 规则
- 单用户本地桌面，但 Phase 4 workflow 自动执行时风险上升

**2026 年 4 月，Claude Code 的 npm source map 意外泄漏了 512K 行 TypeScript 源码（1906 文件，2026-03-31 快照）。** 网上有多篇深度技术分析文章。

## 任务

请做一份**超级认真、全面、细致到具体实现方式和代码级别**的研究，聚焦在 **Claude Code 的权限与安全系统**。

具体要研究并写清楚：

### 1. 5 层权限 Cascade
- 每一层的实现代码位置和逻辑
- **Tool 自身的 checkPermissions()**：函数签名，返回类型，Bash 里拦截危险命令的规则是什么（黑名单？正则？）
- **Settings allowlist/denylist**：glob 模式的语法（`Bash(npm:*)`, `Read(./.env)`, `WebFetch(domain:x.com)`），匹配算法，配置文件 schema
- **OS 级沙箱**：macOS Seatbelt profile 在哪里定义，Linux bubblewrap 参数，子进程继承怎么实现
- **Permission mode**：default / acceptEdits / plan / bypassPermissions / auto 每个模式的行为差异
- **Hook 覆盖**：PreToolUse hook 怎么插入到这个 cascade 里（在第几层之后）

### 2. ML 分类器（"YOLO classifier"）
- auto 模式下 HIGH / MEDIUM / LOW 风险分类的具体实现
- 哪些操作被认为是 HIGH 风险（rm -rf？写 .gitconfig？）
- 分类结果如何影响是否弹出确认对话框

### 3. Protected Files
- 保护文件的完整列表（`.gitconfig`, `.bashrc`, `.zshrc`, `.mcp.json`, `.claude.json` 等）
- 保护的检查在哪里（Write tool 里？还是权限层）
- 尝试编辑时的错误信息

### 4. Permission Explainer
- 高风险操作前单独调 LLM 解释的 prompt 格式
- 解释结果如何展示给用户
- 用户确认/拒绝的 UI 交互

### 5. 路径安全
- URL 编码检测的实现（`%2F` → `/` 这类）
- Unicode 归一化的方式（NFD/NFC 判断）
- backslash injection 检测
- 大小写 bypass 检测（macOS 大小写不敏感文件系统）

### 6. 对 Forgify 的改进建议
Forgify 单用户本地，列出**最小可用的安全加固方案**：
- Tool 接口加 `PermissionLevel()` 方法的设计（ReadOnly / WorkspaceWrite / DangerFullAccess）
- Protected paths 检查的实现（write_file / run_shell 里怎么加）
- Bash tool 危险命令黑名单设计
- settings allow/deny 配置的最小实现（不需要 OS 沙箱）

## 输出要求
- 语言：**中文**
- 保存到：`项目目录/documents/version-1.2/claude-code-research-documents/08-permissions.md`
- 细致到代码级别
- 找不到的细节明确标注"未在泄漏源码分析中找到"

## 信息来源建议
搜索关键词：
- "Claude Code permission system checkPermissions implementation"
- "Claude Code allowlist denylist glob pattern tool permissions"
- "Claude Code Seatbelt sandbox macOS security"
- "Claude Code YOLO classifier risk assessment"
- "Claude Code protected files path traversal defense"

---Research End---


---Research Start--- 09 / MCP 集成

## 背景

我在做一个项目叫 **Forgify**，是一个本地优先的 AI 工具工作台（桌面 Electron app）。后端用 Go 实现了自有 ReAct Agent 管线。现有 MCP 状态：
- 计划 Phase 5 实现
- 尚未有任何 MCP 代码
- 已有统一 Tool 接口（`app/agent/tool.go`），MCP tool 将注册到同一接口

**2026 年 4 月，Claude Code 的 npm source map 意外泄漏了 512K 行 TypeScript 源码（1906 文件，2026-03-31 快照）。** 网上有多篇深度技术分析文章。

## 任务

请做一份**超级认真、全面、细致到具体实现方式和代码级别**的研究，聚焦在 **Claude Code 的 MCP 集成实现**。

具体要研究并写清楚：

### 1. MCP 工具延迟加载
- session 启动时只加载 tool name 的实现（哪个函数，返回什么结构）
- 首次调用时加载完整 schema 的触发时机和实现
- schema 的缓存策略（内存缓存？session 级别？）
- 延迟加载带来的 context 节省有多大（token 估算）

### 2. MCP Server 配置与发现
- 多 scope 配置（project / user / local / enterprise / 插件 server）的优先级和合并规则
- 配置文件格式（`.mcp.json` / `settings.json` 里的 mcpServers 字段 schema）
- server 启动方式（stdio？HTTP？）
- 如何发现和连接 MCP server（自动还是手动配置）

### 3. MCPTool 到统一 Tool 接口的映射
- `mcp__<server>__<tool>` 命名规范的生成代码
- MCP tool 的 parameters schema 如何转成 Claude Code Tool 接口
- tool result 格式转换（MCP content blocks → Claude Code result string）
- 错误处理（server 断开、工具不存在等）

### 4. Transport 实现
- stdio transport：子进程管理（启动/停止/重启），stdin/stdout 协议格式
- HTTP transport：OAuth 2.0 认证流程，Protected Resource Metadata discovery 的实现
- 两种 transport 的切换逻辑

### 5. 权限与 Hook 集成
- MCP tool 走 permission + hook 系统的实现（在哪里插入）
- 用户如何配置 MCP server 的 allowlist（trust model）
- MCP tool 的 PreToolUse hook 触发方式

### 6. MCP Resources 和 Prompts
- MCP resources（数据资源）是如何暴露给 agent 的
- MCP prompts（提示模板）的使用方式
- 与 tool 的区别和适用场景

### 7. 对 Forgify 的改进建议
Forgify 使用 `mark3labs/mcp-go`（已在 backend-design.md 计划中），列出**具体的集成方案**：
- Go 里如何用 mcp-go 实现 stdio transport 的 server 连接
- MCPTool 注册到现有 Tool 接口的适配器设计
- 延迟加载在 Go 里的实现（sync.Once？）
- 优先实现 stdio 的最小可行版本代码结构

## 输出要求
- 语言：**中文**
- 保存到：`项目目录/documents/version-1.2/claude-code-research-documents/09-mcp.md`
- 细致到代码级别
- 找不到的细节明确标注"未在泄漏源码分析中找到"

## 信息来源建议
搜索关键词：
- "Claude Code MCP integration implementation internals"
- "Claude Code MCPTool lazy loading schema"
- "Claude Code MCP server configuration .mcp.json"
- "Claude Code MCP stdio HTTP transport implementation"
- "Claude Code MCP OAuth authentication"

---Research End---
