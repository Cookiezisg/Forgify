import { useState, useEffect, useCallback } from 'react'
import { Plus, Search, Upload, Trash2, RotateCcw } from 'lucide-react'
import { api } from '@/lib/api'
import { ToolCard } from '@/components/tools/ToolCard'
import { useTabContext } from '@/context/TabContext'
import { useT } from '@/lib/i18n'

interface Tool {
  id: string
  name: string
  displayName: string
  description: string
  category: string
  status: string
  builtin: boolean
  lastTestAt?: string
  lastTestPassed?: boolean
}

const CATEGORIES = ['all', 'email', 'data', 'web', 'file', 'system', 'other'] as const

export function AssetsLeftPanel() {
  const t = useT()
  const { openTab } = useTabContext()
  const [tools, setTools] = useState<Tool[]>([])
  const [query, setQuery] = useState('')
  const [category, setCategory] = useState('all')

  const load = useCallback(() => {
    const params = new URLSearchParams()
    if (category !== 'all') params.set('category', category)
    if (query) params.set('q', query)
    api<Tool[]>(`/api/tools?${params}`).then(setTools).catch(() => {})
  }, [category, query])

  useEffect(() => { load() }, [load])

  // Refresh when tools change (delete, restore, create)
  useEffect(() => {
    const handler = () => load()
    window.addEventListener('tool:changed', handler)
    return () => window.removeEventListener('tool:changed', handler)
  }, [load])

  const handleNewTool = () => {
    window.dispatchEvent(new CustomEvent('nav:goTo', { detail: 'chat' }))
  }

  const handleImport = async () => {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = '.forgify-tool,.json'
    input.onchange = async () => {
      const file = input.files?.[0]
      if (!file) return
      const text = await file.text()
      try {
        const data = JSON.parse(text)
        await api('/api/tools/import/confirm', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ data, action: 'new' }),
        })
        load()
      } catch (e: any) {
        alert(e.message)
      }
    }
    input.click()
  }

  const handleOpenTool = useCallback((tool: Tool) => {
    openTab({ layout: 'tool', label: tool.displayName, toolId: tool.id })
  }, [openTab])

  const categoryLabel = (c: string) => {
    const map: Record<string, () => string> = {
      all: () => t('tools.all'),
      email: () => t('tools.email'),
      data: () => t('tools.data'),
      web: () => t('tools.web'),
      file: () => t('tools.file'),
      system: () => t('tools.system'),
      other: () => t('tools.other'),
    }
    return map[c]?.() || c
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div style={{ padding: '8px 12px 4px' }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 6 }}>
          <p style={{ fontSize: 11, fontWeight: 600, color: '#9b9a97', textTransform: 'uppercase', letterSpacing: '0.05em', padding: '0 2px' }}>
            {t('tools.title')}
          </p>
          <div style={{ display: 'flex', gap: 4 }}>
            <button onClick={handleImport} title={t('tools.import')} style={{
              width: 24, height: 24, borderRadius: 5, border: 'none',
              background: 'transparent', cursor: 'pointer', color: '#9b9a97',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
            }}>
              <Upload size={13} strokeWidth={1.8} />
            </button>
            <button onClick={handleNewTool} title={t('tools.newTool')} style={{
              width: 24, height: 24, borderRadius: 5, border: 'none',
              background: 'transparent', cursor: 'pointer', color: '#9b9a97',
              display: 'flex', alignItems: 'center', justifyContent: 'center',
            }}>
              <Plus size={14} strokeWidth={2} />
            </button>
          </div>
        </div>

        {/* Search */}
        <div style={{
          display: 'flex', alignItems: 'center', gap: 6,
          padding: '5px 8px', borderRadius: 6, background: '#f7f7f5',
          border: '1px solid transparent', transition: 'border-color 150ms', marginBottom: 6,
        }}
          onFocusCapture={(e) => { e.currentTarget.style.borderColor = '#d1d5db' }}
          onBlurCapture={(e) => {
            if (!e.currentTarget.contains(e.relatedTarget)) e.currentTarget.style.borderColor = 'transparent'
          }}
        >
          <Search size={13} strokeWidth={1.8} style={{ color: '#9b9a97', flexShrink: 0 }} />
          <input value={query} onChange={(e) => setQuery(e.target.value)}
            placeholder={t('tools.search')}
            style={{ flex: 1, border: 'none', background: 'transparent', outline: 'none', fontSize: 13, color: '#1a1a1a' }}
          />
        </div>

        {/* Category filter */}
        <div style={{ display: 'flex', gap: 3, flexWrap: 'wrap', padding: '0 0 4px' }}>
          {CATEGORIES.map((c) => (
            <button key={c} onClick={() => setCategory(c)} style={{
              padding: '3px 8px', fontSize: 11, borderRadius: 10,
              border: 'none', cursor: 'pointer',
              background: category === c ? '#111827' : '#f3f4f6',
              color: category === c ? 'white' : '#6b7280',
              transition: 'all 100ms',
            }}>
              {categoryLabel(c)}
            </button>
          ))}
        </div>
      </div>

      {/* Tool list */}
      <div style={{ flex: 1, overflowY: 'auto', padding: '0 8px' }}>
        {tools.length === 0 ? (
          <div style={{ padding: '40px 16px', textAlign: 'center' }}>
            <p style={{ fontSize: 14, fontWeight: 500, color: '#374151', marginBottom: 6 }}>{t('tools.emptyTitle')}</p>
            <p style={{ fontSize: 12, color: '#9b9a97', marginBottom: 14 }}>{t('tools.emptyHint')}</p>
            <button onClick={handleNewTool} style={{
              padding: '6px 16px', fontSize: 13, borderRadius: 6,
              border: 'none', background: '#111827', color: 'white', cursor: 'pointer',
            }}>{t('tools.emptyAction')}</button>
          </div>
        ) : (
          tools.map((tool) => (
            <ToolCard key={tool.id} tool={tool} active={false}
              onClick={() => handleOpenTool(tool)} />
          ))
        )}

      </div>

      {/* Recycle Bin — fixed at bottom of sidebar */}
      <div style={{ flexShrink: 0, borderTop: '1px solid #f3f4f6', padding: '0 8px' }}>
        <RecycleBin onRestored={load} />
      </div>
    </div>
  )
}

