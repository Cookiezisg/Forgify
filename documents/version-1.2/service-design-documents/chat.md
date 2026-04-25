# chat domain — 详细设计文档

**所属 Phase**：Phase 2 起（每个 Phase 都会升级）
**状态**：🔄 设计已定，Phase 2 代码待实现
**地位**：**全系统最核心的 domain**——用户的每一次对话都从这里进入，一切能力都通过这里编排。

**关联文档**：
- [`../backend-design.md`](../backend-design.md) — 总规范
- [`../service-contract-documents/api-design.md`](../service-contract-documents/api-design.md) — API 索引
- [`../service-contract-documents/events-design.md`](../service-contract-documents/events-design.md) — 事件索引

---

## 1. 核心思想：一切都是 Tool Call

### 1.1 为什么

Forgify 的终极形态是：用户一句话，AI 自主完成"创建工具→测试→组建工作流→挂知识库→部署"的完整链路，中间多次迭代，用户实时看到每一步。

这本质上是一个**自主 Agent 循环**，而不是简单的"识别意图→路由→执行一次"。

### 1.2 是什么

从 LLM 的视角，它只有两种输出：
- **直接回复**（= 任务完成）
- **调一个 Tool**（= 还有事情要做）

所有 Forgify 的能力——创建工具、运行沙箱、搜知识库、创建工作流——对 LLM 都是 Tool。Agent 每轮只做一个决策（调哪个 Tool 或直接回复），拿到结果后再想下一步，直到认为任务完成。

这就是 **ReAct 循环**（Reasoning + Acting），和 Claude Code 的工作方式完全一致。

### 1.3 关键约束

**每个小轮次只有一次 Tool Call。** 这不是限制，这是优点：
- 每一步都可观测（实时推事件给前端）
- 每一步都可中断
- LLM 的推理链清晰可追溯
- 不会一口气做完所有事情让用户措手不及

---

## 2. 两层工具体系

这是整个设计最关键的决策。

### 2.1 问题

用户最终可能创建数百个工具。如果把所有工具都塞进 LLM context，性能严重下降，LLM 会选错工具，最重要的系统工具会被淹没。

### 2.2 解法

```
┌─────────────────────────────────────────────────────┐
│                  Agent Context                       │
│                                                      │
│  System Tools（永远在 context，~8 个）               │
│  ┌────────────┐ ┌──────────┐ ┌────────────────────┐ │
│  │ create_tool│ │ edit_tool│ │     run_tool(id)    │ │
│  └────────────┘ └──────────┘ └────────────────────┘ │
│  ┌─────────────┐ ┌──────────────────────────────┐   │
│  │ search_tools│ │  create_workflow / run_workflow│   │
│  └─────────────┘ └──────────────────────────────┘   │
│  ┌──────────────────┐ ┌──────────┐                  │
│  │ search_knowledge  │ │ mcp_call │                  │
│  └──────────────────┘ └──────────┘                  │
└─────────────────────────────────────────────────────┘

用户工具库（不在 context，通过 search_tools 发现，run_tool 执行）
┌──────────────┐ ┌──────────────┐ ┌──────────────┐
│ email_parser │ │ csv_processor│ │  ...（数百个）│
└──────────────┘ └──────────────┘ └──────────────┘
```

**System Tools** 是 meta-tools：用来创建/管理其他工具和工作流。永远可见。

**User Tools** 不直接注入 context。Agent 通过：
1. `search_tools(query)` → 语义搜索工具库，得到相关工具列表
2. `run_tool(id, input)` → 通用执行器，执行任意用户工具

这本质上是 **Tool RAG**——与知识库 RAG 同一个思路，检索对象是工具描述。

### 2.3 System Tools 完整目录

| Tool | Phase | 描述 | 对接的 domain |
|---|---|---|---|
| `create_tool` | 3 | 创建新工具（名称/描述/代码）| forge + tool |
| `edit_tool` | 3 | 修改已有工具的代码或描述 | tool |
| `run_tool` | 3 | 通用执行器，按 id 运行任意工具 | tool sandbox |
| `search_tools` | 3 | 语义搜索工具库，返回相关工具列表 | tool（FTS5）|
| `create_workflow` | 4 | 创建工作流（DAG 节点定义）| workflow |
| `edit_workflow` | 4 | 修改已有工作流 | workflow |
| `run_workflow` | 4 | 执行工作流，返回运行结果 | flowrun |
| `search_knowledge` | 5 | RAG 检索知识库 | knowledge |
| `mcp_call` | 5 | 调用 MCP 服务器的方法 | mcpserver |

**Phase 2**：tools 列表为空。Agent 就是一个没有工具的 ReAct Agent，行为等同于纯 LLM 流式对话，但架构已经是可扩展的。

