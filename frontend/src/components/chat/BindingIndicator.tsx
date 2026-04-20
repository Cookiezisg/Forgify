import { useState, useEffect } from 'react'
import { Paperclip, X } from 'lucide-react'
import { api } from '@/lib/api'
import { useChatContext } from '@/context/ChatContext'
import { useT } from '@/lib/i18n'

interface Props {
  conversationId: string
}

export function BindingIndicator({ conversationId }: Props) {
  const t = useT()
  const { conversations } = useChatContext()
  const conv = conversations.find(c => c.id === conversationId)
  const [assetName, setAssetName] = useState<string | null>(null)

  const assetId = conv?.assetId
  const assetType = conv?.assetType

  // Load asset name
  useEffect(() => {
    if (!assetId || !assetType) {
      setAssetName(null)
      return
    }
    if (assetType === 'tool') {
      api<{ displayName: string }>(`/api/tools/${assetId}`)
        .then(t => setAssetName(t.displayName))
        .catch(() => setAssetName(assetId))
    } else {
      setAssetName(assetId) // workflow — will be fetched when workflows are implemented
    }
  }, [assetId, assetType])

  if (!assetId || !assetType || !assetName) return null

  const icon = assetType === 'workflow' ? '⚡' : '📦'

  const handleUnbind = async () => {
    try {
      await api(`/api/conversations/${conversationId}/unbind`, { method: 'PATCH' })
      // ChatBound event will update the context
    } catch {}
  }

  const handleGoToAsset = () => {
    window.dispatchEvent(new CustomEvent('nav:goTo', { detail: 'assets' }))
  }

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '6px 16px',
        background: '#fafaf9',
        borderBottom: '1px solid #e5e7eb',
        fontSize: 12,
        color: '#6b7280',
        flexShrink: 0,
      }}
    >
      <Paperclip size={12} strokeWidth={1.8} style={{ color: '#9b9a97', flexShrink: 0 }} />
      <span>{t('chat.boundTo')}</span>
      <button
        onClick={handleGoToAsset}
        style={{
          padding: 0, border: 'none', background: 'none', cursor: 'pointer',
          fontSize: 12, fontWeight: 500, color: '#111827',
        }}
      >
        {icon} {assetName}
      </button>
      <div style={{ flex: 1 }} />
      <button
        onClick={handleUnbind}
        title={t('chat.unbind')}
        style={{
          padding: '2px 6px', border: 'none', background: 'none', cursor: 'pointer',
          fontSize: 11, color: '#9b9a97', borderRadius: 4,
        }}
        onMouseEnter={(e) => (e.currentTarget.style.color = '#374151')}
        onMouseLeave={(e) => (e.currentTarget.style.color = '#9b9a97')}
      >
        <X size={12} />
      </button>
    </div>
  )
}
