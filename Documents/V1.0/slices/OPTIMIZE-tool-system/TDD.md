# 工具系统优化 — 技术设计文档

**版本**：v1.0  
**日期**：2026-04-20  
**配套 PRD**：OPTIMIZE-tool-system/PRD.md

---

## 1. 技术决策总览

| 决策 | 选择 | 理由 |
|---|---|---|
| 版本存储 | `tool_versions` 表（存完整代码快照） | 简单可靠，不依赖 diff 库 |
| 标签 | `tool_tags` 表（多对多） | 灵活，支持筛选和搜索 |
| 元数据意图识别 | 扩展 ForgeSystemPrompt + JSON 指令格式 | 复用已有 SSE 事件机制 |
| Inline 编辑 | 前端 PATCH 请求 + 乐观更新 | Notion 风格，响应快 |
| 测试用例 | `tool_test_cases` 表 | 独立于测试历史，可复用 |
| AI 工具列表注入 | 在 loadHistory 中动态生成 | 不增加额外 API 调用 |

---

## 2. 数据库迁移

### `005_tool_enhancements.sql`

```sql
-- Tool versions (code change history)
CREATE TABLE IF NOT EXISTS tool_versions (
    id          TEXT PRIMARY KEY,
    tool_id     TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    version     INTEGER NOT NULL,
    code        TEXT NOT NULL,
    change_summary TEXT NOT NULL DEFAULT '',
    created_at  DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_tool_versions_tool ON tool_versions(tool_id, version DESC);

-- Tool tags (free-form labels)
CREATE TABLE IF NOT EXISTS tool_tags (
    tool_id     TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    tag         TEXT NOT NULL,
    PRIMARY KEY (tool_id, tag)
);
CREATE INDEX IF NOT EXISTS idx_tool_tags_tag ON tool_tags(tag);

-- Saved test cases (reusable parameter sets)
CREATE TABLE IF NOT EXISTS tool_test_cases (
    id          TEXT PRIMARY KEY,
    tool_id     TEXT NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    name        TEXT NOT NULL DEFAULT 'Default',
    params_json TEXT NOT NULL DEFAULT '{}',
    created_at  DATETIME DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_tool_test_cases_tool ON tool_test_cases(tool_id);

-- Add tags column to tools for quick access (denormalized comma-separated)
ALTER TABLE tools ADD COLUMN tags TEXT NOT NULL DEFAULT '';
```

---

## 3. 后端改动

### 3.1 ToolService 扩展

```
service/tool.go — 新增方法
```

| 方法 | 用途 |
|---|---|
| `UpdateMeta(id, displayName, description, category)` | Inline 编辑元数据（PATCH 语义） |
| `AddTag(id, tag)` / `RemoveTag(id, tag)` | 标签管理 |
| `ListTags(id)` | 获取工具标签 |
| `SaveVersion(toolID, code, summary)` | 代码变更时自动创建版本 |
| `ListVersions(toolID)` | 版本历史 |
| `RestoreVersion(toolID, versionNum)` | 回滚到指定版本 |
| `SaveTestCase(toolID, name, params)` | 保存测试用例 |
| `ListTestCases(toolID)` | 获取测试用例 |
| `DeleteTestCase(id)` | 删除测试用例 |

### 3.2 版本管理逻辑

**触发时机**：每次 `Save()` 被调用且 code 字段变化时。

```go
func (s *ToolService) Save(t *Tool) error {
    // ... 现有逻辑 ...
    
    // 如果是更新且代码变化，创建版本快照
    if existing != nil && existing.Code != t.Code {
        s.SaveVersion(t.ID, existing.Code, diffSummary(existing.Code, t.Code))
    }
    
    // ... 写入 DB ...
}

func diffSummary(oldCode, newCode string) string {
    oldLines := strings.Split(oldCode, "\n")
    newLines := strings.Split(newCode, "\n")
    added := countNew(oldLines, newLines)
    removed := countNew(newLines, oldLines)
    return fmt.Sprintf("+%d 行 / -%d 行", added, removed)
}
```

