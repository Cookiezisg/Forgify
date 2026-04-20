import { useT } from '@/lib/i18n'

export function ChatLeftPanel() {
  const t = useT()
  return (
    <div className="flex flex-col h-full">
      <div style={{ padding: '8px 12px' }}>
        <p style={{ fontSize: 11, fontWeight: 600, color: '#9b9a97', textTransform: 'uppercase', letterSpacing: '0.05em', padding: '6px 2px' }}>
          {t('nav.chat')}
        </p>
        <p style={{ fontSize: 12, color: '#c7c7c5', padding: '8px 2px' }}>{t('chat.noChats')}</p>
      </div>
    </div>
  )
}
