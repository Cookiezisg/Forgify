import { useState } from 'react'
import { Play, Save, Loader } from 'lucide-react'
import { TestParamsModal } from './TestParamsModal'
import { api } from '@/lib/api'
import { useT } from '@/lib/i18n'

interface Props {
  toolId?: string           // Set when tool already exists (bound conversation)
  code?: string             // Set when code detected but no tool yet
  funcName?: string
  displayName?: string      // From @display_name in code comments
  description?: string      // From @description
  category?: string         // From @category
  conversationId?: string
  onToolSaved?: (tool: { id: string; displayName: string }) => void
}

export function ForgeCodeBlock({
  toolId, code, funcName, displayName, description, category,
  conversationId, onToolSaved,
}: Props) {
  const t = useT()
  const [showTest, setShowTest] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)

  // If tool already exists (bound conversation), show test button only
  if (toolId) {
    return (
      <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
        <button onClick={() => setShowTest(true)} style={{
          display: 'flex', alignItems: 'center', gap: 5,
          padding: '5px 12px', fontSize: 12, fontWeight: 500, borderRadius: 6,
          border: '1px solid #e5e7eb', background: 'white', color: '#374151', cursor: 'pointer',
        }}>
          <Play size={12} strokeWidth={2.5} />
          {t('tools.testRun')}
        </button>
        {showTest && (
          <TestParamsModal toolId={toolId} onClose={() => setShowTest(false)} onResult={() => setShowTest(false)} />
        )}
      </div>
    )
  }

  // New code — show "Save as Tool?" button
  if (!code) return null

  const handleSave = async () => {
    setSaving(true)
    try {
      const tool = await api<{ id: string; displayName: string }>('/api/tools', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: funcName,
          displayName: displayName || funcName,
          description: description || '',
          code,
          category: category || 'other',
        }),
      })

      // Bind conversation to tool
      if (conversationId) {
        await api(`/api/conversations/${conversationId}/bind`, {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ assetId: tool.id, assetType: 'tool' }),
        }).catch(() => {})
      }

      setSaved(true)
      window.dispatchEvent(new CustomEvent('tool:changed'))
      window.dispatchEvent(new CustomEvent('conversation:changed'))
      onToolSaved?.(tool)
    } catch (e: any) {
      alert(e.message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div style={{ display: 'flex', gap: 8, marginTop: 8, alignItems: 'center' }}>
      <button
        onClick={handleSave}
        disabled={saving || saved}
        style={{
          display: 'flex', alignItems: 'center', gap: 5,
          padding: '5px 12px', fontSize: 12, fontWeight: 500, borderRadius: 6,
          border: 'none',
          background: saved ? '#ecfdf5' : saving ? '#e5e7eb' : '#111827',
          color: saved ? '#16a34a' : saving ? '#9b9a97' : 'white',
          cursor: saved || saving ? 'default' : 'pointer',
        }}
      >
        {saving ? (
          <><Loader size={12} style={{ animation: 'spin 1s linear infinite' }} /> 保存中...</>
        ) : saved ? (
          <>✓ 已保存</>
        ) : (
          <><Save size={12} strokeWidth={2} /> 保存为工具</>
        )}
      </button>
      {displayName && !saved && (
        <span style={{ fontSize: 11, color: '#9b9a97' }}>
          {displayName}
        </span>
      )}
    </div>
  )
}
