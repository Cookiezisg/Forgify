import { useState, useRef, useCallback } from 'react'
import { ArrowUp, Square } from 'lucide-react'

interface Props {
  isStreaming: boolean
  onSend: (text: string) => void
  onStop: () => void
  disabled?: boolean
}

export function ChatInput({ isStreaming, onSend, onStop, disabled }: Props) {
  const [text, setText] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const handleSend = useCallback(() => {
    const trimmed = text.trim()
    if (!trimmed || disabled) return
    onSend(trimmed)
    setText('')
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto'
    }
  }, [text, disabled, onSend])

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

  return (
    <div
      style={{
        borderTop: '1px solid #e5e7eb',
        padding: '12px 16px',
        background: 'white',
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'flex-end',
          gap: 8,
          background: '#f9f9f8',
          border: '1px solid #e5e7eb',
          borderRadius: 10,
          padding: '8px 10px',
          transition: 'border-color 150ms',
        }}
        onFocus={(e) => {
          const parent = e.currentTarget
          parent.style.borderColor = '#c7c7c5'
        }}
        onBlur={(e) => {
          const parent = e.currentTarget
          if (!e.currentTarget.contains(e.relatedTarget)) {
            parent.style.borderColor = '#e5e7eb'
          }
        }}
      >
        <textarea
          ref={textareaRef}
          value={text}
          onChange={handleInput}
          onKeyDown={handleKeyDown}
          placeholder={disabled ? '请先配置 API Key 和模型' : '输入消息… (Shift+Enter 换行)'}
          disabled={disabled}
          rows={1}
          style={{
            flex: 1,
            background: 'transparent',
            border: 'none',
            outline: 'none',
            resize: 'none',
            fontSize: 14,
            lineHeight: '1.5',
            color: '#1a1a1a',
            fontFamily: 'inherit',
            maxHeight: 200,
            overflowY: 'auto',
          }}
        />
        <button
          onClick={isStreaming ? onStop : handleSend}
          disabled={disabled || (!isStreaming && !text.trim())}
          style={{
            flexShrink: 0,
            width: 30,
            height: 30,
            borderRadius: 8,
            border: 'none',
            cursor: disabled || (!isStreaming && !text.trim()) ? 'default' : 'pointer',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            background:
              disabled || (!isStreaming && !text.trim())
                ? '#e5e7eb'
                : isStreaming
                  ? '#f9f9f8'
                  : '#1a1a1a',
            color:
              disabled || (!isStreaming && !text.trim())
                ? '#9b9a97'
                : isStreaming
                  ? '#1a1a1a'
                  : 'white',
            transition: 'background 150ms',
          }}
        >
          {isStreaming ? (
            <Square size={12} strokeWidth={2} />
          ) : (
            <ArrowUp size={14} strokeWidth={2} />
          )}
        </button>
      </div>
      <p style={{ fontSize: 11, color: '#c7c7c5', textAlign: 'center', marginTop: 6 }}>
        AI 可能会出错，请核实重要信息
      </p>
    </div>
  )
}
