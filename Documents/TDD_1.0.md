# Forgify — 技术设计文档

**版本**：v0.2 草稿  
**日期**：2026-04-19  
**配套 PRD**：PRD_1.0.md（v0.2）

---

## 0. 技术选型总览

| 层级 | 技术 | 理由 |
|---|---|---|
| 桌面框架 | **Wails v3** | Go 原生，单二进制，WebView，原生 systray 支持 |
| 后端语言 | **Go 1.23+** | 性能、并发、单二进制分发 |
| LLM 编排 | **Eino (CloudWeGo)** | Go 原生，Graph/ReAct/Interrupt-Resume |
| 前端 | **React + TypeScript + Vite** | 嵌入 Wails WebView |
| 工作流画布 | **ReactFlow** | 节点/边拖拽，成熟生态 |
| 本地数据库 | **SQLite (modernc)** | 零 CGO，纯 Go |
| 向量检索 | **viant/sqlite-vec** | 纯 Go 封装，零 CGO，主题记忆语义检索 |
| Python 环境管理 | **uv** (Astral) | 单二进制，自带 Python 版本管理，venv 创建 35ms |
| Python 运行时 | **subprocess + uv venv** | 每工具独立环境，无需 Docker，无需用户预装 Python |
| LLM 调用统一 | **Eino ChatModel 抽象** | 支持 OpenAI / Anthropic / 本地模型 |

**不用 Docker**：本地桌面 app 要求安装 Docker 门槛太高，uv venv + subprocess 足够。  
**不开 HTTP 端口**：前后端通信全走 Wails Bindings 和 Events，无网络监听。  
**Wails v3**：API 稳定，已有生产应用，原生支持 systray 和后台常驻，v2 的 systray 是半成品。

---

## 1. 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                        Wails Desktop App                         │
│                                                                   │
│  ┌────────────────────────────┐                                   │
│  │     React + TypeScript     │  ← WebView (OS原生)               │
│  │                            │                                   │
│  │  ┌──────────┐ ┌─────────┐  │                                   │
│  │  │ Chat UI  │ │ Canvas  │  │  Wails Bindings (同步调用)         │
│  │  │  (流式)  │ │ReactFlow│  │ ◄─────────────────────────────┐   │
│  │  └──────────┘ └─────────┘  │                               │   │
│  │  ┌──────────┐ ┌─────────┐  │  Wails Events (异步推送)      │   │
│  │  │Tool Lib  │ │  Home   │  │ ◄─────────────────────────────┤   │
│  │  └──────────┘ └─────────┘  │                               │   │
│  └────────────────────────────┘                               │   │
│                                                                │   │
│  ┌─────────────────────────────────────────────────────────────┘  │
│  │                        Go Backend                               │
│  │                                                                  │
│  │  ┌─────────────────────────┐  ┌──────────────────────────────┐  │
│  │  │       Eino 层           │  │         业务层               │  │
│  │  │                         │  │                              │  │
│  │  │  ConversationAgent      │  │  AppService (Wails App)      │  │
│  │  │  ├─ ReAct Loop          │  │  ToolRegistry                │  │
│  │  │  ├─ ToolsNode           │  │  WorkflowService             │  │
│  │  │  └─ ContextManager      │  │  TopicService                │  │
│  │  │                         │  │  PermissionGate              │  │
│  │  │  CoCreationGraph        │  │  Scheduler (cron)            │  │
│  │  │  ├─ SpecGenNode         │  │  EventBridge                 │  │
│  │  │  ├─ CodeGenNode         │  │                              │  │
│  │  │  └─ TestRunNode         │  └──────────────────────────────┘  │
│  │  │                         │                                    │
│  │  │  FlowExecutor           │  ┌──────────────────────────────┐  │
│  │  │  └─ Eino Graph (DAG)    │  │        数据层                │  │
│  │  │     ├─ ToolNode         │  │  SQLite (modernc)            │  │
│  │  │     ├─ AgentNode        │  │  sqlite-vec                  │  │
│  │  │     ├─ ConditionNode    │  │  本地文件系统                │  │
│  │  │     ├─ HumanInputNode   │  └──────────────────────────────┘  │
│  │  │     └─ LLMNode          │                                    │
│  │  │                         │  ┌──────────────────────────────┐  │
│  │  │  TopicMemoryAgent       │  │      Python 沙箱             │  │
│  │  │  └─ autoDream goroutine │  │  subprocess + venv           │  │
│  │  └─────────────────────────┘  │  per-tool 权限门控           │  │
│  │                                └──────────────────────────────┘  │
│  └──────────────────────────────────────────────────────────────── │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. 目录结构

