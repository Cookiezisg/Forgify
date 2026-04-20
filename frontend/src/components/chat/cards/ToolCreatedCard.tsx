import { useT } from '@/lib/i18n'

interface Props {
  toolName: string
  status: string
}

export function ToolCreatedCard({ toolName, status }: Props) {
  const t = useT()

  return (
    <div
      style={{
        border: '1px solid #e5e7eb',
        borderRadius: 10,
        padding: '12px 16px',
        maxWidth: 320,
        margin: '4px 0',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 6 }}>
        <span style={{ fontSize: 14 }}>✅</span>
        <span style={{ fontSize: 13, fontWeight: 600, color: '#111827' }}>{t('tools.saveAsTool')}</span>
      </div>
      <p style={{ fontSize: 13, color: '#374151', marginBottom: 4 }}>📦 {toolName}</p>
      <p style={{ fontSize: 12, color: '#9b9a97' }}>
        {status === 'tested' ? t('tools.tested') : t('tools.draft')}
      </p>
      <button
        onClick={() => window.dispatchEvent(new CustomEvent('nav:goTo', { detail: 'assets' }))}
        style={{
          marginTop: 8, fontSize: 12, color: '#2383e2', background: 'none',
          border: 'none', cursor: 'pointer', padding: 0,
        }}
      >
        {t('tools.detail')} →
      </button>
    </div>
  )
}
