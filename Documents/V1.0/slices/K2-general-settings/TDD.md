# K2 · 通用设置 — 技术设计文档

**切片**：K2  
**状态**：待 Review

---

## 1. 设置数据结构

在 K1 的 settings 表基础上，新增 key `general_config`：

```json
{
  "run_history_limit": 100,
  "node_timeout_seconds": 30,
  "workflow_timeout_minutes": 10,
  "notify_on_approval": true,
  "notify_on_run_complete": false
}
```

---

## 2. GeneralSettings Go 类型

```go
// service/settings.go（扩展）
type GeneralSettings struct {
    RunHistoryLimit        int  `json:"run_history_limit"`
    NodeTimeoutSeconds     int  `json:"node_timeout_seconds"`
    WorkflowTimeoutMinutes int  `json:"workflow_timeout_minutes"`
    NotifyOnApproval       bool `json:"notify_on_approval"`
    NotifyOnRunComplete    bool `json:"notify_on_run_complete"`
}

func (s *SettingsService) GetGeneralSettings() (*GeneralSettings, error) {
    var raw string
    err := storage.DB().QueryRow(`SELECT value FROM settings WHERE key='general_config'`).Scan(&raw)
    if err == sql.ErrNoRows {
        return &GeneralSettings{
            RunHistoryLimit:        100,
            NodeTimeoutSeconds:     30,
            WorkflowTimeoutMinutes: 10,
            NotifyOnApproval:       true,
            NotifyOnRunComplete:    false,
        }, nil
    }
    var gs GeneralSettings
    json.Unmarshal([]byte(raw), &gs)
    return &gs, nil
}

func (s *SettingsService) SaveGeneralSettings(gs *GeneralSettings) error {
    raw, _ := json.Marshal(gs)
    _, err := storage.DB().Exec(`
        INSERT INTO settings (key, value) VALUES ('general_config', ?)
        ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(raw))
    return err
}
```

---

## 3. 数据导出

```go
// app.go
func (a *App) ExportData(destPath string) error {
    return storage.BackupDB(destPath)
}

// storage/backup.go
func BackupDB(destPath string) error {
    src, _ := os.ReadFile(storage.DBPath())
    return os.WriteFile(destPath, src, 0644)
}
```

前端使用 Wails 文件对话框：

```typescript
// SettingsPage.tsx
const handleExport = async () => {
    const path = await SaveFileDialog({
        DefaultFilename: 'forgify-backup.db',
        Filters: [{ DisplayName: 'Forgify Backup', Pattern: '*.db' }],
    })
    if (path) await ExportData(path)
}
```

---

## 4. 危险操作

```go
// app.go
func (a *App) ClearRunHistory() error {
    _, err := storage.DB().Exec(`DELETE FROM runs`)
    return err
}

func (a *App) ResetAllPermissions() error {
    _, err := storage.DB().Exec(`DELETE FROM tool_permissions`)
    return err
}

func (a *App) ClearConversations() error {
    _, err := storage.DB().Exec(`DELETE FROM messages; DELETE FROM conversations`)
    return err
}
```

---

## 5. 系统通知（macOS）

```go
// service/notification.go
import "github.com/gen2brain/beeep"

