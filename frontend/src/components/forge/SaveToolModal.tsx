import { useState, useEffect } from 'react'
import { X } from 'lucide-react'
import { api } from '@/lib/api'
import { useT } from '@/lib/i18n'

interface Tool {
  id: string
  name: string
  displayName: string
  description: string
  category: string
  code: string
}

interface Props {
  toolId: string
  onClose: () => void
  onSaved: (tool: Tool) => void
}

const CATEGORIES = ['email', 'data', 'web', 'file', 'system', 'other']

export function SaveToolModal({ toolId, onClose, onSaved }: Props) {
  const t = useT()
  const [tool, setTool] = useState<Tool | null>(null)
  const [displayName, setDisplayName] = useState('')
  const [description, setDescription] = useState('')
  const [category, setCategory] = useState('other')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    api<Tool>(`/api/tools/${toolId}`).then((t) => {
      setTool(t)
      setDisplayName(t.displayName)
      setDescription(t.description)
      setCategory(t.category)
    }).catch(() => {})
  }, [toolId])

  const save = async () => {
    if (!tool || !displayName.trim()) return
    setSaving(true)
    try {
      const updated = await api<Tool>(`/api/tools/${toolId}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          displayName: displayName.trim(),
          description: description.trim(),
          code: tool.code || '',
          category,
        }),
      })
      onSaved(updated)
      onClose()
    } catch (e: any) {
      alert(e.message)
    } finally {
      setSaving(false)
    }
  }

  const categoryLabel = (c: string) => {
    const map: Record<string, string> = {
      email: t('tools.email'), data: t('tools.data'), web: t('tools.web'),
      file: t('tools.file'), system: t('tools.system'), other: t('tools.other'),
    }
    return map[c] || c
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
          <span style={{ fontSize: 15, fontWeight: 600, color: '#111827' }}>{t('tools.saveAsTool')}</span>
          <button onClick={onClose} style={{ border: 'none', background: 'none', cursor: 'pointer', color: '#9b9a97', padding: 2 }}>
            <X size={16} />
          </button>
        </div>

        <div style={{ display: 'flex', flexDirection: 'column', gap: 12, marginBottom: 20 }}>
          <div>
            <label style={{ fontSize: 12, fontWeight: 500, color: '#374151', display: 'block', marginBottom: 4 }}>
              {t('tools.toolName')}
            </label>
            <input value={displayName} onChange={(e) => setDisplayName(e.target.value)}
              style={{
                width: '100%', padding: '7px 10px', fontSize: 13,
                border: '1px solid #e5e7eb', borderRadius: 6, outline: 'none', boxSizing: 'border-box',
              }} />
          </div>
          <div>
            <label style={{ fontSize: 12, fontWeight: 500, color: '#374151', display: 'block', marginBottom: 4 }}>
              {t('tools.toolDesc')}
            </label>
            <input value={description} onChange={(e) => setDescription(e.target.value)}
              style={{
                width: '100%', padding: '7px 10px', fontSize: 13,
                border: '1px solid #e5e7eb', borderRadius: 6, outline: 'none', boxSizing: 'border-box',
              }} />
          </div>
          <div>
            <label style={{ fontSize: 12, fontWeight: 500, color: '#374151', display: 'block', marginBottom: 4 }}>
              {t('tools.toolCategory')}
            </label>
            <select value={category} onChange={(e) => setCategory(e.target.value)}
              style={{
                width: '100%', padding: '7px 10px', fontSize: 13,
                border: '1px solid #e5e7eb', borderRadius: 6, outline: 'none', boxSizing: 'border-box',
                background: 'white', cursor: 'pointer',
              }}>
              {CATEGORIES.map(c => (
                <option key={c} value={c}>{categoryLabel(c)}</option>
              ))}
            </select>
          </div>
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
          <button onClick={onClose} style={{
            padding: '7px 14px', fontSize: 13, borderRadius: 6,
            border: '1px solid #e5e7eb', background: 'white', color: '#374151', cursor: 'pointer',
          }}>{t('tools.cancel')}</button>
          <button onClick={save} disabled={saving || !displayName.trim()} style={{
            padding: '7px 14px', fontSize: 13, borderRadius: 6, border: 'none',
            background: saving || !displayName.trim() ? '#e5e7eb' : '#111827',
            color: saving || !displayName.trim() ? '#9b9a97' : 'white',
            cursor: saving || !displayName.trim() ? 'default' : 'pointer',
          }}>
            {saving ? t('apikey.saving') : t('tools.save')}
          </button>
        </div>
      </div>
    </>
  )
}