function RecycleBin({ onRestored }: { onRestored: () => void }) {
  const [show, setShow] = useState(false)
  const [deleted, setDeleted] = useState<Tool[]>([])

  const loadDeleted = useCallback(() => {
    api<Tool[]>('/api/tools/deleted').then(setDeleted).catch(() => {})
  }, [])

  useEffect(() => { if (show) loadDeleted() }, [show, loadDeleted])

  const handleRestore = async (id: string) => {
    await api(`/api/tools/${id}/restore`, { method: 'POST' }).catch(() => {})
    window.dispatchEvent(new CustomEvent('tool:changed'))
    loadDeleted()
    onRestored()
  }

  const handlePermanent = async (id: string) => {
    if (!window.confirm('永久删除此工具？此操作不可恢复。')) return
    await api(`/api/tools/${id}/permanent`, { method: 'DELETE' }).catch(() => {})
    window.dispatchEvent(new CustomEvent('tool:changed'))
    loadDeleted()
  }

  return (
    <div style={{ marginTop: 8 }}>
      <button
        onClick={() => setShow(!show)}
        style={{
          display: 'flex', alignItems: 'center', gap: 6, width: '100%',
          padding: '6px 10px', borderRadius: 6, border: 'none',
          background: 'transparent', cursor: 'pointer', fontSize: 12, color: '#9b9a97',
        }}
        onMouseEnter={(e) => (e.currentTarget.style.background = '#f9fafb')}
        onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
      >
        <Trash2 size={12} strokeWidth={1.6} />
        {show ? '隐藏回收站' : '回收站'}
        {!show && deleted.length > 0 && (
          <span style={{ fontSize: 10, color: '#c7c7c5' }}>({deleted.length})</span>
        )}
      </button>

      {show && deleted.map(tool => (
        <div key={tool.id} style={{
          display: 'flex', alignItems: 'center', gap: 6,
          padding: '4px 10px', fontSize: 12, color: '#9b9a97',
        }}>
          <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {tool.displayName}
          </span>
          <button onClick={() => handleRestore(tool.id)} title="恢复"
            style={{ border: 'none', background: 'none', cursor: 'pointer', color: '#9b9a97', padding: 2, display: 'flex' }}
            onMouseEnter={(e) => (e.currentTarget.style.color = '#374151')}
            onMouseLeave={(e) => (e.currentTarget.style.color = '#9b9a97')}
          >
            <RotateCcw size={12} />
          </button>
          <button onClick={() => handlePermanent(tool.id)} title="永久删除"
            style={{ border: 'none', background: 'none', cursor: 'pointer', color: '#9b9a97', padding: 2, display: 'flex' }}
            onMouseEnter={(e) => (e.currentTarget.style.color = '#dc2626')}
            onMouseLeave={(e) => (e.currentTarget.style.color = '#9b9a97')}
          >
            <Trash2 size={12} />
          </button>
        </div>
      ))}
      {show && deleted.length === 0 && (
        <p style={{ fontSize: 11, color: '#c7c7c5', padding: '4px 10px' }}>回收站为空</p>
      )}
    </div>
  )
}

// AssetsContent is no longer needed — tools open as tabs via LayoutRouter
