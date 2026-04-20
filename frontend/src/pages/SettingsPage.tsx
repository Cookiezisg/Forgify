import { useState, useCallback } from 'react'
import { ApiKeySettings } from '@/components/settings/ApiKeySettings'
import { ModelSettings } from '@/components/settings/ModelSettings'
import { useI18n, useT } from '@/lib/i18n'
import type { Locale } from '@/lib/i18n'

function LanguageSettings() {
  const { locale, setLocale } = useI18n()
  const t = useT()

  const options: { value: Locale; label: string }[] = [
    { value: 'zh-CN', label: '简体中文' },
    { value: 'en',    label: 'English' },
  ]

  return (
    <section style={{ marginBottom: 36 }}>
      <p style={{ fontSize: 11, fontWeight: 600, color: '#9b9a97', textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: 12 }}>
        {t('settings.languageSection')}
      </p>
      <div style={{ display: 'flex', gap: 8 }}>
        {options.map((opt) => (
          <button
            key={opt.value}
            onClick={() => setLocale(opt.value)}
            style={{
              padding: '6px 16px',
              fontSize: 13,
              fontWeight: locale === opt.value ? 500 : 400,
              borderRadius: 6,
              border: `1px solid ${locale === opt.value ? '#111827' : '#e5e7eb'}`,
              background: locale === opt.value ? '#111827' : 'white',
              color: locale === opt.value ? 'white' : '#374151',
              cursor: 'pointer',
              transition: 'all 120ms',
            }}
          >
            {opt.label}
          </button>
        ))}
      </div>
    </section>
  )
}

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
  // Bump this counter after any API key change to trigger ModelSettings re-fetch
  const [keyVersion, setKeyVersion] = useState(0)
  const onKeysChanged = useCallback(() => setKeyVersion(v => v + 1), [])

  return (
    <div style={{ height: '100%', overflowY: 'auto' }}>
      <div style={{ maxWidth: 600, margin: '0 auto', padding: '28px 32px' }}>
        <h1 style={{ fontSize: 20, fontWeight: 600, color: '#111827', marginBottom: 32 }}>
          {t('settings.title')}
        </h1>

        <LanguageSettings />

        <div style={{ height: 1, background: '#f3f4f6', marginBottom: 32 }} />

        <section style={{ marginBottom: 36 }}>
          <ApiKeySettings onKeysChanged={onKeysChanged} />
        </section>

        <div style={{ height: 1, background: '#f3f4f6', marginBottom: 32 }} />

        <section>
          <ModelSettings refreshKey={keyVersion} />
        </section>
      </div>
    </div>
  )
}
