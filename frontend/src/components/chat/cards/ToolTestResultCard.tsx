import { useT } from '@/lib/i18n'

interface Props {
  toolName: string
  passed: boolean
  durationMs: number
  output?: string
  error?: string
}

export function ToolTestResultCard({ toolName, passed, durationMs, output, error }: Props) {
  const t = useT()

  return (
    <div
      style={{
        border: `1px solid ${passed ? '#bbf7d0' : '#fecaca'}`,
        borderRadius: 10,
        padding: '12px 16px',
        maxWidth: 400,
        margin: '4px 0',
        background: passed ? '#f0fdf4' : '#fef2f2',
      }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginBottom: 6 }}>
        <span style={{ fontSize: 14 }}>{passed ? '✅' : '❌'}</span>
        <span style={{ fontSize: 13, fontWeight: 600, color: '#111827' }}>
          {t('tools.testRun')}
        </span>
      </div>
      <p style={{ fontSize: 13, color: '#374151', marginBottom: 4 }}>
        📦 {toolName} · {passed ? t('tools.tested') : t('tools.failed')}
      </p>
      <p style={{ fontSize: 12, color: '#6b7280', marginBottom: 6 }}>
        {durationMs}ms
      </p>
      {error && (
        <pre style={{
          fontSize: 11, color: '#991b1b', background: '#fef2f2',
          padding: '6px 8px', borderRadius: 4, whiteSpace: 'pre-wrap',
          wordBreak: 'break-word', maxHeight: 100, overflow: 'auto',
        }}>
          {error}
        </pre>
      )}
      {output && (
        <pre style={{
          fontSize: 11, color: '#166534', background: '#f0fdf4',
          padding: '6px 8px', borderRadius: 4, whiteSpace: 'pre-wrap',
          wordBreak: 'break-word', maxHeight: 100, overflow: 'auto',
        }}>
          {output}
        </pre>
      )}
    </div>
  )
}