```
forgify/
├── main.go                    # Wails 入口
├── app.go                     # Wails App struct，暴露给前端的方法
├── wails.json
├── build/                     # Wails 打包资源
│
├── internal/
│   ├── agent/                 # Eino 层
│   │   ├── conversation.go    # ConversationAgent (ReAct)
│   │   ├── cocreation.go      # CoCreationGraph (spec→code→test)
│   │   ├── flow_executor.go   # FlowExecutor (DAG)
│   │   └── topic_memory.go    # TopicMemoryAgent + autoDream
│   │
│   ├── service/               # 业务层
│   │   ├── tool.go            # ToolRegistry, ToolService
│   │   ├── workflow.go        # WorkflowService
│   │   ├── topic.go           # TopicService
│   │   ├── conversation.go    # ConversationService
│   │   └── scheduler.go       # Cron 调度器
│   │
│   ├── permission/            # 权限系统
│   │   ├── gate.go            # per-tool 权限门控
│   │   └── mailbox.go         # Mailbox 审批队列
│   │
│   ├── sandbox/               # Python 沙箱
│   │   ├── runner.go          # subprocess 执行器
│   │   ├── venv.go            # venv 生命周期管理
│   │   └── scanner.go         # 静态权限扫描
│   │
│   ├── storage/               # 数据层
│   │   ├── db.go              # SQLite 连接 + 迁移
│   │   ├── models.go          # 数据模型
│   │   └── queries/           # SQL 查询
│   │
│   ├── events/                # Wails EventBridge
│   │   └── bridge.go          # runtime.EventsEmit 封装
│   │
│   └── context/               # 上下文压缩
│       └── compressor.go      # 三层压缩策略
│
├── tools/                     # 内置工具定义
│   ├── builtin/
│   │   ├── gmail/
│   │   │   ├── tool.yaml
│   │   │   └── main.py
│   │   ├── excel/
│   │   └── ...
│   └── registry.go            # 内置工具加载器
│
└── frontend/                  # React 前端
    ├── src/
    │   ├── App.tsx
    │   ├── pages/
    │   │   ├── Home.tsx
    │   │   ├── Chat.tsx        # 三栏布局
    │   │   ├── Tools.tsx
    │   │   └── Workflows.tsx
    │   ├── components/
    │   │   ├── Canvas/         # ReactFlow 画布
    │   │   │   ├── nodes/      # 各类节点组件
    │   │   │   └── FlowCanvas.tsx
    │   │   ├── Chat/           # 消息流
    │   │   └── ToolForge/      # 工具锻造面板
    │   ├── hooks/
    │   │   ├── useChat.ts      # 流式消息接收
    │   │   └── useCanvas.ts    # 画布状态管理
    │   └── wailsjs/            # Wails 自动生成的 JS bindings
    ├── package.json
    └── vite.config.ts
```

---

## 3. 前后端通信

### 3.1 两种通信方式

**Wails Bindings（同步调用）**：前端直接调用 Go 函数，返回 Promise。

```typescript
// 前端调用示例
import { CreateConversation, GetTools } from '../wailsjs/go/main/App'

const conv = await CreateConversation({ title: "新对话" })
const tools = await GetTools({ status: "ready" })
```

```go
// app.go — 暴露给前端
func (a *App) CreateConversation(req CreateConversationReq) (*Conversation, error) { ... }
func (a *App) GetTools(req GetToolsReq) ([]*Tool, error) { ... }
```

**Wails Events（异步推送）**：Go 主动推送到前端，用于流式 token、运行状态更新。

```go
// Go 推送
runtime.EventsEmit(ctx, "chat.token", ChatTokenEvent{
    ConversationID: convID,
    Token:          token,
    Done:           false,
})
```

```typescript
// 前端接收
import { EventsOn } from '../wailsjs/runtime'

EventsOn("chat.token", (event: ChatTokenEvent) => {
    appendToken(event.conversationID, event.token)
})
```

### 3.2 Event 类型清单

