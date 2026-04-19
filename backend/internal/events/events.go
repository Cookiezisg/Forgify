package events

// Event name constants
const (
	ChatToken     = "chat.token"
	ChatDone      = "chat.done"
	ChatError     = "chat.error"
	ChatCompacted = "chat.compacted"

	NodeStatusChanged = "node.status_changed"
	NodeOutput        = "node.output"
	RunCompleted      = "run.completed"
	RunFailed         = "run.failed"

	ApprovalPending    = "approval.pending"
	ApprovalExpired    = "approval.expired"
	PermissionRequired = "permission.required"

	CanvasUpdated = "canvas.updated"

	MailboxUpdated = "mailbox.updated"

	WorkflowDeployed = "workflow.deployed"
	OpenConversation = "open.conversation"
)

// Bridge lets business logic emit events to the SSE broker.
type Bridge struct {
	publish func(event string, payload any)
}

func NewBridge(publish func(string, any)) *Bridge {
	return &Bridge{publish: publish}
}

func (b *Bridge) Emit(event string, payload any) {
	b.publish(event, payload)
}
