import { useState, useEffect, useCallback } from 'react'
import { Loader } from 'lucide-react'
import Editor from '@monaco-editor/react'
import { api } from '@/lib/api'
import { useT } from '@/lib/i18n'

interface Tool {
  id: string
  name: string
  displayName: string
  description: string
  code: string
  category: string
  status: string
  builtin: boolean
  parameters: { name: string; type: string; required: boolean; default?: string; doc?: string }[]
}

interface TestRecord {
  id: string
  passed: boolean
  durationMs: number
  outputJson: string
  errorMsg: string
  createdAt: string
}

type Tab = 'code' | 'params' | 'test'

export function ToolMainView({ toolId, onDeleted }: { toolId: string; onDeleted: () => void }) {
  const t = useT()
  const [tool, setTool] = useState<Tool | null>(null)
  const [tab, setTab] = useState<Tab>('code')

  const load = useCallback(() => {
    api<Tool>(`/api/tools/${toolId}`).then(setTool).catch(() => {})
  }, [toolId])

  useEffect(() => { load() }, [load])

  if (!tool) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader size={20} strokeWidth={1.8} style={{ color: '#9b9a97', animation: 'spin 1s linear infinite' }} />
      </div>
    )
  }

  const handleDelete = async () => {
    if (!window.confirm(t('tools.confirmDelete'))) return
    await api(`/api/tools/${toolId}`, { method: 'DELETE' })
    onDeleted()
  }

  const handleExport = async () => {
    const data = await api<any>(`/api/tools/${toolId}/export`)
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${tool.name}.forgify-tool`
    a.click()
    URL.revokeObjectURL(url)
  }

  const tabs: { id: Tab; label: string }[] = [
    { id: 'code', label: t('tools.codeTab') },
    { id: 'params', label: t('tools.paramsTab') },
    { id: 'test', label: t('tools.testTab') },
  ]

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div style={{
        padding: '14px 20px', borderBottom: '1px solid #e5e7eb',
        display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', flexShrink: 0,
      }}>
        <div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <span style={{ fontSize: 15, fontWeight: 600, color: '#111827' }}>📦 {tool.displayName}</span>
            {tool.builtin && (
              <span style={{ fontSize: 10, padding: '2px 6px', borderRadius: 4, background: '#f3f4f6', color: '#9b9a97' }}>
                {t('tools.builtin')}
              </span>
            )}
          </div>
          <p style={{ fontSize: 12, color: '#9b9a97', marginTop: 3 }}>
            {tool.description} · {tool.category}
          </p>
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          {!tool.builtin && (
            <>
              <button onClick={handleExport} style={{
                padding: '5px 10px', fontSize: 12, borderRadius: 5,
                border: '1px solid #e5e7eb', background: 'white', color: '#374151', cursor: 'pointer',
              }}>{t('tools.export')}</button>
              <button onClick={handleDelete} style={{
                padding: '5px 10px', fontSize: 12, borderRadius: 5,
                border: '1px solid #fca5a5', background: 'white', color: '#dc2626', cursor: 'pointer',
              }}>{t('tools.delete')}</button>
            </>
          )}
        </div>
      </div>

      {/* Tabs */}
      <div style={{
        display: 'flex', gap: 0, paddingLeft: 20,
        borderBottom: '1px solid #e5e7eb', flexShrink: 0,
      }}>
        {tabs.map(({ id, label }) => (
          <button key={id} onClick={() => setTab(id)} style={{
            padding: '8px 16px', fontSize: 13, fontWeight: tab === id ? 500 : 400,
            color: tab === id ? '#111827' : '#9b9a97',
            borderBottom: tab === id ? '2px solid #111827' : '2px solid transparent',
            background: 'none', border: 'none', borderBottomWidth: 2, borderBottomStyle: 'solid',
            cursor: 'pointer', transition: 'color 100ms',
          }}>{label}</button>
        ))}
      </div>

      {/* Tab content */}
      <div style={{ flex: 1, overflow: 'hidden' }}>
        {tab === 'code' && <CodeTab tool={tool} onSave={load} />}
        {tab === 'params' && <ParamsTab params={tool.parameters} />}
        {tab === 'test' && <TestTab tool={tool} />}
      </div>
    </div>
  )
}

// ─── Code Tab ───

function CodeTab({ tool, onSave }: { tool: Tool; onSave: () => void }) {
  const t = useT()
  const [editing, setEditing] = useState(false)
  const [code, setCode] = useState(tool.code)
  const [error, setError] = useState('')

  useEffect(() => { setCode(tool.code); setEditing(false) }, [tool.id, tool.code])

  const save = async () => {
    try {
      await api(`/api/tools/${tool.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          displayName: tool.displayName,
          description: tool.description,
          code,
          category: tool.category,
        }),
      })
      setEditing(false)
      setError('')
      onSave()
    } catch (e: any) {
      setError(e.message)
    }
  }

  return (
    <div className="flex flex-col h-full" style={{ padding: '12px 0' }}>
      {!tool.builtin && (
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 6, padding: '0 16px 8px' }}>
          {editing ? (
            <>
              <button onClick={() => { setEditing(false); setCode(tool.code); setError('') }}
                style={{ padding: '5px 12px', fontSize: 12, borderRadius: 5, border: '1px solid #e5e7eb', background: 'white', color: '#374151', cursor: 'pointer' }}>
                {t('tools.cancel')}
              </button>
              <button onClick={save}
                style={{ padding: '5px 12px', fontSize: 12, borderRadius: 5, border: 'none', background: '#111827', color: 'white', cursor: 'pointer' }}>
                {t('tools.save')}
              </button>
            </>
          ) : (
            <button onClick={() => setEditing(true)}
              style={{ padding: '5px 12px', fontSize: 12, borderRadius: 5, border: '1px solid #e5e7eb', background: 'white', color: '#374151', cursor: 'pointer' }}>
              {t('tools.edit')}
            </button>
          )}
        </div>
      )}
      {error && <p style={{ fontSize: 12, color: '#dc2626', padding: '0 16px 6px' }}>{error}</p>}
      <div style={{ flex: 1 }}>
        <Editor
          height="100%"
          language="python"
          value={code}
          onChange={(v) => setCode(v ?? '')}
          options={{ readOnly: !editing, minimap: { enabled: false }, fontSize: 13, lineNumbers: 'on', scrollBeyondLastLine: false }}
          theme="light"
        />
      </div>
    </div>
  )
}

