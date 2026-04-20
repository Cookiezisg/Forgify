import { useState, useEffect, useCallback, useRef } from 'react'
import { api } from '@/lib/api'
import { onEvent, EventNames } from '@/lib/events'

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  contentType: string
  modelId?: string
  status: 'done' | 'streaming' | 'error'
}

export function useChat(conversationId: string | null) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [isStreaming, setIsStreaming] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const loadedForRef = useRef<string | null>(null)

  // Load history when conversation changes
  useEffect(() => {
    if (!conversationId) {
      setMessages([])
      loadedForRef.current = null
      return
    }
    if (loadedForRef.current === conversationId) return
    loadedForRef.current = conversationId
    setMessages([])
    setError(null)
    setIsStreaming(false)
    api<{
      id: string
      role: string
      content: string
      contentType?: string
      modelId?: string
      createdAt: string
    }[]>(`/api/conversations/${conversationId}/messages`)
      .then((msgs) =>
        setMessages(
          msgs.map((m) => ({
            id: m.id,
            role: m.role as ChatMessage['role'],
            content: m.content,
            contentType: m.contentType || 'text',
            modelId: m.modelId || undefined,
            status: 'done' as const,
          }))
        )
      )
      .catch((e) => setError(String(e)))
  }, [conversationId])

  // SSE event listeners
  useEffect(() => {
    if (!conversationId) return
    const offs = [
      onEvent<{ conversationId: string; token: string }>(
        EventNames.ChatToken,
        (e) => {
          if (e.conversationId !== conversationId) return
          setIsStreaming(true)
          setMessages((prev) => appendToken(prev, e.token))
        }
      ),
      onEvent<{ conversationId: string; modelId?: string }>(
        EventNames.ChatDone,
        (e) => {
          if (e.conversationId !== conversationId) return
          setIsStreaming(false)
          setMessages((prev) => finalizeLastMessage(prev, e.modelId))
        }
      ),
      onEvent<{ conversationId: string; error: string }>(
        EventNames.ChatError,
        (e) => {
          if (e.conversationId !== conversationId) return
          setIsStreaming(false)
          setMessages((prev) => setLastMessageError(prev, e.error))
        }
      ),
    ]
    return () => offs.forEach((off) => off())
  }, [conversationId])

  const sendMessage = useCallback(
    async (text: string) => {
      if (!conversationId || isStreaming) return
      const userMsg: ChatMessage = {
        id: crypto.randomUUID(),
        role: 'user',
        content: text,
        contentType: 'text',
        status: 'done',
      }
      const assistantMsg: ChatMessage = {
        id: crypto.randomUUID(),
        role: 'assistant',
        content: '',
        contentType: 'text',
        status: 'streaming',
      }
      setMessages((prev) => [...prev, userMsg, assistantMsg])
      setError(null)
      try {
        await api('/api/chat/send', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ conversationId, message: text }),
        })
      } catch (e) {
        setIsStreaming(false)
        setMessages((prev) => setLastMessageError(prev, String(e)))
      }
    },
    [conversationId, isStreaming]
  )

  const stopGeneration = useCallback(() => {
    if (!conversationId) return
    api('/api/chat/stop', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ conversationId }),
    }).catch(() => {})
  }, [conversationId])

  return { messages, isStreaming, error, sendMessage, stopGeneration }
}

function appendToken(messages: ChatMessage[], token: string): ChatMessage[] {
  if (messages.length === 0) return messages
  return messages.map((m, i) =>
    i === messages.length - 1 && m.role === 'assistant' && m.status === 'streaming'
      ? { ...m, content: m.content + token }
      : m
  )
}

function finalizeLastMessage(messages: ChatMessage[], modelId?: string): ChatMessage[] {
  if (messages.length === 0) return messages
  return messages.map((m, i) =>
    i === messages.length - 1
      ? { ...m, status: 'done' as const, modelId: modelId || m.modelId }
      : m
  )
}

function setLastMessageError(messages: ChatMessage[], error: string): ChatMessage[] {
  if (messages.length === 0) return messages
  return messages.map((m, i) =>
    i === messages.length - 1 ? { ...m, content: error, status: 'error' as const } : m
  )
}