| Event 名 | 方向 | 说明 |
|---|---|---|
| `chat.token` | Go→前端 | 流式 token |
| `chat.done` | Go→前端 | 本轮对话完成 |
| `canvas.update` | Go→前端 | 工作流节点状态变化 |
| `flow.node.status` | Go→前端 | 节点运行状态（running/done/error）|
| `flow.node.output` | Go→前端 | 节点输出数据 |
| `approval.request` | Go→前端 | Mailbox 审批请求 |
| `topic.memory.updated` | Go→前端 | autoDream 整理完成 |
| `notification` | Go→前端 | 桌面通知 |

---

## 4. Eino 层设计

### 4.1 ConversationAgent

```
用户输入
    │
    ▼
┌─────────────────────────────┐
│      ConversationAgent       │
│                              │
│  ChatModel (可配置)          │
│  + ToolsNode                 │
│    ├─ 内置工具 (Go 实现)     │
│    │   ├─ CreateTool         │
│    │   ├─ CreateWorkflow     │
│    │   ├─ RunWorkflow        │
│    │   └─ SearchMemory       │
│    └─ 用户工具 (Python 沙箱) │
│                              │
│  ContextManager              │
│  └─ 三层压缩                 │
└─────────────────────────────┘
    │
    ▼
流式 token → EventBridge → 前端
```

Agent 使用 Eino 的 ReAct 模式，内置工具全部是 Go 函数（不走沙箱），只有用户自定义工具才走 Python 沙箱。

### 4.2 CoCreationGraph

工具锻造流程，三节点串行 Graph：

```
SpecGenNode ──► CodeGenNode ──► TestRunNode
    │               │               │
  生成接口定义    生成 Python 代码   运行测试用例
  (tool.yaml)    (main.py)         返回测试结果
    │               │               │
    ▼               ▼               ▼
 用户确认 ◄────── 用户确认 ◄────── 用户确认
```

每个节点完成后都有 **Interrupt**，等待用户在前端确认，确认后 **Resume**。这直接使用 Eino 的 Interrupt/Resume 机制。

### 4.3 FlowExecutor

用户创建的工作流在运行时编译成 Eino Graph：

```go
// FlowDefinition (JSON) → Eino Graph
func CompileFlow(def *FlowDefinition) (*eino.Graph, error) {
    g := eino.NewGraph()
    for _, node := range def.Nodes {
        switch node.Type {
        case "tool":     g.AddNode(newToolNode(node))
        case "agent":    g.AddNode(newAgentNode(node))
        case "llm":      g.AddNode(newLLMNode(node))
        case "condition":g.AddNode(newConditionNode(node))
        case "loop":     g.AddNode(newLoopNode(node))
        case "parallel": g.AddNode(newParallelNode(node))
        case "human":    g.AddNode(newHumanInputNode(node))  // Interrupt
        }
    }
    for _, edge := range def.Edges {
        g.AddEdge(edge.From, edge.To)
    }
    return g.Build()
}
```

**HumanInputNode** 使用 Eino Interrupt，将审批请求发送到 Mailbox，等待用户响应后 Resume。

### 4.4 TopicMemoryAgent + autoDream

```go
// 主题记忆存储结构
type TopicMemory struct {
    Context   string      // 当前积累的上下文
    Decisions []Decision  // 历史决策列表
    Version   int         // 版本号，用于回滚
    UpdatedAt time.Time
}

// autoDream goroutine
func (a *TopicMemoryAgent) StartDreamLoop(ctx context.Context) {
    ticker := time.NewTicker(1 * time.Hour)
    for {
        select {
        case <-ticker.C:
            for _, topic := range a.getEligibleTopics() {
                a.dream(ctx, topic)
            }
        case <-ctx.Done():
            return
        }
    }
}

// 触发条件检查
func (a *TopicMemoryAgent) isEligible(topic *Topic) bool {
    return time.Since(topic.LastDreamAt) >= 24*time.Hour &&
        topic.SessionsSinceLastDream >= 3 &&
        !topic.DreamLock
}

// 四阶段整理
func (a *TopicMemoryAgent) dream(ctx context.Context, topic *Topic) {
    // 1. Orient  — 评估当前记忆状态
    // 2. Gather  — 收集近期新信息
    // 3. Consolidate — LLM 合并整理
    // 4. Prune   — 裁剪到 200 行以内
    // 整理完成后 emit topic.memory.updated 事件
}
```