// ─── Params Tab ───

function ParamsTab({ params }: { params: Tool['parameters'] }) {
  const t = useT()
  if (!params.length) {
    return <div style={{ padding: 20, fontSize: 13, color: '#9b9a97' }}>{t('tools.noParams')}</div>
  }
  return (
    <div style={{ padding: 20, overflowY: 'auto' }}>
      <table style={{ width: '100%', fontSize: 13, borderCollapse: 'collapse' }}>
        <thead>
          <tr style={{ textAlign: 'left', fontSize: 11, color: '#9b9a97', borderBottom: '1px solid #e5e7eb' }}>
            <th style={{ padding: '6px 12px 6px 0' }}>{t('tools.paramName')}</th>
            <th style={{ padding: '6px 12px 6px 0' }}>{t('tools.paramType')}</th>
            <th style={{ padding: '6px 12px 6px 0' }}>{t('tools.paramRequired')}</th>
            <th style={{ padding: '6px 0' }}>{t('tools.paramDoc')}</th>
          </tr>
        </thead>
        <tbody>
          {params.map((p) => (
            <tr key={p.name} style={{ borderBottom: '1px solid #f3f4f6' }}>
              <td style={{ padding: '8px 12px 8px 0', fontFamily: 'monospace', fontSize: 12, color: '#2383e2' }}>{p.name}</td>
              <td style={{ padding: '8px 12px 8px 0', color: '#6b7280', fontSize: 12 }}>{p.type}</td>
              <td style={{ padding: '8px 12px 8px 0', fontSize: 12 }}>{p.required ? t('tools.yes') : t('tools.no')}</td>
              <td style={{ padding: '8px 0', fontSize: 12, color: '#6b7280' }}>{p.doc || p.default || '—'}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ─── Test Tab ───

function TestTab({ tool }: { tool: Tool }) {
  const t = useT()
  const [values, setValues] = useState<Record<string, string>>({})
  const [running, setRunning] = useState(false)
  const [result, setResult] = useState<{ output?: any; error?: string; durationMs?: number } | null>(null)
  const [history, setHistory] = useState<TestRecord[]>([])

  useEffect(() => {
    api<TestRecord[]>(`/api/tools/${tool.id}/test-history`).then(setHistory).catch(() => {})
  }, [tool.id])

  const run = async () => {
    setRunning(true)
    setResult(null)
    try {
      // Parse values to appropriate types
      const params: Record<string, any> = {}
      for (const p of tool.parameters) {
        const v = values[p.name]
        if (v === undefined || v === '') continue
        if (p.type === 'int') params[p.name] = parseInt(v, 10)
        else if (p.type === 'float') params[p.name] = parseFloat(v)
        else if (p.type === 'bool') params[p.name] = v === 'true'
        else if (p.type === 'dict' || p.type === 'list') {
          try { params[p.name] = JSON.parse(v) } catch { params[p.name] = v }
        } else {
          params[p.name] = v
        }
      }

      const res = await api<{ output?: any; error?: string; durationMs: number }>(
        `/api/tools/${tool.id}/run`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ params }),
        }
      )
      setResult(res)

      // Refresh history
      api<TestRecord[]>(`/api/tools/${tool.id}/test-history`).then(setHistory).catch(() => {})
    } catch (e: any) {
      setResult({ error: e.message })
    } finally {
      setRunning(false)
    }
  }

  return (
    <div style={{ padding: 20, overflowY: 'auto', height: '100%' }}>
      {/* Parameter inputs */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: 10, marginBottom: 16 }}>
        {tool.parameters.map((p) => (
          <div key={p.name}>
            <label style={{ fontSize: 12, color: '#374151', display: 'block', marginBottom: 4 }}>
              {p.name} <span style={{ color: '#9b9a97' }}>({p.type})</span>
              {!p.required && p.default && <span style={{ color: '#c7c7c5' }}> = {p.default}</span>}
            </label>
            <input
              value={values[p.name] ?? ''}
              onChange={(e) => setValues((v) => ({ ...v, [p.name]: e.target.value }))}
              placeholder={p.default || ''}
              style={{
                width: '100%', padding: '7px 10px', fontSize: 13,
                border: '1px solid #e5e7eb', borderRadius: 6, outline: 'none',
                boxSizing: 'border-box',
              }}
            />
          </div>
        ))}
      </div>

      <button onClick={run} disabled={running} style={{
        width: '100%', padding: '8px 0', fontSize: 13, fontWeight: 500,
        borderRadius: 6, border: 'none', cursor: running ? 'default' : 'pointer',
        background: running ? '#e5e7eb' : '#111827', color: running ? '#9b9a97' : 'white',
        marginBottom: 16,
      }}>
        {running ? t('tools.running') : `▶ ${t('tools.runTest')}`}
      </button>

      {/* Result */}
      {result && (
        <div style={{
          padding: '10px 14px', borderRadius: 6, fontSize: 12, marginBottom: 16,
          background: result.error ? '#fef2f2' : '#ecfdf5',
          color: result.error ? '#991b1b' : '#166534',
          whiteSpace: 'pre-wrap', wordBreak: 'break-word', fontFamily: 'monospace',
        }}>
          {result.error || JSON.stringify(result.output, null, 2)}
          {result.durationMs != null && (
            <span style={{ display: 'block', marginTop: 4, color: '#6b7280' }}>
              {result.durationMs}ms
            </span>
          )}
        </div>
      )}

      {/* History */}
      {history.length > 0 && (
        <div>
          <p style={{ fontSize: 11, color: '#9b9a97', marginBottom: 8 }}>{t('tools.testHistory')}</p>
          {history.map((r) => (
            <div key={r.id} style={{
              display: 'flex', alignItems: 'center', gap: 8, padding: '6px 0',
              borderTop: '1px solid #f3f4f6', fontSize: 12,
            }}>
              <span>{r.passed ? '✅' : '❌'}</span>
              <span style={{ color: '#6b7280' }}>{r.durationMs}ms</span>
              {r.errorMsg && <span style={{ color: '#dc2626', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{r.errorMsg}</span>}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
