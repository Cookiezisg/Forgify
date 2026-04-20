import { ToolMainView } from '@/components/tools/ToolMainView'
import { useTabContext } from '@/context/TabContext'

interface Props {
  toolId: string
}

export function ToolLayout({ toolId }: Props) {
  const { closeTab, activeTabId } = useTabContext()

  return (
    <ToolMainView
      toolId={toolId}
      onDeleted={() => {
        if (activeTabId) closeTab(activeTabId)
      }}
    />
  )
}