### 3.3 AI 工具列表注入

在 `loadHistory()` 中，forge system prompt 后追加已有工具摘要：

```go
func (s *ChatService) loadHistory(conversationID string) ([]*schema.Message, error) {
    // 已有：注入 forge system prompt
    msgs := []*schema.Message{
        schema.SystemMessage(forge.ForgeSystemPrompt),
    }
    
    // 新增：注入已有工具列表
    toolSummary := s.buildToolSummary()
    if toolSummary != "" {
        msgs = append(msgs, schema.SystemMessage(toolSummary))
    }
    
    // ... 加载历史消息 ...
}

func (s *ChatService) buildToolSummary() string {
    tools, _ := s.toolSvc.List("", "")
    if len(tools) == 0 { return "" }
    
    var sb strings.Builder
    sb.WriteString("[用户已有工具]\n")
    for _, t := range tools {
        status := map[string]string{"draft":"草稿","tested":"已测试","failed":"失败"}[t.Status]
        bi := ""
        if t.Builtin { bi = ", 内置" }
        sb.WriteString(fmt.Sprintf("- %s (%s, %s%s)\n", t.Name, t.Category, status, bi))
    }
    sb.WriteString(fmt.Sprintf("共 %d 个工具。如果用户需要的功能已有工具可用，优先推荐使用已有工具。\n", len(tools)))
    return sb.String()
}
```

### 3.4 元数据意图检测

扩展 `detectForgeCode` 为 `detectForgeIntent`，同时识别：
1. **代码块** → 保存/更新工具（现有逻辑）
2. **元数据指令** → AI 回复中包含特定格式的元数据变更

AI 在 system prompt 中被告知，当用户要求修改工具元数据时，用以下格式回复：

```
[TOOL_META_UPDATE]
tool: send_email
displayName: 发送报价邮件
description: 自动读取供应商报价Excel并汇总后发邮件
category: email
tags: +供应商, +Q2
[/TOOL_META_UPDATE]
```

后端检测到此格式后，直接更新工具元数据并发送 SSE 事件。

### 3.5 新增 HTTP 路由

```go
// 元数据编辑
PATCH  /api/tools/{id}/meta          — 更新名称/描述/分类

// 标签
GET    /api/tools/{id}/tags          — 获取标签
POST   /api/tools/{id}/tags          — 添加标签
DELETE /api/tools/{id}/tags/{tag}    — 删除标签

// 版本
GET    /api/tools/{id}/versions      — 版本列表
POST   /api/tools/{id}/versions/{v}/restore  — 回滚

// 测试用例
GET    /api/tools/{id}/test-cases    — 列表
POST   /api/tools/{id}/test-cases    — 保存
DELETE /api/tools/{id}/test-cases/{caseId}  — 删除

// AI 修复
POST   /api/chat/fix-tool            — 发送错误信息让 AI 修复
```

---

## 4. 前端改动

### 4.1 ToolMainView Header — Inline 编辑

```tsx
// 名称：点击 → contentEditable div → blur 发送 PATCH
<div
  contentEditable={!tool.builtin}
  suppressContentEditableWarning
  onBlur={(e) => {
    const newName = e.currentTarget.textContent?.trim()
    if (newName && newName !== tool.displayName) {
      api(`/api/tools/${tool.id}/meta`, {
        method: 'PATCH',
        body: JSON.stringify({ displayName: newName })
      })
    }
  }}
>
  {tool.displayName}
</div>
```

### 4.2 标签 UI

```tsx
function TagBar({ toolId, tags }: { toolId: string; tags: string[] }) {
    const [adding, setAdding] = useState(false)
    
    return (
        <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
            {tags.map(tag => (
                <span key={tag} style={{ 
                    padding: '2px 8px', borderRadius: 10, fontSize: 11,
                    background: '#f3f4f6', color: '#374151',
                    display: 'flex', alignItems: 'center', gap: 4
                }}>
                    {tag}
                    <X size={10} onClick={() => removeTag(toolId, tag)} />
                </span>
            ))}
            {adding ? <TagInput /> : <button onClick={() => setAdding(true)}>+ 标签</button>}
        </div>
    )
}
```

