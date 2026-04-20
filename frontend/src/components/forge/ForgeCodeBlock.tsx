import { useState } from 'react'
import { Play, Save } from 'lucide-react'
import { TestParamsModal } from './TestParamsModal'
import { SaveToolModal } from './SaveToolModal'
import { api } from '@/lib/api'
import { useT } from '@/lib/i18n'

interface Props {
  toolId: string
  conversationId?: string
  onTestResult?: (result: { passed: boolean; output?: any; error?: string; durationMs: number }) => void
  onToolSaved?: (tool: { id: string; displayName: string }) => void
}

export function ForgeCodeBlock({ toolId, conversationId, onTestResult, onToolSaved }: Props) {
  const t = useT()
  const [showTest, setShowTest] = useState(false)
  const [showSave, setShowSave] = useState(false)
  const [testResult, setTestResult] = useState<{ passed: boolean; output?: any; error?: string; durationMs: number } | null>(null)
  const [saved, setSaved] = useState(false)

  return (
    <div>
      {/* Action buttons */}
      <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
        <button
          onClick={() => setShowTest(true)}
          style={{
            display: 'flex', alignItems: 'center', gap: 5,
            padding: '5px 12px', fontSize: 12, fontWeight: 500, borderRadius: 6,
            border: '1px solid #e5e7eb', background: 'white', color: '#374151',
            cursor: 'pointer', transition: 'background 100ms',
          }}
          onMouseEnter={(e) => (e.currentTarget.style.background = '#f9fafb')}
          onMouseLeave={(e) => (e.currentTarget.style.background = 'white')}
        >
          <Play size={12} strokeWidth={2.5} />
          {t('tools.testRun')}
        </button>

        <button
          onClick={() => setShowSave(true)}
          disabled={saved}
          style={{
            display: 'flex', alignItems: 'center', gap: 5,
            padding: '5px 12px', fontSize: 12, fontWeight: 500, borderRadius: 6,
            border: 'none',
            background: saved ? '#ecfdf5' : '#111827',
            color: saved ? '#16a34a' : 'white',
            cursor: saved ? 'default' : 'pointer',
            transition: 'background 100ms',
          }}
        >
          <Save size={12} strokeWidth={2} />
          {saved ? '✓ ' + t('model.saved') : t('tools.saveAsTool')}
        </button>
      </div>

      {/* Test result inline */}
      {testResult && (
        <div style={{
          marginTop: 8, padding: '8px 12px', borderRadius: 6, fontSize: 12,
          background: testResult.passed ? '#ecfdf5' : '#fef2f2',
          color: testResult.passed ? '#166534' : '#991b1b',
          fontFamily: 'monospace', whiteSpace: 'pre-wrap', wordBreak: 'break-word',
        }}>
          {testResult.error
            ? `❌ ${testResult.error}`
            : `✅ ${JSON.stringify(testResult.output, null, 2)}`
          }
          <span style={{ display: 'block', marginTop: 4, color: '#6b7280', fontFamily: 'inherit' }}>
            {testResult.durationMs}ms
          </span>
        </div>
      )}

      {/* Modals */}
      {showTest && (
        <TestParamsModal
          toolId={toolId}
          onClose={() => setShowTest(false)}
          onResult={(r) => {
            setTestResult(r)
            onTestResult?.(r)
          }}
        />
      )}
      {showSave && (
        <SaveToolModal
          toolId={toolId}
          onClose={() => setShowSave(false)}
          onSaved={async (tool) => {
            setSaved(true)
            // Bind conversation to tool if we have conversationId
            if (conversationId) {
              await api(`/api/conversations/${conversationId}/bind`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ assetId: tool.id, assetType: 'tool' }),
              }).catch(() => {})
            }
            window.dispatchEvent(new CustomEvent('tool:changed'))
            window.dispatchEvent(new CustomEvent('conversation:changed'))
            onToolSaved?.(tool)
          }}
        />
      )}
    </div>
  )
}
