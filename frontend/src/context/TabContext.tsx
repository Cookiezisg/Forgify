import { createContext, useContext, useState, useCallback, useEffect } from 'react'
import type { ReactNode } from 'react'
import { onEvent, EventNames } from '@/lib/events'

export type LayoutType = 'chat' | 'tool' | 'workflow' | 'chat-tool' | 'chat-workflow'

export interface TabItem {
  id: string
  layout: LayoutType
  label: string
  icon?: string
  pinned?: boolean
  conversationId?: string
  toolId?: string
  workflowId?: string
}

interface TabContextValue {
  tabs: TabItem[]
  activeTabId: string | null
  openTab: (tab: Omit<TabItem, 'id'>) => string
  closeTab: (id: string) => void
  setActiveTab: (id: string) => void
  updateTab: (id: string, patch: Partial<TabItem>) => void
  findTab: (predicate: (t: TabItem) => boolean) => TabItem | undefined
}

const TabContext = createContext<TabContextValue | null>(null)

const STORAGE_KEY = 'forgify.tabs'
const DEFAULT_TABS: TabItem[] = []

const VALID_LAYOUTS: Set<string> = new Set(['chat', 'tool', 'workflow', 'chat-tool', 'chat-workflow'])

function loadPersistedTabs(): { tabs: TabItem[]; activeTabId: string | null } {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const data = JSON.parse(raw)
      if (data.tabs?.length > 0) {
        // Filter out tabs with invalid/stale layout types (e.g. "home", "inbox", "settings" from old versions)
        const validTabs = (data.tabs as TabItem[]).filter(t => VALID_LAYOUTS.has(t.layout))
        if (validTabs.length > 0) {
          const activeOk = validTabs.some(t => t.id === data.activeTabId)
          return { tabs: validTabs, activeTabId: activeOk ? data.activeTabId : validTabs[0].id }
        }
      }
    }
  } catch {}
  return { tabs: DEFAULT_TABS, activeTabId: null }
}

export function TabProvider({ children }: { children: ReactNode }) {
  const [tabs, setTabs] = useState<TabItem[]>(() => loadPersistedTabs().tabs)
  const [activeTabId, setActiveTabId] = useState<string | null>(() => loadPersistedTabs().activeTabId)

  // Persist tabs to localStorage
  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ tabs, activeTabId }))
  }, [tabs, activeTabId])

  // Listen for ChatBound event: upgrade chat tab → chat-tool when tool is bound
  useEffect(() => {
    return onEvent<{ conversationId: string; assetId: string; assetType: string }>(
      EventNames.ChatBound,
      ({ conversationId, assetId, assetType }) => {
        if (assetType !== 'tool' || !assetId) return
        setTabs(prev => prev.map(t => {
          if (t.conversationId === conversationId && t.layout === 'chat') {
            return { ...t, layout: 'chat-tool' as const, toolId: assetId }
          }
          return t
        }))
      }
    )
  }, [])

  const openTab = useCallback((tab: Omit<TabItem, 'id'>) => {
    // Check if a tab with the same content already exists
    const existing = tabs.find(t => {
      if (tab.layout === 'chat' && t.layout === 'chat' && t.conversationId === tab.conversationId) return true
      if (tab.layout === 'tool' && t.layout === 'tool' && t.toolId === tab.toolId) return true
      if (tab.layout === 'workflow' && t.layout === 'workflow' && t.workflowId === tab.workflowId) return true
      if (tab.layout === 'chat-tool' && t.layout === 'chat-tool' && t.conversationId === tab.conversationId) return true
      if (tab.layout === 'chat-workflow' && t.layout === 'chat-workflow' && t.conversationId === tab.conversationId) return true
      // Pinned tabs: check layout match
      if (tab.pinned && t.layout === tab.layout && t.pinned) return true
      return false
    })
    if (existing) {
      setActiveTabId(existing.id)
      return existing.id
    }

    const id = crypto.randomUUID()
    const newTab: TabItem = { ...tab, id }
    setTabs(prev => [...prev, newTab])
    setActiveTabId(id)
    return id
  }, [tabs])

  const closeTab = useCallback((id: string) => {
    setTabs(prev => {
      const idx = prev.findIndex(t => t.id === id)
      const next = prev.filter(t => t.id !== id)

      // If closing the active tab, switch to adjacent
      if (id === activeTabId && next.length > 0) {
        const newIdx = Math.min(idx, next.length - 1)
        setActiveTabId(next[newIdx].id)
      } else if (next.length === 0) {
        setActiveTabId(null)
      }

      return next
    })
  }, [activeTabId])

  const setActiveTab = useCallback((id: string) => {
    setActiveTabId(id)
  }, [])

  const updateTab = useCallback((id: string, patch: Partial<TabItem>) => {
    setTabs(prev => prev.map(t => t.id === id ? { ...t, ...patch } : t))
  }, [])

  const findTab = useCallback((predicate: (t: TabItem) => boolean) => {
    return tabs.find(predicate)
  }, [tabs])

  return (
    <TabContext.Provider value={{ tabs, activeTabId, openTab, closeTab, setActiveTab, updateTab, findTab }}>
      {children}
    </TabContext.Provider>
  )
}

export function useTabContext() {
  const ctx = useContext(TabContext)
  if (!ctx) throw new Error('useTabContext must be used within TabProvider')
  return ctx
}
