# A3 · 事件系统 — 技术设计文档

**切片**：A3  
**状态**：待 Review

---

## 1. 目录结构

```
internal/events/
├── events.go        # 事件名常量 + Bridge struct
└── payloads.go      # 所有事件的 payload 类型定义

frontend/src/
└── lib/
    └── events.ts    # TypeScript 事件类型 + 订阅工具函数
```

---

## 2. Go 层

### 2.1 events.go — 常量 + Bridge

```go
package events

import (
    "context"
    "encoding/json"

    "github.com/wailsapp/wails/v3/pkg/application"
)

// 事件名常量，避免字符串拼写错误
const (
    // 对话类
    ChatToken    = "chat.token"
    ChatDone     = "chat.done"
    ChatError    = "chat.error"
    ChatCompacted = "chat.compacted"

    // 工作流执行类
    FlowNodeStatus = "flow.node.status"
    FlowNodeOutput = "flow.node.output"
    FlowRunDone    = "flow.run.done"
    FlowRunError   = "flow.run.error"

    // 审批类
    ApprovalRequest = "approval.request"
    ApprovalExpired = "approval.expired"

    // 画布类
    CanvasUpdated = "canvas.updated"

    // 主题记忆类
    TopicMemoryUpdated = "topic.memory.updated"

    // 系统类
    TrayFirstHide = "tray.first-hide"
    Notification  = "notification"
)

// Bridge 封装 Wails EventsEmit，业务层通过它发事件
type Bridge struct {
    ctx context.Context
}

func NewBridge(ctx context.Context) *Bridge {
    return &Bridge{ctx: ctx}
}

func (b *Bridge) Emit(event string, payload any) {
    data, _ := json.Marshal(payload)
    application.EmitEvent(b.ctx, event, string(data))
}
```

### 2.2 payloads.go — Payload 类型

```go
package events

// --- 对话类 ---
type ChatTokenPayload struct {
    ConversationID string `json:"conversationId"`
    Token          string `json:"token"`
    Done           bool   `json:"done"`
}

type ChatDonePayload struct {
    ConversationID string `json:"conversationId"`
    MessageID      string `json:"messageId"`
}

type ChatErrorPayload struct {
    ConversationID string `json:"conversationId"`
    Error          string `json:"error"`
}

type ChatCompactedPayload struct {
    ConversationID string `json:"conversationId"`
    Level          string `json:"level"` // "micro" | "auto" | "full"
}

// --- 工作流执行类 ---
type FlowNodeStatusPayload struct {
    RunID  string `json:"runId"`
    NodeID string `json:"nodeId"`
    Status string `json:"status"` // "pending" | "running" | "done" | "error"
}

type FlowNodeOutputPayload struct {
    RunID  string `json:"runId"`
    NodeID string `json:"nodeId"`
    Output any    `json:"output"`
}

type FlowRunDonePayload struct {
    RunID    string `json:"runId"`
    Status   string `json:"status"`
    Duration int64  `json:"duration"` // ms
}

type FlowRunErrorPayload struct {
    RunID  string `json:"runId"`
    NodeID string `json:"nodeId"`
    Error  string `json:"error"`
}

// --- 审批类 ---
type ApprovalRequestPayload struct {
    RequestID   string         `json:"requestId"`
    ToolID      string         `json:"toolId"`
    Operation   string         `json:"operation"`
    Description string         `json:"description"`
    Params      map[string]any `json:"params"`
}

type ApprovalExpiredPayload struct {
    RequestID string `json:"requestId"`
}

// --- 画布类 ---
type CanvasUpdatedPayload struct {
    WorkflowID string `json:"workflowId"`
    Summary    string `json:"summary"` // 紧凑文字描述当前状态
}

// --- 主题记忆类 ---
type TopicMemoryUpdatedPayload struct {
    TopicID string `json:"topicId"`
    Diff    string `json:"diff"` // 本次整理的变化描述
}

// --- 系统类 ---
type NotificationPayload struct {
    Title string `json:"title"`
    Body  string `json:"body"`
    Level string `json:"level"` // "info" | "warn" | "error"
}
```

---

## 3. 前端层

### 3.1 `lib/events.ts` — 类型 + 工具函数

