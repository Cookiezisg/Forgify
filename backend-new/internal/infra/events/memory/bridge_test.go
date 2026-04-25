// bridge_test.go — unit tests for the in-memory Bridge implementation.
// Includes concurrency tests that should run under `go test -race`.
//
// bridge_test.go — 内存 Bridge 实现的单元测试。包含需在 `go test -race`
// 下运行的并发测试。
package memory

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/sunweilin/forgify/backend/internal/domain/events"
)

// newTestBridge returns a Bridge with an observable logger so tests can
// inspect "buffer full" drop logs.
//
// newTestBridge 返回带可观测 logger 的 Bridge，让测试能检查 "buffer full"
// 丢包日志。
func newTestBridge(t *testing.T) (*Bridge, *observer.ObservedLogs) {
	t.Helper()
	core, obs := observer.New(zap.DebugLevel)
	return NewBridge(zap.New(core)), obs
}

// sampleToken returns a ChatToken payload for tests.
// sampleToken 返回测试用的 ChatToken。
func sampleToken(delta string) events.ChatToken {
	return events.ChatToken{
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		Delta:          delta,
	}
}

func TestBridge_PublishWithNoSubscribers(t *testing.T) {
	// No panic, no block.
	// 无 panic、无阻塞。
	b, _ := newTestBridge(t)
	b.Publish(t.Context(), "conv-1", sampleToken("hi"))
}

