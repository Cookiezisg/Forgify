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

## 3. LLM 客户端层（`infra/llm`）

> **Eino 已完全移除**（2026-04-27）。chat 管线使用完全自有的 LLM 流式客户端，
> 零框架依赖，完全掌控 SSE 解析和请求构建。

### 3.1 核心组件

```
chat.Service
    ↓ 依赖
llminfra.Factory          ← 按 provider dispatch，返回 Client
    ↓ Build(Config)
llminfra.Client           ← 唯一方法：Stream(ctx, Request) iter.Seq[StreamEvent]
    ├── openAIClient      ← 覆盖 OpenAI/DeepSeek/Qwen/Moonshot/Ollama 等 OpenAI-compat
    └── anthropicClient   ← Anthropic 原生 /v1/messages 协议
```

### 3.2 核心类型（`infra/llm/llm.go`）

```go
// StreamEvent 是 LLM 流式响应中一个带类型标签的事件
type StreamEvent struct {
    Type           StreamEventType
    Delta          string   // EventText: 文字增量
    ReasoningDelta string   // EventReasoning: 推理增量（DeepSeek-R1 等）
    ToolIndex      int      // EventToolStart / EventToolDelta
    ToolID         string   // EventToolStart: LLM 分配的 tool call id
    ToolName       string   // EventToolStart
    ArgsDelta      string   // EventToolDelta: arguments 片段
    FinishReason   string   // EventFinish
    InputTokens    int      // EventFinish
    OutputTokens   int      // EventFinish
    Err            error    // EventError
}

type StreamEventType string
const (
    EventText      StreamEventType = "text"
    EventReasoning StreamEventType = "reasoning"
    EventToolStart StreamEventType = "tool_start"  // tool name 已知，立刻可推 SSE
    EventToolDelta StreamEventType = "tool_delta"  // arguments 片段
    EventFinish    StreamEventType = "finish"
    EventError     StreamEventType = "error"
)

// Client 是唯一的 LLM 流式接口
type Client interface {
    Stream(ctx context.Context, req Request) iter.Seq[StreamEvent]
}

type Request struct {
    ModelID  string
    Key      string
    BaseURL  string
    System   string
    Messages []LLMMessage
    Tools    []ToolDef
}
```

**设计关键**：
- `iter.Seq[StreamEvent]` 替代 channel：拉式迭代，无 goroutine 泄漏，break 干净退出
- `EventToolStart` 在 tool name 首次出现时立刻 emit，不等 arguments 完整（让前端尽快展示"正在调用 X…"）
- `Generate()` helper 消费 Stream 实现非流式调用，不引入独立接口

### 3.3 OpenAI 兼容客户端（`infra/llm/openai.go`）

覆盖所有 OpenAI-compat provider：openai / deepseek / qwen / moonshot / doubao / openrouter / ollama 等。

- 自写 SSE line reader（`data: {...}\n\n` 格式）
- 解析 delta chunks：`choices[0].delta.content` / `reasoning_content`（DeepSeek-R1）/ `tool_calls`
- `classifyHTTPError` 区分 401/429/400/404/5xx 返回对应 Go error
- 畸形 chunk → emit EventError，不 panic

### 3.4 Anthropic 原生客户端（`infra/llm/anthropic.go`）

使用 Anthropic 原生 `/v1/messages` 协议（SSE 格式）：
- `content_block_start` → 识别 text / tool_use block
- `content_block_delta` → 分发 EventText / EventToolDelta
- `content_block_stop` → 关闭当前 block
- tool result 消息格式与 OpenAI 不同：按 Anthropic 协议将 tool results 合并为一条 `role="user"` 消息（`content = [{type:"tool_result", tool_use_id, content}...]`）

### 3.5 Factory（`infra/llm/factory.go`）

```go
// Factory.Build 按 provider 返回对应 Client
func (f *Factory) Build(cfg Config) (Client, string, error) {
    // anthropic → anthropicClient{baseURL}
    // 其余全部 → openAIClient{baseURL}（含 ollama 等）
}
```

