import { useState, useEffect } from 'react'
import { Check } from 'lucide-react'
import { api } from '@/lib/api'

interface ModelInfo {
  id: string
  name: string
  tier: string
  provider: string
}

interface ModelAssignment {
  provider: string
  modelId: string
}

interface ModelConfig {
  conversation: ModelAssignment
  codegen: ModelAssignment
  cheap: ModelAssignment
  fallback: ModelAssignment
}

const PURPOSE_LABELS: Record<keyof ModelConfig, string> = {
  conversation: '主对话模型',
  codegen: '代码生成模型',
  cheap: '低成本模型',
  fallback: '备用模型（降级）',
}

const EMPTY: ModelAssignment = { provider: '', modelId: '' }

export function ModelSettings() {
  const [models, setModels] = useState<ModelInfo[]>([])
  const [config, setConfig] = useState<ModelConfig>({
    conversation: EMPTY,
    codegen: EMPTY,
    cheap: EMPTY,
    fallback: EMPTY,
  })
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    api<ModelInfo[]>('/api/models').then(setModels).catch(() => {})
    api<ModelConfig>('/api/model-config').then(setConfig).catch(() => {})
  }, [])

  const handleSave = async () => {
    try {
      await api('/api/model-config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(config),
      })
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
    } catch (e) {
      alert('保存失败: ' + String(e))
    }
  }

  const setAssignment = (key: keyof ModelConfig, provider: string, modelId: string) => {
    setConfig((prev) => ({ ...prev, [key]: { provider, modelId } }))
  }

  if (models.length === 0) {
    return (
      <div style={{ padding: '12px 0' }}>
        <p style={{ fontSize: 11, fontWeight: 600, color: '#9b9a97', textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: 12 }}>
          模型设置
        </p>
        <p style={{ fontSize: 13, color: '#9b9a97' }}>
          请先在上方配置至少一个 API Key，才能选择模型。
        </p>
      </div>
    )
  }

  const modelsByProvider: Record<string, ModelInfo[]> = {}
  for (const m of models) {
    if (!modelsByProvider[m.provider]) modelsByProvider[m.provider] = []
    modelsByProvider[m.provider].push(m)
  }

  return (
    <div>
      <p style={{ fontSize: 11, fontWeight: 600, color: '#9b9a97', textTransform: 'uppercase', letterSpacing: '0.05em', marginBottom: 12 }}>
        模型设置
      </p>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 14 }}>
        {(Object.keys(PURPOSE_LABELS) as (keyof ModelConfig)[]).map((key) => (
          <div key={key}>
            <label style={{ fontSize: 12, fontWeight: 500, color: '#374151', display: 'block', marginBottom: 6 }}>
              {PURPOSE_LABELS[key]}
              {(key === 'codegen' || key === 'cheap') && (
                <span style={{ color: '#9b9a97', fontWeight: 400 }}> (空 = 同主对话)</span>
              )}
            </label>
            <select
              value={config[key].provider && config[key].modelId ? `${config[key].provider}:${config[key].modelId}` : ''}
              onChange={(e) => {
                const val = e.target.value
                if (!val) {
                  setAssignment(key, '', '')
                } else {
                  const idx = val.indexOf(':')
                  setAssignment(key, val.slice(0, idx), val.slice(idx + 1))
                }
              }}
              style={{
                width: '100%', padding: '7px 10px', fontSize: 13,
                border: '1px solid #e5e7eb', borderRadius: 6, outline: 'none',
                background: 'white', color: '#111827', cursor: 'pointer',
                boxSizing: 'border-box',
              }}
            >
              <option value="">— 不设置 —</option>
              {Object.entries(modelsByProvider).map(([provider, provModels]) => (
                <optgroup key={provider} label={provider}>
                  {provModels.map((m) => (
                    <option key={m.id} value={`${provider}:${m.id}`}>
                      {m.name} ({m.tier})
                    </option>
                  ))}
                </optgroup>
              ))}
            </select>
          </div>
        ))}
      </div>

      <button
        onClick={handleSave}
        style={{
          marginTop: 20, padding: '8px 16px', fontSize: 13, fontWeight: 500,
          borderRadius: 6, border: 'none', cursor: 'pointer',
          background: saved ? '#ecfdf5' : '#111827',
          color: saved ? '#16a34a' : 'white',
          display: 'flex', alignItems: 'center', gap: 6,
          transition: 'background 200ms',
        }}
      >
        {saved && <Check size={13} />}
        {saved ? '已保存' : '保存设置'}
      </button>
    </div>
  )
}