---

## 3. Eino 集成架构

### 3.1 核心组件

```
chat.Service
    ↓ 构造
react.Agent (eino/agents/react)
    ├── ToolCallingModel   ← infra/eino.ModelFactory 构建
    ├── ToolsConfig        ← System Tools 列表（Phase 3+ 注入）
    ├── MessageModifier    ← 注入 System Prompt（含 locale）
    ├── MaxStep: 20        ← 最多 10 次 Tool Call 迭代
    └── StreamToolCallChecker ← 各 provider 自定义实现
```

### 3.2 Eino ReAct Agent 精确 API

```go
import (
    "github.com/cloudwego/eino/flow/agent/react"
    "github.com/cloudwego/eino/schema"
)

agent, err := react.NewAgent(ctx, &react.AgentConfig{
    // 必填：支持 tool calling 的 ChatModel
    ToolCallingModel: chatModel,

    // 工具列表（Phase 2: nil；Phase 3+: system tools）
    ToolsConfig: compose.ToolsNodeConfig{
        Tools: systemTools,
    },

    // 每次 ChatModel 调用前注入 System Prompt + locale
    MessageModifier: func(ctx context.Context, msgs []*schema.Message, _ string) []*schema.Message {
        return append([]*schema.Message{
            schema.SystemMessage(buildSystemPrompt(ctx)),
        }, msgs...)
    },

    // 最多 20 步（= 10 次 Tool Call 往返）
    MaxStep: 20,

    // Claude 需要自定义（见 §3.4）
    StreamToolCallChecker: providerStreamChecker(provider),
})
```

### 3.3 Agent 流式执行

```go
streamResult, err := agent.Stream(ctx, historyMessages)
defer streamResult.Close()

for {
    chunk, err := streamResult.Recv()
    if errors.Is(err, io.EOF) { break }
    if err != nil { /* push chat.error event */ break }

    // chunk 是 *schema.Message
    // chunk.Content != "" → LLM 正在输出 token → 推 chat.token 事件
    // chunk.ToolCalls != nil → LLM 决定调工具 → 推 chat.tool_call 事件
    // 工具执行后 chunk.Role == "tool" → 推 chat.tool_result 事件
}
// 推 chat.done 事件
```

### 3.4 ⚠️ Claude StreamToolCallChecker（关键陷阱）

Eino 默认的 StreamToolCallChecker 只检查第一个 streaming chunk 是否包含 tool call。**OpenAI 在第一个 chunk 就发 tool call，但 Claude 先输出文本再在末尾发 tool call**——用默认实现会导致 Claude 的 tool calling 完全失效。

```go
// 必须为 Anthropic 提供自定义实现
anthropicChecker := func(ctx context.Context, sr *schema.StreamReader[*schema.Message]) (bool, error) {
    // 读完所有 chunks，检查任意一个包含 ToolCalls
    defer sr.Close()
    for {
        msg, err := sr.Recv()
        if errors.Is(err, io.EOF) { return false, nil }
        if err != nil { return false, err }
        if len(msg.ToolCalls) > 0 { return true, nil }
    }
}
```

**每个 provider 的 StreamToolCallChecker 实现放在 `infra/eino/<provider>.go`。**

### 3.5 Callbacks：SSE 事件推送的接入点

Eino 的 Callback 系统在每个 Agent 步骤的 OnStart/OnEnd 处 hook，这是将 Agent 内部状态推送为 SSE 事件的最佳位置：

```go
handler := callbacks.HandlerBuilder[any]{}.
    OnStart(func(ctx context.Context, info *callbacks.RunInfo, input any) context.Context {
        if info.Component == callbacks.ComponentOfChatModel {
            // LLM 开始推理 → 推 chat.thinking 事件（如果模型支持）
        }
        if info.Component == callbacks.ComponentOfTool {
            // Tool 开始执行 → 推 chat.tool_call 事件（含 tool name + input）
            events.Bridge.Publish(ctx, ChatToolCallEvent{...})
        }
        return ctx
    }).
    OnEnd(func(ctx context.Context, info *callbacks.RunInfo, output any) context.Context {
        if info.Component == callbacks.ComponentOfTool {
            // Tool 执行完成 → 推 chat.tool_result 事件（含 result）
            events.Bridge.Publish(ctx, ChatToolResultEvent{...})
        }
        return ctx
    }).
    Build()
```

Token streaming（chat.token）直接在 Agent.Stream() 的 Recv() 循环里推送，不走 Callback。

---

## 4. Provider 抽象层（`infra/eino`）

### 4.1 为什么需要

