import { useState, useEffect, useRef, useCallback } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { ChatContent } from '@/pages/ChatContent'
import { ToolMainView } from '@/components/tools/ToolMainView'
import { api } from '@/lib/api'

interface Props {
  conversationId: string
  toolId: string
  tabId: string
  chatLabel: string
}

const MIN_LEFT = 300
const MIN_RIGHT = 320
const COLLAPSED_WIDTH = 36

type CollapsedSide = 'none' | 'left' | 'right'

export function ChatToolLayout({ conversationId, toolId, tabId, chatLabel }: Props) {
  const storageKey = `forgify.split.${conversationId}`
  const [collapsedSide, setCollapsedSide] = useState<CollapsedSide>(() => {
    try {
      const v = localStorage.getItem(storageKey + '.collapsed')
      if (v === 'left' || v === 'right') return v
      if (v === 'true') return 'right' // backwards compat
      return 'none'
    } catch { return 'none' }
  })
  const [rightWidth, setRightWidth] = useState(() => {
    try { return parseInt(localStorage.getItem(storageKey + '.width') || '0', 10) || 0 } catch { return 0 }
  })
  const [toolName, setToolName] = useState('')
  const containerRef = useRef<HTMLDivElement>(null)
  const dragging = useRef(false)
  const dragHandleRef = useRef<HTMLDivElement>(null)

  // Load tool name for collapsed strip
  useEffect(() => {
    api<{ displayName: string }>(`/api/tools/${toolId}`)
      .then(t => setToolName(t.displayName))
      .catch(() => setToolName('Tool'))
  }, [toolId])

  // Persist state
  useEffect(() => {
    localStorage.setItem(storageKey + '.collapsed', collapsedSide)
  }, [collapsedSide, storageKey])
  useEffect(() => {
    if (rightWidth > 0) localStorage.setItem(storageKey + '.width', String(rightWidth))
  }, [rightWidth, storageKey])

  // Initialize rightWidth to 50% of container on first render
  useEffect(() => {
    if (rightWidth === 0 && containerRef.current) {
      setRightWidth(Math.floor(containerRef.current.offsetWidth / 2))
    }
  }, [rightWidth])

  const onResizeStart = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    dragging.current = true
    const onMove = (ev: MouseEvent) => {
      if (!dragging.current || !containerRef.current) return
      const rect = containerRef.current.getBoundingClientRect()
      const leftWidth = ev.clientX - rect.left
      const newRight = rect.width - leftWidth
      if (leftWidth >= MIN_LEFT && newRight >= MIN_RIGHT) {
        setRightWidth(Math.round(newRight))
      }
    }
    const onUp = () => {
      dragging.current = false
      if (dragHandleRef.current) dragHandleRef.current.style.background = 'transparent'
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }, [])

  /* ─── Left collapsed ─── */
  if (collapsedSide === 'left') {
    return (
      <div ref={containerRef} className="flex h-full">
        <CollapsedStrip
          side="left"
          icon="💬"
          label={chatLabel || '对话'}
          onExpand={() => setCollapsedSide('none')}
        />
        <div style={{ flex: 1, borderLeft: '1px solid #e5e7eb', overflow: 'hidden' }}>
          <ToolMainView toolId={toolId} conversationId={conversationId} onDeleted={() => {
            window.dispatchEvent(new CustomEvent('tool:changed'))
          }} />
        </div>
      </div>
    )
  }

  /* ─── Right collapsed ─── */
  if (collapsedSide === 'right') {
    return (
      <div ref={containerRef} className="flex h-full">
        <div style={{ flex: 1, minWidth: 0 }}>
          <ChatContent conversationId={conversationId} hideBinding />
        </div>
        <CollapsedStrip
          side="right"
          icon="📦"
          label={toolName || 'Tool'}
          onExpand={() => setCollapsedSide('none')}
        />
      </div>
    )
  }

  /* ─── Both expanded ─── */
  return (
    <div ref={containerRef} className="flex h-full" style={{ overflow: 'hidden' }}>
      {/* Chat pane — floating collapse button with gradient */}
      <div style={{ position: 'relative', flex: 1, minWidth: MIN_LEFT, overflow: 'hidden' }}>
        <ChatContent conversationId={conversationId} hideBinding />
        {/* Gradient overlay + collapse button */}
        <div style={{
          position: 'absolute', top: 0, left: 0, right: 0, height: 40,
          background: 'linear-gradient(to bottom, rgba(255,255,255,0.95) 30%, rgba(255,255,255,0) 100%)',
          pointerEvents: 'none', zIndex: 5,
          display: 'flex', alignItems: 'flex-start', justifyContent: 'flex-end',
          padding: '8px 10px',
        }}>
          <button
            onClick={() => setCollapsedSide('left')}
            title="收起对话"
            style={{
              pointerEvents: 'auto',
              width: 24, height: 24, borderRadius: 4, border: 'none',
              background: 'transparent', cursor: 'pointer', color: '#9b9a97',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
            }}
            onMouseEnter={(e) => (e.currentTarget.style.color = '#374151')}
            onMouseLeave={(e) => (e.currentTarget.style.color = '#9b9a97')}
          >
            <ChevronLeft size={14} strokeWidth={2} />
          </button>
        </div>
      </div>

      {/* Drag handle */}
      <div
        ref={dragHandleRef}
        onMouseDown={onResizeStart}
        style={{
          width: 4, cursor: 'col-resize', background: 'transparent',
          flexShrink: 0, transition: 'background 100ms',
        }}
        onMouseEnter={(e) => (e.currentTarget.style.background = '#2383e2')}
        onMouseLeave={(e) => {
          if (!dragging.current) e.currentTarget.style.background = 'transparent'
        }}
      />

      {/* Tool pane — header bar with collapse button */}
      <div style={{
        width: rightWidth || '50%', flexShrink: 0, minWidth: MIN_RIGHT,
        borderLeft: '1px solid #e5e7eb', overflow: 'hidden',
        display: 'flex', flexDirection: 'column',
      }}>
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'flex-end',
          padding: '4px 8px', borderBottom: '1px solid #f3f4f6', flexShrink: 0,
        }}>
          <button
            onClick={() => setCollapsedSide('right')}
            title="收起"
            style={{
              width: 24, height: 24, borderRadius: 4, border: 'none',
              background: 'transparent', cursor: 'pointer', color: '#9b9a97',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
            }}
            onMouseEnter={(e) => (e.currentTarget.style.color = '#374151')}
            onMouseLeave={(e) => (e.currentTarget.style.color = '#9b9a97')}
          >
            <ChevronRight size={14} strokeWidth={2} />
          </button>
        </div>
        <div style={{ flex: 1, overflow: 'hidden' }}>
          <ToolMainView toolId={toolId} conversationId={conversationId} onDeleted={() => {
            window.dispatchEvent(new CustomEvent('tool:changed'))
          }} />
        </div>
      </div>
    </div>
  )
}