Provider 基础 URL 由 `resolveBaseURL` 按 provider 名称给出，调用方传入的 `BaseURL` 会覆盖默认值。

---

## 4. Tool 接口 & summary 注入（`app/agent/tool.go`）

### 4.1 Tool 接口

```go
// Tool 是每个 system tool 必须实现的接口
type Tool interface {
    Name()        string
    Description() string
    Parameters()  json.RawMessage  // JSON Schema object，禁止包含 "summary" 字段
    Execute(ctx context.Context, argsJSON string) (string, error)
}
```

`Parameters()` 返回的 JSON Schema **不含** `summary` 字段，由框架在 `ToLLMDef` 时自动注入。

### 4.2 Summary 注入机制

```
ToLLMDef(tool)
  → injectSummaryField(tool.Parameters())
    → 在 properties 里加入 "summary": {type: string, description: "One sentence..."}
    → 在 required 里把 "summary" 插到第一位（引导 LLM 优先输出）
  → 返回发给 LLM 的 ToolDef（含 summary 字段）

runOneTool(ctx, tc)
  → StripSummary(tc.Arguments)
    → 提取 summary 字段值 → 发 ChatToolCall SSE
    → 返回去掉 summary 后的 argsJSON
  → tool.Execute(ctx, strippedArgsJSON)   ← Execute 永不看到 summary
```

**为什么**：summary 是给前端展示的人类可读摘要（如 `"$ git status"`），不是工具的真实入参。让 LLM 输出 summary 比 tool 自己生成更准确，也无需为每个 tool 写摘要逻辑。

### 4.3 Context Helpers

```go
// 6 个 context helpers，供 pipeline 注入、forge tools 读取
func WithConversationID(ctx, id) context.Context
func GetConversationID(ctx) (string, bool)

func WithMessageID(ctx, id) context.Context
func GetMessageID(ctx) (string, bool)

func WithToolCallID(ctx, id) context.Context
func GetToolCallID(ctx) (string, bool)
```

`runOneTool`（tools.go）在调用 `tool.Execute` 前注入 msgID 和 toolCallID，`forge.go` 中的 `streamCode` / `CreateTool.Execute` / `EditTool.Execute` 读取并填充 `ToolCodeStreaming` / `ToolCreated` / `ToolPendingCreated` SSE 事件的对应字段。

### 4.4 System Tools 完整目录

| Tool | 实现文件 | Phase | 描述 |
|---|---|---|---|
| `datetime` | system.go | 2+ | 当前日期时间 |
| `read_file` | system.go | 2+ | 读本地文件 |
| `write_file` | system.go | 2+ | 写本地文件 |
| `list_dir` | system.go | 2+ | 列目录 |
| `run_shell` | system.go | 2+ | 执行 shell 命令 |
| `run_python` | system.go | 2+ | 执行 Python 代码 |
| `web_search` | web.go | 2+ | DuckDuckGo Lite 搜索（POST 表单）|
| `fetch_url` | web.go | 2+ | Jina Reader 抓取 URL 为 Markdown |
| `search_tools` | forge.go | 3+ | 语义搜索用户工具库 |
| `get_tool` | forge.go | 3+ | 获取工具完整代码 |
| `create_tool` | forge.go | 3+ | LLM 生成代码 + 保存工具 |
| `edit_tool` | forge.go | 3+ | LLM 改写代码 + 创建 pending |
| `run_tool` | forge.go | 3+ | 运行用户工具（沙箱） |

---

## 5. Pipeline 架构（`app/chat/`）

### 5.1 文件结构

```
app/chat/
  chat.go     ← 公开 API（Send / Cancel / ListMessages / UploadAttachment）+ 队列管理
  pipeline.go ← processTask / runReactLoop / runStep / persistMsg / finalPersist
  stream.go   ← consumeStream（iter.Seq 驱动）+ assembleAssistantBlocks
  tools.go    ← executeToolCalls（并行）+ runOneTool + executeTool
  history.go  ← buildLLMHistory + blocksToAssistantLLM + buildUserLLMMessage
  util.go     ← ID 生成器 + readAndEncode + truncate
```

