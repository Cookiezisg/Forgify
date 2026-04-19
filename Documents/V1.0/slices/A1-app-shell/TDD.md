# A1 · App Shell — 技术设计文档

**切片**：A1  
**状态**：待 Review

---

## 1. 技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 桌面框架 | Wails v3 | 原生 systray、Go 绑定直接、单二进制 |
| 布局模型 | NavSidebar + Tab 页面各自管理内部布局 | 每个 Tab 的左右分栏逻辑不同，各自封装更清晰 |
| 前端路由 | React 状态管理（非 URL Router）| 桌面 app 没有真实 URL |
| CSS 方案 | Tailwind CSS | 快速布局，设计稿一致性 |
| 图标库 | Lucide React | 轻量、风格统一 |
| Inbox 角标 | React Context | 多处读取未读数，避免 prop drilling |

---

## 2. 布局模型

```
┌────────────────────────────────────────────────────────────┐
│  [64px NavSidebar] │ [当前 Tab 的页面（自管理内部布局）]     │
└────────────────────────────────────────────────────────────┘
```

每个 Tab 页面内部管理自己的"左侧二级面板 + 右侧主区"：
- **Home**：左侧信息面板 + 右侧大对话输入
- **Chat**：左侧对话列表 + 右侧（单栏对话 / 对话+画布 / 对话+代码）
- **Assets**：左侧资产列表 + 右侧（空 / 关联对话列表+画布 / 对话+画布 / 等）
- **Inbox**：左侧通知列表 + 右侧（空 / 画布 / 代码 / 纯文本）

---

## 3. 目录结构

```
forgify/
├── main.go
├── app.go
├── wails.json
├── build/
│   ├── appicon.png
│   └── trayicon.png
├── internal/
│   └── tray/
│       └── tray.go
└── frontend/src/
    ├── main.tsx
    ├── App.tsx
    ├── context/
    │   └── InboxContext.tsx
    ├── types/
    │   └── navigation.ts
    ├── components/
    │   ├── layout/
    │   │   ├── Sidebar.tsx      # 64px/220px 导航栏
    │   │   ├── TabPanel.tsx     # 渲染当前 Tab 页面
    │   │   └── SplitView.tsx    # 可复用的左右分栏容器
    │   └── common/
    │       └── EmptyState.tsx
    └── pages/
        ├── Home.tsx
        ├── Chat.tsx
        ├── Assets.tsx
        └── Inbox.tsx
```

---

## 4. Go 层

### main.go

```go
package main

import (
    "embed"
    "forgify/internal/tray"
    "github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
    app := application.New(application.Options{
        Name:   "Forgify",
        Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
        Mac:    application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: false},
    })

    window := app.NewWebviewWindowWithOptions(application.WebviewWindowOptions{
        Title: "Forgify", Width: 1280, Height: 800, MinWidth: 960, MinHeight: 640,
        ShouldClose: func(w *application.WebviewWindow) bool { w.Hide(); return false },
    })

    tray.Setup(app, window)
    app.Run()
}
```

### internal/tray/tray.go

```go
package tray

import "github.com/wailsapp/wails/v3/pkg/application"

func Setup(app *application.App, window *application.WebviewWindow) {
    t := app.NewSystemTray()
    t.SetIcon("build/trayicon.png")
    t.SetDarkModeIcon("build/trayicon-dark.png")
    t.OnClick(func() {
        if window.IsVisible() { window.Hide() } else { window.Show(); window.Focus() }
    })
    menu := app.NewMenu()
    menu.Add("显示 Forgify").OnClick(func(_ *application.MenuItem) { window.Show(); window.Focus() })
    menu.AddSeparator()
    status := menu.Add("今日已运行 0 个工作流")
    status.SetEnabled(false)
    menu.AddSeparator()
    menu.Add("退出 Forgify").OnClick(func(_ *application.MenuItem) { app.Quit() })
    t.SetMenu(menu)
}
```

---

## 5. 前端层

### `types/navigation.ts`

```typescript
export type TabId = 'home' | 'chat' | 'assets' | 'inbox'

export const TABS = [
    { id: 'home'   as TabId, label: 'Home',  icon: 'House' },
    { id: 'chat'   as TabId, label: '对话',  icon: 'MessageCircle' },
    { id: 'assets' as TabId, label: '资产',  icon: 'Package' },
    { id: 'inbox'  as TabId, label: 'Inbox', icon: 'Inbox' },
]
```

### `context/InboxContext.tsx`

```tsx
import { createContext, useContext, useState, ReactNode } from 'react'

const InboxContext = createContext({ unreadCount: 0, setUnreadCount: (_: number) => {} })

export function InboxProvider({ children }: { children: ReactNode }) {
    const [unreadCount, setUnreadCount] = useState(0)
    return <InboxContext.Provider value={{ unreadCount, setUnreadCount }}>{children}</InboxContext.Provider>
}

export const useInbox = () => useContext(InboxContext)
```

### `App.tsx`

```tsx
import { useState, useEffect } from 'react'
import { Sidebar } from './components/layout/Sidebar'
import { TabPanel } from './components/layout/TabPanel'
import { InboxProvider } from './context/InboxContext'
import { TabId } from './types/navigation'

export default function App() {
    const [activeTab, setActiveTab] = useState<TabId>('home')
    const [expanded, setExpanded] = useState(
        () => localStorage.getItem('sidebar-expanded') !== 'false'
    )

    useEffect(() => {
        localStorage.setItem('sidebar-expanded', String(expanded))
    }, [expanded])

    return (
        <InboxProvider>
            <div className="flex h-screen w-screen overflow-hidden bg-neutral-950 text-neutral-100">
                <Sidebar
                    activeTab={activeTab}
                    expanded={expanded}
                    onTabChange={setActiveTab}
                    onToggleExpand={() => setExpanded(v => !v)}
                />
                <TabPanel activeTab={activeTab} />
            </div>
        </InboxProvider>
    )
}
```

