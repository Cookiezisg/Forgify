import { useState, useMemo, useCallback } from 'react'
import { Plus, MessageCircle, Search, Archive, RotateCcw, CheckSquare, Square } from 'lucide-react'
import { api } from '@/lib/api'
import { ContextMenu } from '@/components/common/ContextMenu'
import { useChatContext, type Conversation } from '@/context/ChatContext'
import { useTabContext } from '@/context/TabContext'
import { useT } from '@/lib/i18n'

// ─── Relative time helper ───

function relativeTime(
  dateStr: string,
  t: (k: any) => string
): string {
  if (!dateStr) return ''
  // Go's time.Time marshals to ISO 8601 (e.g. "2026-04-20T03:17:27Z")
  const date = new Date(dateStr)
  if (isNaN(date.getTime())) return ''

  const now = Date.now()
  const diff = now - date.getTime()
  const minutes = Math.floor(diff / 60000)
  const hours = Math.floor(diff / 3600000)

  if (minutes < 1) return t('chat.justNow')
  if (minutes < 60) return `${minutes} ${t('chat.minutesAgo')}`
  if (hours < 24) return `${hours} ${t('chat.hoursAgo')}`
  if (hours < 48) return t('chat.yesterday')

  // Format as short date
  const m = date.getMonth() + 1
  const d = date.getDate()
  return `${m}/${d}`
}

// ─── Conversation item ───

function ConversationItem({
  conv,
  active,
  onClick,
  onRename,
  onArchive,
  onDelete,
}: {
  conv: Conversation
  active: boolean
  onClick: () => void
  onRename: () => void
  onArchive: () => void
  onDelete: () => void
}) {
  const t = useT()

  const menuItems = [
    { label: t('chat.rename'), onClick: onRename },
    { label: t('chat.archive'), onClick: onArchive },
    {
      label: t('chat.delete'),
      danger: true,
      onClick: () => {
        if (window.confirm(t('chat.confirmDelete'))) onDelete()
      },
    },
  ]

  return (
    <div
      onClick={onClick}
      className="group"
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '6px 10px',
        borderRadius: 6,
        cursor: 'pointer',
        background: active ? '#f3f4f6' : 'transparent',
        color: active ? '#111827' : '#374151',
        fontSize: 13,
        transition: 'background 100ms',
      }}
      onMouseEnter={(e) => {
        if (!active) e.currentTarget.style.background = '#f9fafb'
        // Show context menu button
        const btn = e.currentTarget.querySelector('.ctx-btn') as HTMLElement
        if (btn) btn.style.opacity = '1'
      }}
      onMouseLeave={(e) => {
        if (!active) e.currentTarget.style.background = 'transparent'
        const btn = e.currentTarget.querySelector('.ctx-btn') as HTMLElement
        if (btn) btn.style.opacity = '0'
      }}
    >
      <MessageCircle
        size={13}
        strokeWidth={1.6}
        style={{ flexShrink: 0, color: '#9b9a97' }}
      />
      <span
        style={{
          flex: 1,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}
      >
        {conv.title}
      </span>

      {/* Asset badge */}
      {conv.assetType === 'workflow' && (
        <span style={{ fontSize: 11, flexShrink: 0, color: '#eab308' }} title="Workflow">
          ⚡
        </span>
      )}
      {conv.assetType === 'tool' && (
        <span style={{ fontSize: 11, flexShrink: 0, color: '#3b82f6' }} title="Tool">
          📦
        </span>
      )}

      {/* Relative time */}
      <span
        style={{
          fontSize: 11,
          color: '#c7c7c5',
          flexShrink: 0,
          whiteSpace: 'nowrap',
        }}
      >
        {relativeTime(conv.updatedAt, t)}
      </span>

      <ContextMenu items={menuItems} />
    </div>
  )
}

// ─── Archived conversation item ───

function ArchivedItem({
  conv,
  onRestore,
  onDelete,
}: {
  conv: Conversation
  onRestore: () => void
  onDelete: () => void
}) {
  const t = useT()

  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '5px 10px',
        fontSize: 13,
        color: '#9b9a97',
      }}
    >
      <Archive size={12} strokeWidth={1.6} style={{ flexShrink: 0 }} />
      <span
        style={{
          flex: 1,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
        }}
      >
        {conv.title}
      </span>
      <button
        onClick={onRestore}
        title={t('chat.restore')}
        style={{
          padding: 2,
          border: 'none',
          background: 'none',
          cursor: 'pointer',
          color: '#9b9a97',
          borderRadius: 4,
          display: 'flex',
        }}
        onMouseEnter={(e) => (e.currentTarget.style.color = '#374151')}
        onMouseLeave={(e) => (e.currentTarget.style.color = '#9b9a97')}
      >
        <RotateCcw size={12} />
      </button>
      <ContextMenu
        items={[
          {
            label: t('chat.delete'),
            danger: true,
            onClick: () => {
              if (window.confirm(t('chat.confirmDelete'))) onDelete()
            },
          },
        ]}
      />
    </div>
  )
}

