package events

import "context"

// Bridge is the event dispatcher contract. Implementations must be safe
// for concurrent Publish and Subscribe.
//
// Bridge 是事件分发契约。实现必须支持 Publish 和 Subscribe 并发调用。
type Bridge interface {
	// Publish sends e to every subscriber whose filterKey equals key.
	// Best-effort: slow subscribers drop events, never block the publisher.
	//
	// Publish 把 e 发给所有 filterKey 等于 key 的订阅者。尽力投递：
	// 慢订阅者丢事件，绝不阻塞 publisher。
	Publish(ctx context.Context, key string, e Event)

	// Subscribe returns a receive channel + cancel func. The channel is
	// never closed — callers select on ctx.Done() (or cancel) to stop.
	//
	// Subscribe 返回接收 channel 和取消函数。channel **永不关闭**——
	// 调用方通过 ctx.Done() 或 cancel 停止读取。
	Subscribe(ctx context.Context, key string) (<-chan Event, func())
}
