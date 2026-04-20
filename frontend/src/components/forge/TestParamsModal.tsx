import { useState, useEffect } from 'react'
import { X, Loader } from 'lucide-react'
import { api } from '@/lib/api'
import { useT } from '@/lib/i18n'

interface Tool {
  id: string
  name: string
  parameters: { name: string; type: string; required: boolean; default?: string }[]
}

interface Props {
  toolId: string
  onClose: () => void
  onResult: (result: { passed: boolean; output?: any; error?: string; durationMs: number }) => void
}

export function TestParamsModal({ toolId, onClose, onResult }: Props) {
  const t = useT()
  const [tool, setTool] = useState<Tool | null>(null)
  const [values, setValues] = useState<Record<string, string>>({})
  const [running, setRunning] = useState(false)

  useEffect(() => {
    api<Tool>(`/api/tools/${toolId}`).then(setTool).catch(() => {})
  }, [toolId])

  const run = async () => {
    if (!tool) return
    setRunning(true)
    try {
      const params: Record<string, any> = {}
      for (const p of tool.parameters) {
        const v = values[p.name]
        if (v === undefined || v === '') continue
        if (p.type === 'int') params[p.name] = parseInt(v, 10)
        else if (p.type === 'float') params[p.name] = parseFloat(v)
        else if (p.type === 'bool') params[p.name] = v === 'true'
        else if (p.type === 'dict' || p.type === 'list') {
          try { params[p.name] = JSON.parse(v) } catch { params[p.name] = v }
        } else params[p.name] = v
      }

      const res = await api<{ output?: any; error?: string; durationMs: number }>(
        `/api/tools/${toolId}/run`,
        { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ params }) }
      )
      onResult({ passed: !res.error, ...res })
      onClose()
    } catch (e: any) {
      onResult({ passed: false, error: e.message, durationMs: 0 })
      onClose()
    } finally {
      setRunning(false)
    }
  }

  return (
    <>
      <div onClick={onClose} style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.25)', zIndex: 100 }} />
      <div style={{
        position: 'fixed', top: '50%', left: '50%', transform: 'translate(-50%, -50%)',
        width: 380, background: 'white', borderRadius: 12, zIndex: 101,
        boxShadow: '0 8px 30px rgba(0,0,0,0.12)', padding: '20px 24px',
      }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
          <span style={{ fontSize: 15, fontWeight: 600, color: '#111827' }}>{t('tools.testRun')}</span>
          <button onClick={onClose} style={{ border: 'none', background: 'none', cursor: 'pointer', color: '#9b9a97', padding: 2 }}>
            <X size={16} />
          </button>
        </div>

        {!tool ? (
          <div style={{ textAlign: 'center', padding: 20 }}>
            <Loader size={18} style={{ color: '#9b9a97', animation: 'spin 1s linear infinite' }} />
          </div>
        ) : (
          <>
            {tool.parameters.length === 0 ? (
              <p style={{ fontSize: 13, color: '#9b9a97', marginBottom: 16 }}>{t('tools.noParams')}</p>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 10, marginBottom: 16 }}>
                {tool.parameters.map((p) => (
                  <div key={p.name}>
                    <label style={{ fontSize: 12, color: '#374151', display: 'block', marginBottom: 4 }}>
                      {p.name} <span style={{ color: '#9b9a97' }}>({p.type})</span>
                    </label>
                    <input
                      value={values[p.name] ?? ''}
                      onChange={(e) => setValues(v => ({ ...v, [p.name]: e.target.value }))}
                      placeholder={p.default || ''}
                      style={{
                        width: '100%', padding: '7px 10px', fontSize: 13,
                        border: '1px solid #e5e7eb', borderRadius: 6, outline: 'none', boxSizing: 'border-box',
                      }}
                    />
                  </div>
                ))}
              </div>
            )}
            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
              <button onClick={onClose} style={{
                padding: '7px 14px', fontSize: 13, borderRadius: 6,
                border: '1px solid #e5e7eb', background: 'white', color: '#374151', cursor: 'pointer',
              }}>{t('tools.cancel')}</button>
              <button onClick={run} disabled={running} style={{
                padding: '7px 14px', fontSize: 13, borderRadius: 6, border: 'none',
                background: running ? '#e5e7eb' : '#111827', color: running ? '#9b9a97' : 'white',
                cursor: running ? 'default' : 'pointer', display: 'flex', alignItems: 'center', gap: 6,
              }}>
                {running && <Loader size={12} style={{ animation: 'spin 1s linear infinite' }} />}
                {running ? t('tools.running') : t('tools.runTest')}
              </button>
            </div>
          </>
        )}
      </div>
    </>
  )
}