不同 provider 用不同的 Eino 实现（openai-ext、anthropic-ext 等），且 key/baseURL 是运行时才知道的（从 apikey.ResolveCredentials 取）。需要一个工厂统一构建。

### 4.2 ModelFactory

```go
// infra/eino/factory.go

type ChatModelFactory interface {
    // Build 根据 provider + 运行时凭证构建 Eino ChatModel
    //
    // Build 根据 provider + 运行时凭证构建 Eino ChatModel。
    Build(ctx context.Context, cfg ModelConfig) (schema.ToolCallingChatModel, error)
}

type ModelConfig struct {
    Provider string
    ModelID  string
    Key      string
    BaseURL  string
}
```

### 4.3 Provider 映射

| Provider | Eino 实现 | 备注 |
|---|---|---|
| `openai` | `eino-ext/components/model/openai` | 原生 |
| `deepseek` / `qwen` / `zhipu` / `moonshot` / `doubao` / `openrouter` | 同 openai，传自定义 BaseURL | OpenAI 兼容 |
| `anthropic` | `eino-ext/components/model/anthropic`（或 openai-compat）| 需自定义 StreamToolCallChecker |
| `google` | `eino-ext/components/model/gemini` | |
| `ollama` | `eino-ext/components/model/ollama` | 本地 |
| `custom` | 按 APIFormat 选 openai 或 anthropic 实现 | |

```
infra/eino/
  factory.go    ← ChatModelFactory 接口 + 默认实现（dispatch by provider）
  openai.go     ← openai-compatible 实现（含 StreamToolCallChecker）
  anthropic.go  ← anthropic 实现（含自定义 StreamToolCallChecker + Prompt Cache）
  google.go     ← gemini 实现
  ollama.go     ← ollama 实现
```

### 4.4 ⚡ Anthropic Prompt Caching

Anthropic 支持将 System Prompt 缓存起来，后续调用节省最多 90% 的 input token 费用。在 `infra/eino/anthropic.go` 实现时加上 `cache_control`：

```go
// 构建请求时，system prompt 块加 cache_control
systemBlocks := []anthropic.TextBlockParam{
    {Type: "text", Text: baseSystemPrompt},
    {
        Type: "text",
        Text: conversationSystemPrompt,  // 对话级自定义 prompt
        CacheControl: &anthropic.CacheControlEphemeralParam{Type: "ephemeral"},
    },
}
```

缓存命中时 API 返回 `cache_read_input_tokens`，写入 `messages.token_usage.cacheReadTokens`，可用于展示实际费用节省。

---

## 5. 消息存储

### 5.1 messages 表（chat domain 所有）

```go
// domain/chat/chat.go

type Message struct {
    ID             string         `gorm:"primaryKey;type:text" json:"id"`
    ConversationID string         `gorm:"not null;index;type:text" json:"conversationId"`
    UserID         string         `gorm:"not null;type:text" json:"-"`
    Role           string         `gorm:"not null;type:text" json:"role"`
    Content        string         `gorm:"not null;type:text" json:"content"`
    Status         string         `gorm:"not null;type:text;default:'completed'" json:"status"`
    StopReason     string         `gorm:"type:text;default:''" json:"stopReason,omitempty"`
    TokenUsage     string         `gorm:"type:text;default:''" json:"tokenUsage,omitempty"`
    ToolCalls      string         `gorm:"type:text" json:"toolCalls,omitempty"`
    ToolCallID     string         `gorm:"type:text" json:"toolCallId,omitempty"`
    AttachmentIDs  string         `gorm:"type:text;default:''" json:"attachmentIds,omitempty"`
    CreatedAt      time.Time      `json:"createdAt"`
    DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Message) TableName() string { return "messages" }
```

**Role 值**：

| Role | 来源 | 说明 |
|---|---|---|
| `user` | 用户输入 | 文本 + 可选附件 |
| `assistant` | LLM 输出 | Content 是文字回复；ToolCalls 非空时是 tool call 指令 |
| `tool` | Tool 执行结果 | Content 是工具返回内容；ToolCallID 关联 assistant 消息 |

**Status 常量**：

```go
const (
    MessageStatusPending   = "pending"    // 已入队，等待 Agent 处理
    MessageStatusStreaming  = "streaming"  // 正在流式输出
    MessageStatusCompleted = "completed"  // 正常完成
    MessageStatusError     = "error"      // 出错
    MessageStatusCancelled = "cancelled"  // 用户取消
)
```

**StopReason 值**（assistant 消息）：

| 值 | 含义 |
|---|---|
| `end_turn` | 正常结束 |
| `max_tokens` | 达到 token 上限，回复被截断（前端展示"继续"按钮）|
| `cancelled` | 用户主动取消 |
| `error` | 出错中止 |