---

## 5. 权限系统

### 5.1 per-tool 权限门控

每个工具的 tool.yaml 声明权限，运行前检查：

```go
type PermissionGate struct {
    // 已授权的工具权限缓存
    granted map[string][]Permission
}

func (g *PermissionGate) Check(toolID string, required []Permission) error {
    granted := g.granted[toolID]
    for _, p := range required {
        if p.Level == Execute && !contains(granted, p) {
            return ErrRequiresApproval{Tool: toolID, Permission: p}
        }
    }
    return nil
}
```

### 5.2 Mailbox 审批队列

执行级操作不阻塞工作流，进入 Mailbox 等待用户审批：

```go
type Mailbox struct {
    queue   chan ApprovalRequest
    results map[string]chan ApprovalResult
}

type ApprovalRequest struct {
    ID          string
    ToolID      string
    Operation   string
    Params      map[string]any  // 用户可修改
    Description string          // 人类可读描述
}

func (m *Mailbox) RequestApproval(req ApprovalRequest) ApprovalResult {
    m.queue <- req
    // 推送到前端
    events.Emit("approval.request", req)
    // 阻塞等待用户响应（只阻塞该节点的 goroutine，不影响其他节点）
    return <-m.results[req.ID]
}
```

---

## 6. 数据层 Schema

```sql
-- 对话
CREATE TABLE conversations (
    id          TEXT PRIMARY KEY,
    title       TEXT,
    type        TEXT CHECK(type IN ('free', 'linked')),
    linked_type TEXT,  -- 'tool' | 'workflow' | 'topic'
    linked_id   TEXT,
    folder_id   TEXT,
    created_at  DATETIME,
    updated_at  DATETIME
);

-- 消息
CREATE TABLE messages (
    id              TEXT PRIMARY KEY,
    conversation_id TEXT REFERENCES conversations(id),
    role            TEXT CHECK(role IN ('user', 'assistant', 'system')),
    content         TEXT,
    content_type    TEXT DEFAULT 'text',  -- 'text' | 'card' | 'canvas_echo'
    metadata        JSON,
    created_at      DATETIME
);

-- 工具
CREATE TABLE tools (
    id          TEXT PRIMARY KEY,
    name        TEXT,
    description TEXT,
    status      TEXT CHECK(status IN ('draft','testing','ready','deprecated')),
    version     TEXT,
    yaml_path   TEXT,   -- 指向 tool.yaml 文件
    code_path   TEXT,   -- 指向 main.py 文件
    chat_id     TEXT REFERENCES conversations(id),  -- 专属对话
    folder_id   TEXT,
    created_at  DATETIME,
    updated_at  DATETIME
);

-- 工作流
CREATE TABLE workflows (
    id          TEXT PRIMARY KEY,
    name        TEXT,
    status      TEXT CHECK(status IN ('draft','ready','deployed','paused','archived')),
    definition  JSON,   -- FlowDefinition (节点 + 边)
    chat_id     TEXT REFERENCES conversations(id),
    folder_id   TEXT,
    created_at  DATETIME,
    updated_at  DATETIME
);

-- 工作流运行记录
CREATE TABLE flow_runs (
    id          TEXT PRIMARY KEY,
    workflow_id TEXT REFERENCES workflows(id),
    status      TEXT CHECK(status IN ('running','success','failed','cancelled')),
    trigger     TEXT,
    started_at  DATETIME,
    finished_at DATETIME,
    error       TEXT
);

-- 节点运行记录
CREATE TABLE node_runs (
    id          TEXT PRIMARY KEY,
    flow_run_id TEXT REFERENCES flow_runs(id),
    node_id     TEXT,
    node_type   TEXT,
    status      TEXT,
    input       JSON,
    output      JSON,
    started_at  DATETIME,
    finished_at DATETIME,
    error       TEXT
);

-- 主题
CREATE TABLE topics (
    id                      TEXT PRIMARY KEY,
    name                    TEXT,
    goal                    TEXT,
    memory_context          TEXT,
    memory_version          int DEFAULT 0,
    last_dream_at           DATETIME,
    sessions_since_dream    INT DEFAULT 0,
    dream_lock              BOOLEAN DEFAULT FALSE,
    created_at              DATETIME,
    updated_at              DATETIME
);

-- 主题关联资产
CREATE TABLE topic_assets (
    topic_id   TEXT REFERENCES topics(id),
    asset_type TEXT CHECK(asset_type IN ('chat','tool','workflow')),
    asset_id   TEXT,
    PRIMARY KEY (topic_id, asset_type, asset_id)
);

-- 主题决策记录
CREATE TABLE topic_decisions (
    id         TEXT PRIMARY KEY,
    topic_id   TEXT REFERENCES topics(id),
    content    TEXT,
    created_at DATETIME
);

-- 主题记忆历史（用于 autoDream 回滚）
CREATE TABLE topic_memory_snapshots (
    id         TEXT PRIMARY KEY,
    topic_id   TEXT REFERENCES topics(id),
    context    TEXT,
    version    INT,
    created_at DATETIME
);

-- 文件夹
CREATE TABLE folders (
    id          TEXT PRIMARY KEY,
    name        TEXT,
    parent_id   TEXT REFERENCES folders(id),
    asset_type  TEXT CHECK(asset_type IN ('chat','tool','workflow')),
    created_at  DATETIME
);
```

