// SSE-based event system — replaces @wailsio/runtime Events

import { setBackendPort } from './api'

let source: EventSource | null = null;

export function initBackend(port: number): void {
  setBackendPort(port)
  if (source) source.close();
  source = new EventSource(`http://127.0.0.1:${port}/events`);
}

export const EventNames = {
  ChatToken:          "chat.token",
  ChatDone:           "chat.done",
  ChatError:          "chat.error",
  ChatCompacted:      "chat.compacted",
  Notification:       "notification",
  NodeStatusChanged:  "node.status_changed",
  NodeOutput:         "node.output",
  RunCompleted:       "run.completed",
  RunFailed:          "run.failed",
  ApprovalPending:    "approval.pending",
  ApprovalExpired:    "approval.expired",
  PermissionRequired: "permission.required",
  CanvasUpdated:      "canvas.updated",
  MailboxUpdated:     "mailbox.updated",
  WorkflowDeployed:   "workflow.deployed",
  OpenConversation:   "open.conversation",
} as const;

export type EventPayload<T = unknown> = T;

export function onEvent<T = unknown>(
  name: string,
  handler: (payload: T) => void
): () => void {
  if (!source) return () => {};

  const listener = (e: MessageEvent) => {
    try { handler(JSON.parse(e.data) as T); } catch { /* ignore malformed */ }
  };
  source.addEventListener(name, listener as EventListener);
  return () => source?.removeEventListener(name, listener as EventListener);
}
