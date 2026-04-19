import { useEffect, useRef } from 'react'
import { MessageItem } from './MessageItem'
import type { ChatMessage } from '@/hooks/useChat'
import { useT } from '@/lib/i18n'

interface Props {
  messages: ChatMessage[]
}

export function MessageList({ messages }: Props) {
  const t = useT()
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  if (messages.length === 0) {
    return (
      <div
        className="flex flex-col items-center justify-center h-full"
        style={{ color: '#9b9a97', fontSize: 14 }}
      >
        <p style={{ fontWeight: 500, marginBottom: 4 }}>{t('chat.startConversation')}</p>
        <p style={{ fontSize: 13 }}>{t('chat.typeBelow')}</p>
      </div>
    )
  }

  return (
    <div className="flex flex-col py-4" style={{ gap: 4 }}>
      {messages.map((msg) => (
        <MessageItem key={msg.id} message={msg} />
      ))}
      <div ref={bottomRef} />
    </div>
  )
}
