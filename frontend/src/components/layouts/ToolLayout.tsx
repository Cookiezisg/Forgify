import { useState, useEffect, useCallback } from 'react'
import { Plus, MessageCircle } from 'lucide-react'
import { ToolMainView } from '@/components/tools/ToolMainView'
import { useTabContext } from '@/context/TabContext'
import { api } from '@/lib/api'
import { useT } from '@/lib/i18n'

interface Props {
  toolId: string
}

interface AssetConversation {
  id: string
  title: string
  updatedAt: string
}

export function ToolLayout({ toolId }: Props) {
  const { closeTab, activeTabId } = useTabContext()

  return (
    <div className="flex h-full">
      <MiniSidebar toolId={toolId} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <ToolMainView
          toolId={toolId}
          onDeleted={() => { if (activeTabId) closeTab(activeTabId) }}
        />
      </div>
    </div>
  )
}

function MiniSidebar({ toolId }: { toolId: string }) {
  const t = useT()
  const { openTab } = useTabContext()
  const [convs, setConvs] = useState<AssetConversation[]>([])

  const load = useCallback(() => {
    api<AssetConversation[]>(`/api/asset-conversations/${toolId}`)
      .then(setConvs).catch(() => setConvs([]))
  }, [toolId])

  useEffect(() => { load() }, [load])

  const handleNewConv = async () => {
    try {
      const conv = await api<AssetConversation>('/api/conversations', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ assetId: toolId, assetType: 'tool' }),
      })
      openTab({ layout: 'chat-tool', label: conv.title, conversationId: conv.id, toolId })
    } catch {}
  }

  const handleOpenConv = (conv: AssetConversation) => {
    openTab({ layout: 'chat-tool', label: conv.title, conversationId: conv.id, toolId })
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
            <div key={c.id} onClick={() => handleOpenConv(c)} style={{
              display: 'flex', alignItems: 'center', gap: 6,
              padding: '5px 8px', borderRadius: 5, cursor: 'pointer',
              fontSize: 12, color: '#374151',
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
