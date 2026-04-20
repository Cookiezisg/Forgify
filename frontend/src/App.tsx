import { useState, useEffect } from 'react'
import { LocaleProvider } from './lib/i18n'
import { SidebarNav, type NavTab } from './components/SidebarNav'
import { HomeLeftPanel, HomeContent } from './pages/HomePage'
import { ChatLeftPanel } from './pages/ChatPage'
import { AssetsLeftPanel } from './pages/AssetsPage'
import { InboxLeftPanel, InboxContent } from './pages/InboxPage'
import { SettingsLeftPanel, SettingsContent } from './pages/SettingsPage'

const TRAFFIC_LIGHT_HEIGHT = 38
const SIDEBAR_WIDTH = 280

function LeftPanel({ nav }: { nav: NavTab }) {
  switch (nav) {
    case 'home':     return <HomeLeftPanel />
    case 'chat':     return <ChatLeftPanel />
    case 'assets':   return <AssetsLeftPanel />
    case 'inbox':    return <InboxLeftPanel />
    case 'settings': return <SettingsLeftPanel />
  }
}

function MainContent({ nav }: { nav: NavTab }) {
  switch (nav) {
    case 'home':     return <HomeContent />
    case 'chat':     return <div className="flex items-center justify-center h-full"><p style={{ color: '#9b9a97' }}>Chat UI coming next</p></div>
    case 'assets':   return <div className="flex items-center justify-center h-full"><p style={{ color: '#9b9a97' }}>Assets UI coming next</p></div>
    case 'inbox':    return <InboxContent />
    case 'settings': return <SettingsContent />
  }
}

function App() {
  const [nav, setNav] = useState<NavTab>('home')
  const [isFullscreen, setIsFullscreen] = useState(false)
  const isElectron = !!window.electronAPI
  const sidebarPad = isElectron && !isFullscreen ? TRAFFIC_LIGHT_HEIGHT : 0

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
    <LocaleProvider>
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
        <main className="flex-1 min-w-0 h-full overflow-hidden bg-white">
          <MainContent nav={nav} />
        </main>
      </div>
    </LocaleProvider>
  )
}

export default App
