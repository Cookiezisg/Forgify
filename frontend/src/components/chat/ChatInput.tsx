import { useState, useRef, useCallback, forwardRef, useImperativeHandle } from 'react'
import { ArrowUp, Square, Paperclip, Minimize2 } from 'lucide-react'
import { AttachmentBar, type PendingFile } from './AttachmentBar'
import { useT } from '@/lib/i18n'

export interface ChatInputHandle {
  addFiles: (files: File[]) => void
}

interface Props {
  isStreaming: boolean
  onSend: (text: string, files?: PendingFile[]) => void
  onStop: () => void
  onCompact?: () => void
  disabled?: boolean
}

export const ChatInput = forwardRef<ChatInputHandle, Props>(function ChatInput(
  { isStreaming, onSend, onStop, onCompact, disabled },
  ref
) {
  const t = useT()
  const [text, setText] = useState('')
  const [files, setFiles] = useState<PendingFile[]>([])
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const handleSend = useCallback(() => {
    const trimmed = text.trim()
    if (!trimmed || disabled) return
    onSend(trimmed, files.length > 0 ? files : undefined)
    setText('')
    setFiles([])
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
    }
  }, [text, disabled, onSend, files])

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      if (isStreaming) return
      handleSend()
    }
  }

  const handleInput = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setText(e.target.value)
    const el = e.target
    el.style.height = 'auto'
    el.style.height = Math.min(el.scrollHeight, 200) + 'px'
  }

  const addFiles = useCallback((newFiles: File[]) => {
    const pending: PendingFile[] = newFiles
      .slice(0, 5 - files.length) // enforce max 5
      .map(f => ({ file: f, id: crypto.randomUUID() }))
    setFiles(prev => [...prev, ...pending].slice(0, 5))
  }, [files.length])

  const removeFile = useCallback((id: string) => {
    setFiles(prev => prev.filter(f => f.id !== id))
  }, [])

  useImperativeHandle(ref, () => ({ addFiles }), [addFiles])

  const canSend = !disabled && !isStreaming && !!text.trim()
  const btnActive = canSend || isStreaming

  return (
    <div
      style={{
        borderTop: '1px solid #e5e7eb',
        padding: '12px 16px 10px',
        background: 'white',
      }}
    >
      <div
        style={{
          position: 'relative',
          background: '#f9f9f8',
          border: '1px solid #e5e7eb',
          borderRadius: 12,
          transition: 'border-color 150ms',
        }}
        onFocusCapture={(e) => {
          e.currentTarget.style.borderColor = '#c7c7c5'
        }}
        onBlurCapture={(e) => {
          if (!e.currentTarget.contains(e.relatedTarget)) {
            e.currentTarget.style.borderColor = '#e5e7eb'
          }
        }}
      >
        {/* Attachment preview bar */}
        <AttachmentBar files={files} onRemove={removeFile} />

        <textarea
          ref={textareaRef}
          value={text}
          onChange={handleInput}
          onKeyDown={handleKeyDown}
          placeholder={disabled ? t('chat.disabledHint') : t('chat.placeholder')}
          disabled={disabled}
          rows={1}
          style={{
            display: 'block',
            width: '100%',
            background: 'transparent',
            border: 'none',
            outline: 'none',
            resize: 'none',
            fontSize: 14,
            lineHeight: '24px',
            color: '#1a1a1a',
            fontFamily: 'inherit',
            maxHeight: 200,
            overflowY: 'auto',
            boxSizing: 'border-box',
            padding: '10px 76px 10px 14px',
          }}
        />

        {/* Toolbar buttons */}
        <div
          style={{
            position: 'absolute',
            right: 8,
            bottom: 8,
            display: 'flex',
            alignItems: 'center',
            gap: 4,
          }}
        >
          {/* Compact conversation button */}
          {onCompact && (
            <button
              onClick={() => {
                if (window.confirm(t('chat.compactConfirm'))) onCompact()
              }}
              disabled={disabled || isStreaming}
              title={t('chat.compactConversation')}
              style={{
                width: 28,
                height: 28,
                borderRadius: 7,
                border: 'none',
                cursor: disabled || isStreaming ? 'default' : 'pointer',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                background: 'transparent',
                color: '#9b9a97',
                transition: 'color 120ms',
              }}
              onMouseEnter={(e) => {
                if (!disabled && !isStreaming) e.currentTarget.style.color = '#374151'
              }}
              onMouseLeave={(e) => {
                e.currentTarget.style.color = '#9b9a97'
              }}
            >
              <Minimize2 size={13} strokeWidth={2} />
            </button>
          )}

          {/* File attachment button */}
          <button
            onClick={() => fileInputRef.current?.click()}
            disabled={disabled || isStreaming}
            title={t('chat.attachFile')}
            style={{
              width: 28,
              height: 28,
              borderRadius: 7,
              border: 'none',
              cursor: disabled || isStreaming ? 'default' : 'pointer',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              background: 'transparent',
              color: '#9b9a97',
              transition: 'color 120ms',
            }}
            onMouseEnter={(e) => {
              if (!disabled && !isStreaming) e.currentTarget.style.color = '#374151'
            }}
            onMouseLeave={(e) => {
              e.currentTarget.style.color = '#9b9a97'
            }}
          >
            <Paperclip size={14} strokeWidth={2} />
          </button>

          {/* Send / Stop button */}
          <button
            onClick={isStreaming ? onStop : handleSend}
            disabled={!btnActive}
            style={{
              width: 28,
              height: 28,
              borderRadius: 7,
              border: 'none',
              cursor: btnActive ? 'pointer' : 'default',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              flexShrink: 0,
              background: btnActive
                ? isStreaming ? '#f3f4f6' : '#1a1a1a'
                : '#e5e7eb',
              color: btnActive
                ? isStreaming ? '#374151' : 'white'
                : '#9b9a97',
              transition: 'background 120ms',
            }}
          >
            {isStreaming
              ? <Square size={11} strokeWidth={2.5} />
              : <ArrowUp size={14} strokeWidth={2.5} />
            }
          </button>
        </div>

        {/* Hidden file input */}
        <input
          ref={fileInputRef}
          type="file"
          multiple
          style={{ display: 'none' }}
          onChange={(e) => {
            if (e.target.files) {
              addFiles(Array.from(e.target.files))
              e.target.value = '' // allow re-selecting same file
            }
          }}
        />
      </div>

      <p style={{ fontSize: 11, color: '#c7c7c5', textAlign: 'center', marginTop: 6 }}>
        {t('chat.shiftEnterHint')} · {t('chat.disclaimer')}
      </p>
    </div>
  )
})
