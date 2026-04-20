import { useState, useEffect } from 'react'
import { ErrorBoundary } from './components/ErrorBoundary'
import { LocaleProvider } from './lib/i18n'
import { InboxProvider } from './context/InboxContext'
import { ChatProvider } from './context/ChatContext'
import { TabProvider, useTabContext } from './context/TabContext'
import { SidebarNav, type NavTab } from './components/SidebarNav'
import { TabBar } from './components/TabBar'
import { LayoutRouter } from './components/layouts/LayoutRouter'
import { HomeLeftPanel, HomeContent } from './pages/HomePage'
import { ChatLeftPanel } from './pages/ChatPage'
import { AssetsLeftPanel } from './pages/AssetsPage'
import { InboxLeftPanel, InboxContent } from './pages/InboxPage'
import { SettingsLeftPanel, SettingsContent } from './pages/SettingsPage'

const TRAFFIC_LIGHT_HEIGHT = 38
const SIDEBAR_WIDTH = 280

function TabManagedContent() {
  const { tabs, activeTabId } = useTabContext()
  if (tabs.length === 0) {
    return (
      <div className="flex items-center justify-center h-full">
        <p style={{ fontSize: 14, color: '#9b9a97' }}>打开一个对话或资产开始工作</p>
      </div>
    )
  }
  return (
    <div className="flex flex-col h-full">
      <TabBar />
      <div className="flex-1 overflow-hidden">
        {tabs.map(tab => (
          <div key={tab.id} style={{
            display: tab.id === activeTabId ? 'flex' : 'none',
            height: '100%', flexDirection: 'column',
          }}>
            <LayoutRouter tab={tab} />
          </div>
        ))}
      </div>
    </div>
  )
}

function MainContent({ nav }: { nav: NavTab }) {
  switch (nav) {
    case 'home':     return <HomeContent />
    case 'chat':
    case 'assets':   return <TabManagedContent />
    case 'inbox':    return <InboxContent />
    case 'settings': return <SettingsContent />
  }
}

function LeftPanel({ nav }: { nav: NavTab }) {
  switch (nav) {
    case 'home':     return <HomeLeftPanel />
    case 'chat':     return <ChatLeftPanel />
    case 'assets':   return <AssetsLeftPanel />
    case 'inbox':    return <InboxLeftPanel />
    case 'settings': return <SettingsLeftPanel />
  }
}

function App() {
  const [nav, setNav] = useState<NavTab>('home')
  const [isFullscreen, setIsFullscreen] = useState(false)
  const isElectron = !!window.electronAPI
  // Non-fullscreen: 38px for macOS traffic lights. Fullscreen: 6px breathing room. Browser: 0.
  const sidebarPad = isElectron ? (isFullscreen ? 6 : TRAFFIC_LIGHT_HEIGHT) : 0

  useEffect(() => {
    const unsub = window.electronAPI?.onFullscreenChange(setIsFullscreen)
    return () => unsub?.()
  }, [])

  useEffect(() => {
    const handler = (e: Event) => setNav((e as CustomEvent).detail as NavTab)
    window.addEventListener('nav:goTo', handler)
    return () => window.removeEventListener('nav:goTo', handler)
  }, [])

  return (
    <ErrorBoundary>
    <LocaleProvider>
    <InboxProvider>
    <ChatProvider>
    <TabProvider>
      <div className="flex h-screen w-screen overflow-hidden bg-white text-gray-900">
        <aside style={{ width: SIDEBAR_WIDTH }} className="flex flex-col flex-shrink-0 bg-white relative">
          {sidebarPad > 0 && (
            <div style={{ height: sidebarPad, WebkitAppRegion: 'drag' } as React.CSSProperties} className="flex-shrink-0" />
          )}
          <SidebarNav active={nav} onSelect={setNav} />
          <div className="flex-1 overflow-y-auto">
            <LeftPanel nav={nav} />
          </div>
        </aside>
        <div className="w-px bg-gray-200 flex-shrink-0" />
        <main className="flex flex-col flex-1 min-w-0 h-full overflow-hidden bg-white">
          {isElectron && <div style={{ height: 6, flexShrink: 0, WebkitAppRegion: 'drag' } as React.CSSProperties} />}
          <div className="flex-1 overflow-hidden">
            <MainContent nav={nav} />
          </div>
        </main>
      </div>
    </TabProvider>
    </ChatProvider>
    </InboxProvider>
    </LocaleProvider>
    </ErrorBoundary>
  )
}

export default App