```typescript
import { Events } from '@wailsapp/runtime'

// --- Payload 类型（与 Go 侧对应）---
export interface ChatTokenPayload {
    conversationId: string
    token: string
    done: boolean
}
export interface ChatDonePayload { conversationId: string; messageId: string }
export interface ChatErrorPayload { conversationId: string; error: string }
export interface ChatCompactedPayload { conversationId: string; level: 'micro' | 'auto' | 'full' }

export interface FlowNodeStatusPayload {
    runId: string; nodeId: string
    status: 'pending' | 'running' | 'done' | 'error'
}
export interface FlowNodeOutputPayload { runId: string; nodeId: string; output: unknown }
export interface FlowRunDonePayload { runId: string; status: string; duration: number }
export interface FlowRunErrorPayload { runId: string; nodeId: string; error: string }

export interface ApprovalRequestPayload {
    requestId: string; toolId: string; operation: string
    description: string; params: Record<string, unknown>
}
export interface ApprovalExpiredPayload { requestId: string }

export interface CanvasUpdatedPayload { workflowId: string; summary: string }
export interface TopicMemoryUpdatedPayload { topicId: string; diff: string }
export interface NotificationPayload { title: string; body: string; level: 'info' | 'warn' | 'error' }

// --- 事件名常量 ---
export const EV = {
    ChatToken:          'chat.token',
    ChatDone:           'chat.done',
    ChatError:          'chat.error',
    ChatCompacted:      'chat.compacted',
    FlowNodeStatus:     'flow.node.status',
    FlowNodeOutput:     'flow.node.output',
    FlowRunDone:        'flow.run.done',
    FlowRunError:       'flow.run.error',
    ApprovalRequest:    'approval.request',
    ApprovalExpired:    'approval.expired',
    CanvasUpdated:      'canvas.updated',
    TopicMemoryUpdated: 'topic.memory.updated',
    TrayFirstHide:      'tray.first-hide',
    Notification:       'notification',
} as const

// --- 类型安全的订阅工具 ---
type EventPayloadMap = {
    [EV.ChatToken]:          ChatTokenPayload
    [EV.ChatDone]:           ChatDonePayload
    [EV.ChatError]:          ChatErrorPayload
    [EV.ChatCompacted]:      ChatCompactedPayload
    [EV.FlowNodeStatus]:     FlowNodeStatusPayload
    [EV.FlowNodeOutput]:     FlowNodeOutputPayload
    [EV.FlowRunDone]:        FlowRunDonePayload
    [EV.FlowRunError]:       FlowRunErrorPayload
    [EV.ApprovalRequest]:    ApprovalRequestPayload
    [EV.ApprovalExpired]:    ApprovalExpiredPayload
    [EV.CanvasUpdated]:      CanvasUpdatedPayload
    [EV.TopicMemoryUpdated]: TopicMemoryUpdatedPayload
    [EV.TrayFirstHide]:      undefined
    [EV.Notification]:       NotificationPayload
}

export function onEvent<K extends keyof EventPayloadMap>(
    event: K,
    handler: (payload: EventPayloadMap[K]) => void
): () => void {
    return Events.On(event, (raw: string) => {
        handler(JSON.parse(raw) as EventPayloadMap[K])
    })
}
```

### 3.2 使用示例

```typescript
// 在 React hook 里订阅
import { onEvent, EV } from '@/lib/events'

useEffect(() => {
    const off = onEvent(EV.ChatToken, (e) => {
        // e 是 ChatTokenPayload，有完整类型提示
        appendToken(e.conversationId, e.token)
    })
    return off  // 组件卸载时自动取消订阅
}, [])
```

---

## 4. 在业务层中使用 Bridge

```go
// 在 app.go 初始化
type App struct {
    events *events.Bridge
}

func (a *App) OnStartup(ctx application.Context) {
    a.events = events.NewBridge(ctx)
}

// 在 ConversationAgent 里发事件
a.events.Emit(events.ChatToken, events.ChatTokenPayload{
    ConversationID: convID,
    Token:          token,
    Done:           false,
})
```

---

## 5. 验收测试

```
1. Go emit ChatToken → 前端 onEvent(EV.ChatToken) 收到，payload 类型正确
2. 多个 listener 订阅同一事件，都收到
3. 调用返回的 off() 后，不再收到事件
4. TypeScript 编译：handler 参数类型不匹配时报错（类型安全验证）
5. payload 为 undefined 的事件（TrayFirstHide）正常处理
```
