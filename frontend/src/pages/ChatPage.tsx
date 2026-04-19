import { useState, useEffect, useCallback } from 'react'
import { Plus, MessageCircle, Trash2 } from 'lucide-react'
import { api } from '@/lib/api'
import { useChat } from '@/hooks/useChat'
import { MessageList } from '@/components/chat/MessageList'
import { ChatInput } from '@/components/chat/ChatInput'
import { useT } from '@/lib/i18n'

interface Conversation {
  id: string
  title: string
  updatedAt: string
}

export function ChatLeftPanel() {
  const t = useT()
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [activeId, setActiveId] = useState<string | null>(null)

  const load = useCallback(() => {
    api<Conversation[]>('/api/conversations').then(setConversations).catch(() => {})
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const handleNew = async () => {
    try {
      const c = await api<Conversation>('/api/conversations', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title: '新对话' }),
      })
      setConversations((prev) => [c, ...prev])
      setActiveId(c.id)
    } catch {}
  }

  const handleDelete = async (e: React.MouseEvent, id: string) => {
    e.stopPropagation()
    await api(`/api/conversations/${id}`, { method: 'DELETE' }).catch(() => {})
    setConversations((prev) => prev.filter((c) => c.id !== id))
    if (activeId === id) setActiveId(null)
  }

  // Expose active conversation to parent via window event (simple approach)
  useEffect(() => {
    window.dispatchEvent(new CustomEvent('chat:conversationChange', { detail: activeId }))
  }, [activeId])

  return (
    <div className="flex flex-col h-full">
      <div style={{ padding: '8px 12px 6px' }}>
        <button
          onClick={handleNew}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            width: '100%',
            padding: '6px 10px',
            borderRadius: 6,
            border: 'none',
            background: 'transparent',
            cursor: 'pointer',
            fontSize: 13,
            color: '#374151',
            transition: 'background 100ms',
          }}
          onMouseEnter={(e) => (e.currentTarget.style.background = '#f3f4f6')}
          onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
        >
          <Plus size={14} strokeWidth={2} />
          {t('chat.newChat')}
        </button>
      </div>

      <div style={{ flex: 1, overflowY: 'auto', padding: '0 8px' }}>
        {conversations.length === 0 ? (
          <p style={{ fontSize: 12, color: '#9b9a97', padding: '8px 10px' }}>{t('chat.noChats')}</p>
        ) : (
          conversations.map((c) => (
            <div
              key={c.id}
              onClick={() => setActiveId(c.id)}
              style={{
                display: 'flex',
                alignItems: 'center',
                gap: 8,
                padding: '6px 10px',
                borderRadius: 6,
                cursor: 'pointer',
                background: activeId === c.id ? '#f3f4f6' : 'transparent',
                color: activeId === c.id ? '#111827' : '#374151',
                fontSize: 13,
                transition: 'background 100ms',
                group: 'true',
              } as React.CSSProperties}
              onMouseEnter={(e) => {
                if (activeId !== c.id) e.currentTarget.style.background = '#f9fafb'
              }}
              onMouseLeave={(e) => {
                if (activeId !== c.id) e.currentTarget.style.background = 'transparent'
              }}
            >
              <MessageCircle size={13} strokeWidth={1.6} style={{ flexShrink: 0, color: '#9b9a97' }} />
              <span style={{ flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {c.title}
              </span>
              <button
                onClick={(e) => handleDelete(e, c.id)}
                style={{
                  padding: 2,
                  border: 'none',
                  background: 'none',
                  cursor: 'pointer',
                  color: '#d1d5db',
                  borderRadius: 4,
                  display: 'flex',
                  flexShrink: 0,
                }}
                onMouseEnter={(e) => (e.currentTarget.style.color = '#6b7280')}
                onMouseLeave={(e) => (e.currentTarget.style.color = '#d1d5db')}
              >
                <Trash2 size={12} />
              </button>
            </div>
          ))
        )}
      </div>
    </div>
  )
}

export function ChatContent() {
  const t = useT()
  const [activeConversationId, setActiveConversationId] = useState<string | null>(null)
  const [hasKeys, setHasKeys] = useState<boolean | null>(null)

  useEffect(() => {
    api<{ id: string }[]>('/api/api-keys')
      .then((keys) => setHasKeys(keys.length > 0))
      .catch(() => setHasKeys(false))
  }, [])

  useEffect(() => {
    const handler = (e: Event) => {
      setActiveConversationId((e as CustomEvent).detail)
    }
    window.addEventListener('chat:conversationChange', handler)
    return () => window.removeEventListener('chat:conversationChange', handler)
  }, [])

  const { messages, isStreaming, sendMessage, stopGeneration } = useChat(activeConversationId)

  if (!activeConversationId) {
    return (
      <div className="flex flex-col items-center justify-center h-full" style={{ gap: 8 }}>
        <p style={{ fontSize: 16, fontWeight: 500, color: '#374151' }}>{t('chat.selectOrNew')}</p>
        {hasKeys === false && (
          <p style={{ fontSize: 13, color: '#9b9a97' }}>
            {t('chat.configureKeyHint')}{' '}
            <button
              onClick={() => window.dispatchEvent(new CustomEvent('nav:goTo', { detail: 'settings' }))}
              style={{ color: '#2383e2', background: 'none', border: 'none', cursor: 'pointer', fontSize: 13, padding: 0 }}
            >
              {t('chat.settingsLink')}
            </button>
            {' '}{t('chat.configureKeyHint2')}
          </p>
        )}
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full">
      <div style={{ flex: 1, overflowY: 'auto', minHeight: 0 }}>
        <MessageList messages={messages} />
      </div>
      <ChatInput
        isStreaming={isStreaming}
        onSend={sendMessage}
        onStop={stopGeneration}
        disabled={hasKeys === false}
      />
    </div>
  )
}