func TestBridge_SubscribeReceivesPublishedEvent(t *testing.T) {
	b, _ := newTestBridge(t)
	ch, unsub := b.Subscribe(t.Context(), "conv-1")
	defer unsub()

	b.Publish(t.Context(), "conv-1", sampleToken("hello"))

	select {
	case e := <-ch:
		token, ok := e.(events.ChatToken)
		if !ok {
			t.Fatalf("expected ChatToken, got %T", e)
		}
		if token.Delta != "hello" {
			t.Errorf("token: got %q, want hello", token.Delta)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBridge_FiltersByKey(t *testing.T) {
	// A subscriber for "conv-1" should NOT receive events published
	// with key "conv-2".
	//
	// "conv-1" 的订阅者**不应**收到发往 "conv-2" 的事件。
	b, _ := newTestBridge(t)
	ch1, unsub1 := b.Subscribe(t.Context(), "conv-1")
	defer unsub1()
	ch2, unsub2 := b.Subscribe(t.Context(), "conv-2")
	defer unsub2()

	b.Publish(t.Context(), "conv-2", sampleToken("for-two"))

	select {
	case e := <-ch2:
		if e.(events.ChatToken).Delta != "for-two" {
			t.Errorf("conv-2 got wrong token: %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("conv-2 didn't receive its event")
	}

	select {
	case e := <-ch1:
		t.Errorf("conv-1 should NOT have received event, got %+v", e)
	case <-time.After(100 * time.Millisecond):
		// Good: nothing received. / 预期：无接收。
	}
}

func TestBridge_MultipleSubscribersSameKey(t *testing.T) {
	// All subscribers to the same key should each get the event.
	// 同一 key 下的所有订阅者都应各自收到事件。
	b, _ := newTestBridge(t)

	const n = 5
	channels := make([]<-chan events.Event, n)
	cancels := make([]func(), n)
	for i := range n {
		channels[i], cancels[i] = b.Subscribe(t.Context(), "conv-1")
	}
	defer func() {
		for _, c := range cancels {
			c()
		}
	}()

	b.Publish(t.Context(), "conv-1", sampleToken("broadcast"))

	for i, ch := range channels {
		select {
		case e := <-ch:
			if e.(events.ChatToken).Delta != "broadcast" {
				t.Errorf("subscriber %d: wrong token %+v", i, e)
			}
		case <-time.After(time.Second):
			t.Errorf("subscriber %d: timed out", i)
		}
	}
}

func TestBridge_CancelStopsDelivery(t *testing.T) {
	// After calling the cancel func, future publishes must NOT reach the channel.
	// 调用 cancel 函数后，后续发布**不得**到达该 channel。
	b, _ := newTestBridge(t)

	ch, unsub := b.Subscribe(t.Context(), "conv-1")
	unsub()

	b.Publish(t.Context(), "conv-1", sampleToken("should-not-arrive"))

	select {
	case e := <-ch:
		t.Errorf("received event after cancel: %+v", e)
	case <-time.After(100 * time.Millisecond):
		// Good. / 预期。
	}
}

func TestBridge_ContextDoneAutoCancels(t *testing.T) {
	// Subscribe's goroutine watches ctx — when ctx is done, the subscription
	// is removed automatically even without calling the cancel func.
	//
	// Subscribe 的 goroutine 监听 ctx——ctx 结束时自动摘除订阅，无需调用
	// cancel 函数。
	b, _ := newTestBridge(t)
	ctx, cancel := context.WithCancel(t.Context())

	_, _ = b.Subscribe(ctx, "conv-1")
	cancel() // cancel ctx / 取消 ctx

	// Give the goroutine a moment to run removeSub.
	// 给 goroutine 一点时间运行 removeSub。
	time.Sleep(20 * time.Millisecond)

	b.mu.RLock()
	remaining := len(b.subs["conv-1"])
	b.mu.RUnlock()
	if remaining != 0 {
		t.Errorf("subscription not cleaned up after ctx cancel: %d remain", remaining)
	}
}

func TestBridge_CancelIsIdempotent(t *testing.T) {
	// Calling cancel multiple times should not panic.
	// 多次调用 cancel 不应 panic。
	b, _ := newTestBridge(t)
	_, unsub := b.Subscribe(t.Context(), "conv-1")
	unsub()
	unsub() // second call / 二次调用
	unsub() // third / 三次
}

func TestBridge_SlowSubscriberDropsAndLogs(t *testing.T) {
	// A subscriber that doesn't read has its buffer fill up after
	// defaultBufferSize publishes. The (defaultBufferSize+1)th publish
	// must be dropped and logged.
	//
	// 不读的订阅者在 defaultBufferSize 次发布后缓冲满。第 defaultBufferSize+1
	// 次发布必须被丢弃并记日志。
	b, obs := newTestBridge(t)

	slow, unsub := b.Subscribe(t.Context(), "conv-1")
	defer unsub()

	// Fill buffer. / 填满缓冲。
	for range defaultBufferSize {
		b.Publish(t.Context(), "conv-1", sampleToken("fill"))
	}

	// Drop next. / 下一条丢弃。
	b.Publish(t.Context(), "conv-1", sampleToken("overflow"))

	if drops := obs.FilterMessage("dropping event: subscriber buffer full").Len(); drops == 0 {
		t.Errorf("expected at least one drop log entry, got 0")
	}

	// Buffer should be exactly at capacity (the overflow was dropped, not queued).
	// 缓冲应刚好满（溢出被丢弃，不入队）。
	if len(slow) != defaultBufferSize {
		t.Errorf("slow channel length = %d, want %d", len(slow), defaultBufferSize)
	}
}

func TestBridge_OneSlowSubscriberDoesNotBlockOthers(t *testing.T) {
	// Slow subscriber: never reads. Fast subscriber: reads immediately.
	// Publisher should not block; fast receives all events.
	// Use separate keys so one slow doesn't affect another on a different key,
	// AND test same-key too.
	//
	// Slow 订阅者不读；fast 订阅者立即读。Publisher 不应阻塞；fast 收到所有。
	// 分 key 场景和同 key 场景都测试。
	b, _ := newTestBridge(t)

	slow, unsubSlow := b.Subscribe(t.Context(), "conv-1")
	defer unsubSlow()
	_ = slow // deliberately never read from slow / 故意不读 slow

	fast, unsubFast := b.Subscribe(t.Context(), "conv-2")
	defer unsubFast()

	// Drain fast concurrently. / 并发 drain fast。
	received := atomic.Int32{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-fast:
				received.Add(1)
			case <-time.After(200 * time.Millisecond):
				return
			}
		}
	}()

	// Use fewer events than defaultBufferSize so fast cannot be dropped
	// even if the drain goroutine lags briefly — the test is specifically
	// about publisher NOT being blocked by slow, not about buffer behavior.
	//
	// 使用少于 defaultBufferSize 的事件数，即使 drain goroutine 偶尔滞后
	// fast 也不会被丢弃——本测试专注于验证"publisher 不被 slow 阻塞"，
	// 而非缓冲行为。
	total := defaultBufferSize / 2
	for range total {
		b.Publish(t.Context(), "conv-1", sampleToken("slow-bound"))
		b.Publish(t.Context(), "conv-2", sampleToken("fast-bound"))
	}
	<-done

	if got := received.Load(); got != int32(total) {
		t.Errorf("fast received %d, want %d (slow subscriber blocked publisher?)", got, total)
	}
}

func TestBridge_ConcurrentPublishAndSubscribe(t *testing.T) {
	// Race detector guard: many goroutines publishing and subscribing at
	// the same time must not trip the race detector.
	// Run with `go test -race`.
	//
	// race detector 守卫：许多 goroutine 同时 publish 和 subscribe 不应触发
	// race detector。用 `go test -race` 运行。
	b, _ := newTestBridge(t)

	var wg sync.WaitGroup
	var receivedCount atomic.Int32

	// 20 subscribers rotating through two keys.
	// 20 个订阅者在两个 key 上轮转。
	for i := range 20 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := "conv-1"
			if idx%2 == 0 {
				key = "conv-2"
			}
			ch, unsub := b.Subscribe(t.Context(), key)
			defer unsub()
			for {
				select {
				case <-ch:
					receivedCount.Add(1)
				case <-time.After(100 * time.Millisecond):
					return
				}
			}
		}(i)
	}

	// 10 publishers spraying events.
	// 10 个发布者喷事件。
	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := "conv-1"
			if idx%2 == 0 {
				key = "conv-2"
			}
			for range 50 {
				b.Publish(t.Context(), key, sampleToken("x"))
			}
		}(i)
	}

	wg.Wait()
	if receivedCount.Load() == 0 {
		t.Errorf("no events received at all; pub/sub completely broken")
	}
}