### 5.2 ReAct Loop（`runReactLoop`）

```
Send(userMsg) → 保存 user message → 入队 queuedTask{userMsgID}
  ↓ worker goroutine
processTask
  → buildLLMHistory(ctx, convID, userMsgID)   // 加载历史，userMsgID 末尾追加
  → for step < maxSteps:
      runStep(ctx, client, req, convID, assistantMsgID)
        → consumeStream(stream)                 // iter.Seq 驱动
            → 遇到 EventToolStart → Publish(ChatToolCallStart)
            → 遇到 EventText → Publish(ChatToken)
            → 遇到 EventReasoning → Publish(ChatReasoningToken)
            → 遇到 EventFinish → 记录 usage
            → 遇到 EventError → return error
        → assembleAssistantBlocks()             // 组装当前步 blocks
        → executeToolCalls(toolCalls)           // 并行执行所有 tool
            → foreach tool: Publish(ChatToolCall) → Execute → Publish(ChatToolResult)
        → 返回 stepBlocks（含 tool_result）

      allBlocks = append(allBlocks, stepBlocks)
      if no tool calls → finalPersist(completed) → break
      else → persistMsg(streaming)              // 中间态，buildLLMHistory 会跳过

  if maxSteps reached → finalPersist(max_tokens)
  → Publish(ChatDone)
  → auto-title goroutine（conv.Title 为空时）
```

**关键设计**：
- **allBlocks 累积**：所有步骤的 blocks 全部累积进一个 slice，最终一次性写入同一条 assistant 消息。一次用户发言对应一条完整的 DB 记录，工具调用链不丢失。
- **中间步 streaming**：中间步写 `status=streaming`，`buildLLMHistory` 自动跳过 streaming/pending 状态的消息，避免把未完成的步骤放进历史重建。

### 5.3 consumeStream（`stream.go`）

```go
// iter.Seq 拉式迭代：只要 for range 不 break，就一直消费
for event := range client.Stream(ctx, req) {
    switch event.Type {
    case EventToolStart:
        bridge.Publish(ChatToolCallStart{ToolCallID: event.ToolID, ToolName: event.ToolName})
        accum[event.ToolIndex] = newAccum(event.ToolID, event.ToolName)
    case EventToolDelta:
        accum[event.ToolIndex].args.WriteString(event.ArgsDelta)
    case EventText:
        textBuf.WriteString(event.Delta)
        bridge.Publish(ChatToken{Delta: event.Delta})
    case EventReasoning:
        reasonBuf.WriteString(event.ReasoningDelta)
        bridge.Publish(ChatReasoningToken{Delta: event.ReasoningDelta})
    case EventFinish:
        usage = {event.InputTokens, event.OutputTokens}
    case EventError:
        return nil, event.Err
    }
}
```

`assembleAssistantBlocks` 把 buffers 组装为 blocks：顺序为 reasoning block → tool_call blocks（按 index 排） → text block。

### 5.4 并行 Tool Call（`tools.go`）

```go
func (s *Service) executeToolCalls(ctx, calls, convID, msgID) []chatdomain.Block {
    ch := make(chan result, len(calls))
    var wg sync.WaitGroup
    for i, call := range calls {
        wg.Add(1)
        go func(idx int, tc ToolCallData) {
            defer wg.Done()
            ch <- result{idx, s.runOneTool(ctx, tc, convID, msgID, idx)}
        }(i, call)
    }
    wg.Wait()
    close(ch)
    // 还原原始 index 顺序，保证 block seq 确定
    blocks := make([]Block, len(calls))
    for r := range ch { blocks[r.idx] = r.block }
    return blocks
}
```

`runOneTool` 在调用 `executeTool` 前注入 `WithMessageID` / `WithToolCallID` 到 ctx，供 forge.go 中的工具读取并填充 SSE 事件字段。

