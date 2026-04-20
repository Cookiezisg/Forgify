import { useState, useEffect, useMemo, useCallback, useRef } from 'react'
import { Plus, MessageCircle, Search, Archive, RotateCcw } from 'lucide-react'
import { api } from '@/lib/api'
import { useChat } from '@/hooks/useChat'
import { MessageList } from '@/components/chat/MessageList'
import { ChatInput, type ChatInputHandle } from '@/components/chat/ChatInput'
import { DropZone } from '@/components/chat/DropZone'
import { CompactBanner } from '@/components/chat/CompactBanner'
import { ContextMenu } from '@/components/common/ContextMenu'
import { useChatContext, type Conversation } from '@/context/ChatContext'
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
    activeId,
    setActiveId,
    createConversation,
    renameConversation,
    archiveConversation,
    restoreConversation,
    deleteConversation,
    showArchived,
    setShowArchived,
  } = useChatContext()

  const [searchQuery, setSearchQuery] = useState('')
  const [renamingId, setRenamingId] = useState<string | null>(null)

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

      {/* New chat button */}
      <div style={{ padding: '2px 12px 6px' }}>
        <button
          onClick={createConversation}
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
              <ConversationItem
                key={c.id}
                conv={c}
                active={activeId === c.id}
                onClick={() => setActiveId(c.id)}
                onRename={() => setRenamingId(c.id)}
                onArchive={() => archiveConversation(c.id)}
                onDelete={() => deleteConversation(c.id)}
              />
            )
          )
        )}

        {/* Archived section */}
        {!searchQuery && (
          <div style={{ marginTop: 8 }}>
            <button
              onClick={() => setShowArchived(!showArchived)}
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
                fontSize: 12,
                color: '#9b9a97',
                transition: 'background 100ms',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.background = '#f9fafb')}
              onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
            >
              <Archive size={12} strokeWidth={1.6} />
              {showArchived ? t('chat.hideArchived') : t('chat.showArchived')}
            </button>

            {showArchived &&
              archivedConversations.map((c) => (
                <ArchivedItem
                  key={c.id}
                  conv={c}
                  onRestore={() => restoreConversation(c.id)}
                  onDelete={() => deleteConversation(c.id)}
                />
              ))}
          </div>
        )}
      </div>
    </div>
  )
}

// ─── Main Content ───

export function ChatContent() {
  const t = useT()
  const { activeId } = useChatContext()
  const [hasKeys, setHasKeys] = useState<boolean | null>(null)
  const [hasModel, setHasModel] = useState<boolean | null>(null)

  // Check keys & model config on every mount (happens on each tab switch)
  useEffect(() => {
    api<{ id: string }[]>('/api/api-keys')
      .then((keys) => setHasKeys(keys.length > 0))
      .catch(() => setHasKeys(false))
    api<{ conversation: { provider: string; modelId: string } }>('/api/model-config')
      .then((cfg) => setHasModel(!!(cfg.conversation.provider && cfg.conversation.modelId)))
      .catch(() => setHasModel(false))
  }, [])

  const { messages, isStreaming, sendMessage, stopGeneration } = useChat(activeId)
  const chatInputRef = useRef<ChatInputHandle>(null)

  const handleDropFiles = useCallback((files: File[]) => {
    chatInputRef.current?.addFiles(files)
  }, [])

  const handleCompact = useCallback(async () => {
    if (!activeId) return
    try {
      await api(`/api/conversations/${activeId}/compact`, { method: 'POST' })
      // Reload messages to show the summary
      window.location.reload()
    } catch {}
  }, [activeId])

  const needsSetup = hasKeys === false || hasModel === false
  const goToSettings = () =>
    window.dispatchEvent(new CustomEvent('nav:goTo', { detail: 'settings' }))

  if (!activeId) {
    return (
      <div className="flex flex-col items-center justify-center h-full" style={{ gap: 8 }}>
        <p style={{ fontSize: 16, fontWeight: 500, color: '#374151' }}>
          {t('chat.selectOrNew')}
        </p>
        {needsSetup && (
          <p style={{ fontSize: 13, color: '#9b9a97' }}>
            {t('chat.configureKeyHint')}{' '}
            <button
              onClick={goToSettings}
              style={{ color: '#2383e2', background: 'none', border: 'none', cursor: 'pointer', fontSize: 13, padding: 0 }}
            >
              {t('chat.settingsLink')}
            </button>{' '}
            {t('chat.configureKeyHint2')}
          </p>
        )}
      </div>
    )
  }

  return (
    <DropZone onFiles={handleDropFiles}>
      <div className="flex flex-col h-full">
        <CompactBanner conversationId={activeId} />
        <div style={{ flex: 1, overflowY: 'auto', minHeight: 0 }}>
          <MessageList messages={messages} />
        </div>
        <ChatInput
          ref={chatInputRef}
          isStreaming={isStreaming}
          onSend={sendMessage}
          onStop={stopGeneration}
          onCompact={handleCompact}
          disabled={needsSetup}
        />
      </div>
    </DropZone>
  )
}
