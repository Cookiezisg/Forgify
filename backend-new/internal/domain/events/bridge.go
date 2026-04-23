package events

import "context"

// Bridge is the event dispatcher contract. Publishers fan out events
// through Bridge; subscribers receive the subset they ask for.
//
// Bridge 是事件分发契约。发布者通过 Bridge 扇出事件；订阅者接收自己
// 感兴趣的子集。
//
// Filtering is done by filterKey — an opaque string chosen by the caller.
// Typical uses:
//   - filterKey = conversationID → only the SSE connection for that
//     conversation sees its tokens.
//   - filterKey = userID → future SaaS scenario.
//
// 按 filterKey 过滤——一个由调用方选择的不透明字符串。典型用法：
//   - filterKey = conversationID → 某个对话的 SSE 连接只看到自己的 token。
//   - filterKey = userID → 未来 SaaS 场景。
//
// Implementations MUST be safe for concurrent Publish and Subscribe calls.
// Today we have an in-process memory implementation; tomorrow an
// infra/events/redis implementation will satisfy the same contract.
//
// 实现必须支持 Publish 和 Subscribe 的并发调用。今天有内存内实现；
// 未来 infra/events/redis 会满足同一契约。
type Bridge interface {
	// Publish sends e to every subscriber whose filterKey equals key.
	// Best-effort delivery: if a subscriber's buffer is full it drops the
	// event (the implementation logs). Publish does NOT block waiting for
	// slow subscribers — one slow client can't stall the whole backend.
	//
	// Publish 把 e 发给所有 filterKey 等于 key 的订阅者。
	// 尽力投递：若订阅者缓冲满了则丢弃该事件（实现会记日志）。Publish
	// **不会**阻塞等待慢订阅者——单个慢客户端不能拖累整个后端。
	Publish(ctx context.Context, key string, e Event)

	// Subscribe registers a new subscription for the given filterKey and
	// returns a receive-only event channel plus a cancel func. The channel
	// is never closed — the caller must select on ctx.Done() (or another
	// termination signal) to know when to stop reading. Calling the cancel
	// func removes the subscription from the bridge.
	//
	// Subscribe 为给定 filterKey 注册新订阅，返回一个只读事件 channel
	// 和一个取消函数。channel **永远不会被关闭**——调用方必须通过
	// ctx.Done()（或其他终止信号）判断何时停止读取。调用取消函数会
	// 把订阅从 bridge 中摘除。
	Subscribe(ctx context.Context, key string) (<-chan Event, func())
}