func (s *NotificationService) SendIfEnabled(title, body string, notifType string) {
    gs, _ := s.settingsSvc.GetGeneralSettings()
    switch notifType {
    case "approval":
        if !gs.NotifyOnApproval { return }
    case "run_complete":
        if !gs.NotifyOnRunComplete { return }
    }
    beeep.Notify(title, body, "")
}
```

---

## 6. 前端：通用设置页

```tsx
// pages/settings/GeneralSettingsPage.tsx
export function GeneralSettingsPage() {
    const [gs, setGs] = useState<GeneralSettings | null>(null)
    const [permissions, setPermissions] = useState<ToolPermission[]>([])
    const [confirmAction, setConfirmAction] = useState<string | null>(null)

    useEffect(() => {
        Promise.all([GetGeneralSettings(), ListGrantedPermissions()])
            .then(([g, p]) => { setGs(g); setPermissions(p) })
    }, [])

    if (!gs) return null

    return (
        <div className="p-6 space-y-8 max-w-xl">
            <h2 className="text-base font-semibold">通用设置</h2>

            {/* 运行历史 */}
            <section className="space-y-3">
                <p className="text-sm font-medium">运行历史</p>
                <div className="flex items-center gap-3">
                    <span className="text-xs text-neutral-400">保留数量</span>
                    <select value={gs.run_history_limit}
                        onChange={e => setGs(g => g ? { ...g, run_history_limit: parseInt(e.target.value) } : g)}
                        className="px-3 py-2 bg-neutral-800 rounded text-sm">
                        {[20, 50, 100, 0].map(v => (
                            <option key={v} value={v}>{v === 0 ? '不限制' : v + ' 条'}</option>
                        ))}
                    </select>
                </div>
            </section>

            {/* 超时 */}
            <section className="space-y-3">
                <p className="text-sm font-medium">执行超时</p>
                <div className="flex items-center gap-3">
                    <span className="text-xs text-neutral-400 w-24">节点超时</span>
                    <input type="number" value={gs.node_timeout_seconds}
                        onChange={e => setGs(g => g ? { ...g, node_timeout_seconds: parseInt(e.target.value) } : g)}
                        className="w-20 px-3 py-2 bg-neutral-800 rounded text-sm" />
                    <span className="text-xs text-neutral-500">秒</span>
                </div>
            </section>

            {/* 已授权工具 */}
            <section className="space-y-3">
                <p className="text-sm font-medium">已授权工具</p>
                {permissions.length === 0
                    ? <p className="text-xs text-neutral-600">暂无已授权工具</p>
                    : permissions.map(p => (
                        <div key={p.toolName} className="flex items-center gap-3">
                            <span className="text-sm flex-1">{p.toolName}</span>
                            <button onClick={() => RevokePermission(p.toolName).then(() =>
                                setPermissions(ps => ps.filter(x => x.toolName !== p.toolName))
                            )} className="text-xs text-red-400 hover:text-red-300">撤销</button>
                        </div>
                    ))}
            </section>

            {/* 危险操作 */}
            <section className="space-y-2 border-t border-neutral-800 pt-6">
                <p className="text-sm font-medium text-red-400">危险操作</p>
                {[
                    { label: '清空所有运行历史', action: 'clear_runs', fn: ClearRunHistory },
                    { label: '重置所有权限授权', action: 'reset_perms', fn: ResetAllPermissions },
                ].map(({ label, action, fn }) => (
                    <div key={action} className="flex items-center justify-between">
                        <span className="text-sm text-neutral-400">{label}</span>
                        <button onClick={() => setConfirmAction(action)}
                            className="text-xs px-3 py-1 rounded bg-red-900 text-red-300">清空</button>
                    </div>
                ))}
            </section>

            <button onClick={() => SaveGeneralSettings(gs!)} className="px-4 py-2 bg-blue-600 rounded text-sm">保存</button>

            {/* 二次确认弹窗 */}
            {confirmAction && (
                <ConfirmDialog
                    message="此操作不可撤销，确认继续？"
                    onConfirm={async () => {
                        const fn = { clear_runs: ClearRunHistory, reset_perms: ResetAllPermissions }[confirmAction!]
                        await fn?.()
                        setConfirmAction(null)
                    }}
                    onCancel={() => setConfirmAction(null)}
                />
            )}
        </div>
    )
}
```

---

## 7. 验收测试

```
1. GetGeneralSettings() 返回默认值（首次启动）
2. SaveGeneralSettings() → 持久化 → 重启后仍有效
3. 修改 run_history_limit → 下次 Prune() 使用新值
4. RevokePermission() → tool_permissions 表更新 → 列表刷新
5. ClearRunHistory() 弹二次确认 → 确认后 runs 表清空
6. ExportData() → SQLite 文件复制到用户指定路径
```
