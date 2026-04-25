# D6 · 内置工具 — 产品需求文档

**切片**：D6  
**状态**：待 Review  
**依赖**：D3（Python 沙箱）、B1（API Key 管理）  
**下游**：E3（工作流工具节点从内置工具库选择）

---

## 1. 这块做什么

Forgify 预装一批开箱即用的工具，覆盖办公自动化高频场景。内置工具和用户锻造的工具在界面上一视同仁，都出现在 Nav 侧边栏"资产"下的工具列表中。内置工具代码顶部有 `# @builtin` 标注，用户工具有 `# @custom` 标注。内置工具在 ToolMainView 中所有编辑控件为只读。

---

## 2. 内置工具的特殊性

| 属性 | 说明 |
|---|---|
| 来源 | 随 Forgify 打包，内嵌在二进制里 |
| 代码 | 可以查看，但**不能修改**（显示为只读，标注"内置工具"）|
| 删除 | 不能删除，只能"隐藏"（从工具列表中隐藏，设置里可恢复）|
| 分享 | 不能导出为 .forgify-tool（无意义）|
| 权限 | 首次使用前弹出权限确认，用户批准后记住 |

---

## 3. 内置工具目录（MVP）

### 邮件类
| 工具名 | 功能 | 所需权限 |
|---|---|---|
| `gmail_read` | 读取 Gmail 邮件列表，支持过滤条件 | Gmail 读取 |
| `gmail_send` | 发送 Gmail 邮件（含附件）| Gmail 发送（执行级）|
| `outlook_read` | 读取 Outlook 邮件 | Outlook 读取 |
| `outlook_send` | 发送 Outlook 邮件 | Outlook 发送（执行级）|

### 文件类
| 工具名 | 功能 |
|---|---|
| `excel_read` | 读取 Excel 文件，返回指定 sheet 的数据 |
| `excel_write` | 向 Excel 文件写入数据 |
| `csv_read` | 读取 CSV 文件 |
| `csv_write` | 写入 CSV 文件 |
| `file_read` | 读取本地文本文件 |
| `file_write` | 写入本地文本文件 |
| `pdf_read` | 提取 PDF 文本内容 |

### 网络类
| 工具名 | 功能 |
|---|---|
| `http_request` | 发送 HTTP 请求（GET/POST/PUT/DELETE）|
| `web_scrape` | 抓取网页文本内容 |
| `webhook_send` | 向指定 URL 发送 Webhook |

### 通讯类
| 工具名 | 功能 | 所需权限 |
|---|---|---|
| `feishu_send` | 发送飞书消息 | 飞书 Bot Token（执行级）|
| `slack_send` | 发送 Slack 消息 | Slack Bot Token（执行级）|
| `dingtalk_send` | 发送钉钉消息 | 钉钉 Token（执行级）|

### 数据处理类
| 工具名 | 功能 |
|---|---|
| `json_parse` | 解析 JSON 字符串，支持 JSONPath 提取 |
| `data_filter` | 按条件过滤列表数据 |
| `data_sort` | 按字段排序列表数据 |
| `text_regex` | 正则表达式提取 |

### AI 类
| 工具名 | 功能 |
|---|---|
| `llm_call` | 调用 LLM，支持自定义 prompt |
| `text_summarize` | 文本摘要（使用轻量模型）|
| `text_translate` | 文本翻译 |

### 系统类
| 工具名 | 功能 |
|---|---|
| `notify_desktop` | 发送桌面通知 |
| `delay` | 等待指定秒数 |

---

## 4. API Key 依赖

需要外部 API 的工具（Gmail、Outlook、飞书等），首次使用时引导用户在设置页配置对应的 API Key / OAuth 授权。如果未配置，工具显示"未配置"状态，不可运行。

---

## 5. 验收标准

- [ ] 内置工具出现在资产 Tab 工具列表，有"内置"标签
- [ ] 内置工具代码可查看，不可编辑（编辑按钮灰色）
- [ ] 内置工具不可删除（删除按钮灰色）
- [ ] 需要 API Key 的工具，未配置时显示"未配置"状态，点击引导到设置
- [ ] `excel_read` 测试：读取一个 xlsx 文件，返回数据
- [ ] `http_request` 测试：GET 一个公网 URL，返回响应体
