// Package memory provides an in-process implementation of
// domain/events.Bridge. Events are fanned out through buffered channels
// with non-blocking sends — a slow subscriber sees events dropped, not the
// whole backend stall.
//
// Swappable: a future infra/events/redis package will implement the same
// domain/events.Bridge interface for multi-instance deployments (SaaS).
//
// Package memory 提供 domain/events.Bridge 的进程内实现。事件通过带
// 缓冲的 channel 扇出，发送非阻塞——慢订阅者会丢事件，但**不会**拖慢
// 整个后端。
//
// 可替换：未来的 infra/events/redis 包会实现相同的 domain/events.Bridge
// 接口，用于多实例部署（SaaS）。
package memory

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/sunweilin/forgify/backend/internal/domain/events"
)

// defaultBufferSize caps the number of unread events per subscriber. The
// value is tuned for SSE: during a streaming LLM reply a subscriber can
// lag 64 tokens behind before we start dropping — generous enough for
// jittery browser connections, tight enough to catch stuck clients.
//
// defaultBufferSize 限制单个订阅者的未读事件数。按 SSE 场景调校：
// LLM 流式回复中，订阅者可以落后 64 个 token 才开始丢——对网络抖动
// 的浏览器连接够宽松，对卡死的客户端又够紧。
const defaultBufferSize = 64

// Bridge is a thread-safe, in-process fan-out event bus.
//
// Bridge 是线程安全的进程内扇出事件总线。
type Bridge struct {
	log *zap.Logger

	mu   sync.RWMutex
	subs map[string][]*subscription // filterKey → live subscriptions
}

// subscription is one live Subscribe call's private state.
//
// subscription 是一次 Subscribe 调用产生的私有状态。
type subscription struct {
	ch     chan events.Event
	done   chan struct{}
	closed sync.Once
}

// NewBridge constructs an empty Bridge.
// NewBridge 构造一个空的 Bridge。
func NewBridge(log *zap.Logger) *Bridge {
	return &Bridge{
		log:  log,
		subs: make(map[string][]*subscription),
	}
}

// Publish implements events.Bridge. Takes a snapshot of subscribers under
// an RLock, then sends without holding the lock so slow sends can't block
// new Subscribe calls.
//
// Publish 实现 events.Bridge。在 RLock 下做订阅者快照，然后**释放锁**
// 再发送——这样慢的发送不会阻塞新的 Subscribe。
func (b *Bridge) Publish(_ context.Context, key string, e events.Event) {
	b.mu.RLock()
	snapshot := make([]*subscription, len(b.subs[key]))
	copy(snapshot, b.subs[key])
	b.mu.RUnlock()

	for _, s := range snapshot {
		select {
		case <-s.done:
			// Subscriber already cancelled — skip.
			// 订阅者已取消——跳过。
		case s.ch <- e:
			// Delivered. / 已投递。
		default:
			// Buffer full — drop, log, keep flowing.
			// 缓冲满——丢弃、记日志、继续。
			b.log.Warn("dropping event: subscriber buffer full",
				zap.String("event", e.EventName()),
				zap.String("key", key),
			)
		}
	}
}

// Subscribe implements events.Bridge. Registers a new subscription,
// returns its receive channel and a cancel function. The cancel function
// is idempotent and also runs automatically when ctx is done.
//
// Subscribe 实现 events.Bridge。注册新订阅，返回接收 channel 和取消
// 函数。取消函数幂等，ctx 结束时也自动触发。
func (b *Bridge) Subscribe(ctx context.Context, key string) (<-chan events.Event, func()) {
	sub := &subscription{
		ch:   make(chan events.Event, defaultBufferSize),
		done: make(chan struct{}),
	}

	b.mu.Lock()
	b.subs[key] = append(b.subs[key], sub)
	b.mu.Unlock()

	cancel := func() {
		sub.closed.Do(func() {
			close(sub.done)
			b.removeSub(key, sub)
		})
	}

	// Auto-cancel when ctx is done. Runs in a goroutine so Subscribe can
	// return immediately. If cancel() was called manually first, sub.done
	// is already closed and this goroutine exits promptly.
	//
	// ctx 结束时自动取消。用 goroutine 运行，这样 Subscribe 可以立即返回。
	// 如果 cancel() 已经被手动调用，sub.done 已关闭，本 goroutine 立即退出。
	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-sub.done:
		}
	}()

	return sub.ch, cancel
}

// removeSub deletes sub from the bridge's index under the given key.
// No-op if not present (idempotent).
//
// removeSub 从 bridge 索引中删除指定 key 下的 sub。不存在时为 no-op（幂等）。
func (b *Bridge) removeSub(key string, target *subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()

	list := b.subs[key]
	for i, s := range list {
		if s == target {
			b.subs[key] = append(list[:i], list[i+1:]...)
			break
		}
	}
	if len(b.subs[key]) == 0 {
		delete(b.subs, key)
	}
}

// Ensure *Bridge satisfies the domain interface at compile time.
// 编译期确认 *Bridge 满足 domain 接口。
var _ events.Bridge = (*Bridge)(nil)
