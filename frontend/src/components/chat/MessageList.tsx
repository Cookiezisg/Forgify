import { useState, useEffect, useRef, useCallback } from 'react'
import { ArrowDown, Loader } from 'lucide-react'
import { MessageItem } from './MessageItem'
import { EmptyChat } from './EmptyChat'
import type { ChatMessage } from '@/hooks/useChat'
import { useT } from '@/lib/i18n'

interface Props {
  messages: ChatMessage[]
  isLoading?: boolean
}

export function MessageList({ messages, isLoading }: Props) {
  const t = useT()
  const bottomRef = useRef<HTMLDivElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)
  const [userScrolled, setUserScrolled] = useState(false)

  // Auto-scroll to bottom when new messages arrive (unless user scrolled up)
  useEffect(() => {
    if (!userScrolled) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
    }
  }, [messages, userScrolled])

  // Reset scroll state when conversation changes (messages reset to empty then load)
  useEffect(() => {
    setUserScrolled(false)
  }, [messages.length === 0])

  // Detect user scroll
  const onScroll = useCallback(() => {
    const el = containerRef.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 60
    setUserScrolled(!atBottom)
  }, [])

  const scrollToBottom = useCallback(() => {
    setUserScrolled(false)
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [])

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader size={20} strokeWidth={1.8} style={{ color: '#9b9a97', animation: 'spin 1s linear infinite' }} />
      </div>
    )
  }

  if (messages.length === 0) {
    return <EmptyChat />
  }

  return (
    <div
      ref={containerRef}
      onScroll={onScroll}
      className="flex flex-col h-full"
      style={{ overflowY: 'auto', position: 'relative' }}
    >
      <div className="flex flex-col py-4" style={{ gap: 4 }}>
        {messages.map((msg) => (
          <MessageItem key={msg.id} message={msg} />
        ))}
        <div ref={bottomRef} />
      </div>

      {/* Back to latest button */}
      {userScrolled && (
        <button
          onClick={scrollToBottom}
          style={{
            position: 'sticky',
            bottom: 16,
            alignSelf: 'center',
            display: 'flex',
            alignItems: 'center',
            gap: 4,
            padding: '6px 14px',
            borderRadius: 999,
            border: '1px solid #e5e7eb',
            background: 'white',
            boxShadow: '0 2px 8px rgba(0,0,0,0.08)',
            cursor: 'pointer',
            fontSize: 12,
            color: '#374151',
            zIndex: 10,
            transition: 'box-shadow 150ms',
          }}
          onMouseEnter={(e) => {
            e.currentTarget.style.boxShadow = '0 4px 12px rgba(0,0,0,0.12)'
          }}
          onMouseLeave={(e) => {
            e.currentTarget.style.boxShadow = '0 2px 8px rgba(0,0,0,0.08)'
          }}
        >
          <ArrowDown size={12} strokeWidth={2} />
          {t('chat.backToLatest')}
        </button>
      )}
    </div>
  )
}