### 5.5 finalPersist 与取消安全

```go
func (s *Service) finalPersist(ctx, msgID, convID, uid, blocks, status, ...) {
    // 关键：取消时 ctx 已 cancelled，用全新 context 确保终态 DB 写入不失败
    saveCtx := reqctx.SetUserID(context.Background(), uid)
    if err := s.persistMsg(saveCtx, ...); err != nil {
        // 写失败：推 chat.error，记 CRITICAL log
    }
}
```

取消流程：Cancel() → context cancelled → consumeStream break → runStep 返回已有 blocks → finalPersist(status=cancelled)。终态必然落库。

---

## 6. 消息存储（Block 模型）

### 6.1 messages 表（精简为纯元数据）

```go
type Message struct {
    ID             string         `gorm:"primaryKey;type:text" json:"id"`
    ConversationID string         `gorm:"not null;index;type:text" json:"conversationId"`
    UserID         string         `gorm:"not null;type:text" json:"-"`
    Role           Role           `gorm:"not null;type:text" json:"role"`  // user | assistant
    Status         Status         `gorm:"not null;type:text" json:"status"`
    StopReason     string         `gorm:"type:text" json:"stopReason,omitempty"`
    InputTokens    int            `json:"inputTokens,omitempty"`
    OutputTokens   int            `json:"outputTokens,omitempty"`
    Blocks         []Block        `gorm:"-" json:"blocks"`  // 查询时填充，不存这列
    CreatedAt      time.Time      `json:"createdAt"`
    DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}
```

**已移除**：`Content`、`ReasoningContent`、`ToolCalls`、`ToolCallID`、`AttachmentIDs`、`TokenUsage`（全部转为 `message_blocks`）。

**Role 值**：`user` | `assistant`（`tool` 角色已移除，tool result 变为 assistant 消息内的 block）。

**Status 常量**：`pending` | `streaming` | `completed` | `error` | `cancelled`

**StopReason**：`end_turn` | `max_tokens` | `cancelled` | `error`

### 6.2 message_blocks 表（新增，存所有内容）

```go
type Block struct {
    ID        string    `gorm:"primaryKey;type:text" json:"id"`   // blk_<16hex>
    MessageID string    `gorm:"not null;index;type:text" json:"-"`
    Seq       int       `gorm:"not null" json:"seq"`               // 消息内排序
    Type      BlockType `gorm:"not null;type:text" json:"type"`
    Data      string    `gorm:"not null;type:text" json:"data"`    // JSON，结构随 type
    CreatedAt time.Time `json:"createdAt"`
}
```

**Block 类型 & data 结构**：

| Type | data JSON 结构 | 说明 |
|---|---|---|
| `text` | `{"text":"..."}` | 普通文字（user 输入或 assistant 回复）|
| `reasoning` | `{"text":"..."}` | 推理型模型的 thinking 内容 |
| `tool_call` | `{"id":"call_xxx","name":"datetime","summary":"获取时间","arguments":{...}}` | LLM 决定调用某工具 |
| `tool_result` | `{"toolCallId":"call_xxx","ok":true,"result":"..."}` | 工具执行结果 |
| `attachment_ref` | `{"attachmentId":"att_xxx","fileName":"report.pdf","mimeType":"application/pdf"}` | 附件引用 |

### 6.3 chatstore.Save 的 ON CONFLICT 保护

```go
// infra/store/chat/chat.go
tx.Clauses(clause.OnConflict{
    Columns:   []clause.Column{{Name: "id"}},
    DoUpdates: clause.AssignmentColumns([]string{
        "status", "stop_reason", "input_tokens", "output_tokens",
    }),
}).Create(m)
```

`created_at` **不在** DoUpdates 列表里，保证首次 INSERT 写入的时间戳在后续 status 更新时不被覆盖。这解决了 GORM `Save()` upsert 会把零值 `created_at` 写回 DB 的问题。

### 6.4 chat_attachments 表

