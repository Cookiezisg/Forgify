import { useState, useEffect, useRef, useCallback } from 'react'
import { Plus, X, Pin, Home, MessageCircle, Zap, Inbox, Settings } from 'lucide-react'
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

/* ─── Main tab bar ─────────────────────────────────────── */

export function TabBar() {
  const {
    tabs, activeTabId, setActiveTab, closeTab, openTab, reorderTab,
  } = useTabContext()
  const scrollRef = useRef<HTMLDivElement>(null)

  // Drag state
  const [dragIdx, setDragIdx] = useState<number | null>(null)
  const [dropTarget, setDropTarget] = useState<{ index: number; side: 'left' | 'right' } | null>(null)

  // Context menu state
  const [ctxMenu, setCtxMenu] = useState<{ x: number; y: number; tabId: string } | null>(null)

  // Dismiss context menu on outside click / escape
  useEffect(() => {
    if (!ctxMenu) return
    const dismiss = () => setCtxMenu(null)
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') dismiss() }
    window.addEventListener('mousedown', dismiss)
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('mousedown', dismiss)
      window.removeEventListener('keydown', onKey)
    }
  }, [ctxMenu])

  // Drag handlers
  const handleDragOver = useCallback((e: React.DragEvent, index: number) => {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    const rect = e.currentTarget.getBoundingClientRect()
    const side: 'left' | 'right' = e.clientX < rect.left + rect.width / 2 ? 'left' : 'right'
    setDropTarget(prev =>
      prev?.index === index && prev.side === side ? prev : { index, side },
    )
  }, [])

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault()
    if (dragIdx == null || !dropTarget) return
    let to = dropTarget.side === 'right' ? dropTarget.index + 1 : dropTarget.index
    if (dragIdx < to) to--
    if (dragIdx !== to) reorderTab(dragIdx, to)
    setDragIdx(null)
    setDropTarget(null)
  }, [dragIdx, dropTarget, reorderTab])

  const endDrag = useCallback(() => {
    setDragIdx(null)
    setDropTarget(null)
  }, [])

  return (
    <div style={{
      height: 36, borderBottom: '1px solid #e5e7eb',
      display: 'flex', alignItems: 'stretch', flexShrink: 0, background: 'white',
      WebkitAppRegion: 'drag',
    } as React.CSSProperties}>
      {/* Scrollable tab strip */}
      <div ref={scrollRef} style={{
        flex: 1, display: 'flex', alignItems: 'stretch',
        overflowX: 'auto', scrollbarWidth: 'none',
      }}>
        {tabs.map((tab, i) => (
          <TabButton
            key={tab.id}
            tab={tab}
            active={tab.id === activeTabId}
            isDragging={dragIdx === i}
            dropSide={dropTarget?.index === i && dragIdx !== i ? dropTarget.side : null}
            onClick={() => setActiveTab(tab.id)}
            onClose={() => closeTab(tab.id)}
            onContextMenu={(e) => { e.preventDefault(); setCtxMenu({ x: e.clientX, y: e.clientY, tabId: tab.id }) }}
            onDragStart={(e) => { e.dataTransfer.effectAllowed = 'move'; setDragIdx(i) }}
            onDragOver={(e) => handleDragOver(e, i)}
            onDrop={handleDrop}
            onDragEnd={endDrag}
          />
        ))}
      </div>

      {/* New-tab button */}
      <button
        onClick={() => openTab({ layout: 'chat', label: '新对话', icon: '💬' })}
        title="新建对话"
        style={{
          width: 32, display: 'flex', alignItems: 'center', justifyContent: 'center',
          border: 'none', background: 'transparent', cursor: 'pointer', color: '#9b9a97',
          flexShrink: 0, borderLeft: '1px solid #f3f4f6',
          WebkitAppRegion: 'no-drag',
        } as React.CSSProperties}
        onMouseEnter={(e) => (e.currentTarget.style.color = '#374151')}
        onMouseLeave={(e) => (e.currentTarget.style.color = '#9b9a97')}
      >
        <Plus size={14} strokeWidth={2} />
      </button>

      {/* Right-click context menu */}
      {ctxMenu && (
        <TabContextMenu
          x={ctxMenu.x} y={ctxMenu.y} tabId={ctxMenu.tabId}
          onClose={() => setCtxMenu(null)}
        />
      )}
    </div>
  )
}

/* ─── Single tab button ────────────────────────────────── */

