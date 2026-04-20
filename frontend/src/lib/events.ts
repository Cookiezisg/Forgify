import { setBackendPort } from './api'

let source: EventSource | null = null
let backendPort = 0

export function initBackend(port: number): void {
  backendPort = port
  setBackendPort(port)
  connect()
}

function connect(): void {
  if (source) source.close()
  source = new EventSource(`http://127.0.0.1:${backendPort}/events`)
  source.addEventListener('error', () => {
    if (source?.readyState === EventSource.CLOSED) {
      setTimeout(() => { if (backendPort > 0) connect() }, 3000)
    }
  })
}

export const EV = {
  ChatToken:         'chat.token',
  ChatDone:          'chat.done',
  ChatError:         'chat.error',
  ChatCompacted:     'chat.compacted',
  ChatBound:         'chat.bound',
  ChatTitleUpdated:  'chat.title_updated',
  ForgeCodeDetected: 'forge.code_detected',
  ForgeCodeUpdated:  'forge.code_updated',
  Notification:      'notification',
} as const

export function onEvent<T = unknown>(name: string, handler: (payload: T) => void): () => void {
  if (!source) return () => {}
  const listener = (e: MessageEvent) => {
    try { handler(JSON.parse(e.data) as T) } catch {}
  }
  source.addEventListener(name, listener as EventListener)
  return () => source?.removeEventListener(name, listener as EventListener)
}
