import { useT } from '@/lib/i18n'

export function EmptyChat() {
  const t = useT()

  return (
    <div
      className="flex flex-col items-center justify-center h-full"
      style={{ gap: 10, userSelect: 'none' }}
    >
      <p
        style={{
          fontSize: 22,
          fontWeight: 600,
          color: '#1a1a1a',
          letterSpacing: '-0.01em',
        }}
      >
        {t('chat.startConversation')}
      </p>
      <p
        style={{
          fontSize: 14,
          color: '#9b9a97',
        }}
      >
        {t('chat.startHint')}
      </p>
    </div>
  )
}
