import { useT } from '@/lib/i18n'

interface Tool {
  id: string
  name: string
  displayName: string
  description: string
  category: string
  status: string
  builtin: boolean
  lastTestAt?: string
  lastTestPassed?: boolean
}

interface Props {
  tool: Tool
  active: boolean
  onClick: () => void
}

export function ToolCard({ tool, active, onClick }: Props) {
  const t = useT()

  const statusStyle = {
    draft:  { bg: '#f9fafb', color: '#6b7280' },
    tested: { bg: '#ecfdf5', color: '#16a34a' },
    failed: { bg: '#fef2f2', color: '#dc2626' },
  }[tool.status] || { bg: '#f9fafb', color: '#6b7280' }

  const statusLabel = {
    draft: t('tools.draft'),
    tested: t('tools.tested'),
    failed: t('tools.failed'),
  }[tool.status] || tool.status

  const categoryLabel = {
    email: t('tools.email'),
    data: t('tools.data'),
    web: t('tools.web'),
    file: t('tools.file'),
    system: t('tools.system'),
    other: t('tools.other'),
  }[tool.category] || tool.category

  return (
    <div
      onClick={onClick}
      style={{
        display: 'flex',
        alignItems: 'center',
        gap: 8,
        padding: '8px 10px',
        borderRadius: 6,
        cursor: 'pointer',
        background: active ? '#f3f4f6' : 'transparent',
        color: active ? '#111827' : '#374151',
        fontSize: 13,
        transition: 'background 100ms',
      }}
      onMouseEnter={(e) => {
        if (!active) e.currentTarget.style.background = '#f9fafb'
      }}
      onMouseLeave={(e) => {
        if (!active) e.currentTarget.style.background = 'transparent'
      }}
    >
      <span style={{ fontSize: 14, flexShrink: 0 }}>📦</span>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <span style={{
            overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap',
            fontWeight: 500, fontSize: 13,
          }}>
            {tool.displayName}
          </span>
          {tool.builtin && (
            <span style={{
              fontSize: 10, padding: '1px 5px', borderRadius: 3,
              background: '#f3f4f6', color: '#9b9a97', flexShrink: 0,
            }}>
              {t('tools.builtin')}
            </span>
          )}
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginTop: 2 }}>
          <span style={{ fontSize: 11, color: '#9b9a97' }}>{categoryLabel}</span>
          <span style={{
            fontSize: 10, padding: '1px 5px', borderRadius: 10,
            background: statusStyle.bg, color: statusStyle.color,
          }}>
            {statusLabel}
          </span>
        </div>
      </div>
    </div>
  )
}