// ─── Inline rename input ───

function RenameInput({
  initial,
  onSave,
  onCancel,
}: {
  initial: string
  onSave: (v: string) => void
  onCancel: () => void
}) {
  const [value, setValue] = useState(initial)

  return (
    <input
      autoFocus
      value={value}
      onChange={(e) => setValue(e.target.value)}
      onBlur={() => {
        const trimmed = value.trim()
        if (trimmed && trimmed !== initial) onSave(trimmed)
        else onCancel()
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {
          const trimmed = value.trim()
          if (trimmed && trimmed !== initial) onSave(trimmed)
          else onCancel()
        }
        if (e.key === 'Escape') onCancel()
      }}
      style={{
        width: '100%',
        padding: '5px 10px',
        borderRadius: 6,
        border: '1px solid #d1d5db',
        fontSize: 13,
        outline: 'none',
        background: 'white',
        color: '#1a1a1a',
        margin: '0 0 0 0',
      }}
    />
  )
}

// ─── Left Panel ───

export function ChatLeftPanel() {
  const t = useT()
  const {
    conversations,
    archivedConversations,
    createConversation,
    renameConversation,
    archiveConversation,
    restoreConversation,
    deleteConversation,
    showArchived,
    setShowArchived,
    refreshConversations,
  } = useChatContext()
  const { openTab, activeTabId, tabs } = useTabContext()

  // Derive activeId from the currently active tab's conversationId
  const activeTab = tabs.find(t => t.id === activeTabId)
  const activeId = activeTab?.conversationId ?? null

  const [searchQuery, setSearchQuery] = useState('')
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [selectMode, setSelectMode] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())

  const toggleSelect = useCallback((id: string) => {
    setSelected(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id); else next.add(id)
      return next
    })
  }, [])

  const handleBatchArchive = useCallback(async () => {
    if (selected.size === 0) return
    await api('/api/conversations/batch-archive', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ids: Array.from(selected) }),
    }).catch(() => {})
    refreshConversations()
    setSelected(new Set())
    setSelectMode(false)
  }, [selected, refreshConversations])

  const handleBatchDelete = useCallback(async () => {
    if (selected.size === 0) return
    if (!window.confirm(`永久删除 ${selected.size} 个对话？`)) return
    await api('/api/conversations/batch-delete', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ ids: Array.from(selected) }),
    }).catch(() => {})
    refreshConversations()
    setSelected(new Set())
    setSelectMode(false)
  }, [selected, refreshConversations])

  // Filter conversations by search
  const displayed = useMemo(() => {
    if (!searchQuery.trim()) return conversations
    const q = searchQuery.toLowerCase()
    return conversations.filter((c) =>
      c.title.toLowerCase().includes(q)
    )
  }, [conversations, searchQuery])

  return (
    <div className="flex flex-col h-full">
      {/* Search bar */}
      <div style={{ padding: '8px 12px 4px' }}>
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            padding: '5px 8px',
            borderRadius: 6,
            background: '#f7f7f5',
            border: '1px solid transparent',
            transition: 'border-color 150ms',
          }}
          onFocusCapture={(e) => {
            e.currentTarget.style.borderColor = '#d1d5db'
          }}
          onBlurCapture={(e) => {
            if (!e.currentTarget.contains(e.relatedTarget)) {
              e.currentTarget.style.borderColor = 'transparent'
            }
          }}
        >
          <Search size={13} strokeWidth={1.8} style={{ color: '#9b9a97', flexShrink: 0 }} />
          <input
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder={t('chat.searchPlaceholder')}
            style={{
              flex: 1,
              border: 'none',
              background: 'transparent',
              outline: 'none',
              fontSize: 13,
              color: '#1a1a1a',
            }}
          />
        </div>
      </div>

      {/* Batch mode toolbar */}
      {selectMode && (
        <div style={{ padding: '4px 12px', display: 'flex', gap: 4, alignItems: 'center' }}>
          <span style={{ fontSize: 11, color: '#6b7280', flex: 1 }}>{selected.size} 已选</span>
          <button onClick={handleBatchArchive} disabled={selected.size === 0}
            style={{ padding: '3px 8px', fontSize: 11, borderRadius: 4, border: '1px solid #e5e7eb', background: 'white', color: '#374151', cursor: 'pointer' }}>
            归档
          </button>
          <button onClick={handleBatchDelete} disabled={selected.size === 0}
            style={{ padding: '3px 8px', fontSize: 11, borderRadius: 4, border: '1px solid #fca5a5', background: 'white', color: '#dc2626', cursor: 'pointer' }}>
            删除
          </button>
          <button onClick={() => { setSelectMode(false); setSelected(new Set()) }}
            style={{ padding: '3px 8px', fontSize: 11, borderRadius: 4, border: 'none', background: 'transparent', color: '#9b9a97', cursor: 'pointer' }}>
            取消
          </button>
        </div>
      )}

      {/* New chat + select mode toggle */}
      <div style={{ padding: '2px 12px 6px', display: 'flex', gap: 4 }}>
        <button
          onClick={async () => {
            const conv = await createConversation()
            if (conv) {
              openTab({ layout: 'chat', label: conv.title, conversationId: conv.id })
            }
          }}
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            flex: 1,
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
        {!selectMode && conversations.length > 0 && (
          <button onClick={() => setSelectMode(true)} title="批量操作"
            style={{
              width: 28, height: 28, borderRadius: 6, border: 'none',
              background: 'transparent', cursor: 'pointer', color: '#9b9a97',
              display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0,
            }}
            onMouseEnter={(e) => (e.currentTarget.style.color = '#374151')}
            onMouseLeave={(e) => (e.currentTarget.style.color = '#9b9a97')}
          >
            <CheckSquare size={14} strokeWidth={1.6} />
          </button>
        )}
      </div>

      {/* Conversation list */}
      <div style={{ flex: 1, overflowY: 'auto', padding: '0 8px' }}>
        {displayed.length === 0 && !searchQuery ? (
          <p style={{ fontSize: 12, color: '#9b9a97', padding: '8px 10px' }}>
            {t('chat.noChats')}
          </p>
        ) : displayed.length === 0 && searchQuery ? (
          <p style={{ fontSize: 12, color: '#9b9a97', padding: '8px 10px' }}>
            —
          </p>
        ) : (
          displayed.map((c) =>
            renamingId === c.id ? (
              <div key={c.id} style={{ padding: '2px 0' }}>
                <RenameInput
                  initial={c.title}
                  onSave={(title) => {
                    renameConversation(c.id, title)
                    setRenamingId(null)
                  }}
                  onCancel={() => setRenamingId(null)}
                />
              </div>
            ) : (
              <div key={c.id} style={{ display: 'flex', alignItems: 'center' }}>
                {selectMode && (
                  <button onClick={() => toggleSelect(c.id)} style={{
                    width: 24, height: 24, flexShrink: 0, border: 'none', background: 'none',
                    cursor: 'pointer', color: selected.has(c.id) ? '#2383e2' : '#d1d5db',
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                  }}>
                    {selected.has(c.id) ? <CheckSquare size={14} /> : <Square size={14} />}
                  </button>
                )}
                <div style={{ flex: 1, minWidth: 0 }}>
              <ConversationItem
                conv={c}
                active={activeId === c.id}
                onClick={() => {
                  if (selectMode) { toggleSelect(c.id); return }
                  const layout = c.assetType === 'tool' && c.assetId ? 'chat-tool' as const : 'chat' as const
                  openTab({
                    layout,
                    label: c.title,
                    conversationId: c.id,
                    toolId: layout === 'chat-tool' ? (c.assetId ?? undefined) : undefined,
                  })
                }}
                onRename={() => setRenamingId(c.id)}
                onArchive={() => archiveConversation(c.id)}
                onDelete={() => deleteConversation(c.id)}
              />
                </div>
              </div>
            )
          )
        )}

      </div>

      {/* Archived section — fixed at bottom */}
      {!searchQuery && (
        <div style={{ flexShrink: 0, borderTop: '1px solid #f3f4f6', padding: '0 8px' }}>
          <button
            onClick={() => setShowArchived(!showArchived)}
            style={{
              display: 'flex', alignItems: 'center', gap: 6, width: '100%',
              padding: '6px 10px', borderRadius: 6, border: 'none',
              background: 'transparent', cursor: 'pointer', fontSize: 12, color: '#9b9a97',
            }}
            onMouseEnter={(e) => (e.currentTarget.style.background = '#f9fafb')}
            onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
          >
            <Archive size={12} strokeWidth={1.6} />
            {showArchived ? t('chat.hideArchived') : t('chat.showArchived')}
          </button>
          {showArchived && (
            <div style={{ maxHeight: 200, overflowY: 'auto' }}>
              {archivedConversations.map((c) => (
                <ArchivedItem key={c.id} conv={c}
                  onRestore={() => restoreConversation(c.id)}
                  onDelete={() => deleteConversation(c.id)} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

// ChatContent is now in pages/ChatContent.tsx (standalone, accepts conversationId prop)