---

## 7. Python 沙箱

### 7.1 uv 管理 Python 环境

Forgify 将 `uv` 二进制打包进应用，用户无需预装 Python：

```
工具首次使用
    │
    ▼
uv python install 3.11      # 下载预编译 Python，~秒级，只需一次
    │
    ▼
uv venv .venv/              # 35ms，比 python -m venv 快 ~60x
    │
    ▼
uv pip install -r requirements.txt   # 比 pip 快 10~100x，有全局缓存
    │
    ▼
运行 (subprocess, timeout=30s)
    │
    ├── stdout → 解析为 JSON output
    ├── stderr → 错误日志
    └── exitcode != 0 → 报错给 Agent
```

venv 创建后缓存复用，后续调用直接跳到运行步骤。用户体验：首次 < 10s，后续 < 1s。

### 7.2 跨平台 subprocess 调用

**核心原则：始终用 Args 列表，不用 shell string**，Windows 路径带空格时完全安全：

```go
// ✅ 正确：列表传参，Windows CreateProcess 自动处理空格
uvBin := filepath.Join(forgifyDataDir, "uv")  // Windows 自动加 .exe
venvPython := filepath.Join(toolDir, ".venv", "bin", "python")
if runtime.GOOS == "windows" {
    venvPython = filepath.Join(toolDir, ".venv", "Scripts", "python.exe")
}

cmd := exec.Command(venvPython, "main.py")
cmd.Dir = toolDir
cmd.Env = append(os.Environ(),
    "FORGIFY_WORK_DIR="+workDir,
    "FORGIFY_TOOL_ID="+toolID,
)

// ❌ 错误：字符串拼接 + shell=true，路径有空格就炸
// cmd := exec.Command("cmd", "/C", venvPython+" main.py")
```

### 7.3 权限扫描

运行前静态扫描 main.py，检测未声明的权限使用：

```go
var permissionPatterns = map[Permission][]string{
    PermFileWrite:    {"open(.*'w'", "write(", "os.remove", "shutil."},
    PermNetworkCall:  {"requests.", "httpx.", "urllib", "socket."},
    PermShellExecute: {"subprocess.", "os.system(", "os.popen("},
}
```

### 7.4 文件系统隔离

工具只能访问用户指定的工作目录，通过环境变量传入，Python 侧读取并强制 chdir：

```python
# main.py 头部固定模板
import os
work_dir = os.environ.get("FORGIFY_WORK_DIR", ".")
os.chdir(work_dir)
```

---

## 8. 上下文压缩（三层）

```go
type ContextManager struct {
    model ChatModel
}

// 在每次 Agent 调用前检查并压缩
func (cm *ContextManager) MaybeCompress(messages []Message, limit int) []Message {
    usage := estimateTokens(messages)

    switch {
    case usage > limit*90/100:
        // AutoCompact: LLM 摘要历史
        return cm.autoCompact(messages)
    case usage > limit*80/100:
        // MicroCompact: 本地裁剪（去掉工具调用详情、保留结果）
        return cm.microCompact(messages)
    default:
        return messages
    }
}

// FullCompact: 用户手动触发，完整重新生成对话摘要
func (cm *ContextManager) FullCompact(messages []Message) []Message { ... }
```

