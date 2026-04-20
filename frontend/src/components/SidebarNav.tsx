import { Home, MessageCircle, Zap, Inbox, Settings } from 'lucide-react'
import { useT } from '@/lib/i18n'

export type NavTab = 'home' | 'chat' | 'assets' | 'inbox' | 'settings'

interface Props {
  active: NavTab
  onSelect: (tab: NavTab) => void
}

export function SidebarNav({ active, onSelect }: Props) {
  const t = useT()

  const tabs: { id: NavTab; icon: React.ReactNode; label: string }[] = [
    { id: 'home',     icon: <Home size={16} strokeWidth={1.6} />,          label: t('nav.home') },
    { id: 'chat',     icon: <MessageCircle size={16} strokeWidth={1.6} />, label: t('nav.chat') },
    { id: 'assets',   icon: <Zap size={16} strokeWidth={1.6} />,           label: t('nav.assets') },
    { id: 'inbox',    icon: <Inbox size={16} strokeWidth={1.6} />,         label: t('nav.inbox') },
    { id: 'settings', icon: <Settings size={16} strokeWidth={1.6} />,      label: t('nav.settings') },
  ]

  return (
    <div style={{
      width: '100%', height: 36, padding: '0 8px',
      borderBottom: '1px solid #f3f4f6',
      display: 'flex', alignItems: 'center', flexShrink: 0,
    }}>
      <div style={{ display: 'inline-flex', alignItems: 'center', gap: 2 }}>
        {tabs.map(({ id, icon, label }) => {
          const isActive = active === id
          return (
            <button
              key={id}
              onClick={() => onSelect(id)}
              title={isActive ? undefined : label}
              style={{
                position: 'relative', display: 'inline-flex', alignItems: 'center',
                gap: isActive ? 4 : 0,
                height: 28, width: isActive ? 'auto' : 28,
                padding: isActive ? '0 10px 0 8px' : 0,
                borderRadius: 999,
                backgroundColor: isActive ? '#ebebeb' : 'transparent',
                color: isActive ? '#1f2937' : '#9ca3af',
                border: 'none', cursor: 'pointer',
                transition: 'background-color 150ms, color 150ms',
                flexShrink: 0,
              }}
            >
              <span style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', width: 20, height: 20, flexShrink: 0 }}>
                {icon}
              </span>
              {isActive && (
                <span style={{ fontSize: 12.5, fontWeight: 500, whiteSpace: 'nowrap' }}>{label}</span>
              )}
            </button>
          )
        })}
      </div>
    </div>
  )
}
