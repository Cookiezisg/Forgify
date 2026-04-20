import { useState, useEffect } from 'react'
import { onEvent, EventNames } from '@/lib/events'
import { useT } from '@/lib/i18n'

interface Props {
  conversationId: string | null
}

export function CompactBanner({ conversationId }: Props) {
  const t = useT()
  const [level, setLevel] = useState<string | null>(null)
  const [summaryVisible, setSummaryVisible] = useState(false)

  // Listen for compression events
  useEffect(() => {
    if (!conversationId) return
    return onEvent<{ conversationId: string; level: string }>(
      EventNames.ChatCompacted,
      (e) => {
        if (e.conversationId === conversationId) {
          setLevel(e.level)
        }
      }
    )
  }, [conversationId])

  // Reset on conversation change
  useEffect(() => {
    setLevel(null)
    setSummaryVisible(false)
  }, [conversationId])

  if (!level || level === 'micro') return null

  return (
    <>
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '6px 16px',
          background: '#fafaf9',
          borderBottom: '1px solid #e5e7eb',
          fontSize: 12,
          color: '#9b9a97',
        }}
      >
        <span>{t('chat.compactBanner')}</span>
        <button
          onClick={() => setSummaryVisible(!summaryVisible)}
          style={{
            padding: 0,
            border: 'none',
            background: 'none',
            cursor: 'pointer',
            fontSize: 12,
            color: '#2383e2',
          }}
        >
          {t('chat.viewSummary')}
        </button>
      </div>

      {summaryVisible && (
        <div
          style={{
            padding: '12px 16px',
            background: '#f7f7f5',
            borderBottom: '1px solid #e5e7eb',
            fontSize: 13,
            lineHeight: '1.6',
            color: '#374151',
            maxHeight: 200,
            overflowY: 'auto',
            whiteSpace: 'pre-wrap',
          }}
        >
          {'(摘要将在下次加载时显示)'}
        </div>
      )}
    </>
  )
}
