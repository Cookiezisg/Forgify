import { createContext, useContext, useState, useEffect, useCallback } from 'react'
import type { ReactNode } from 'react'
import { api } from '@/lib/api'
import { onEvent, EventNames } from '@/lib/events'

export interface Conversation {
  id: string
  title: string
  assetId: string | null
  assetType: string | null
  status: string
  createdAt: string
  updatedAt: string
}

interface ChatContextValue {
  conversations: Conversation[]
  archivedConversations: Conversation[]
  activeId: string | null
  setActiveId: (id: string | null) => void
  createConversation: () => Promise<Conversation | undefined>
  renameConversation: (id: string, title: string) => Promise<void>
  archiveConversation: (id: string) => Promise<void>
  restoreConversation: (id: string) => Promise<void>
  deleteConversation: (id: string) => Promise<void>
  refreshConversations: () => void
  showArchived: boolean
  setShowArchived: (v: boolean) => void
}

const ChatContext = createContext<ChatContextValue | null>(null)

export function ChatProvider({ children }: { children: ReactNode }) {
  const [conversations, setConversations] = useState<Conversation[]>([])
  const [archivedConversations, setArchivedConversations] = useState<Conversation[]>([])
  const [activeId, setActiveId] = useState<string | null>(null)
  const [showArchived, setShowArchived] = useState(false)

  const loadConversations = useCallback(() => {
    api<Conversation[]>('/api/conversations')
      .then(setConversations)
      .catch(() => {})
  }, [])

  const loadArchived = useCallback(() => {
    api<Conversation[]>('/api/conversations/archived')
      .then(setArchivedConversations)
      .catch(() => {})
  }, [])

  // Initial load
  useEffect(() => {
    loadConversations()
  }, [loadConversations])

  // Load archived when toggled
  useEffect(() => {
    if (showArchived) loadArchived()
  }, [showArchived, loadArchived])

  // Listen for auto-title updates
  useEffect(() => {
    return onEvent<{ conversationId: string; title: string }>(
      EventNames.ChatTitleUpdated,
      ({ conversationId, title }) => {
        setConversations(prev =>
          prev.map(c => c.id === conversationId ? { ...c, title } : c)
        )
      }
    )
  }, [])

  const createConversation = useCallback(async () => {
    try {
      const conv = await api<Conversation>('/api/conversations', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      })
      setConversations(prev => [conv, ...prev])
      setActiveId(conv.id)
      return conv
    } catch {
      return undefined
    }
  }, [])

  const renameConversation = useCallback(async (id: string, title: string) => {
    try {
      await api(`/api/conversations/${id}/rename`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ title }),
      })
      setConversations(prev =>
        prev.map(c => c.id === id ? { ...c, title } : c)
      )
    } catch {}
  }, [])

  const archiveConversation = useCallback(async (id: string) => {
    try {
      await api(`/api/conversations/${id}/archive`, { method: 'PATCH' })
      setConversations(prev => prev.filter(c => c.id !== id))
      if (activeId === id) setActiveId(null)
      if (showArchived) loadArchived()
    } catch {}
  }, [activeId, showArchived, loadArchived])

  const restoreConversation = useCallback(async (id: string) => {
    try {
      await api(`/api/conversations/${id}/restore`, { method: 'PATCH' })
      loadConversations()
      loadArchived()
    } catch {}
  }, [loadConversations, loadArchived])

  const deleteConversation = useCallback(async (id: string) => {
    try {
      await api(`/api/conversations/${id}`, { method: 'DELETE' })
      setConversations(prev => prev.filter(c => c.id !== id))
      setArchivedConversations(prev => prev.filter(c => c.id !== id))
      if (activeId === id) setActiveId(null)
    } catch {}
  }, [activeId])

  return (
    <ChatContext.Provider
      value={{
        conversations,
        archivedConversations,
        activeId,
        setActiveId,
        createConversation,
        renameConversation,
        archiveConversation,
        restoreConversation,
        deleteConversation,
        refreshConversations: loadConversations,
        showArchived,
        setShowArchived,
      }}
    >
      {children}
    </ChatContext.Provider>
  )
}

export function useChatContext() {
  const ctx = useContext(ChatContext)
  if (!ctx) throw new Error('useChatContext must be used within ChatProvider')
  return ctx
}
