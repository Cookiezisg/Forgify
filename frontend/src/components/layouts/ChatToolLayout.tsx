import { useState, useEffect, useRef, useCallback } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { ChatContent } from '@/pages/ChatContent'
import { ToolMainView } from '@/components/tools/ToolMainView'
import { api } from '@/lib/api'

interface Props {
  conversationId: string
  toolId: string
  tabId: string
}

const MIN_LEFT = 300
const MIN_RIGHT = 320
const COLLAPSED_WIDTH = 36

export function ChatToolLayout({ conversationId, toolId, tabId }: Props) {
  const storageKey = `forgify.split.${conversationId}`
  const [collapsed, setCollapsed] = useState(() => {
    try { return localStorage.getItem(storageKey + '.collapsed') === 'true' } catch { return false }
  })
  const [rightWidth, setRightWidth] = useState(() => {
    try { return parseInt(localStorage.getItem(storageKey + '.width') || '0', 10) || 0 } catch { return 0 }
  })
  const [toolName, setToolName] = useState('')
  const containerRef = useRef<HTMLDivElement>(null)
  const dragging = useRef(false)

  // Load tool name for collapsed strip
  useEffect(() => {
    api<{ displayName: string }>(`/api/tools/${toolId}`)
      .then(t => setToolName(t.displayName))
      .catch(() => setToolName('Tool'))
  }, [toolId])

  // Persist state
  useEffect(() => {
    localStorage.setItem(storageKey + '.collapsed', String(collapsed))
  }, [collapsed, storageKey])
  useEffect(() => {
    if (rightWidth > 0) localStorage.setItem(storageKey + '.width', String(rightWidth))
  }, [rightWidth, storageKey])

  // Initialize rightWidth to 50% of container on first render
  useEffect(() => {
    if (rightWidth === 0 && containerRef.current) {
      setRightWidth(Math.floor(containerRef.current.offsetWidth / 2))
    }
  }, [rightWidth])

  const dragHandleRef = useRef<HTMLDivElement>(null)

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
      // Reset drag handle color when mouse releases
      if (dragHandleRef.current) {
        dragHandleRef.current.style.background = 'transparent'
      }
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }, [])

  if (collapsed) {
    return (
      <div ref={containerRef} className="flex h-full">
        {/* Chat takes full width */}
        <div style={{ flex: 1, minWidth: 0 }}>
          <ChatContent conversationId={conversationId} hideBinding />
        </div>
        {/* Collapsed strip */}
        <div
          onClick={() => setCollapsed(false)}
          style={{
            width: COLLAPSED_WIDTH,
            flexShrink: 0,
            borderLeft: '1px solid #e5e7eb',
            background: '#fafaf9',
            cursor: 'pointer',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            paddingTop: 12,
            gap: 8,
            transition: 'background 100ms',
          }}
          onMouseEnter={(e) => (e.currentTarget.style.background = '#f3f4f6')}
          onMouseLeave={(e) => (e.currentTarget.style.background = '#fafaf9')}
          title={toolName}
        >
          <ChevronLeft size={14} strokeWidth={1.8} style={{ color: '#9b9a97' }} />
          <span style={{ fontSize: 12 }}>📦</span>
          <span style={{
            writingMode: 'vertical-rl',
            textOrientation: 'mixed',
            fontSize: 11,
            color: '#6b7280',
            fontWeight: 500,
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            maxHeight: 150,
          }}>
            {toolName}
          </span>
        </div>
      </div>
    )
  }

  return (
    <div ref={containerRef} className="flex h-full" style={{ overflow: 'hidden' }}>
      {/* Chat pane */}
      <div style={{ flex: 1, minWidth: MIN_LEFT, overflow: 'hidden' }}>
        <ChatContent conversationId={conversationId} hideBinding />
      </div>

      {/* Drag handle */}
      <div
        ref={dragHandleRef}
        onMouseDown={onResizeStart}
        style={{
          width: 4,
          cursor: 'col-resize',
          background: 'transparent',
          flexShrink: 0,
          transition: 'background 100ms',
        }}
        onMouseEnter={(e) => (e.currentTarget.style.background = '#2383e2')}
        onMouseLeave={(e) => {
          if (!dragging.current) e.currentTarget.style.background = 'transparent'
        }}
      />

      {/* Tool pane */}
      <div style={{
        width: rightWidth || '50%',
        flexShrink: 0,
        minWidth: MIN_RIGHT,
        borderLeft: '1px solid #e5e7eb',
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column',
      }}>
        {/* Collapse button header */}
        <div style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-end',
          padding: '4px 8px',
          borderBottom: '1px solid #f3f4f6',
          flexShrink: 0,
        }}>
          <button
            onClick={() => setCollapsed(true)}
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
        {/* Tool view */}
        <div style={{ flex: 1, overflow: 'hidden' }}>
          <ToolMainView toolId={toolId} onDeleted={() => {}} />
        </div>
      </div>
    </div>
  )
}
