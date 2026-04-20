import { useState, useRef, useEffect } from 'react'
import { MoreHorizontal } from 'lucide-react'

export interface MenuItem {
  label: string
  onClick: () => void
  danger?: boolean
}

interface Props {
  items: MenuItem[]
}

export function ContextMenu({ items }: Props) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClick)
    return () => document.removeEventListener('mousedown', handleClick)
  }, [open])

  return (
    <div ref={ref} style={{ position: 'relative', flexShrink: 0 }}>
      <button
        onClick={(e) => {
          e.stopPropagation()
          setOpen(v => !v)
        }}
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          width: 22,
          height: 22,
          padding: 0,
          border: 'none',
          borderRadius: 4,
          background: open ? '#f3f4f6' : 'transparent',
          cursor: 'pointer',
          color: '#9b9a97',
          opacity: open ? 1 : 0,
          transition: 'opacity 100ms',
        }}
        className="ctx-btn"
      >
        <MoreHorizontal size={14} />
      </button>

      {open && (
        <div
          style={{
            position: 'absolute',
            right: 0,
            top: '100%',
            marginTop: 4,
            minWidth: 140,
            background: 'white',
            border: '1px solid #e5e7eb',
            borderRadius: 8,
            boxShadow: '0 4px 12px rgba(0,0,0,0.08), 0 1px 3px rgba(0,0,0,0.05)',
            zIndex: 100,
            padding: '4px 0',
            overflow: 'hidden',
          }}
        >
          {items.map((item, i) => (
            <button
              key={i}
              onClick={(e) => {
                e.stopPropagation()
                setOpen(false)
                item.onClick()
              }}
              style={{
                display: 'block',
                width: '100%',
                padding: '6px 12px',
                border: 'none',
                background: 'none',
                cursor: 'pointer',
                fontSize: 13,
                textAlign: 'left',
                color: item.danger ? '#eb5757' : '#374151',
                transition: 'background 80ms',
              }}
              onMouseEnter={(e) => {
                e.currentTarget.style.background = item.danger ? '#fef2f2' : '#f3f4f6'
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.background = 'none'
              }}
            >
              {item.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
