import { useState, useEffect, useCallback, useRef } from 'react'
import { Loader } from 'lucide-react'
import Editor, { DiffEditor } from '@monaco-editor/react'
import { api } from '@/lib/api'
import { onEvent, EventNames } from '@/lib/events'
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
  version: string
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

interface ToolVersion {
  id: string
  version: number
  code: string
  changeSummary: string
  createdAt: string
}

type Tab = 'code' | 'params' | 'test'

export function ToolMainView({ toolId, onDeleted }: { toolId: string; onDeleted: () => void }) {
  const t = useT()
  const [tool, setTool] = useState<Tool | null>(null)
  const [tab, setTab] = useState<Tab>('code')
  const [historyMode, setHistoryMode] = useState(false)

  const load = useCallback(() => {
    api<Tool>(`/api/tools/${toolId}`).then(setTool).catch(() => {})
  }, [toolId])

  useEffect(() => { load() }, [load])

  // Listen for external tool changes (e.g. deleted from another component)
  useEffect(() => {
    const handler = () => load()
    window.addEventListener('tool:changed', handler)
    return () => window.removeEventListener('tool:changed', handler)
  }, [load])

  if (!tool) {
    return (
      <div className="flex items-center justify-center h-full">
        <Loader size={20} strokeWidth={1.8} style={{ color: '#9b9a97', animation: 'spin 1s linear infinite' }} />
      </div>
    )
  }

  // Show deleted state
  if (tool.status === 'deleted') {
    return (
      <div className="flex flex-col items-center justify-center h-full" style={{ gap: 12 }}>
        <p style={{ fontSize: 16, fontWeight: 500, color: '#9b9a97' }}>🗑️ 工具已删除</p>
        <p style={{ fontSize: 13, color: '#c7c7c5' }}>{tool.displayName}</p>
        <button onClick={async () => {
          await api(`/api/tools/${toolId}/restore`, { method: 'POST' })
          window.dispatchEvent(new CustomEvent('tool:changed'))
          load()
        }} style={{
          padding: '6px 16px', fontSize: 13, borderRadius: 6,
          border: '1px solid #e5e7eb', background: 'white', color: '#374151', cursor: 'pointer',
        }}>
          恢复工具
        </button>
      </div>
    )
  }

  const handleDelete = async () => {
    if (!window.confirm(t('tools.confirmDelete'))) return
    await api(`/api/tools/${toolId}`, { method: 'DELETE' })
    window.dispatchEvent(new CustomEvent('tool:changed'))
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
      {/* Header — inline editable */}
      <div style={{
        padding: '12px 20px 8px', borderBottom: '1px solid #e5e7eb', flexShrink: 0,
      }}>
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between' }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
              <span style={{ fontSize: 14 }}>📦</span>
              {/* Inline editable name */}
              <InlineEdit
                value={tool.displayName}
                readonly={tool.builtin}
                style={{ fontSize: 15, fontWeight: 600, color: '#111827' }}
                onSave={(v) => {
                  api(`/api/tools/${tool.id}/meta`, {
                    method: 'PATCH', headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ displayName: v }),
                  }).then(load)
                }}
              />
              {tool.builtin && (
                <span style={{ fontSize: 10, padding: '2px 6px', borderRadius: 4, background: '#f3f4f6', color: '#9b9a97' }}>
                  {t('tools.builtin')}
                </span>
              )}
            </div>
            {/* Inline editable description */}
            <InlineEdit
              value={tool.description || '添加描述...'}
              readonly={tool.builtin}
              style={{ fontSize: 12, color: '#9b9a97', marginTop: 2 }}
              onSave={(v) => {
                api(`/api/tools/${tool.id}/meta`, {
                  method: 'PATCH', headers: { 'Content-Type': 'application/json' },
                  body: JSON.stringify({ description: v }),
                }).then(load)
              }}
            />
            {/* Category + version */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginTop: 4 }}>
              <InlineSelect
                value={tool.category}
                readonly={tool.builtin}
                options={[
                  { value: 'email', label: t('tools.email') },
                  { value: 'data', label: t('tools.data') },
                  { value: 'web', label: t('tools.web') },
                  { value: 'file', label: t('tools.file') },
                  { value: 'system', label: t('tools.system') },
                  { value: 'other', label: t('tools.other') },
                ]}
                style={{ fontSize: 11, color: '#6b7280' }}
                onSave={(v) => {
                  api(`/api/tools/${tool.id}/meta`, {
                    method: 'PATCH', headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ category: v }),
                  }).then(load)
                }}
              />
              <button
                onClick={() => setHistoryMode(true)}
                title="版本历史"
                style={{
                  fontSize: 10, padding: '1px 6px', borderRadius: 4,
                  background: '#f3f4f6', color: '#9b9a97',
                  border: 'none', cursor: 'pointer', transition: 'background 100ms',
                }}
                onMouseEnter={(e) => { e.currentTarget.style.background = '#e5e7eb' }}
                onMouseLeave={(e) => { e.currentTarget.style.background = '#f3f4f6' }}
              >
                v{tool.version || '1.0'} ↗
              </button>
            </div>
            {/* Tag bar */}
            {!tool.builtin && <TagBar toolId={tool.id} />}
          </div>
          <div style={{ display: 'flex', gap: 6, flexShrink: 0 }}>
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
      </div>

      {historyMode ? (
        <VersionHistoryView
          tool={tool}
          onBack={() => setHistoryMode(false)}
          onRestore={() => { setHistoryMode(false); load() }}
        />
      ) : (
        <>
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
        </>
      )}
    </div>
  )
}