/* ─── Collapsed strip (shared by both sides) ─── */

function CollapsedStrip({ side, icon, label, onExpand }: {
  side: 'left' | 'right'
  icon: string
  label: string
  onExpand: () => void
}) {
  const chevron = side === 'left'
    ? <ChevronRight size={14} strokeWidth={1.8} style={{ color: '#9b9a97' }} />
    : <ChevronLeft size={14} strokeWidth={1.8} style={{ color: '#9b9a97' }} />

  return (
    <div
      onClick={onExpand}
      style={{
        width: COLLAPSED_WIDTH, flexShrink: 0,
        borderLeft: side === 'right' ? '1px solid #e5e7eb' : undefined,
        borderRight: side === 'left' ? '1px solid #e5e7eb' : undefined,
        background: '#fafaf9', cursor: 'pointer',
        display: 'flex', flexDirection: 'column', alignItems: 'center',
        paddingTop: 12, gap: 8, transition: 'background 100ms',
      }}
      onMouseEnter={(e) => (e.currentTarget.style.background = '#f3f4f6')}
      onMouseLeave={(e) => (e.currentTarget.style.background = '#fafaf9')}
      title={label}
    >
      {chevron}
      <span style={{ fontSize: 12 }}>{icon}</span>
      <span style={{
        writingMode: 'vertical-rl', textOrientation: 'mixed',
        fontSize: 11, color: '#6b7280', fontWeight: 500,
        overflow: 'hidden', textOverflow: 'ellipsis', maxHeight: 150,
      }}>
        {label}
      </span>
    </div>
  )
}
