import { useState, useEffect, useCallback, useRef } from 'react'
import { api } from '@/lib/api'
import { useChat } from '@/hooks/useChat'
import { MessageList } from '@/components/chat/MessageList'
import { ChatInput, type ChatInputHandle } from '@/components/chat/ChatInput'
import { DropZone } from '@/components/chat/DropZone'
import { CompactBanner } from '@/components/chat/CompactBanner'
import { useT, useI18n } from '@/lib/i18n'

interface Props {
  conversationId?: string
  /** If true, hide binding indicator (used in ChatToolLayout where the tool panel replaces it) */
  hideBinding?: boolean
}

/**
 * Standalone chat content component.
 * Accepts conversationId as prop (from Tab system) instead of from ChatContext.activeId.
 */
export function ChatContent({ conversationId, hideBinding }: Props) {
  const t = useT()
  const { locale } = useI18n()
  const [hasKeys, setHasKeys] = useState<boolean | null>(null)
  const [hasModel, setHasModel] = useState<boolean | null>(null)

  useEffect(() => {
    api<{ id: string }[]>('/api/api-keys')
      .then((keys) => setHasKeys(keys.length > 0))
      .catch(() => setHasKeys(false))
    api<{ conversation: { provider: string; modelId: string } }>('/api/model-config')
      .then((cfg) => setHasModel(!!(cfg.conversation.provider && cfg.conversation.modelId)))
      .catch(() => setHasModel(false))
  }, [])

  const { messages, isStreaming, isLoading, sendMessage, stopGeneration, reloadMessages } = useChat(conversationId ?? null)
  const chatInputRef = useRef<ChatInputHandle>(null)

  const handleDropFiles = useCallback((files: File[]) => {
    chatInputRef.current?.addFiles(files)
  }, [])

  // Listen for "Fix with AI" requests from the tool panel (Chat+Tool layout).
  // The tool panel emits a broadcast event with its conversationId; only the
  // matching ChatContent instance acts on it and sends a fix prompt to Eino.
  useEffect(() => {
    if (!conversationId) return
    const handler = (e: Event) => {
      const detail = (e as CustomEvent).detail as
        | { conversationId: string; toolName: string; error: string }
        | undefined
      if (!detail || detail.conversationId !== conversationId) return
      const prompt = locale === 'zh-CN'
        ? `工具 \`${detail.toolName}\` 运行失败：\n\`\`\`\n${detail.error}\n\`\`\`\n请修复代码。`
        : `Tool \`${detail.toolName}\` failed:\n\`\`\`\n${detail.error}\n\`\`\`\nPlease fix the code.`
      sendMessage(prompt)
    }
    window.addEventListener('forge:fix-requested', handler)
    return () => window.removeEventListener('forge:fix-requested', handler)
  }, [conversationId, sendMessage, locale])

  const handleCompact = useCallback(async () => {
    if (!conversationId) return
    try {
      await api(`/api/conversations/${conversationId}/compact`, { method: 'POST' })
      reloadMessages()
    } catch {}
  }, [conversationId, reloadMessages])

  const needsSetup = hasKeys === false || hasModel === false
  const goToSettings = () => window.dispatchEvent(new CustomEvent('nav:goTo', { detail: 'settings' }))

  if (!conversationId) {
    return (
      <div className="flex flex-col items-center justify-center h-full" style={{ gap: 8 }}>
        <p style={{ fontSize: 16, fontWeight: 500, color: '#374151' }}>
          {t('chat.selectOrNew')}
        </p>
        {needsSetup && (
          <p style={{ fontSize: 13, color: '#9b9a97' }}>
            {t('chat.configureKeyHint')}{' '}
            <button onClick={goToSettings}
              style={{ color: '#2383e2', background: 'none', border: 'none', cursor: 'pointer', fontSize: 13, padding: 0 }}>
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
        <CompactBanner conversationId={conversationId} />
        <div style={{ flex: 1, overflowY: 'auto', minHeight: 0 }}>
          <div style={{ maxWidth: hideBinding ? undefined : 768, margin: '0 auto', width: '100%', height: '100%', padding: '0 8px' }}>
            <MessageList messages={messages} isLoading={isLoading} conversationId={conversationId} />
          </div>
        </div>
        <div style={{ maxWidth: hideBinding ? undefined : 768, margin: '0 auto', width: '100%', padding: '0 16px' }}>
          <ChatInput
            ref={chatInputRef}
            isStreaming={isStreaming}
            onSend={sendMessage}
            onStop={stopGeneration}
            onCompact={handleCompact}
            disabled={needsSetup}
          />
        </div>
      </div>
    </DropZone>
  )
}