**TokenUsage JSON 结构**：
```json
{ "inputTokens": 1024, "outputTokens": 512, "cacheReadTokens": 800 }
```
`cacheReadTokens` 仅 Anthropic prompt cache 命中时非零（见 §4.4）。

### 5.2 chat_attachments 表

```go
type Attachment struct {
    ID          string    `gorm:"primaryKey;type:text" json:"id"`       // att_<16hex>
    UserID      string    `gorm:"not null;type:text" json:"-"`
    FileName    string    `gorm:"not null;type:text" json:"fileName"`
    MimeType    string    `gorm:"not null;type:text" json:"mimeType"`
    SizeBytes   int64     `gorm:"not null" json:"sizeBytes"`
    StoragePath string    `gorm:"not null;type:text" json:"-"`           // 相对 dataDir 的路径，不对外暴露
    CreatedAt   time.Time `json:"createdAt"`
}

func (Attachment) TableName() string { return "chat_attachments" }
```

文件写到 `{dataDir}/attachments/{att_id}/original.{ext}`，上传即复制，后端不持有用户原始路径。**大小限制 50MB**。

### 5.3 FTS5 全文搜索索引

在 `infra/db/schema_extras.go` 追加，对 messages.content 建 FTS5 虚拟表，支持未来"搜索对话历史"功能：

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts
  USING fts5(content, content='messages', content_rowid='rowid');

CREATE TRIGGER IF NOT EXISTS messages_fts_insert AFTER INSERT ON messages BEGIN
  INSERT INTO messages_fts(rowid, content) VALUES (new.rowid, new.content);
END;
CREATE TRIGGER IF NOT EXISTS messages_fts_update AFTER UPDATE ON messages BEGIN
  INSERT INTO messages_fts(messages_fts, rowid, content) VALUES ('delete', old.rowid, old.content);
  INSERT INTO messages_fts(rowid, content) VALUES (new.rowid, new.content);
END;
CREATE TRIGGER IF NOT EXISTS messages_fts_delete AFTER DELETE ON messages BEGIN
  INSERT INTO messages_fts(messages_fts, rowid, content) VALUES ('delete', old.rowid, old.content);