```go
type Attachment struct {
    ID          string    `gorm:"primaryKey;type:text" json:"id"`       // att_<16hex>
    UserID      string    `gorm:"not null;type:text" json:"-"`
    FileName    string    `gorm:"not null;type:text" json:"fileName"`
    MimeType    string    `gorm:"not null;type:text" json:"mimeType"`
    SizeBytes   int64     `gorm:"not null" json:"sizeBytes"`
    StoragePath string    `gorm:"not null;type:text" json:"-"`  // 不对外暴露
    CreatedAt   time.Time `json:"createdAt"`
}
```

文件存 `{dataDir}/attachments/{att_id}/original.{ext}`，50MB 限制。

### 6.5 历史重建（`history.go`）

#### buildLLMHistory

```go
func (s *Service) buildLLMHistory(ctx, conversationID, currentUserMsgID string) ([]LLMMessage, error)
```

扫描所有非 streaming/pending 消息，跳过 `currentUserMsgID`，末尾显式追加当前用户消息。

**为什么要追加末尾**：同一对话快速连发两条消息时，第二条 user 消息的 `created_at` 可能早于第一条 assistant 回复（队列中并发写入），导致历史排序错乱，LLM 看到 `[user1, user2(current), assistant1]` 末尾是 assistant、无法确定回复对象。

#### blocksToAssistantLLM

把一条 assistant 消息的 blocks 转为 OpenAI wire 格式：

```
[assistant{content, toolCalls, reasoningContent}] + [N × role=tool messages]
```

tool_call blocks → `assistant.ToolCalls[]`；tool_result blocks → 独立的 `role="tool"` 消息（OpenAI 协议要求）。

---

## 7. 附件与多模态支持

### 7.1 上传流程

```
POST /api/v1/attachments (multipart/form-data)
→ 写到 {dataDir}/attachments/{att_id}/original.{ext}
→ 201 { id, fileName, mimeType, sizeBytes }
```

### 7.2 内容提取（`infra/chat/extractor.go`）

`chatinfra.Extract(storagePath, mimeType)` 按 MIME 类型分派：

| 格式 | 实现 |
|---|---|
| `text/*` / `.go` / `.py` / `.json` / `.csv` 等 | `os.ReadFile` |
| `application/pdf` | `dslipak/pdf`（纯 Go）|
| `.docx` / `.odt` / `.rtf` | `lu4p/cat`（纯 Go）|
| `.xlsx` / `.xlsm` | `xuri/excelize`（纯 Go）|
| `.pptx` | stdlib zip + XML 解析 |
| `text/html` | HTML 标签剥离 |
| `image/*` | `IsImage()` → base64 Vision 路径 |

### 7.3 LLM 消息组装

```
图片附件
  → buildUserLLMMessage → attachmentToPart
      → readAndEncode(storagePath) → base64
      → ContentPart{type:"image_url", imageURL:"data:<mime>;base64,..."}

文本附件（提取成功）
  → ContentPart{type:"text", text:"[附件: report.pdf]\n{提取内容}"}

提取失败
  → 软失败：log.Warn + 跳过，其余 parts 正常发送

多 parts → msg.Parts = parts（OpenAI array content 格式）
单 text → msg.Content = text（简化格式，不用 array）
```

---

## 8. SSE 事件

### 8.1 传输机制

```
前端                                  后端
 │                                     │
 ├──GET /api/v1/events?convId=──────→  │  长连接，Bridge 订阅
 │                                     │
 ├──POST /conversations/{id}/messages→ │  202（异步），入队
 │                                     │  ↓ worker goroutine
 │←── event: chat.tool_call_start ───  │  tool name 出现即推（arguments 尚未完整）
 │←── event: chat.token ─────────────  │  每个文字 delta
 │←── event: chat.tool_call ─────────  │  arguments 完整，执行前推
 │←── event: chat.tool_result ───────  │  执行完成
 │←── event: chat.done ──────────────  │  全部结束
```