// ─── Code Tab (with diff review mode) ───

interface PendingChange {
  hasPending: boolean
  currentCode?: string
  pendingCode?: string
  summary?: string
}

function CodeTab({ tool, onSave }: { tool: Tool; onSave: () => void }) {
  const t = useT()
  const [editing, setEditing] = useState(false)
  const [code, setCode] = useState(tool.code)
  const [error, setError] = useState('')
  const [pending, setPending] = useState<PendingChange>({ hasPending: false })
  const [accepting, setAccepting] = useState(false)
  const [generating, setGenerating] = useState(false)
  const [genStatus, setGenStatus] = useState('AI 正在思考...')

  useEffect(() => { setCode(tool.code); setEditing(false) }, [tool.id, tool.code])

  // Listen for "AI is generating code" indicator — only for this tool
  useEffect(() => {
    const offGen = onEvent<{ conversationId: string; event: string; status?: string }>(
      EventNames.ForgeCodeStreaming,
      (e) => {
        if (e.event === 'generating') {
          setGenerating(true)
          if (e.status) setGenStatus(e.status)
        }
      }
    )
    // Reset generating state when chat is done (no tool call happened = normal response)
    const offDone = onEvent<{ conversationId: string }>(
      EventNames.ChatDone,
      () => { setGenerating(false) }
    )
    // Also reset when pending change arrives (tool call DID happen and completed)
    const offUpdated = onEvent<{ toolId: string }>(
      EventNames.ForgeCodeUpdated,
      (e) => { if (e.toolId === tool.id) setGenerating(false) }
    )
    return () => { offGen(); offDone(); offUpdated() }
  }, [tool.id])

  // Poll for pending changes
  useEffect(() => {
    const check = () => {
      api<PendingChange>(`/api/tools/${tool.id}/pending`).then((p) => {
        setPending(p)
        if (p.hasPending) setGenerating(false)
      }).catch(() => {})
    }
    check()
    // Also listen for forge.code_updated event
    const off = onEvent<{ toolId: string }>(EventNames.ForgeCodeUpdated, (e) => {
      if (e.toolId === tool.id) check()
    })
    return off
  }, [tool.id])

  const save = async () => {
    try {
      await api(`/api/tools/${tool.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          displayName: tool.displayName, description: tool.description,
          code, category: tool.category,
        }),
      })
      setEditing(false)
      setError('')
      onSave()
    } catch (e: any) { setError(e.message) }
  }

  const acceptChange = async () => {
    setAccepting(true)
    try {
      await api(`/api/tools/${tool.id}/accept`, { method: 'POST' })
      setPending({ hasPending: false })
      onSave() // reload tool data
    } catch (e: any) { setError(e.message) }
    finally { setAccepting(false) }
  }

  const rejectChange = async () => {
    try {
      await api(`/api/tools/${tool.id}/reject`, { method: 'POST' })
      setPending({ hasPending: false })
    } catch {}
  }

  // Note: generating state is shown as a small banner, NOT blocking the entire view

  // ─── Diff review mode ───
  if (pending.hasPending && pending.currentCode != null && pending.pendingCode != null) {
    return (
      <div className="flex flex-col h-full">
        {/* Diff header */}
        <div style={{
          display: 'flex', alignItems: 'center', justifyContent: 'space-between',
          padding: '8px 16px', borderBottom: '1px solid #e5e7eb', flexShrink: 0,
          background: '#fffbeb',
        }}>
          <div>
            <span style={{ fontSize: 13, fontWeight: 500, color: '#92400e' }}>
              AI 提议修改
            </span>
            {pending.summary && (
              <span style={{ fontSize: 12, color: '#b45309', marginLeft: 8 }}>
                ({pending.summary})
              </span>
            )}
          </div>
          <div style={{ display: 'flex', gap: 6 }}>
            <button onClick={rejectChange} style={{
              padding: '5px 12px', fontSize: 12, borderRadius: 5,
              border: '1px solid #e5e7eb', background: 'white', color: '#374151', cursor: 'pointer',
            }}>
              拒绝
            </button>
            <button onClick={acceptChange} disabled={accepting} style={{
              padding: '5px 12px', fontSize: 12, borderRadius: 5,
              border: 'none', background: '#16a34a', color: 'white', cursor: 'pointer',
              opacity: accepting ? 0.7 : 1,
            }}>
              {accepting ? '应用中...' : '✓ 接受修改'}
            </button>
          </div>
        </div>
        {/* Inline diff view */}
        <div style={{ flex: 1, overflow: 'auto', padding: '0' }}>
          <InlineDiff oldCode={pending.currentCode} newCode={pending.pendingCode} />
        </div>
      </div>
    )
  }

  // ─── Normal editor mode ───
  return (
    <div className="flex flex-col h-full" style={{ padding: '12px 0' }}>
      {/* Generating banner — small, non-blocking */}
      {generating && (
        <div style={{
          display: 'flex', alignItems: 'center', gap: 8,
          padding: '6px 16px', background: '#eff6ff', borderBottom: '1px solid #dbeafe',
          flexShrink: 0,
        }}>
          <div style={{ width: 12, height: 12, border: '2px solid #93c5fd', borderTopColor: '#3b82f6', borderRadius: '50%', animation: 'spin 0.8s linear infinite' }} />
          <span style={{ fontSize: 12, color: '#1d4ed8' }}>{genStatus}</span>
        </div>
      )}
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

// ─── Version History View ───

function VersionHistoryView({ tool, onBack, onRestore }: {
  tool: Tool; onBack: () => void; onRestore: () => void
}) {
  const [versions, setVersions] = useState<ToolVersion[]>([])
  const [selected, setSelected] = useState<ToolVersion | null>(null)
  const [restoring, setRestoring] = useState(false)

  useEffect(() => {
    api<ToolVersion[]>(`/api/tools/${tool.id}/versions`).then(vs => {
      setVersions(vs)
      if (vs.length > 0) setSelected(vs[0])
    }).catch(() => {})
  }, [tool.id])

  const handleRestore = async () => {
    if (!selected || !window.confirm(`确定恢复到版本 ${selected.version}？`)) return
    setRestoring(true)
    try {
      await api(`/api/tools/${tool.id}/versions/${selected.version}/restore`, { method: 'POST' })
      onRestore()
    } catch {} finally { setRestoring(false) }
  }

  return (
    <div className="flex flex-col" style={{ flex: 1, overflow: 'hidden' }}>
      {/* Back header */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 8,
        padding: '8px 16px', borderBottom: '1px solid #e5e7eb', flexShrink: 0,
      }}>
        <button
          onClick={onBack}
          style={{
            border: 'none', background: 'none', cursor: 'pointer',
            fontSize: 13, color: '#2383e2', padding: 0,
          }}
        >
          ← 返回编辑
        </button>
        <span style={{ fontSize: 13, color: '#9b9a97' }}>版本历史</span>
      </div>

      {versions.length === 0 ? (
        <div className="flex items-center justify-center" style={{ flex: 1 }}>
          <p style={{ fontSize: 13, color: '#9b9a97' }}>暂无历史版本</p>
        </div>
      ) : (
        <div className="flex" style={{ flex: 1, overflow: 'hidden' }}>
          {/* Version list */}
          <div style={{
            width: 180, flexShrink: 0, borderRight: '1px solid #e5e7eb',
            display: 'flex', flexDirection: 'column', overflow: 'hidden',
          }}>
            <div style={{ flex: 1, overflowY: 'auto' }}>
              {versions.map(v => (
                <div
                  key={v.id}
                  onClick={() => setSelected(v)}
                  style={{
                    padding: '10px 12px', cursor: 'pointer',
                    background: selected?.id === v.id ? '#f3f4f6' : 'transparent',
                    borderBottom: '1px solid #f3f4f6',
                    transition: 'background 100ms',
                  }}
                  onMouseEnter={(e) => { if (selected?.id !== v.id) e.currentTarget.style.background = '#fafaf9' }}
                  onMouseLeave={(e) => { if (selected?.id !== v.id) e.currentTarget.style.background = 'transparent' }}
                >
                  <div style={{ fontSize: 12, fontWeight: 500, color: '#374151' }}>
                    v{v.version}
                    <span style={{ fontWeight: 400, color: '#9b9a97', marginLeft: 6 }}>
                      {timeAgo(v.createdAt)}
                    </span>
                  </div>
                  {v.changeSummary && (
                    <div style={{
                      fontSize: 11, color: '#6b7280', marginTop: 2,
                      overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
                    }}>
                      {v.changeSummary}
                    </div>
                  )}
                </div>
              ))}
            </div>
            {/* Restore button */}
            {selected && (
              <div style={{ padding: 10, borderTop: '1px solid #e5e7eb', flexShrink: 0 }}>
                <button
                  onClick={handleRestore}
                  disabled={restoring}
                  style={{
                    width: '100%', padding: '6px 0', fontSize: 12,
                    borderRadius: 5, border: '1px solid #e5e7eb',
                    background: 'white', color: '#374151', cursor: 'pointer',
                  }}
                >
                  {restoring ? '恢复中...' : `恢复 v${selected.version}`}
                </button>
              </div>
            )}
          </div>

          {/* Diff viewer */}
          <div style={{ flex: 1, overflow: 'hidden' }}>
            {selected && (
              <DiffEditor
                height="100%"
                original={selected.code}
                modified={tool.code}
                language="python"
                theme="light"
                options={{
                  readOnly: true,
                  renderSideBySide: true,
                  minimap: { enabled: false },
                  scrollBeyondLastLine: false,
                  fontSize: 13,
                }}
              />
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function timeAgo(dateStr: string): string {
  const diff = Date.now() - new Date(dateStr).getTime()
  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`
  if (diff < 86400000) return `${Math.floor(diff / 3600000)} 小时前`
  return `${Math.floor(diff / 86400000)} 天前`
}

// ─── Inline Diff Component (red/green line-level) ───

function InlineDiff({ oldCode, newCode }: { oldCode: string; newCode: string }) {
  const oldLines = oldCode.split('\n')
  const newLines = newCode.split('\n')

  // Simple LCS-based diff
  const diff = computeDiff(oldLines, newLines)

  return (
    <div style={{ fontFamily: '"JetBrains Mono", "Fira Code", monospace', fontSize: 13, lineHeight: '1.6' }}>
      {diff.map((line, i) => (
        <div key={i} style={{
          padding: '0 16px',
          background: line.type === 'add' ? '#dcfce7' : line.type === 'remove' ? '#fee2e2' : 'transparent',
          color: line.type === 'add' ? '#166534' : line.type === 'remove' ? '#991b1b' : '#374151',
          borderLeft: line.type === 'add' ? '3px solid #16a34a' : line.type === 'remove' ? '3px solid #dc2626' : '3px solid transparent',
          whiteSpace: 'pre',
          minHeight: '1.6em',
        }}>
          <span style={{ display: 'inline-block', width: 20, color: '#9b9a97', userSelect: 'none', textAlign: 'right', marginRight: 12 }}>
            {line.type === 'add' ? '+' : line.type === 'remove' ? '-' : ' '}
          </span>
          {line.content}
        </div>
      ))}
    </div>
  )
}

type DiffLine = { type: 'same' | 'add' | 'remove'; content: string }

function computeDiff(oldLines: string[], newLines: string[]): DiffLine[] {
  // Simple diff: find matching lines, mark others as add/remove
  const result: DiffLine[] = []
  let oi = 0, ni = 0

  while (oi < oldLines.length || ni < newLines.length) {
    if (oi < oldLines.length && ni < newLines.length && oldLines[oi] === newLines[ni]) {
      result.push({ type: 'same', content: oldLines[oi] })
      oi++; ni++
    } else {
      // Look ahead to find next match
      let foundOld = -1, foundNew = -1
      for (let k = 1; k <= 5; k++) {
        if (ni + k < newLines.length && oi < oldLines.length && oldLines[oi] === newLines[ni + k]) { foundNew = ni + k; break }
        if (oi + k < oldLines.length && ni < newLines.length && newLines[ni] === oldLines[oi + k]) { foundOld = oi + k; break }
      }

      if (foundOld >= 0) {
        // Old lines were removed
        while (oi < foundOld) { result.push({ type: 'remove', content: oldLines[oi++] }) }
      } else if (foundNew >= 0) {
        // New lines were added
        while (ni < foundNew) { result.push({ type: 'add', content: newLines[ni++] }) }
      } else {
        // No match nearby — treat as remove old + add new
        if (oi < oldLines.length) { result.push({ type: 'remove', content: oldLines[oi++] }) }
        if (ni < newLines.length) { result.push({ type: 'add', content: newLines[ni++] }) }
      }
    }
  }
  return result
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

// ─── InlineEdit (Notion-style click-to-edit) ───

function InlineEdit({
  value, readonly, style, onSave,
}: {
  value: string; readonly?: boolean; style?: React.CSSProperties; onSave: (v: string) => void
}) {
  const [editing, setEditing] = useState(false)
  const [text, setText] = useState(value)

  useEffect(() => { setText(value) }, [value])

  if (readonly || !editing) {
    return (
      <span
        onClick={() => !readonly && setEditing(true)}
        style={{
          ...style,
          cursor: readonly ? 'default' : 'pointer',
          borderBottom: readonly ? 'none' : '1px dashed transparent',
          transition: 'border-color 100ms',
        }}
        onMouseEnter={(e) => { if (!readonly) (e.currentTarget.style.borderBottomColor = '#d1d5db') }}
        onMouseLeave={(e) => { e.currentTarget.style.borderBottomColor = 'transparent' }}
      >
        {value}
      </span>
    )
  }

  return (
    <input
      autoFocus
      value={text}
      onChange={(e) => setText(e.target.value)}
      onBlur={() => {
        setEditing(false)
        if (text.trim() && text !== value) onSave(text.trim())
        else setText(value)
      }}
      onKeyDown={(e) => {
        if (e.key === 'Enter') { e.currentTarget.blur() }
        if (e.key === 'Escape') { setText(value); setEditing(false) }
      }}
      style={{
        ...style,
        border: 'none', borderBottom: '1px solid #2383e2',
        outline: 'none', background: 'transparent',
        padding: 0, margin: 0, fontFamily: 'inherit',
        width: '100%',
      }}
    />
  )
}

// ─── InlineSelect (dropdown version of InlineEdit) ───

function InlineSelect({
  value, options, readonly, style, onSave,
}: {
  value: string
  options: { value: string; label: string }[]
  readonly?: boolean
  style?: React.CSSProperties
  onSave: (v: string) => void
}) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  // Close on outside click or Escape
  useEffect(() => {
    if (!open) return
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false) }
    const onClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', onClick)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onClick)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const label = options.find(o => o.value === value)?.label || value

  return (
    <div ref={ref} style={{ position: 'relative', display: 'inline-block' }}>
      <span
        onClick={() => !readonly && setOpen(!open)}
        style={{
          ...style,
          cursor: readonly ? 'default' : 'pointer',
          borderBottom: readonly ? 'none' : '1px dashed transparent',
          transition: 'border-color 100ms',
        }}
        onMouseEnter={(e) => { if (!readonly) (e.currentTarget.style.borderBottomColor = '#d1d5db') }}
        onMouseLeave={(e) => { e.currentTarget.style.borderBottomColor = 'transparent' }}
      >
        {label}
      </span>
      {open && (
        <div style={{
          position: 'absolute', top: '100%', left: 0, marginTop: 4, zIndex: 20,
          background: 'white', border: '1px solid #e5e7eb', borderRadius: 6,
          boxShadow: '0 4px 12px rgba(0,0,0,0.08)', padding: '4px 0', minWidth: 120,
        }}>
          {options.map(opt => (
            <button
              key={opt.value}
              onClick={() => {
                if (opt.value !== value) onSave(opt.value)
                setOpen(false)
              }}
              style={{
                display: 'block', width: '100%', padding: '5px 12px',
                border: 'none', background: opt.value === value ? '#f3f4f6' : 'none',
                textAlign: 'left', cursor: 'pointer',
                color: '#374151', fontSize: 12,
              }}
              onMouseEnter={(e) => { e.currentTarget.style.background = '#f3f4f6' }}
              onMouseLeave={(e) => { e.currentTarget.style.background = opt.value === value ? '#f3f4f6' : 'none' }}
            >
              {opt.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

// ─── TagBar ───

function TagBar({ toolId }: { toolId: string }) {
  const [tags, setTags] = useState<string[]>([])
  const [adding, setAdding] = useState(false)
  const [newTag, setNewTag] = useState('')

  useEffect(() => {
    api<string[]>(`/api/tools/${toolId}/tags`).then(setTags).catch(() => {})
  }, [toolId])

  const addTag = async () => {
    const tag = newTag.trim()
    if (!tag) return
    await api(`/api/tools/${toolId}/tags`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ tag }),
    }).catch(() => {})
    setTags(prev => [...prev, tag])
    setNewTag('')
    setAdding(false)
  }

  const removeTag = async (tag: string) => {
    await api(`/api/tools/${toolId}/tags/${encodeURIComponent(tag)}`, { method: 'DELETE' }).catch(() => {})
    setTags(prev => prev.filter(t => t !== tag))
  }

  return (
    <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap', marginTop: 6, alignItems: 'center' }}>
      {tags.map(tag => (
        <span key={tag} style={{
          display: 'inline-flex', alignItems: 'center', gap: 3,
          padding: '1px 8px', borderRadius: 10, fontSize: 11,
          background: '#f3f4f6', color: '#374151',
        }}>
          {tag}
          <button onClick={() => removeTag(tag)} style={{
            border: 'none', background: 'none', cursor: 'pointer',
            color: '#9b9a97', padding: 0, fontSize: 10, lineHeight: 1,
          }}>×</button>
        </span>
      ))}
      {adding ? (
        <input
          autoFocus
          value={newTag}
          onChange={(e) => setNewTag(e.target.value)}
          onBlur={() => { if (newTag.trim()) addTag(); else setAdding(false) }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') addTag()
            if (e.key === 'Escape') { setNewTag(''); setAdding(false) }
          }}
          placeholder="标签名"
          style={{
            width: 60, padding: '1px 6px', borderRadius: 10, fontSize: 11,
            border: '1px solid #d1d5db', outline: 'none',
          }}
        />
      ) : (
        <button onClick={() => setAdding(true)} style={{
          padding: '1px 8px', borderRadius: 10, fontSize: 11,
          border: '1px dashed #d1d5db', background: 'transparent',
          color: '#9b9a97', cursor: 'pointer',
        }}>
          + 标签
        </button>
      )}
    </div>
  )
}
