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

export function ApiKeySettings() {
  const t = useT()
  const [keys, setKeys] = useState<APIKey[]>([])
  const [drawer, setDrawer] = useState<{ provider: string; id?: string } | null>(null)

  const reload = () => api<APIKey[]>('/api/api-keys').then(setKeys).catch(() => {})

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
              onClick={() => setDrawer({ provider: p.id, id: existing?.id })}
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
          existingId={drawer.id}
          onClose={() => setDrawer(null)}
          onSaved={reload}
        />
      )}
    </div>
  )
}

function APIKeyDrawer({
  provider,
  existingId,
  onClose,
  onSaved,
}: {
  provider: string
  existingId?: string
  onClose: () => void
  onSaved: () => void
}) {
  const t = useT()
  const info = PROVIDERS.find((p) => p.id === provider)!
  const [apiKey, setApiKey] = useState('')
  const [baseUrl, setBaseUrl] = useState('')
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<{ ok: boolean; message: string } | null>(null)
  const [saving, setSaving] = useState(false)

  const handleTest = async () => {
    if (!apiKey && info.needsKey) return
    setTesting(true)
    setTestResult(null)
    try {
      const res = await api<{ ok: boolean; message: string }>('/api/api-keys/test', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ provider, key: apiKey, baseUrl }),
      })
      setTestResult(res)
    } catch (e) {
      setTestResult({ ok: false, message: String(e) })
    } finally {
      setTesting(false)
    }
  }

  const handleSave = async () => {
    if (!apiKey && info.needsKey) return
    setSaving(true)
    try {
      await api('/api/api-keys', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          id: existingId,
          provider,
          displayName: info.name,
          key: apiKey,
          baseUrl,
        }),
      })
      if (testResult?.ok) {
        // mark as tested
        const keys = await api<{ id: string }[]>('/api/api-keys')
        const saved = keys.find((k: any) => k.provider === provider)
        if (saved) {
          await api('/api/api-keys/test', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ provider, key: apiKey, baseUrl }),
          }).catch(() => {})
        }
      }
      onSaved()
      onClose()
    } catch (e) {
      alert(t('apikey.saveFailed') + String(e))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async () => {
    if (!existingId) return
    if (!confirm(t('apikey.confirmDelete'))) return
    await api(`/api/api-keys/${existingId}`, { method: 'DELETE' }).catch(() => {})
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

        {info.needsKey && (
          <div>
            <label style={{ fontSize: 12, fontWeight: 500, color: '#374151', display: 'block', marginBottom: 6 }}>
              {t('apikey.keyLabel')}
            </label>
            <input
              type="password"
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
              placeholder={t('apikey.pasteKey')}
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
          disabled={testing || (info.needsKey && !apiKey)}
          style={{
            padding: '7px 12px', fontSize: 13, borderRadius: 6, cursor: 'pointer',
            border: '1px solid #e5e7eb', background: 'white', color: '#374151',
            display: 'flex', alignItems: 'center', gap: 6, alignSelf: 'flex-start',
            opacity: testing || (info.needsKey && !apiKey) ? 0.5 : 1,
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
          <button
            onClick={handleSave}
            disabled={saving || (info.needsKey && !apiKey)}
            style={{
              flex: 1, padding: '8px 0', fontSize: 13, fontWeight: 500,
              borderRadius: 6, border: 'none', cursor: saving || (info.needsKey && !apiKey) ? 'default' : 'pointer',
              background: saving || (info.needsKey && !apiKey) ? '#e5e7eb' : '#111827',
              color: saving || (info.needsKey && !apiKey) ? '#9b9a97' : 'white',
            }}
          >
            {saving ? t('apikey.saving') : t('apikey.save')}
          </button>
          {existingId && (
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