压缩后向前端发送 `chat.compacted` 事件，显示"部分历史已压缩"提示。

---

## 9. 调度器

使用 `robfig/cron` 管理定时工作流：

```go
type Scheduler struct {
    cron *cron.Cron
    jobs map[string]cron.EntryID  // workflowID → cronEntryID
}

func (s *Scheduler) Deploy(workflow *Workflow) error {
    entryID, err := s.cron.AddFunc(workflow.CronExpr, func() {
        s.flowExecutor.Run(workflow.ID, TriggerCron)
    })
    s.jobs[workflow.ID] = entryID
    return err
}

func (s *Scheduler) Pause(workflowID string) {
    s.cron.Remove(s.jobs[workflowID])
}
```

电脑唤醒后检查错过的任务（可配置是否补跑）。

---

## 10. 前端关键设计

### 10.1 ReactFlow 节点类型

每种工作流节点对应一个 ReactFlow 自定义节点组件：

| 节点类型 | 组件 | 颜色 |
|---|---|---|
| trigger | TriggerNode | 绿色 |
| tool | ToolNode | 蓝色（ready）/ 黄色（缺失）/ 红色（error）|
| agent | AgentNode | 紫色 |
| llm | LLMNode | 靛蓝 |
| condition | ConditionNode | 橙色 |
| loop | LoopNode | 青色 |
| parallel | ParallelNode | 青色 |
| human | HumanInputNode | 琥珀色 |
| variable | VariableNode | 灰色 |
| subflow | SubflowNode | 深蓝 |

### 10.2 流式消息接收

```typescript
// hooks/useChat.ts
export function useChat(conversationId: string) {
    const [messages, setMessages] = useState<Message[]>([])

    useEffect(() => {
        // 接收流式 token
        const off1 = EventsOn("chat.token", (e: ChatTokenEvent) => {
            if (e.conversationId !== conversationId) return
            setMessages(prev => appendToken(prev, e.token))
        })
        // 对话完成
        const off2 = EventsOn("chat.done", (e: ChatDoneEvent) => {
            if (e.conversationId !== conversationId) return
            setMessages(prev => finalize(prev, e.messageId))
        })
        return () => { off1(); off2() }
    }, [conversationId])

    return { messages }
}
```

### 10.3 画布操作回声

用户在画布上的每个操作，转化为文本注入对话上下文：

```typescript
// 用户拖动节点 → 生成回声
const onNodeDragStop = (node: Node) => {
    SendCanvasEcho({
        conversationId,
        action: "moved",
        target: `节点 "${node.data.label}"`,
        detail: `移到位置 (${node.position.x}, ${node.position.y})`,
        note: "",  // 用户可附加备注
    })
}
```

---

## 11. 打包与分发

使用 Wails 内置打包：

```bash
wails build -platform darwin/amd64,darwin/arm64  # macOS Universal
wails build -platform windows/amd64
wails build -platform linux/amd64
```

内置 Python 工具与主程序分离，支持独立增量更新（通过版本检查 + 下载新版 tools/builtin/）。

---

## 12. 技术决策记录

原有未决问题已全部确定方案：

| # | 问题 | 决策 | 理由 |
|---|---|---|---|
| 1 | Python 版本管理 | **打包 uv 二进制** | uv 自带 Python 版本管理，下载预编译包，秒级完成，无需用户操作 |
| 2 | venv 初始化耗时 | **uv venv 35ms，用户无感知** | uv 比 python -m venv 快 60x，有全局包缓存，首次 < 10s |
| 3 | Wails 系统托盘 | **升级到 Wails v3** | v3 原生支持 systray + 独立窗口生命周期，light/dark 图标，API 已稳定 |
| 4 | sqlite-vec 集成 | **viant/sqlite-vec（纯 Go）** | 专为 modernc 设计，零 CGO，注册 vec 虚拟表到 modernc 连接 |
| 5 | autoDream LLM 成本 | **分层策略** | MicroCompact 纯代码（零成本）；AutoCompact/Consolidate 用最便宜模型（Haiku/Flash）；提取式优先于生成式 |
| 6 | Windows 路径处理 | **exec.Cmd.Args 列表，不用 shell** | Go os/exec 标准实践，Windows CreateProcess 原生处理带空格路径 |

---

**文档结束**
