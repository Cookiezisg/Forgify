import { useEffect, useRef } from 'react'
import { MessageItem } from './MessageItem'
import type { ChatMessage } from '@/hooks/useChat'

interface Props {
  messages: ChatMessage[]
}

export function MessageList({ messages }: Props) {
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
        <p style={{ fontWeight: 500, marginBottom: 4 }}>开始一段对话</p>
        <p style={{ fontSize: 13 }}>在下方输入消息</p>
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