END;
```

### 5.4 消息加载策略

每次 Send 时，从 DB 加载当前对话的全部历史消息，转成 `[]*schema.Message` 传给 Agent。

**Phase 5 优化**：用 `MessageRewriter`（Eino AgentConfig 的可选项）做上下文压缩，超长对话只保留关键消息，避免 context 超限。当前 Phase 2 全量加载。

---

## 6. 附件与多模态支持

### 6.1 上传流程

前端（文件选择框或剪贴板粘贴）将文件字节以 multipart 形式发给后端，后端同步写盘并返回 `attachment_id`。后端**不持有用户原始路径**，剪贴板和文件选择框对后端透明。

```
POST /api/v1/attachments (multipart/form-data)
→ 写到 {dataDir}/attachments/{att_id}/original.{ext}
→ 201 { id, fileName, mimeType, sizeBytes }
```

### 6.2 内容提取器（可插拔）

发给 Agent 前，按 mimeType 路由：

```go
type ContentExtractor interface {
    Supports(mimeType string) bool
    // Extract 返回文本内容；图片类型返回 ("", nil) 走 Vision 路径
    Extract(storagePath string) (string, error)
}
```

| 提取器 | 覆盖类型 | 实现 | Phase |
|---|---|---|---|
| `ImageExtractor` | `image/*` | 不提取，走 Vision | 2 |
| `PlainTextExtractor` | `text/*` / `.json` / `.csv` / `.py` 等 | `os.ReadFile` | 2 |
| `PDFExtractor` | `application/pdf` | `pdfcpu`（纯 Go，Apache 2.0）| 2 |
| `DocExtractor` | `.docx` / `.odt` / `.rtf` | `lu4p/cat`（纯 Go）| 3 |
| `FallbackExtractor` | 其他 | 返回 `ATTACHMENT_TYPE_UNSUPPORTED` | - |

### 6.3 Eino 消息组装

```
图片附件
  provider.SupportsVision == true  → UserInputMultiContent 加 image part（base64）
  provider.SupportsVision == false → push chat.error（VISION_NOT_SUPPORTED），跳过该附件

文本类（提取成功）
  → 追加到 Content 末尾：
    [附件: report.pdf]
    {提取的文本内容}

提取失败
  → push chat.error（ATTACHMENT_PARSE_FAILED），跳过该附件，其余正常发送
```

`ProviderMeta`（`app/apikey/providers.go`）新增 `SupportsVision bool` 字段。

Vision 支持：`openai`（gpt-4o）✅ / `anthropic`（claude-3+）✅ / `google`（gemini）✅ / `deepseek` ❌ / 其余按具体模型。

---

## 7. SSE 事件设计

### 6.1 传输机制

```
前端                            后端
 │                               │
 ├──GET /api/v1/events?convId=───→│  建立 SSE 长连接，订阅 bridge
 │                               │
 ├──POST /conversations/{id}/───→│  202 Accepted（非阻塞）
 │       messages                │  → 异步跑 Agent
 │                               │  → Bridge.Publish(chat.token)
 │←────── event: chat.token ─────┤  → Bridge.Publish(chat.token)
 │←────── event: chat.token ─────┤  → Bridge.Publish(chat.tool_call)
 │←── event: chat.tool_call ─────┤  → Bridge.Publish(chat.tool_result)
 │←── event: chat.tool_result ───┤  → Bridge.Publish(chat.done)
 │←────── event: chat.done ──────┤
```

`/api/v1/events` 的 SSE handler 过滤 `conversationId`，只推对应对话的事件。

**SSE 可靠性**：

两个必要机制确保网络抖动不丢事件：

1. **Keep-alive ping**：服务端每 15 秒发一条 SSE 注释行，防止代理/浏览器因超时断连：
```
: keep-alive
```

2. **Last-Event-ID**：每个 SSE 事件携带自增 ID，客户端断连重连时在请求头带 `Last-Event-ID`，服务端从该 ID 之后续传。Events Bridge 需要为每个 conversationID 维护一个短暂的事件缓冲（内存，最多保留最近 100 条，TTL 5 分钟），断连重连时可以补发。

```
id: 42
event: chat.token
data: {"conversationId":"cv_xxx","delta":"正在"}

id: 43
event: chat.token
data: {"conversationId":"cv_xxx","delta":"创建..."}
```

### 6.2 事件类型（均定义在 `domain/events/types.go`）

```go
// chat.token — 流式 token 增量
type ChatTokenEvent struct {
    ConversationID string `json:"conversationId"`
    MessageID      string `json:"messageId"`     // 当前 assistant 消息 id
    Delta          string `json:"delta"`          // 增量文本
}

// chat.tool_call — Agent 决定调用某个 Tool
type ChatToolCallEvent struct {
    ConversationID string `json:"conversationId"`
    MessageID      string `json:"messageId"`
    ToolCallID     string `json:"toolCallId"`
    ToolName       string `json:"toolName"`
    ToolInput      string `json:"toolInput"`      // JSON string
}

// chat.tool_result — Tool 执行完成
type ChatToolResultEvent struct {
    ConversationID string `json:"conversationId"`
    ToolCallID     string `json:"toolCallId"`
    Result         string `json:"result"`
    OK             bool   `json:"ok"`
}

// chat.done — Agent 完成，含最终 assistant 消息
type ChatDoneEvent struct {
    ConversationID string `json:"conversationId"`
    MessageID      string `json:"messageId"`
}

// chat.error — Agent 出错
type ChatErrorEvent struct {
    ConversationID string `json:"conversationId"`
    Code           string `json:"code"`
    Message        string `json:"message"`
}
```

---

## 7. HTTP API

### 8.1 端点

| Method | Path | 用途 | 状态码 |
|---|---|---|---|
| `POST` | `/api/v1/attachments` | 上传附件（multipart）| 201 |
| `POST` | `/api/v1/conversations/{id}/messages` | 发送消息，触发 Agent | 202 |
| `DELETE` | `/api/v1/conversations/{id}/stream` | 取消正在运行的 Agent | 204 |
| `GET` | `/api/v1/conversations/{id}/messages` | 列出消息历史（cursor 分页）| 200 |
| `GET` | `/api/v1/events` | SSE 事件流（`?conversationId=xxx`）| 200 |

### 8.2 POST /api/v1/attachments

**Request**：`multipart/form-data`，字段名 `file`。

**Response 201**：
```json
{ "data": { "id": "att_xxx", "fileName": "photo.jpg", "mimeType": "image/jpeg", "sizeBytes": 204800 } }
```

**限制**：50MB，超过返回 `ATTACHMENT_TOO_LARGE 413`。

### 8.3 DELETE /api/v1/conversations/{id}/stream — 取消生成（204）

取消该对话正在运行的 Agent。后端调用 `running.Load(conversationID)` 拿到 cancel func 并执行，Agent goroutine 退出，最终消息 status 写 `cancelled`，推 `chat.done` 事件通知前端。

**404 `STREAM_NOT_FOUND`**：该对话当前没有 Agent 在运行。

### 8.4 POST /conversations/{id}/messages

**Request**：
```json
{ "content": "帮我做一个处理 CSV 的工具", "attachmentIds": ["att_xxx"] }
```
`attachmentIds` 可省略或为空数组，支持多个附件。

**Response 202**：
```json
{ "data": { "messageId": "msg_xxx" } }
```

立刻返回，Agent 在后台异步运行，通过 SSE 推进度。

**错误**：
- 404 `CONVERSATION_NOT_FOUND`：对话不存在
- 409 `STREAM_IN_PROGRESS`：该对话已有 Agent 在运行
- 422 `MODEL_NOT_CONFIGURED`：用户未配置模型
- 404 `API_KEY_PROVIDER_NOT_FOUND`：无可用 key

### 7.3 GET /conversations/{id}/messages

**Query**：`?cursor=&limit=50`

**Response 200**：分页列表，含 user + assistant + tool 三种 role 的消息。

### 7.4 GET /api/v1/events（SSE）

```
GET /api/v1/events?conversationId=cv_xxx
Accept: text/event-stream
```

长连接，服务端推：
```
event: chat.token
data: {"conversationId":"cv_xxx","messageId":"msg_yyy","delta":"正在"}

event: chat.token
data: {"conversationId":"cv_xxx","messageId":"msg_yyy","delta":"创建..."}

event: chat.tool_call
data: {"conversationId":"cv_xxx","toolName":"create_tool","toolInput":"{...}"}

event: chat.done
data: {"conversationId":"cv_xxx","messageId":"msg_yyy"}
```

---

## 8. Service 设计

### 8.1 Struct

```go
// app/chat/chat.go

type Service struct {
    repo         domain.Repository       // 消息存储
    modelPicker  modeldomain.ModelPicker // 拿 (provider, modelID)
    keyProvider  apikeydomain.KeyProvider // 拿 (key, baseURL)
    modelFactory eino.ChatModelFactory   // 构建 Eino ChatModel
    tools        []tool.BaseTool         // System Tools（Phase 2: nil）
    events       eventsdomain.Bridge     // 推 SSE 事件
    log          *zap.Logger
}
```

### 8.2 Send 流程（Phase 2）

```
1. repo.Get(conversationID) → 验证对话存在
2. 检查是否有 stream 在运行 → 409 STREAM_IN_PROGRESS
3. 保存 user message → DB
4. goroutine 异步运行 Agent：
   a. model.PickForChat(ctx) → (provider, modelID)
   b. apikey.ResolveCredentials(ctx, provider) → (key, baseURL)
   c. modelFactory.Build(ctx, ModelConfig{...}) → ToolCallingChatModel
   d. 加载 history messages → []schema.Message
   e. 构建 react.Agent（tools 为 nil）
   f. agent.Stream(ctx, messages)
   g. 循环 Recv()：
      - chunk.Content → events.Publish(ChatTokenEvent)
      - 工具事件走 Callbacks
      - EOF → 保存 assistant message → events.Publish(ChatDoneEvent)
   h. 出错 → events.Publish(ChatErrorEvent)
5. 返回 202 {messageId}
```

### 8.3 并发控制与取消

每个 conversationID 最多一个 Agent 在跑。用 `sync.Map` 存运行中的 conversationID → cancel func。

```go
type Service struct {
    ...
    running sync.Map // conversationID → context.CancelFunc
}
```

- Send：已有 → 409；没有 → 注册 cancel → 跑完 → 清除
- Cancel：`running.Load(id)` → 调 cancelFunc → Assistant message status 写 `cancelled` → 推 `chat.done`

### 8.4 System Prompt 组装

每次调用 Agent 前，MessageModifier 按以下优先级组装 System Prompt：

```
[基础系统提示词（代码写死）]
+
[conversation.system_prompt（用户自定义，可为空）]
+
[locale 指令（从 reqctx 读）]
```

`conversation.system_prompt` 字段存在 `conversations` 表（由 conversation domain 管理），chat.Service 通过 `conversationRepo.Get(id)` 读取。

### 8.5 自动命名（Auto-Titling）

第一轮对话完成后（assistant 消息 status=completed），异步起一个 goroutine 调轻量模型生成标题：

```
条件：conversation.title == "" AND conversation.auto_titled == false
  → 调 modelFactory.Build（使用同 provider 的轻量模型，如 haiku / gpt-4o-mini）
  → System: "生成一个 5 字以内的对话标题，只返回标题本身"
  → Input: 前两条消息（user + assistant）
  → 写回 conversations.title + conversations.auto_titled = true
  → 推 conversation.title_updated SSE 事件
```

**非阻塞**：标题生成失败静默忽略，不影响主流程。`conversations` 表需新增 `auto_titled BOOLEAN NOT NULL DEFAULT false` 字段。

---

## 9. 各 Phase 的演化

### Phase 2（当前要做的）

**实现**：
- domain/chat：Message entity + ErrNotFound + ErrStreamInProgress + Repository
- infra/store/chat：messages 表的 CRUD
- infra/eino：ChatModelFactory + openai.go（含 openai-compatible 的所有 provider）
- app/chat：Service（Send + ListMessages），tools = nil
- handlers/chat：POST + GET messages + GET events（SSE handler）
- 配套：events/types.go 加 5 个 chat 事件 struct

**Agent 行为**：没有工具，LLM 只会直接回复文字，流式推 chat.token。

### Phase 3（工具锻造完成后）

**新增 System Tools**：
```go
tools = []tool.BaseTool{
    NewCreateToolTool(forgeSvc, toolSvc),
    NewEditToolTool(toolSvc),
    NewRunToolTool(toolSvc),
    NewSearchToolsTool(toolSvc),
}
```

`main.go` 把这些 tool 注入 chat.Service 即可，Service 本身代码零改动。

**chat domain 新增**：历史消息里会出现 role=tool 的消息，存储和展示需支持。

### Phase 4（工作流完成后）

**新增 System Tools**：
```go
tools = append(tools,
    NewCreateWorkflowTool(workflowSvc),
    NewEditWorkflowTool(workflowSvc),
    NewRunWorkflowTool(flowrunSvc),
)
```

**Agent 能力**：现在可以"对话中创建工作流"——LLM 先设计节点结构，再调 create_workflow，用户实时看到。

### Phase 5（智能化完成后）

**新增 System Tools**：
```go
tools = append(tools,
    NewSearchKnowledgeTool(knowledgeSvc),
    NewMCPCallTool(mcpSvc),
)
```

**新增 MessageModifier 升级**：System Prompt 加入意图引导（"你可以创建工具/工作流/搜知识库，根据用户需求自主决策"）。

**新增 MessageRewriter**：对话超长时自动压缩 context，保留关键消息。

---

## 10. 完整调用链（Phase 2）

### 10.1 用户发消息

```
POST /api/v1/conversations/cv_xxx/messages  body={content}
  → middleware 链
  → ChatHandler.Send
      → 验证对话存在（repo.Get）
      → 检查并发（running.Load）
      → 保存 user message（repo.SaveMessage）
      → goroutine：
          → model.PickForChat(ctx) → ("openai", "gpt-4o")
          → apikey.ResolveCredentials(ctx, "openai") → (key, baseURL)
          → modelFactory.Build(ctx, {openai, gpt-4o, key, baseURL}) → ChatModel
          → repo.LoadHistory(conversationID) → []schema.Message
          → react.NewAgent(ctx, {ChatModel, tools=nil, MessageModifier, MaxStep=20})
          → agent.Stream(ctx, messages)
              → LLM 输出 token:
                  Recv() chunk.Content → Bridge.Publish(ChatTokenEvent)
                  Recv() chunk.Content → Bridge.Publish(ChatTokenEvent)
                  ...
                  Recv() EOF
              → 保存 assistant message
              → Bridge.Publish(ChatDoneEvent)
      → response 202 {messageId}
```

### 10.2 前端收事件

```
GET /api/v1/events?conversationId=cv_xxx
  → SSEHandler
      → Bridge.Subscribe(filter={conversationId: cv_xxx})
      → 持续 write SSE:
          event: chat.token  data: {...}
          event: chat.token  data: {...}
          event: chat.done   data: {...}
```

---

## 11. 错误码

| Code | HTTP | Sentinel | 场景 |
|---|---|---|---|
| `STREAM_NOT_FOUND` | 404 | `chat.ErrStreamNotFound` | 取消不存在的流 |
| `STREAM_IN_PROGRESS` | 409 | `chat.ErrStreamInProgress` | 同一对话已有 Agent 在运行 |
| `LLM_PROVIDER_ERROR` | 502 | `chat.ErrProviderUnavailable` | 上游 LLM 故障（非 401）|
| `ATTACHMENT_TOO_LARGE` | 413 | `chat.ErrAttachmentTooLarge` | 附件超过 50MB |
| `ATTACHMENT_TYPE_UNSUPPORTED` | 415 | `chat.ErrAttachmentTypeUnsupported` | 无法处理的文件格式 |
| `ATTACHMENT_PARSE_FAILED` | 422 | `chat.ErrAttachmentParseFailed` | 文件损坏或解析失败 |
| `VISION_NOT_SUPPORTED` | 422 | `chat.ErrVisionNotSupported` | 当前 provider 不支持图片 |

**401 路径**：LLM 返回 401 → `apikey.MarkInvalid` → 推 `chat.error` 事件（code: `API_KEY_INVALID`）→ Service 返回 `apikey.ErrInvalid` → errmap 翻译 → 前端 SSE 收到。

---

## 12. 为什么这样设计（关键决策总结）

| 决策 | 选择 | 理由 |
|---|---|---|
| 用 ReAct Agent 还是固定 Graph | **ReAct Agent** | 任务序列是运行时 LLM 决定的，不能提前写死；Phase 2-5 的工具列表动态增长 |
| tools 全部注入 vs Tool RAG | **System Tools 注入 + Tool RAG** | System Tools 数量固定（~8个）可全注入；用户工具无上限，靠 search_tools 动态检索 |
| 202 + SSE vs 直接 stream response | **202 + 独立 SSE** | Agent 跑多步需要持久连接；POST 语义是"接受请求"不是"等待结果"；events Bridge 已就绪 |
| messages 在哪存 | **chat domain 自己管** | 消息历史是 chat 专有数据，不应跨 domain 共享；conversation domain 只管线程元数据 |
| Claude StreamToolCallChecker | **各 provider 自定义** | Claude 在 stream 末尾才发 tool call，Eino 默认只检查第一个 chunk，不自定义直接失效 |
| System Prompt 的 locale | **MessageModifier 注入** | 每次 ChatModel 调用前动态注入，locale 从 reqctx 读，Agent 不需要知道 locale 逻辑 |
| Message status | **message 级别字段** | 流式过程中消息状态需持久化；失败/取消场景前端需要准确知道每条消息的最终态 |
| SSE 可靠性 | **keep-alive ping + Last-Event-ID + 短暂缓冲** | 网络抖动断连不丢事件；桌面应用场景常见 |
| Auto-titling | **异步 goroutine，失败静默** | 标题生成不是核心流程；用轻量模型节省费用；失败不影响用户体验 |
| Prompt Caching | **Anthropic 专属，anthropic.go 实现** | 节省最多 90% system prompt token 费用；其他 provider 暂无此能力 |

---

## 13. 实现清单（Phase 2，代码待写）

### infra/eino 层
- [ ] `infra/eino/factory.go` — ChatModelFactory 接口 + dispatch by provider
- [ ] `infra/eino/openai.go` — OpenAI + 所有 OpenAI-compatible provider（含 StreamToolCallChecker）
- [ ] `infra/eino/anthropic.go` — Anthropic 实现 + 自定义 StreamToolCallChecker + **Prompt Cache**（cache_control）
- [ ] `infra/eino/google.go` — Gemini 实现
- [ ] `infra/eino/ollama.go` — Ollama 实现

### domain/chat 层
- [ ] `domain/chat/chat.go` — Message entity（含 Status / StopReason / TokenUsage / AttachmentIDs）+ Attachment entity + Status 常量 + 8 sentinel + Repository
- [ ] `domain/events/types.go` — 追加 5 个 chat 事件 struct（含 `conversation.title_updated`）

### infra/db 层
- [ ] `infra/db/schema_extras.go` — 追加 messages_fts FTS5 虚拟表 + 三个触发器（insert/update/delete）

### infra/store/chat 层
- [ ] `infra/store/chat/chat.go` — Store（SaveMessage / UpdateMessageStatus / LoadHistory / GetMessage / SaveAttachment / GetAttachment）
- [ ] `infra/store/chat/chat_test.go` — 集成测试

### infra/chat 层
- [ ] `infra/chat/extractor.go` — ContentExtractor 接口 + ImageExtractor + PlainTextExtractor + PDFExtractor（pdfcpu）+ FallbackExtractor

### app/chat 层
- [ ] `app/chat/chat.go` — Service（Send / Cancel / ListMessages / UploadAttachment + 并发控制 + Agent 构建 + 附件组装 + **auto-titling goroutine** + system_prompt 组装）
- [ ] `app/chat/system_tools.go` — System Tool 接口定义（Phase 2: 空实现占位）
- [ ] `app/chat/chat_test.go` — 单测

### transport 层
- [ ] `handlers/chat.go` — ChatHandler（POST attachments / POST messages / **DELETE stream** / GET messages / GET events SSE + **keep-alive ping + Last-Event-ID**）
- [ ] `handlers/chat_test.go` — E2E 测试

### conversation domain 联动
- [ ] `domain/conversation/conversation.go` — Conversation 加 `SystemPrompt string` + `AutoTitled bool` 字段
- [ ] `infra/store/conversation/conversation.go` — UpdateTitle 方法（auto-titling 回写）

### 配套
- [ ] `errmap.go` — 8 条 chat sentinel 映射
- [ ] `router/deps.go` — ChatService 字段
- [ ] `main.go` — 装配 + db.Migrate(&chat.Message{}, &chat.Attachment{})
