# D2 · 工具锻造 — 技术设计文档

**切片**：D2  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 代码生成模型 | PurposeCodegen | 代码质量优先 |
| 代码提取方式 | AST 解析 + 正则 fallback | 提取函数名/参数/docstring |
| 需求识别 | system prompt 约束 AI 输出规范格式 | 不需要额外分类器 |
| 锻造会话 | 复用 ConversationService，标记 conversation.mode='forge' | 复用消息流 UI |

---

## 2. 目录结构

```
internal/
└── forge/
    ├── agent.go     # 锻造 Agent（包含代码生成 system prompt）
    └── parser.go    # 从 Python 源码提取函数签名和参数

frontend/src/
└── components/forge/
    ├── ForgeCodeBlock.tsx  # 代码块 + 测试/保存按钮
    └── TestParamsModal.tsx # 测试参数填写弹窗
```

---

## 3. Go 层

### `internal/forge/parser.go`

```go
package forge

import (
    "regexp"
    "strings"

    "forgify/internal/service"
)

var (
    reFuncDef  = regexp.MustCompile(`^def (\w+)\((.*?)\)\s*->\s*dict:`)
    reParam    = regexp.MustCompile(`(\w+)\s*:\s*(\w+)`)
    reImport   = regexp.MustCompile(`^(?:import|from)\s+(\S+)`)
)

var stdlibPackages = map[string]bool{
    "os": true, "sys": true, "json": true, "re": true,
    "datetime": true, "time": true, "math": true, "random": true,
    "collections": true, "itertools": true, "functools": true,
    "pathlib": true, "io": true, "typing": true, "dataclasses": true,
    "enum": true, "abc": true, "copy": true, "hashlib": true,
    "hmac": true, "base64": true, "urllib": true, "http": true,
    "email": true, "smtplib": true, "csv": true, "sqlite3": true,
    "subprocess": true, "threading": true, "multiprocessing": true,
    "logging": true, "unittest": true, "contextlib": true,
}

// ParseFunction 从 Python 代码中提取函数元数据
func ParseFunction(code string) (funcName string, params []service.ToolParameter, requirements []string, err error) {
    lines := strings.Split(code, "\n")

    // 提取函数名和参数
    for _, line := range lines {
        if m := reFuncDef.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
            funcName = m[1]
            for _, pm := range reParam.FindAllStringSubmatch(m[2], -1) {
                params = append(params, service.ToolParameter{
                    Name:     pm[1],
                    Type:     normalizeType(pm[2]),
                    Required: true,
                })
            }
            break
        }
    }

    // 提取第三方依赖
    seen := map[string]bool{}
    for _, line := range lines {
        if m := reImport.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
            pkg := strings.Split(m[1], ".")[0]
            if !stdlibPackages[pkg] && !seen[pkg] {
                requirements = append(requirements, pkg)
                seen[pkg] = true
            }
        }
    }

    return funcName, params, requirements, nil
}

func normalizeType(t string) string {
    switch t {
    case "str": return "string"
    case "int": return "int"
    case "float": return "float"
    case "bool": return "bool"
    case "list", "List": return "list"
    case "dict", "Dict": return "dict"
    default: return "string"
    }
}
```

### `internal/forge/agent.go`

```go
package forge

import (
    "context"
    "strings"

    "forgify/internal/model"
    "forgify/internal/service"
    "forgify/internal/events"

    "github.com/cloudwego/eino/schema"
)

const forgeSystemPrompt = `你是 Forgify 的工具锻造助手。你的任务是帮助用户创建可运行的 Python 工具。

**工具代码规范**（必须严格遵守）：
1. 只有一个顶层函数，使用 snake_case 命名
2. 所有参数必须有类型注解（str/int/float/bool/list/dict）
3. 返回值类型必须是 dict
4. 函数第一行必须是 docstring，说明功能
5. 可以使用 import（依赖会自动安装）

**示例格式**：
\`\`\`python
def send_email(to: str, subject: str, body: str) -> dict:
    """通过 SMTP 发送邮件到指定地址"""
    import smtplib
    # ... 实现
    return {"success": True, "message": "邮件已发送"}
\`\`\`

生成代码后，在代码块下方简短说明用法，不需要解释每一行代码。`

type ForgeAgent struct {
    gateway *model.ModelGateway
    toolSvc *service.ToolService
    bridge  *events.Bridge
}

// extractCodeBlock 从 AI 回复中提取第一个 python 代码块
func extractCodeBlock(content string) string {
    start := strings.Index(content, "```python")
    if start == -1 { return "" }
    start += len("```python")
    end := strings.Index(content[start:], "```")
    if end == -1 { return "" }
    return strings.TrimSpace(content[start : start+end])
}

