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
      {/* Container: position:relative so button can be absolute */}
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
        <textarea
          ref={textareaRef}
          value={text}
          onChange={handleInput}
          onKeyDown={handleKeyDown}
          placeholder={disabled ? '请先配置 API Key 和模型' : '输入消息…'}
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
            lineHeight: '24px',        // fixed line height for predictability
            color: '#1a1a1a',
            fontFamily: 'inherit',
            maxHeight: 200,
            overflowY: 'auto',
            boxSizing: 'border-box',
            padding: '10px 46px 10px 14px',  // right padding leaves room for button
          }}
        />

        {/* Button: absolute, always sticks to bottom-right */}
        <button
          onClick={isStreaming ? onStop : handleSend}
          disabled={!btnActive}
          style={{
            position: 'absolute',
            right: 8,
            bottom: 8,
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

      <p style={{ fontSize: 11, color: '#c7c7c5', textAlign: 'center', marginTop: 6 }}>
        Shift+Enter 换行 · AI 可能会出错，请核实重要信息
      </p>
    </div>
  )
}
