import { ChatContent } from '@/pages/ChatContent'

interface Props {
  conversationId?: string
}

export function ChatLayout({ conversationId }: Props) {
  return <ChatContent conversationId={conversationId} />
}
