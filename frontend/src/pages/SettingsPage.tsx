import { useT } from '@/lib/i18n'

export function SettingsLeftPanel() {
  const t = useT()
  return (
    <div style={{ padding: '8px 12px' }}>
      <p style={{ fontSize: 11, fontWeight: 600, color: '#9b9a97', textTransform: 'uppercase', letterSpacing: '0.05em', padding: '6px 2px' }}>
        {t('settings.title')}
      </p>
    </div>
  )
}

export function SettingsContent() {
  const t = useT()
  return (
    <div style={{ height: '100%', overflowY: 'auto' }}>
      <div style={{ maxWidth: 600, margin: '0 auto', padding: '28px 32px' }}>
        <h1 style={{ fontSize: 20, fontWeight: 600, color: '#111827', marginBottom: 32 }}>
          {t('settings.title')}
        </h1>
        <p style={{ color: '#9b9a97' }}>Settings UI will be rebuilt here.</p>
      </div>
    </div>
  )
}
