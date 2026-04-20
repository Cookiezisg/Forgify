import { useState, useEffect } from 'react'
import { Check, X, ChevronRight, Loader } from 'lucide-react'
import { api } from '@/lib/api'
import { useT } from '@/lib/i18n'

interface APIKey {
  id: string
  provider: string
  displayName: string
  keyMasked: string
  baseUrl: string
  testStatus: string
}

const PROVIDERS = [
  { id: 'anthropic',   name: 'Anthropic',        subtitle: 'Claude Opus/Sonnet/Haiku',  needsKey: true,  needsUrl: false },
  { id: 'openai',      name: 'OpenAI',            subtitle: 'GPT-4o, o3-mini',           needsKey: true,  needsUrl: false },
  { id: 'deepseek',    name: 'DeepSeek',          subtitle: 'DeepSeek V3/R1',            needsKey: true,  needsUrl: false },
  { id: 'siliconflow', name: 'SiliconFlow 硅基流动', subtitle: '聚合模型，性价比高',       needsKey: true,  needsUrl: false },
  { id: 'groq',        name: 'Groq',              subtitle: 'Llama / Mixtral（极速）',   needsKey: true,  needsUrl: false },
  { id: 'mistral',     name: 'Mistral AI',        subtitle: 'Mistral Large / Codestral', needsKey: true,  needsUrl: false },
  { id: 'gemini',      name: 'Google Gemini',     subtitle: 'Gemini 2.0 Flash/Pro',      needsKey: true,  needsUrl: false },
  { id: 'moonshot',    name: 'Moonshot (Kimi)',   subtitle: 'moonshot-v1',               needsKey: true,  needsUrl: false },
  { id: 'zhipu',       name: '智谱 (GLM)',         subtitle: 'GLM-4 Plus/Flash',          needsKey: true,  needsUrl: false },
  { id: 'openrouter',  name: 'OpenRouter',        subtitle: '100+ 模型聚合',             needsKey: true,  needsUrl: false },
  { id: 'ollama',      name: 'Ollama',            subtitle: '本地模型',                  needsKey: false, needsUrl: true  },
  { id: 'openai_compat', name: 'OpenAI 兼容',    subtitle: '自定义端点',                needsKey: true,  needsUrl: true  },
]

export function ApiKeySettings({ onKeysChanged }: { onKeysChanged?: () => void }) {
  const t = useT()
  const [keys, setKeys] = useState<APIKey[]>([])
  const [drawer, setDrawer] = useState<{ provider: string; existing?: APIKey } | null>(null)

  const reload = () => {
    api<APIKey[]>('/api/api-keys').then(setKeys).catch(() => {})
    onKeysChanged?.()
  }

  useEffect(() => { reload() }, [])

  const getKey = (provider: string) => keys.find((k) => k.provider === provider)

  return (
    <div>
      <p style={{ fontSize: 11, fontWeight: 600, color: '#9b9a97', textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: 12 }}>
        {t('settings.apiKeysSection')}
      </p>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
        {PROVIDERS.map((p) => {
          const existing = getKey(p.id)
          return (
            <div
              key={p.id}
              onClick={() => setDrawer({ provider: p.id, existing })}
              style={{
                display: 'flex',
                alignItems: 'center',
                padding: '10px 12px',
                borderRadius: 8,
                cursor: 'pointer',
                border: '1px solid #f3f4f6',
                background: 'white',
                transition: 'border-color 150ms',
              }}
              onMouseEnter={(e) => (e.currentTarget.style.borderColor = '#e5e7eb')}
              onMouseLeave={(e) => (e.currentTarget.style.borderColor = '#f3f4f6')}
            >
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: 14, fontWeight: 500, color: '#111827' }}>{p.name}</div>
                <div style={{ fontSize: 12, color: '#9b9a97', marginTop: 1 }}>{p.subtitle}</div>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                {existing ? (
                  <>
                    <span style={{ fontSize: 12, color: '#6b7280', fontFamily: 'monospace' }}>
                      {existing.keyMasked}
                    </span>
                    <span
                      style={{
                        fontSize: 11,
                        padding: '2px 7px',
                        borderRadius: 20,
                        background: existing.testStatus === 'ok' ? '#ecfdf5' : existing.testStatus === 'error' ? '#fef2f2' : '#f9fafb',
                        color: existing.testStatus === 'ok' ? '#16a34a' : existing.testStatus === 'error' ? '#dc2626' : '#6b7280',
                      }}
                    >
                      {existing.testStatus === 'ok' ? t('apikey.statusOk') : existing.testStatus === 'error' ? t('apikey.statusError') : t('apikey.statusConfigured')}
                    </span>
                  </>
                ) : (
                  <span style={{ fontSize: 12, color: '#c7c7c5' }}>{t('apikey.notConfigured')}</span>
                )}
                <ChevronRight size={14} strokeWidth={1.5} style={{ color: '#d1d5db' }} />
              </div>
            </div>
          )
        })}
      </div>
      {drawer && (
        <APIKeyDrawer
          provider={drawer.provider}
          existing={drawer.existing}
          onClose={() => setDrawer(null)}
          onSaved={reload}
        />
      )}
    </div>
  )
}

