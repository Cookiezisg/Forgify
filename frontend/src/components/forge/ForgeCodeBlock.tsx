import { useState, useEffect } from 'react'
import { Play, Save, Loader } from 'lucide-react'
import { TestParamsModal } from './TestParamsModal'
import { api } from '@/lib/api'
import { onEvent } from '@/lib/events'
import { useT } from '@/lib/i18n'

interface Props {
  toolId?: string         // Set when tool already exists (bound conversation)
  code?: string           // Set when code detected but no tool yet (unbound)
  funcName?: string       // Function name from detected code
  conversationId?: string
  onToolSaved?: (tool: { id: string; displayName: string }) => void
}

export function ForgeCodeBlock({ toolId, code, funcName, conversationId, onToolSaved }: Props) {
  const t = useT()
  const [showTest, setShowTest] = useState(false)
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [aiName, setAiName] = useState<{ name: string; description: string } | null>(null)

  // Listen for AI-generated name/description from backend
  useEffect(() => {
    if (toolId || !funcName) return // Only for unsaved code
    const off = onEvent<{ conversationId: string; funcName: string; aiResponse: string }>(
      'forge.name_generated',
      (e) => {
        if (e.funcName !== funcName) return
        try {
          // Parse JSON from AI response — might have markdown wrapping
          let json = e.aiResponse
          const match = json.match(/\{[\s\S]*\}/)
          if (match) json = match[0]
          const parsed = JSON.parse(json)
          setAiName({ name: parsed.name || funcName, description: parsed.description || '' })
        } catch {
          setAiName({ name: funcName, description: '' })
        }
      }
    )
    return off
  }, [toolId, funcName])

  // If tool already exists, show test button only (for bound conversations)
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
      // Create the tool via API
      const tool = await api<{ id: string; displayName: string }>('/api/tools', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          name: funcName,
          displayName: aiName?.name || funcName,
          description: aiName?.description || '',
          code,
          category: 'other',
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
          <>
            <Loader size={12} style={{ animation: 'spin 1s linear infinite' }} />
            {aiName ? '保存中...' : '正在生成描述...'}
          </>
        ) : saved ? (
          <>✓ 已保存</>
        ) : (
          <>
            <Save size={12} strokeWidth={2} />
            保存为工具
          </>
        )}
      </button>
      {aiName && !saved && !saving && (
        <span style={{ fontSize: 11, color: '#9b9a97' }}>
          {aiName.name}
        </span>
      )}
    </div>
  )
}
