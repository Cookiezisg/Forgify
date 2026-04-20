import type { TabItem } from '@/context/TabContext'
import { ChatLayout } from './ChatLayout'
import { ToolLayout } from './ToolLayout'
import { ChatToolLayout } from './ChatToolLayout'

/**
 * Routes a TabItem to the appropriate layout component.
 * Only handles tab-managed layouts (chat, tool, workflow, chat-tool, chat-workflow).
 * Home/Inbox/Settings are rendered directly by App.tsx, not through tabs.
 */
export function LayoutRouter({ tab }: { tab: TabItem }) {
  switch (tab.layout) {
    case 'chat':
      return <ChatLayout conversationId={tab.conversationId} />

    case 'tool':
      return tab.toolId ? <ToolLayout toolId={tab.toolId} /> : <PlaceholderView text="选择一个工具" />

    case 'workflow':
      return <PlaceholderView text="工作流画布（Tier 5 开发中）" />

    case 'chat-tool':
      return tab.conversationId && tab.toolId
        ? <ChatToolLayout conversationId={tab.conversationId} toolId={tab.toolId} tabId={tab.id} chatLabel={tab.label} />
        : <PlaceholderView text="加载中..." />

    case 'chat-workflow':
      return <PlaceholderView text="对话 + 工作流画布（Tier 5 开发中）" />

    default:
      return <PlaceholderView text="未知页面类型" />
  }
}

function PlaceholderView({ text }: { text: string }) {
  return (
    <div className="flex items-center justify-center h-full">
      <p style={{ fontSize: 14, color: '#9b9a97' }}>{text}</p>
    </div>
  )
}