function APIKeyDrawer({
  provider,
  existing,
  onClose,
  onSaved,
}: {
  provider: string
  existing?: APIKey
  onClose: () => void
  onSaved: () => void
}) {
  const t = useT()
  const info = PROVIDERS.find((p) => p.id === provider)!
  const isEditing = !!existing

  // Pre-fill baseUrl from existing key; apiKey always starts empty (it's encrypted server-side)
  const [apiKey, setApiKey] = useState('')
  const [baseUrl, setBaseUrl] = useState(existing?.baseUrl || '')
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<{ ok: boolean; message: string } | null>(null)
  const [saving, setSaving] = useState(false)

  // Whether we have a key to work with: either newly entered or already saved
  const hasKeyInput = !!apiKey
  const hasExistingKey = isEditing
  const canTest = !testing && (hasKeyInput || hasExistingKey || !info.needsKey)
  const canSave = !saving && (hasKeyInput || !info.needsKey)

  const handleTest = async () => {
    if (!canTest) return
    setTesting(true)
    setTestResult(null)
    try {
      const res = await api<{ ok: boolean; message: string }>('/api/api-keys/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        // Send key if user entered one; otherwise backend uses the saved key
        body: JSON.stringify({
          provider,
          key: apiKey, // empty string means "use saved key"
          baseUrl: baseUrl || undefined,
        }),
      })
      setTestResult(res)
    } catch (e) {
      setTestResult({ ok: false, message: String(e) })
    } finally {
      setTesting(false)
    }
  }

  const handleSave = async () => {
    if (!canSave) return
    setSaving(true)
    try {
      await api('/api/api-keys', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          id: existing?.id,
          provider,
          displayName: info.name,
          key: apiKey,
          baseUrl,
        }),
      })
      // After save, run test to persist test status
      await api('/api/api-keys/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ provider, key: apiKey, baseUrl }),
      }).catch(() => {})
      onSaved()
      onClose()
    } catch (e) {
      alert(t('apikey.saveFailed') + String(e))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!existing) return
    if (!confirm(t('apikey.confirmDelete'))) return
    await api(`/api/api-keys/${existing.id}`, { method: 'DELETE' }).catch(() => {})
    onSaved()
    onClose()
  }

  return (
    <>
      {/* Backdrop */}
      <div
        onClick={onClose}
        style={{
          position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.3)', zIndex: 40,
        }}
      />
      {/* Drawer */}
      <div
        style={{
          position: 'fixed', right: 0, top: 0, bottom: 0,
          width: 360, background: 'white', zIndex: 50,
          borderLeft: '1px solid #e5e7eb',
          display: 'flex', flexDirection: 'column',
          padding: '24px 20px',
          gap: 16,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <span style={{ fontSize: 15, fontWeight: 600, color: '#111827' }}>{t('apikey.configure')} {info.name}</span>
          <button
            onClick={onClose}
            style={{ border: 'none', background: 'none', cursor: 'pointer', color: '#9b9a97', padding: 4 }}
          >
            <X size={16} />
          </button>
        </div>

        {/* Show existing key info */}
        {isEditing && (
          <div style={{ padding: '8px 12px', background: '#f9fafb', borderRadius: 6, fontSize: 12, color: '#6b7280' }}>
            <span style={{ fontFamily: 'monospace' }}>{existing.keyMasked}</span>
            {existing.testStatus === 'ok' && <span style={{ marginLeft: 8, color: '#16a34a' }}>{t('apikey.statusOk')}</span>}
            {existing.testStatus === 'error' && <span style={{ marginLeft: 8, color: '#dc2626' }}>{t('apikey.statusError')}</span>}
          </div>
        )}

        {info.needsKey && (
          <div>
            <label style={{ fontSize: 12, fontWeight: 500, color: '#374151', display: 'block', marginBottom: 6 }}>
              {isEditing ? t('apikey.newKeyLabel') : t('apikey.keyLabel')}
            </label>
            <input
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder={isEditing ? t('apikey.leaveEmpty') : t('apikey.pasteKey')}
              style={{
                width: '100%', padding: '8px 10px', fontSize: 13,
                border: '1px solid #e5e7eb', borderRadius: 6, outline: 'none',
                fontFamily: 'monospace', boxSizing: 'border-box',
              }}
            />
          </div>
        )}

        {info.needsUrl && (
          <div>
            <label style={{ fontSize: 12, fontWeight: 500, color: '#374151', display: 'block', marginBottom: 6 }}>
              {t('apikey.urlLabel')} {!info.needsKey && t('apikey.required')}
            </label>
            <input
              type="text"
              value={baseUrl}
              onChange={(e) => setBaseUrl(e.target.value)}
              placeholder={provider === 'ollama' ? 'http://localhost:11434' : 'https://api.example.com/v1'}
              style={{
                width: '100%', padding: '8px 10px', fontSize: 13,
                border: '1px solid #e5e7eb', borderRadius: 6, outline: 'none',
                boxSizing: 'border-box',
              }}
            />
          </div>
        )}

        <button
          onClick={handleTest}
          disabled={!canTest}
          style={{
            padding: '7px 12px', fontSize: 13, borderRadius: 6, cursor: canTest ? 'pointer' : 'default',
            border: '1px solid #e5e7eb', background: 'white', color: '#374151',
            display: 'flex', alignItems: 'center', gap: 6, alignSelf: 'flex-start',
            opacity: canTest ? 1 : 0.5,
          }}
        >
          {testing ? <Loader size={13} style={{ animation: 'spin 1s linear infinite' }} /> : null}
          {t('apikey.testConnection')}
        </button>

        {testResult && (
          <div
            style={{
              display: 'flex', alignItems: 'flex-start', gap: 8, padding: '8px 12px',
              borderRadius: 6, fontSize: 13,
              background: testResult.ok ? '#f0fdf4' : '#fef2f2',
              color: testResult.ok ? '#166534' : '#991b1b',
            }}
          >
            {testResult.ok ? <Check size={14} style={{ marginTop: 1, flexShrink: 0 }} /> : <X size={14} style={{ marginTop: 1, flexShrink: 0 }} />}
            <span>{testResult.message}</span>
          </div>
        )}

        <div style={{ flex: 1 }} />

        <div style={{ display: 'flex', gap: 8 }}>
          {/* Save: only when user entered a new key (or needsKey is false) */}
          <button
            onClick={handleSave}
            disabled={!canSave}
            style={{
              flex: 1, padding: '8px 0', fontSize: 13, fontWeight: 500,
              borderRadius: 6, border: 'none', cursor: canSave ? 'pointer' : 'default',
              background: canSave ? '#111827' : '#e5e7eb',
              color: canSave ? 'white' : '#9b9a97',
            }}
          >
            {saving ? t('apikey.saving') : t('apikey.save')}
          </button>
          {isEditing && (
            <button
              onClick={handleDelete}
              style={{
                padding: '8px 14px', fontSize: 13, borderRadius: 6,
                border: '1px solid #fca5a5', background: 'white', color: '#dc2626', cursor: 'pointer',
              }}
            >
              {t('apikey.delete')}
            </button>
          )}
        </div>
      </div>
    </>
  )
}