// OnAssistantMessage 在 ChatService 收到完整 AI 回复后调用
// 检测是否包含 Python 代码块，如有则解析并保存草稿工具
func (a *ForgeAgent) OnAssistantMessage(
    ctx context.Context, convID string, content string,
) error {
    code := extractCodeBlock(content)
    if code == "" { return nil }

    funcName, params, reqs, err := ParseFunction(code)
    if err != nil || funcName == "" { return nil }

    // 保存草稿工具（status='draft'）
    tool := &service.Tool{
        Name:         funcName,
        DisplayName:  funcName, // 用户保存时可修改
        Code:         code,
        Requirements: reqs,
        Parameters:   params,
        Status:       "draft",
    }
    if err := a.toolSvc.Save(tool); err != nil { return err }

    // 通知前端：当前消息包含可测试的代码块
    a.bridge.Emit(events.ForgeCodeDetected, map[string]any{
        "conversationId": convID,
        "toolId":         tool.ID,
        "funcName":       funcName,
    })
    return nil
}
```

---

## 4. 事件扩展

在 A3 的事件系统中新增：

```go
// events/events.go
const (
    // ...现有事件...
    ForgeCodeDetected = "forge.code_detected" // AI 生成了工具代码
)
```

TypeScript 侧：
```ts
interface ForgeCodeDetectedPayload {
    conversationId: string
    toolId: string
    funcName: string
}
```

---

## 5. 前端组件

### `ForgeCodeBlock.tsx`

```tsx
// MarkdownContent 组件中，检测到 forge.code_detected 事件时
// 在代码块下方追加操作按钮
export function ForgeCodeBlock({ toolId, code }: { toolId: string; code: string }) {
    const [testing, setTesting] = useState(false)
    const [showSave, setShowSave] = useState(false)

    return (
        <div>
            <SyntaxHighlighter language="python">{code}</SyntaxHighlighter>
            <div className="flex gap-2 mt-2">
                <button
                    onClick={() => setTesting(true)}
                    className="px-3 py-1.5 text-sm bg-neutral-700 rounded-lg flex items-center gap-1"
                >
                    ▶ 测试运行
                </button>
                <button
                    onClick={() => setShowSave(true)}
                    className="px-3 py-1.5 text-sm bg-blue-600 rounded-lg flex items-center gap-1"
                >
                    💾 保存为工具
                </button>
            </div>
            {testing && <TestParamsModal toolId={toolId} onClose={() => setTesting(false)} />}
            {showSave && <SaveToolModal toolId={toolId} onClose={() => setShowSave(false)} />}
        </div>
    )
}
```

### `TestParamsModal.tsx`

```tsx
export function TestParamsModal({ toolId, onClose }: { toolId: string; onClose: () => void }) {
    const [tool, setTool] = useState<Tool | null>(null)
    const [values, setValues] = useState<Record<string, string>>({})
    const [running, setRunning] = useState(false)

    useEffect(() => { GetTool(toolId).then(setTool) }, [toolId])

    const run = async () => {
        setRunning(true)
        const params = Object.fromEntries(
            Object.entries(values).map(([k, v]) => [k, v])
        )
        await RunTool(toolId, params)
        setRunning(false)
        onClose()
    }

    if (!tool) return null
    return (
        <Modal title="测试参数" onClose={onClose}>
            {tool.parameters.map(p => (
                <div key={p.name} className="mb-3">
                    <label className="text-xs text-neutral-400 mb-1 block">{p.name}</label>
                    <input
                        value={values[p.name] ?? ''}
                        onChange={e => setValues(v => ({ ...v, [p.name]: e.target.value }))}
                        className="w-full px-3 py-2 bg-neutral-800 rounded-lg text-sm"
                    />
                </div>
            ))}
            <div className="flex justify-end gap-2">
                <button onClick={onClose} className="px-3 py-1.5 text-sm">取消</button>
                <button onClick={run} disabled={running}
                    className="px-3 py-1.5 text-sm bg-blue-600 rounded-lg">
                    {running ? '运行中...' : '运行测试'}
                </button>
            </div>
        </Modal>
    )
}
```

---

## 6. HTTP API 路由

```go
// backend/internal/server/routes.go
mux.HandleFunc("GET /api/tools/{id}", s.getTool)
mux.HandleFunc("POST /api/tools/{id}/run", s.runTool)
```

---

## 7. 验收测试

```
1. 对话中描述"做一个发送邮件的工具" → AI 回复包含符合规范的 Python 代码
2. 代码块下方有"测试运行"和"保存为工具"按钮
3. 点击"测试运行" → 弹出参数面板 → 填参数 → 运行 → 结果卡片出现
4. 代码有第三方依赖（如 requests）→ 首次运行自动安装
5. 点击"保存为工具" → 命名面板 → 保存 → 工具库可见
6. 对话中继续说"把超时改成 60 秒" → AI 生成新版本代码
```
