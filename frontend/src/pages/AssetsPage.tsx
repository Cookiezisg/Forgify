import { useState, useEffect, useCallback } from 'react'
import { Plus, Search, Upload, MessageCircle } from 'lucide-react'
import { api } from '@/lib/api'
import { ToolCard } from '@/components/tools/ToolCard'
import { ToolMainView } from '@/components/tools/ToolMainView'
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
  const [tools, setTools] = useState<Tool[]>([])
  const [query, setQuery] = useState('')
  const [category, setCategory] = useState('all')
  const [activeId, setActiveId] = useState<string | null>(null)

  const load = useCallback(() => {
    const params = new URLSearchParams()
    if (category !== 'all') params.set('category', category)
    if (query) params.set('q', query)
    api<Tool[]>(`/api/tools?${params}`).then(setTools).catch(() => {})
  }, [category, query])

  useEffect(() => { load() }, [load])

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

  // Expose activeId to content panel
  useEffect(() => {
    window.dispatchEvent(new CustomEvent('assets:toolChange', { detail: activeId }))
  }, [activeId])

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
            <ToolCard key={tool.id} tool={tool} active={activeId === tool.id}
              onClick={() => setActiveId(tool.id)} />
          ))
        )}
      </div>
    </div>
  )
}

interface AssetConversation {
  id: string
  title: string
  updatedAt: string
}

function AssetMiniSidebar({ toolId }: { toolId: string }) {
  const t = useT()
  const [convs, setConvs] = useState<AssetConversation[]>([])

  useEffect(() => {
    api<AssetConversation[]>(`/api/asset-conversations/${toolId}`)
      .then(setConvs)
      .catch(() => setConvs([]))
  }, [toolId])

  const handleNewConv = async () => {
    try {
      await api('/api/conversations', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ assetId: toolId, assetType: 'tool' }),
      })
      // Navigate to chat tab
      window.dispatchEvent(new CustomEvent('nav:goTo', { detail: 'chat' }))
    } catch {}
  }

  const handleGoToConv = (convId: string) => {
    // Navigate to chat tab and select conversation
    window.dispatchEvent(new CustomEvent('nav:goTo', { detail: 'chat' }))
  }

  return (
    <div style={{
      width: 200, borderRight: '1px solid #e5e7eb', flexShrink: 0,
      display: 'flex', flexDirection: 'column', height: '100%',
    }}>
      <div style={{ padding: '10px 12px 6px' }}>
        <p style={{ fontSize: 11, fontWeight: 600, color: '#9b9a97', textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: 8 }}>
          {t('nav.chat')}
        </p>
        <button onClick={handleNewConv} style={{
          display: 'flex', alignItems: 'center', gap: 4, width: '100%',
          padding: '5px 8px', borderRadius: 5, border: 'none',
          background: 'transparent', cursor: 'pointer', fontSize: 12, color: '#374151',
          transition: 'background 100ms',
        }}
          onMouseEnter={(e) => (e.currentTarget.style.background = '#f3f4f6')}
          onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
        >
          <Plus size={12} strokeWidth={2} />
          {t('chat.newChat')}
        </button>
      </div>
      <div style={{ flex: 1, overflowY: 'auto', padding: '0 8px' }}>
        {convs.length === 0 ? (
          <p style={{ fontSize: 11, color: '#c7c7c5', padding: '8px 6px' }}>{t('chat.noChats')}</p>
        ) : (
          convs.map((c) => (
            <div key={c.id} onClick={() => handleGoToConv(c.id)} style={{
              display: 'flex', alignItems: 'center', gap: 6,
              padding: '5px 8px', borderRadius: 5, cursor: 'pointer',
              fontSize: 12, color: '#374151', transition: 'background 100ms',
            }}
              onMouseEnter={(e) => (e.currentTarget.style.background = '#f9fafb')}
              onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
            >
              <MessageCircle size={11} strokeWidth={1.6} style={{ color: '#9b9a97', flexShrink: 0 }} />
              <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {c.title}
              </span>
            </div>
          ))
        )}
      </div>
    </div>
  )
}

export function AssetsContent() {
  const t = useT()
  const [activeToolId, setActiveToolId] = useState<string | null>(null)

  useEffect(() => {
    const handler = (e: Event) => setActiveToolId((e as CustomEvent).detail)
    window.addEventListener('assets:toolChange', handler)
    return () => window.removeEventListener('assets:toolChange', handler)
  }, [])

  if (!activeToolId) {
    return (
      <div className="flex flex-col items-center justify-center h-full" style={{ gap: 8 }}>
        <p style={{ fontSize: 16, fontWeight: 500, color: '#374151' }}>
          {t('tools.emptyTitle')}
        </p>
        <p style={{ fontSize: 13, color: '#9b9a97' }}>
          {t('tools.emptyHint')}
        </p>
      </div>
    )
  }

  return (
    <div className="flex h-full">
      <AssetMiniSidebar toolId={activeToolId} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <ToolMainView
          toolId={activeToolId}
          onDeleted={() => setActiveToolId(null)}
        />
      </div>
    </div>
  )
}