### 4.3 ForgeCodeBlock — "让 AI 修复" 按钮

```tsx
// 测试结果区域增加修复按钮
{testResult && !testResult.passed && (
    <button onClick={() => {
        // 构造修复请求消息
        const fixMsg = `我的工具 \`${funcName}\` 运行失败：\n${testResult.error}\n\n请帮我修复。`
        onRequestFix?.(fixMsg)
    }}>
        🔧 让 AI 修复
    </button>
)}
```

`onRequestFix` 回调由 MessageItem 传入，最终调用 `sendMessage(fixMsg)` 自动发送到当前对话。

### 4.4 版本历史面板

代码 Tab 右上角增加"历史版本"按钮，点击展开版本列表（侧边抽屉或下拉）：

```tsx
function VersionPanel({ toolId }: { toolId: string }) {
    const [versions, setVersions] = useState<Version[]>([])
    
    return (
        <div style={{ maxHeight: 300, overflowY: 'auto' }}>
            {versions.map(v => (
                <div key={v.version}>
                    <span>v{v.version}</span>
                    <span>{v.changeSummary}</span>
                    <span>{relativeTime(v.createdAt)}</span>
                    <button onClick={() => restore(toolId, v.version)}>恢复</button>
                </div>
            ))}
        </div>
    )
}
```

### 4.5 测试用例管理

测试 Tab 增加用例选择器：

```
[测试用例：默认] [▾]    [💾 保存当前参数]
```

下拉可选已保存的用例，点击自动填充参数。

---

## 5. 实现优先级

### P0（核心体验，本轮必做）

| # | 功能 | 工作量 | 文件 |
|---|---|---|---|
| 1 | Inline 编辑名称/描述/分类 | 小 | ToolMainView.tsx, routes_tools.go |
| 2 | "让 AI 修复"按钮 | 中 | ForgeCodeBlock.tsx, ChatPage.tsx |
| 3 | AI 感知已有工具列表 | 小 | chat.go |
| 4 | 代码更新时显示变更摘要（非静默） | 中 | chat.go, MessageItem.tsx |

### P1（完整性补齐）

| # | 功能 | 工作量 | 文件 |
|---|---|---|---|
| 5 | 标签系统 | 中 | 迁移 + tool.go + TagBar.tsx |
| 6 | 版本管理 + 回滚 | 大 | 迁移 + tool.go + VersionPanel.tsx |
| 7 | 参数文档编辑 | 中 | ToolParamsTab.tsx, tool.go |
| 8 | 测试用例保存/加载 | 中 | 迁移 + ToolTestTab.tsx |

### P2（锦上添花）

| # | 功能 | 工作量 |
|---|---|---|
| 9 | 对话控制元数据（自然语言改名/改分类） | 大 |
| 10 | 参数约束校验（enum/range/pattern） | 中 |
| 11 | 测试趋势统计 | 小 |

---

## 6. 验收测试

```
P0 验收：
1. 在 ToolMainView 点击工具名 → 变为输入框 → 改名 → blur 保存 → 列表同步更新
2. 在 ToolMainView 点击描述 → 编辑 → 保存
3. 对话中 AI 生成代码 → 测试失败 → 点"让 AI 修复" → AI 自动修复 → 新代码更新
4. AI 生成同名函数第二次 → 对话显示"已更新工具 xxx：+2行/-1行"
5. 新对话中问"帮我处理邮件" → AI 回复提及"你已有 send_email 工具"

P1 验收：
6. 给工具添加标签"供应商" → 工具库中按标签筛选可见
7. 编辑代码保存 → 版本列表出现 v2 → 可以回滚到 v1
8. 在参数 Tab 编辑参数描述 → 保存 → 刷新后仍在
9. 填参数测试 → 保存为测试用例 → 下次打开可一键加载
```