Keep-alive ping：每 15 秒推 `: keep-alive\n\n` 防代理断连。

### 8.2 Chat 事件完整列表

| 事件 | 触发时机 | 关键字段 |
|---|---|---|
| `chat.token` | 每个文字 delta | `messageId`, `delta` |
| `chat.reasoning_token` | 推理模型 thinking delta | `messageId`, `delta` |
| `chat.tool_call_start` | stream 中 tool name 首次出现 | `messageId`, `toolCallId`, `toolName` |
| `chat.tool_call` | arguments 完整、执行前 | `messageId`, `toolCallId`, `toolName`, `toolInput`, `summary` |
| `chat.tool_result` | 工具执行完成 | `toolCallId`, `result`, `ok` |
| `chat.done` | Agent 全部完成 | `messageId`, `stopReason`, `inputTokens`, `outputTokens` |
| `chat.error` | 不可恢复错误 | `code`, `message` |
| `conversation.title_updated` | auto-titling 回写 | `conversationId`, `title`, `autoTitled` |

---

## 9. HTTP API

### 9.1 端点

| Method | Path | 用途 | 状态码 |
|---|---|---|---|
| `POST` | `/api/v1/attachments` | 上传附件（multipart）| 201 |
| `POST` | `/api/v1/conversations/{id}/messages` | 发送消息，触发 Agent | 202 |
| `DELETE` | `/api/v1/conversations/{id}/stream` | 取消正在运行的 Agent | 204 |
| `GET` | `/api/v1/conversations/{id}/messages` | 消息历史（cursor 分页，含 blocks）| 200 |
| `GET` | `/api/v1/events` | SSE 事件流（`?conversationId=xxx`）| 200 |

### 9.2 GET /conversations/{id}/messages 响应格式

```json
{
  "data": [
    {
      "id": "msg_xxx", "role": "user", "status": "completed",
      "createdAt": "...",
      "blocks": [
        {"id":"blk_1","seq":0,"type":"text","data":"{\"text\":\"帮我...\"}", "createdAt":"..."},
        {"id":"blk_2","seq":1,"type":"attachment_ref","data":"{\"attachmentId\":\"att_xxx\",...}", "createdAt":"..."}
      ]
    },
    {
      "id": "msg_yyy", "role": "assistant", "status": "completed",
      "stopReason": "end_turn", "inputTokens": 1024, "outputTokens": 256,
      "createdAt": "...",
      "blocks": [
        {"id":"blk_3","seq":0,"type":"tool_call","data":"{\"id\":\"call_1\",\"name\":\"datetime\",...}","createdAt":"..."},
        {"id":"blk_4","seq":1,"type":"tool_result","data":"{\"toolCallId\":\"call_1\",\"ok\":true,\"result\":\"...\"}","createdAt":"..."},
        {"id":"blk_5","seq":2,"type":"text","data":"{\"text\":\"当前时间是…\"}","createdAt":"..."}
      ]
    }
  ],
  "nextCursor": "...",
  "hasMore": false
}
```

### 9.3 POST /conversations/{id}/messages

```json
{ "content": "帮我做一个处理 CSV 的工具", "attachmentIds": ["att_xxx"] }
```

→ 202 `{ "data": { "messageId": "msg_xxx" } }`（user 消息 ID，非 assistant）

**错误**：404 `CONVERSATION_NOT_FOUND` / 409 `STREAM_IN_PROGRESS` / SSE 推 `chat.error`（API_KEY_PROVIDER_NOT_FOUND / MODEL_NOT_CONFIGURED）

---

## 10. Service 设计

### 10.1 Struct

```go
// app/chat/chat.go
type Service struct {
    repo        chatdomain.Repository    // messages + blocks + attachments
    convRepo    convdomain.Repository    // 对话 CRUD
    modelPicker modeldomain.ModelPicker  // 拿 (provider, modelID)
    keyProvider apikeydomain.KeyProvider // 拿 (key, baseURL)
    llmFactory  *llminfra.Factory        // 构建 Client（替代原 Eino ModelFactory）
    tools       []agentapp.Tool          // System Tools（实现 Tool 接口）
    bridge      eventsdomain.Bridge      // 推 SSE 事件
    dataDir     string                   // 附件存储根目录
    log         *zap.Logger
    queues      sync.Map                 // conversationID → *convQueue
}
```

