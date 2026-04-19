import { ApiKeySettings } from '@/components/settings/ApiKeySettings'
import { ModelSettings } from '@/components/settings/ModelSettings'

export function SettingsLeftPanel() {
  return (
    <div style={{ padding: '8px 12px' }}>
      <p style={{ fontSize: 11, fontWeight: 600, color: '#9b9a97', textTransform: 'uppercase', letterSpacing: '0.05em', padding: '6px 2px' }}>
        设置
      </p>
    </div>
  )
}

export function SettingsContent() {
  return (
    <div style={{ height: '100%', overflowY: 'auto' }}>
      <div style={{ maxWidth: 600, margin: '0 auto', padding: '28px 32px' }}>
        <h1 style={{ fontSize: 20, fontWeight: 600, color: '#111827', marginBottom: 32 }}>
          设置
        </h1>

        <section style={{ marginBottom: 36 }}>
          <ApiKeySettings />
        </section>

        <div style={{ height: 1, background: '#f3f4f6', marginBottom: 32 }} />

        <section>
          <ModelSettings />
        </section>
      </div>
    </div>
  )
}
