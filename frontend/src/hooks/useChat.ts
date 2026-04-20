import { useState, useEffect, useCallback, useRef } from 'react'
import { api } from '@/lib/api'
import { onEvent, EventNames } from '@/lib/events'
import type { PendingFile } from '@/components/chat/AttachmentBar'

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant' | 'system'
  content: string
  contentType: string
  modelId?: string
  forgeToolId?: string
  forgeCode?: string      // Python code detected in this message (no tool created yet)
  forgeFuncName?: string  // Function name from detected code
  status: 'done' | 'streaming' | 'error'
}

export function useChat(conversationId: string | null) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [isStreaming, setIsStreaming] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const loadedForRef = useRef<string | null>(null)

  // Load messages from server
  const loadMessages = useCallback((convId: string) => {
    setIsLoading(true)
    setMessages([])
    setError(null)
    setIsStreaming(false)
    api<{
      id: string
      role: string
      content: string
      contentType?: string
      metadata?: string
      modelId?: string
      createdAt: string
    }[]>(`/api/conversations/${convId}/messages`)
      .then((msgs) =>
        setMessages(
          msgs.map((m) => {
            // Restore forgeToolId from metadata if present
            let forgeToolId: string | undefined
            if (m.metadata) {
              try {
                const meta = JSON.parse(m.metadata)
                forgeToolId = meta.forgeToolId || undefined
              } catch {}
            }
            return {
              id: m.id,
              role: m.role as ChatMessage['role'],
              content: m.content,
              contentType: m.contentType || 'text',
              modelId: m.modelId || undefined,
              forgeToolId,
              status: 'done' as const,
            }
          })
        )
      )
      .catch((e) => setError(String(e)))
      .finally(() => setIsLoading(false))
  }, [])

  // Load history when conversation changes
  useEffect(() => {
    if (!conversationId) {
      setMessages([])
      loadedForRef.current = null
      return
    }
    if (loadedForRef.current === conversationId) return
    loadedForRef.current = conversationId
    loadMessages(conversationId)
  }, [conversationId, loadMessages])

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
      // Forge: when AI response contains Python code, mark the message for "save as tool" UI
      onEvent<{ conversationId: string; toolId?: string; funcName: string; code?: string }>(
        EventNames.ForgeCodeDetected,
        (e) => {
          if (e.conversationId !== conversationId) return
          if (e.toolId) {
            // Tool already created (bound conversation modification)
            setMessages((prev) => attachForgeToolId(prev, e.toolId!))
          } else if (e.code) {
            // Code detected but no tool yet — mark for "save as tool" button
            setMessages((prev) => attachForgeCode(prev, e.code!, e.funcName))
          }
        }
      ),
    ]
    return () => offs.forEach((off) => off())
  }, [conversationId])

  const sendMessage = useCallback(
    async (text: string, files?: PendingFile[]) => {
      if (!conversationId || isStreaming) return

      // Build attachment summary for display
      let displayContent = text
      if (files && files.length > 0) {
        const names = files.map(f => f.file.name).join(', ')
        displayContent = text + '\n\ud83d\udcce ' + names
      }

      const userMsg: ChatMessage = {
        id: crypto.randomUUID(),
        role: 'user',
        content: displayContent,
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
        // Convert files to base64
        let attachments: { name: string; base64: string; size: number }[] = []
        if (files && files.length > 0) {
          attachments = await Promise.all(
            files.map(async (f) => ({
              name: f.file.name,
              size: f.file.size,
              base64: await fileToBase64(f.file),
            }))
          )
        }

        await api('/api/chat/send', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            conversationId,
            message: text,
            attachments: attachments.length > 0 ? attachments : undefined,
          }),
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

  const reloadMessages = useCallback(() => {
    if (conversationId) {
      loadedForRef.current = null
      loadMessages(conversationId)
    }
  }, [conversationId, loadMessages])

  return { messages, isStreaming, isLoading, error, sendMessage, stopGeneration, reloadMessages }
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

function attachForgeToolId(messages: ChatMessage[], toolId: string): ChatMessage[] {
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].role === 'assistant' && messages[i].status === 'done') {
      return messages.map((m, idx) =>
        idx === i ? { ...m, forgeToolId: toolId } : m
      )
    }
  }
  return messages
}

function attachForgeCode(messages: ChatMessage[], code: string, funcName: string): ChatMessage[] {
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].role === 'assistant' && messages[i].status === 'done') {
      return messages.map((m, idx) =>
        idx === i ? { ...m, forgeCode: code, forgeFuncName: funcName } : m
      )
    }
  }
  return messages
}

function fileToBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => {
      const result = reader.result as string
      // Remove data:...;base64, prefix
      const base64 = result.split(',')[1] || ''
      resolve(base64)
    }
    reader.onerror = reject
    reader.readAsDataURL(file)
  })
}
