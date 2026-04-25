// Package memory provides an in-process implementation of
// domain/events.Bridge. Events fan out through buffered channels with
// non-blocking sends — slow subscribers drop events, never stall the backend.
//
// Package memory 提供 domain/events.Bridge 的进程内实现。事件通过带
// 缓冲的 channel 扇出，发送非阻塞——慢订阅者丢事件，不会拖慢后端。
package memory

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/sunweilin/forgify/backend/internal/domain/events"
)

// defaultBufferSize caps the number of unread events per subscriber.
// Tuned for SSE: ~64 LLM tokens of slack before we drop.
//
// defaultBufferSize 限制单订阅者的未读事件数。
// 按 SSE 场景调校：约 64 个 LLM token 的余量。
const defaultBufferSize = 64

// Bridge is a thread-safe, in-process fan-out event bus.
//
// Bridge 是线程安全的进程内扇出事件总线。
type Bridge struct {
	log *zap.Logger

	mu   sync.RWMutex
	subs map[string][]*subscription
}

type subscription struct {
	ch     chan events.Event
	done   chan struct{}
	closed sync.Once
}

// NewBridge constructs an empty Bridge.
//
// NewBridge 构造一个空的 Bridge。
func NewBridge(log *zap.Logger) *Bridge {
	return &Bridge{
		log:  log,
		subs: make(map[string][]*subscription),
	}
}

// Publish snapshots subscribers under RLock, then sends without the lock
// so slow sends don't block new Subscribe calls.
//
// Publish 在 RLock 下快照订阅者列表，然后**释放锁**再发送——
// 避免慢发送阻塞新的 Subscribe。
func (b *Bridge) Publish(_ context.Context, key string, e events.Event) {
	b.mu.RLock()
	snapshot := make([]*subscription, len(b.subs[key]))
	copy(snapshot, b.subs[key])
	b.mu.RUnlock()

	for _, s := range snapshot {
		select {
		case <-s.done:
			// subscriber cancelled / 订阅者已取消
		case s.ch <- e:
			// delivered / 已投递
		default:
			b.log.Warn("dropping event: subscriber buffer full",
				zap.String("event", e.EventName()),
				zap.String("key", key),
			)
		}
	}
}

// Subscribe registers a new subscription and returns its channel + an
// idempotent cancel func. cancel also runs automatically on ctx.Done().
//
// Subscribe 注册新订阅，返回 channel 和幂等的取消函数。ctx.Done() 时
// cancel 自动触发。
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

	go func() {
		select {
		case <-ctx.Done():
			cancel()
		case <-sub.done:
		}
	}()

	return sub.ch, cancel
}

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

// Compile-time check that *Bridge satisfies events.Bridge.
// 编译期确认 *Bridge 满足 events.Bridge。
var _ events.Bridge = (*Bridge)(nil)
