# D6 · 内置工具 — 技术设计文档

**切片**：D6  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 内置工具存储 | Go embed 打包 Python 文件 | 随二进制分发，无需外部文件 |
| 内置工具注册 | 启动时扫描 embed FS，写入 tools 表（builtin=true）| 与用户工具同表，UI 统一 |
| 内置工具更新 | 版本号对比，旧版则覆盖写入 | 内置工具随 Forgify 升级自动更新 |
| API Key 检查 | 运行前查询 api_keys 表，未配置则返回错误 | 统一权限流程 |

---

## 2. 目录结构

```
internal/
└── builtin/
    ├── registry.go        # 内置工具注册逻辑
    └── tools/             # 所有内置工具 Python 文件
        ├── email/
        │   ├── gmail_read.py
        │   ├── gmail_send.py
        │   ├── outlook_read.py
        │   └── outlook_send.py
        ├── file/
        │   ├── excel_read.py
        │   ├── excel_write.py
        │   ├── csv_read.py
        │   ├── csv_write.py
        │   ├── file_read.py
        │   ├── file_write.py
        │   └── pdf_read.py
        ├── web/
        │   ├── http_request.py
        │   ├── web_scrape.py
        │   └── webhook_send.py
        ├── messaging/
        │   ├── feishu_send.py
        │   ├── slack_send.py
        │   └── dingtalk_send.py
        ├── data/
        │   ├── json_parse.py
        │   ├── data_filter.py
        │   ├── data_sort.py
        │   └── text_regex.py
        ├── ai/
        │   ├── llm_call.py
        │   ├── text_summarize.py
        │   └── text_translate.py
        └── system/
            ├── notify_desktop.py
            └── delay.py
```

---

## 3. 内置工具元数据

每个 Python 文件头部包含元数据注释（structured docstring）：

```python
# @builtin
# @version 1.0
# @category email
# @display_name 读取 Gmail
# @description 读取 Gmail 邮件列表，支持按发件人、主题、时间过滤
# @permission gmail_read
# @requires_key gmail

def gmail_read(query: str = "", max_results: int = 10) -> dict:
    """读取 Gmail 邮件，返回邮件列表"""
    import os
    # ... 实现
    return {"emails": [], "total": 0}
```

---

## 4. Go 层

### `internal/builtin/registry.go`

```go
package builtin

import (
    "embed"
    "io/fs"
    "strings"

    "forgify/internal/service"
)

//go:embed tools/**/*.py
var toolsFS embed.FS

// Register 在应用启动时将所有内置工具同步到 tools 表
func Register(toolSvc *service.ToolService) error {
    return fs.WalkDir(toolsFS, "tools", func(path string, d fs.DirEntry, err error) error {
        if err != nil || d.IsDir() || !strings.HasSuffix(path, ".py") { return err }

        data, _ := toolsFS.ReadFile(path)
        code := string(data)

        meta := parseMeta(code)
        if meta == nil { return nil }

        // 检查是否已存在同版本内置工具
        existing, _ := toolSvc.GetByName(meta.Name)
        if existing != nil && existing.Version == meta.Version { return nil }

        tool := &service.Tool{
            Name:        meta.Name,
            DisplayName: meta.DisplayName,
            Description: meta.Description,
            Code:        code,
            Category:    meta.Category,
            Builtin:     true,
            Version:     meta.Version,
            Status:      "tested", // 内置工具默认已测试
        }
        return toolSvc.Save(tool)
    })
}

type builtinMeta struct {
    Name        string
    DisplayName string
    Description string
    Category    string
    Version     string
    RequiresKey string
}

func parseMeta(code string) *builtinMeta {
    // 解析 @builtin @version @category 等注释
    // ...
    return &builtinMeta{}
}
```

### 数据库 schema 更新

在 `005_tools.sql` 中增加字段：

```sql
-- 在 tools 表 ALTER 或在 005 迁移时直接包含：
ALTER TABLE tools ADD COLUMN builtin  BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE tools ADD COLUMN version  TEXT NOT NULL DEFAULT '1.0';
ALTER TABLE tools ADD COLUMN requires_key TEXT; -- 如 'gmail', 'slack'
```

---

## 5. 内置工具示例实现

### `tools/file/excel_read.py`

```python
# @builtin
# @version 1.0
# @category file
# @display_name 读取 Excel
# @description 读取 Excel 文件，返回指定 Sheet 的数据（最多 100 行）

def excel_read(file_path: str, sheet_name: str = "", max_rows: int = 100) -> dict:
    """读取 Excel 文件内容"""
    import openpyxl
    wb = openpyxl.load_workbook(file_path, read_only=True, data_only=True)
    sheet = wb[sheet_name] if sheet_name else wb.active
    rows = []
    for i, row in enumerate(sheet.iter_rows(values_only=True)):
        if i >= max_rows: break
        rows.append(list(row))
    headers = rows[0] if rows else []
    data = [dict(zip(headers, r)) for r in rows[1:]] if len(rows) > 1 else []
    return {"headers": headers, "data": data, "total_rows": sheet.max_row, "truncated": sheet.max_row > max_rows}
```

### `tools/web/http_request.py`

```python
# @builtin
# @version 1.0
# @category web
# @display_name HTTP 请求
# @description 发送 HTTP 请求，返回响应内容

def http_request(url: str, method: str = "GET", headers: dict = {}, body: str = "", timeout: int = 30) -> dict:
    """发送 HTTP 请求"""
    import requests
    resp = requests.request(method, url, headers=headers, data=body, timeout=timeout)
    return {"status_code": resp.status_code, "body": resp.text, "headers": dict(resp.headers)}
```

---

## 6. 前端：内置工具标识

```tsx
// ToolCard.tsx 已有，增加 builtin 属性渲染
{tool.builtin && (
    <span className="text-xs bg-neutral-700 text-neutral-400 px-1.5 py-0.5 rounded">内置</span>
)}
```

---

## 7. 验收测试

```
1. Forgify 启动后，工具列表出现所有内置工具（约 25 个）
2. 内置工具有"内置"标签，代码只读，无删除按钮
3. excel_read：传入真实 xlsx 路径，返回正确数据
4. http_request：GET https://httpbin.org/get，返回 200 + body
5. 需要 API Key 的工具（gmail_read）未配置时测试返回"未配置 gmail API Key"
6. Forgify 升级后（模拟更新版本号），旧版内置工具自动更新
```