### `components/layout/Sidebar.tsx`

```tsx
import { House, MessageCircle, Package, Inbox, Settings, ChevronLeft, ChevronRight } from 'lucide-react'
import { TABS, TabId } from '../../types/navigation'
import { useInbox } from '../../context/InboxContext'

const ICONS = { House, MessageCircle, Package, Inbox }

export function Sidebar({ activeTab, expanded, onTabChange, onToggleExpand }:
    { activeTab: TabId; expanded: boolean; onTabChange: (t: TabId) => void; onToggleExpand: () => void }) {
    const { unreadCount } = useInbox()

    return (
        <aside className={`${expanded ? 'w-[220px]' : 'w-[64px]'} flex-shrink-0 flex flex-col border-r border-neutral-800 bg-neutral-900 transition-all duration-200`}>
            <div className="flex items-center justify-between px-4 py-4 border-b border-neutral-800">
                {expanded && <span className="font-semibold text-sm">Forgify</span>}
                <button onClick={onToggleExpand} className="p-1 rounded hover:bg-neutral-800 text-neutral-400">
                    {expanded ? <ChevronLeft size={16} /> : <ChevronRight size={16} />}
                </button>
            </div>

            <nav className="flex-1 py-2">
                {TABS.map(tab => {
                    const Icon = ICONS[tab.icon as keyof typeof ICONS]
                    const active = activeTab === tab.id
                    const badge = tab.id === 'inbox' && unreadCount > 0
                    return (
                        <button key={tab.id} onClick={() => onTabChange(tab.id)}
                            className={`relative w-full flex items-center gap-3 px-4 py-2.5 text-sm transition-colors
                                ${active ? 'text-neutral-100 bg-neutral-800' : 'text-neutral-400 hover:text-neutral-200 hover:bg-neutral-800/50'}`}>
                            {active && <span className="absolute left-0 top-1/2 -translate-y-1/2 w-0.5 h-5 bg-blue-500 rounded-r" />}
                            <span className="relative">
                                <Icon size={18} />
                                {badge && <span className="absolute -top-1 -right-1 w-2 h-2 bg-red-500 rounded-full" />}
                            </span>
                            {expanded && <span>{tab.label}</span>}
                            {expanded && badge && (
                                <span className="ml-auto text-xs bg-red-500 text-white rounded-full px-1.5 min-w-[18px] text-center">
                                    {unreadCount > 99 ? '99+' : unreadCount}
                                </span>
                            )}
                        </button>
                    )
                })}
            </nav>

            <div className="border-t border-neutral-800 py-2">
                <button className="w-full flex items-center gap-3 px-4 py-2.5 text-sm text-neutral-400 hover:text-neutral-200 hover:bg-neutral-800/50">
                    <Settings size={18} />
                    {expanded && <span>设置</span>}
                </button>
            </div>
        </aside>
    )
}
```

### `components/layout/TabPanel.tsx`

```tsx
import { TabId } from '../../types/navigation'
import { Home } from '../../pages/Home'
import { Chat } from '../../pages/Chat'
import { Assets } from '../../pages/Assets'
import { Inbox } from '../../pages/Inbox'

export function TabPanel({ activeTab }: { activeTab: TabId }) {
    return (
        <main className="flex-1 overflow-hidden">
            <div className={activeTab === 'home'   ? 'h-full' : 'hidden'}><Home /></div>
            <div className={activeTab === 'chat'   ? 'h-full' : 'hidden'}><Chat /></div>
            <div className={activeTab === 'assets' ? 'h-full' : 'hidden'}><Assets /></div>
            <div className={activeTab === 'inbox'  ? 'h-full' : 'hidden'}><Inbox /></div>
        </main>
    )
}
```

### `components/layout/SplitView.tsx`（可复用的左右分栏）

```tsx
// 所有 Tab 内部需要左右分栏时都用这个组件
export function SplitView({ left, right, leftWidth = '300px' }:
    { left: React.ReactNode; right: React.ReactNode; leftWidth?: string }) {
    return (
        <div className="flex h-full overflow-hidden">
            <div style={{ width: leftWidth }} className="flex-shrink-0 border-r border-neutral-800 overflow-y-auto">
                {left}
            </div>
            <div className="flex-1 overflow-hidden">
                {right}
            </div>
        </div>
    )
}
```

### 页面占位

```tsx
// 每个页面占位，实际内容由后续切片填充
export function Home()   { return <EmptyState text="你的工作台，正在初始化..." /> }
export function Chat()   { return <EmptyState text="开始一段新对话" /> }
export function Assets() { return <EmptyState text="你的工具和工作流会显示在这里" /> }
export function Inbox()  { return <EmptyState text="收件箱是空的" /> }

export function EmptyState({ text }: { text: string }) {
    return <div className="h-full flex items-center justify-center"><p className="text-neutral-500 text-sm">{text}</p></div>
}
```

---

## 6. 验收测试

```
1. wails dev 启动，主窗口正常显示
2. 四个 Tab 可切换，内容区切换正常
3. 导航栏收起/展开，状态持久化（localStorage）
4. 点击关闭 → 窗口隐藏，进程仍在
5. 首次关闭 → toast 提示，第二次不再显示
6. 托盘图标出现，右键菜单正常，"退出"完全退出
7. 托盘左键：toggle 窗口显示
8. 窗口最小 960×640 限制有效
9. Inbox 无未读时无角标（角标实际数据由 I1 填充）
```
