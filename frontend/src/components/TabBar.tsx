import { useRef } from 'react'
import { Plus, X, Home, MessageCircle, Zap, Inbox, Settings } from 'lucide-react'
import { useTabContext, type TabItem } from '@/context/TabContext'

const LAYOUT_ICONS: Record<string, React.ReactNode> = {
  home: <Home size={13} strokeWidth={1.6} />,
  chat: <MessageCircle size={13} strokeWidth={1.6} />,
  'chat-tool': <MessageCircle size={13} strokeWidth={1.6} />,
  'chat-workflow': <MessageCircle size={13} strokeWidth={1.6} />,
  tool: <Zap size={13} strokeWidth={1.6} />,
  workflow: <Zap size={13} strokeWidth={1.6} />,
  inbox: <Inbox size={13} strokeWidth={1.6} />,
  settings: <Settings size={13} strokeWidth={1.6} />,
}

export function TabBar() {
  const { tabs, activeTabId, setActiveTab, closeTab, openTab } = useTabContext()
  const scrollRef = useRef<HTMLDivElement>(null)

  const handleNewChat = () => {
    openTab({ layout: 'chat', label: '新对话', icon: '💬' })
  }

  return (
    <div
      style={{
        height: 36,
        borderBottom: '1px solid #e5e7eb',
        display: 'flex',
        alignItems: 'stretch',
        flexShrink: 0,
        background: 'white',
      }}
    >
      <div
        ref={scrollRef}
        style={{
          flex: 1,
          display: 'flex',
          alignItems: 'stretch',
          overflowX: 'auto',
          scrollbarWidth: 'none',
        }}
      >
        {tabs.map((tab) => (
          <TabButton
            key={tab.id}
            tab={tab}
            active={tab.id === activeTabId}
            onClick={() => setActiveTab(tab.id)}
            onClose={() => closeTab(tab.id)}
          />
        ))}
      </div>

      {/* New tab button */}
      <button
        onClick={handleNewChat}
        title="新建对话"
        style={{
          width: 32,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          border: 'none',
          background: 'transparent',
          cursor: 'pointer',
          color: '#9b9a97',
          flexShrink: 0,
          borderLeft: '1px solid #f3f4f6',
        }}
        onMouseEnter={(e) => (e.currentTarget.style.color = '#374151')}
        onMouseLeave={(e) => (e.currentTarget.style.color = '#9b9a97')}
      >
        <Plus size={14} strokeWidth={2} />
      </button>
    </div>
  )
}

function TabButton({
  tab,
  active,
  onClick,
  onClose,
}: {
  tab: TabItem
  active: boolean
  onClick: () => void
  onClose: () => void
}) {
  return (
    <div
      onClick={onClick}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 5,
        padding: '0 10px',
        maxWidth: 180,
        minWidth: 60,
        cursor: 'pointer',
        fontSize: 12,
        color: active ? '#111827' : '#6b7280',
        background: active ? 'white' : '#fafaf9',
        borderRight: '1px solid #f3f4f6',
        borderBottom: active ? '2px solid #111827' : '2px solid transparent',
        transition: 'color 100ms, background 100ms',
        userSelect: 'none',
        flexShrink: 0,
      }}
      onMouseEnter={(e) => {
        if (!active) e.currentTarget.style.background = '#f3f4f6'
      }}
      onMouseLeave={(e) => {
        if (!active) e.currentTarget.style.background = '#fafaf9'
      }}
    >
      <span style={{ flexShrink: 0, display: 'flex', alignItems: 'center', color: active ? '#111827' : '#9b9a97' }}>
        {LAYOUT_ICONS[tab.layout] || null}
      </span>
      <span style={{
        flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        fontWeight: active ? 500 : 400,
      }}>
        {tab.label}
      </span>
      {!tab.pinned && (
        <button
          onClick={(e) => {
            e.stopPropagation()
            onClose()
          }}
          style={{
            width: 16, height: 16, padding: 0, border: 'none', background: 'none',
            cursor: 'pointer', color: '#c7c7c5', borderRadius: 3, flexShrink: 0,
            display: 'flex', alignItems: 'center', justifyContent: 'center',
          }}
          onMouseEnter={(e) => {
            e.currentTarget.style.color = '#6b7280'
            e.currentTarget.style.background = '#e5e7eb'
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.color = '#c7c7c5'
            e.currentTarget.style.background = 'none'
          }}
        >
          <X size={10} strokeWidth={2} />
        </button>
      )}
    </div>
  )
}