function TabButton({ tab, active, isDragging, dropSide, onClick, onClose, onContextMenu, onDragStart, onDragOver, onDrop, onDragEnd }: {
  tab: TabItem
  active: boolean
  isDragging: boolean
  dropSide: 'left' | 'right' | null
  onClick: () => void
  onClose: () => void
  onContextMenu: (e: React.MouseEvent) => void
  onDragStart: (e: React.DragEvent) => void
  onDragOver: (e: React.DragEvent) => void
  onDrop: (e: React.DragEvent) => void
  onDragEnd: () => void
}) {
  return (
    <div
      draggable
      onClick={onClick}
      onContextMenu={onContextMenu}
      onDragStart={onDragStart}
      onDragOver={onDragOver}
      onDrop={onDrop}
      onDragEnd={onDragEnd}
      style={{
        display: 'flex', alignItems: 'center', gap: 5,
        padding: '0 10px', maxWidth: 180, minWidth: 60,
        cursor: 'pointer', fontSize: 12,
        color: active ? '#111827' : '#6b7280',
        background: active ? 'white' : '#fafaf9',
        borderRight: '1px solid #f3f4f6',
        borderBottom: active ? '2px solid #111827' : '2px solid transparent',
        transition: 'color 100ms, background 100ms',
        userSelect: 'none', flexShrink: 0,
        opacity: isDragging ? 0.4 : 1,
        boxShadow: dropSide === 'left' ? 'inset 2px 0 0 #3b82f6'
                 : dropSide === 'right' ? 'inset -2px 0 0 #3b82f6'
                 : undefined,
        WebkitAppRegion: 'no-drag',
      } as React.CSSProperties}
      onMouseEnter={(e) => { if (!active) e.currentTarget.style.background = '#f3f4f6' }}
      onMouseLeave={(e) => { if (!active) e.currentTarget.style.background = '#fafaf9' }}
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
      {tab.pinned ? (
        <Pin size={11} strokeWidth={2} style={{ flexShrink: 0, color: '#9b9a97' }} />
      ) : (
        <button
          onMouseDown={(e) => e.stopPropagation()}
          onClick={(e) => { e.stopPropagation(); onClose() }}
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

/* ─── Right-click context menu ─────────────────────────── */

function TabContextMenu({ x, y, tabId, onClose }: {
  x: number; y: number; tabId: string; onClose: () => void
}) {
  const { tabs, closeTab, closeOtherTabs, closeTabsToRight, closeAllTabs, togglePin } = useTabContext()
  const tab = tabs.find(t => t.id === tabId)
  const tabIdx = tabs.findIndex(t => t.id === tabId)
  if (!tab) return null

  const hasOthers = tabs.some(t => t.id !== tabId && !t.pinned)
  const hasRight = tabs.some((t, i) => i > tabIdx && !t.pinned)
  const hasCloseable = tabs.some(t => !t.pinned)

  // Keep menu on screen
  const mx = Math.min(x, window.innerWidth - 200)
  const my = Math.min(y, window.innerHeight - 220)

  type Item = { label: string; action: () => void; disabled?: boolean } | 'sep'
  const items: Item[] = [
    { label: tab.pinned ? '取消固定' : '固定标签页', action: () => togglePin(tabId) },
    'sep',
    { label: '关闭', action: () => closeTab(tabId) },
    { label: '关闭其他标签页', action: () => closeOtherTabs(tabId), disabled: !hasOthers },
    { label: '关闭右侧标签页', action: () => closeTabsToRight(tabId), disabled: !hasRight },
    { label: '关闭所有标签页', action: () => closeAllTabs(), disabled: !hasCloseable },
  ]

  return (
    <div
      onMouseDown={(e) => e.stopPropagation()}
      style={{
        position: 'fixed', top: my, left: mx, zIndex: 9999,
        background: 'white', border: '1px solid #e5e7eb', borderRadius: 6,
        boxShadow: '0 4px 16px rgba(0,0,0,0.12), 0 1px 3px rgba(0,0,0,0.08)',
        padding: '4px 0', minWidth: 180, fontSize: 12,
      }}
    >
      {items.map((item, i) =>
        item === 'sep' ? (
          <div key={i} style={{ height: 1, background: '#f3f4f6', margin: '4px 0' }} />
        ) : (
          <button
            key={i}
            disabled={item.disabled}
            onClick={() => { item.action(); onClose() }}
            style={{
              display: 'block', width: '100%', padding: '6px 12px',
              border: 'none', background: 'none', textAlign: 'left',
              cursor: item.disabled ? 'default' : 'pointer',
              color: item.disabled ? '#c7c7c5' : '#374151', fontSize: 12,
            }}
            onMouseEnter={(e) => { if (!(e.currentTarget as HTMLButtonElement).disabled) e.currentTarget.style.background = '#f3f4f6' }}
            onMouseLeave={(e) => { e.currentTarget.style.background = 'none' }}
          >
            {item.label}
          </button>
        ),
      )}
    </div>
  )
}