### 10.2 Send 流程

```
1. convRepo.Get(conversationID) → 验证对话存在
2. buildUserBlocks(ctx, in) → 从 DB 查附件完整元数据构建 blocks
3. repo.Save(userMsg with blocks) → DB
4. getOrCreateQueue(conversationID) → 入队 queuedTask{userMsgID}
5. 立刻返回 202 { messageId }

--- worker goroutine ---
6. model.PickForChat(ctx) → (provider, modelID)
7. apikey.ResolveCredentials(ctx, provider) → (key, baseURL)
8. llmFactory.Build(Config{...}) → Client
9. buildLLMHistory(ctx, convID, userMsgID) → []LLMMessage（末尾是当前 user 消息）
10. runReactLoop(ctx, client, history, convID, assistantMsgID, uid)
11. Publish(ChatDone) → auto-title goroutine（conv.Title 为空时）
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

**实现（已完成）**：
- domain/chat：Message（Block 模型）+ Block 实体 + sentinels + Repository
- infra/llm：openAIClient + anthropicClient + Factory（替代原 infra/eino）
- infra/store/chat：messages + message_blocks 表 CRUD（ON CONFLICT upsert）
- app/chat：Service 5 文件（chat/pipeline/stream/tools/history）
- handlers/chat：5 端点

**Agent 行为**：tools = nil 时 LLM 只直接回复文字，流式推 chat.token。Phase 3+ 注入 System Tools 后激活 ReAct 循环。

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
POST /api/v1/conversations/cv_xxx/messages  body={content, attachmentIds}
  → middleware 链
  → ChatHandler.Send
      → convRepo.Get(conversationID) → 验证对话存在
      → buildUserBlocks(ctx, in) → 查附件完整元数据 → []Block
      → repo.Save(userMsg with blocks) → DB
      → getOrCreateQueue(conversationID) → queuedTask{userMsgID}
      → response 202 {messageId}

--- worker goroutine（processTask）---
  → model.PickForChat(ctx) → ("deepseek", "deepseek-chat")
  → apikey.ResolveCredentials(ctx, "deepseek") → (key, baseURL)
  → llmFactory.Build(Config{...}) → Client
  → buildLLMHistory(ctx, convID, userMsgID) → []LLMMessage（末尾追加当前 user 消息）
  → runReactLoop:
      for step < maxSteps:
          consumeStream → EventToolStart → Publish(ChatToolCallStart)
                       → EventText → Publish(ChatToken)
                       → EventFinish → usage
          assembleAssistantBlocks → stepBlocks
          executeToolCalls（并行）→ toolResultBlocks
          allBlocks = append(allBlocks, stepBlocks + toolResultBlocks)
          if no tool calls → finalPersist(completed) → break
          else persistMsg(streaming)
  → Publish(ChatDone)
  → auto-title goroutine（conv.Title 为空时）
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

## 13. 实现清单 ✅（截止 2026-04-27 全部完成）

### infra/llm 层（替代 infra/eino）
- [x] `infra/llm/llm.go` — StreamEvent / LLMMessage / ToolDef / Client 接口 / Generate helper
- [x] `infra/llm/openai.go` — OpenAI-compat SSE 客户端（iter.Seq），覆盖 OpenAI/DeepSeek/Qwen/Moonshot/Ollama 等
- [x] `infra/llm/anthropic.go` — Anthropic 原生 /v1/messages 客户端（content_block_start/delta/stop）
- [x] `infra/llm/factory.go` — Factory.Build(Config) provider dispatch + resolveBaseURL
- ❌ `infra/eino/` — **已删除**（4 文件，Eino 依赖全部从 go.mod 消除）

### app/agent 层
- [x] `app/agent/tool.go` — Tool 4 方法接口 + injectSummaryField + StripSummary + ToLLMDef/ToLLMDefs + 6 个 context helpers（WithConversationID/MessageID/ToolCallID + 对应 Get）
- [x] `app/agent/system.go` — 6 个 system tool（datetime/read_file/write_file/list_dir/run_shell/run_python）实现新 Tool 接口，Eino import 消除
- [x] `app/agent/web.go` — web_search（DDG lite POST 表单）+ fetch_url（Jina Reader）
- [x] `app/agent/forge.go` — 5 个 forge tool（search/get/create/edit/run）+ 6 context helpers
- [x] `app/agent/tool_test.go` — 20 单测（injectSummaryField / StripSummary / ToLLMDef / context）
- [x] `app/agent/system_test.go` — 15 单测（6 tool Execute + 接口合规）

### domain/chat 层
- [x] `domain/chat/chat.go` — Message（精简纯元数据）+ Block 实体 + 5 种 BlockType + data 结构体 + Attachment + sentinels + Repository
- [x] `domain/events/types.go` — ChatToolCallStart + ChatReasoningToken；ChatDone 改为 inputTokens/outputTokens int；ToolCodeStreaming/ToolCreated/ToolPendingCreated 加 MessageID+ToolCallID

### infra/db 层
- [x] `infra/db/schema_extras.go` — 新增 message_blocks 索引；移除 messages_fts FTS5（原基于已删除 content 列）

### infra/store/chat 层
- [x] `infra/store/chat/chat.go` — Save（ON CONFLICT upsert 保护 created_at，事务写 blocks）；ListByConversation（批量取 blocks 避 N+1）；GetAttachment；SaveAttachment
- [x] 集成测试更新（适配 Block 模型，新增 3 个 block 相关测试）

### infra/chat 层
- [x] `infra/chat/extractor.go` — Extract(storagePath, mimeType)：text/pdf/docx/xlsx/pptx/html 提取；IsImage 分派 Vision 路径

### app/chat 层
- [x] `app/chat/chat.go` — Service struct（llmFactory 替代 modelFactory；[]agentapp.Tool）+ Send / Cancel / ListMessages / UploadAttachment + 队列管理（convQueue + sync.Map）
- [x] `app/chat/pipeline.go` — processTask + runReactLoop（allBlocks 累积）+ runStep + persistMsg + finalPersist（detached context）+ autoTitle
- [x] `app/chat/stream.go` — consumeStream（iter.Seq）+ assembleAssistantBlocks
- [x] `app/chat/tools.go` — executeToolCalls（sync.WaitGroup 并行）+ runOneTool（注入 msgID/toolCallID）+ executeTool
- [x] `app/chat/history.go` — buildLLMHistory(currentUserMsgID) + blocksToAssistantLLM + buildUserLLMMessage + attachmentToPart
- [x] `app/chat/util.go` — newMsgID / newBlockID / newAttachmentID / readAndEncode / truncate
- [x] `app/chat/stream_test.go` — 10 单测（assembleAssistantBlocks 各场景）
- [x] `app/chat/history_test.go` — 8 单测（blocksToAssistantLLM 含 tool call 和多轮）

### transport 层
- [x] `handlers/chat.go` — 5 端点：POST attachments / POST messages / DELETE stream / GET messages / GET events SSE（keep-alive ping）

### app/tool 层（bug 修复）
- [x] `app/tool/tool.go` — GenerateTestCases：Input/ExpectedOutput 改 json.RawMessage + extractJSONFromLLM 预处理；extractTextContent 返回最后一个 text block

### 配套
- [x] `errmap.go` — chat sentinel 映射全部覆盖
- [x] `router/deps.go` — ChatService / EventsBridge 字段
- [x] `main.go` — chatRepo 共享变量；llmFactory；WebTools / SystemTools / ForgeTools 装配；Migrate messages + message_blocks + chat_attachments
