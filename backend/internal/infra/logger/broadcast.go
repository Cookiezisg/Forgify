// broadcast.go — dev-only log broadcaster: implements zapcore.Core and fans
// log entries out to SSE subscribers with a 500-entry ring buffer for replay.
//
// broadcast.go — 仅 dev 模式使用的日志广播器：实现 zapcore.Core，通过
// 500 条环形缓冲区支持回放，并扇出给所有 SSE 订阅者。
package logger

import (
	"encoding/json"
	"sync"
	"time"

	"go.uber.org/zap/zapcore"
)

const (
	ringCap   = 500
	subBufCap = 128
)

// LogEntry is one structured log line sent over SSE.
//
// LogEntry 是通过 SSE 推送的单条结构化日志。
type LogEntry struct {
	Time   string         `json:"time"`
	Level  string         `json:"level"`
	Msg    string         `json:"msg"`
	Fields map[string]any `json:"fields,omitempty"`
}

type logSub struct {
	ch   chan []byte
	once sync.Once
	done chan struct{}
}

// LogBroadcaster implements zapcore.Core and fans encoded log entries to
// SSE subscribers. Ring buffer holds the most recent 500 entries for
// replay on new connections. Design mirrors infra/events/memory/bridge.go:
// snapshot subs under RLock, send outside the lock; slow subs drop entries.
//
// LogBroadcaster 实现 zapcore.Core，把编码后的日志条目扇出给 SSE 订阅者。
// 环形缓冲区保留最近 500 条供新连接回放。设计与 events/memory/bridge.go 对称：
// RLock 下快照订阅者，释放锁后发送；慢订阅者丢弃条目。
type LogBroadcaster struct {
	mu    sync.RWMutex
	ring  [ringCap][]byte
	head  int // next write position / 下一个写入位置
	count int // total entries written / 已写入总条数

	subs []*logSub
	ctx  []zapcore.Field // pre-applied via With / 通过 With 预设的字段
}

// NewLogBroadcaster returns a ready-to-use broadcaster.
//
// NewLogBroadcaster 返回一个可直接使用的广播器。
func NewLogBroadcaster() *LogBroadcaster {
	return &LogBroadcaster{}
}

// Ring returns all buffered entries in chronological order (oldest first).
//
// Ring 按时间顺序（最旧优先）返回所有缓冲条目。
func (b *LogBroadcaster) Ring() [][]byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	n := min(b.count, ringCap)
	out := make([][]byte, n)
	if b.count <= ringCap {
		copy(out, b.ring[:b.count])
	} else {
		for i := range ringCap {
			out[i] = b.ring[(b.head+i)%ringCap]
		}
	}
	return out
}

// Subscribe returns a channel of JSON-encoded LogEntry bytes and an
// idempotent cancel function. Caller should drain the channel promptly.
//
// Subscribe 返回 JSON 编码的 LogEntry 字节 channel 和幂等取消函数。
// 调用方应及时消费 channel，否则条目会被丢弃。
func (b *LogBroadcaster) Subscribe() (<-chan []byte, func()) {
	sub := &logSub{
		ch:   make(chan []byte, subBufCap),
		done: make(chan struct{}),
	}
	b.mu.Lock()
	b.subs = append(b.subs, sub)
	b.mu.Unlock()

	cancel := func() {
		sub.once.Do(func() {
			close(sub.done)
			b.removeSub(sub)
		})
	}
	return sub.ch, cancel
}

func (b *LogBroadcaster) removeSub(target *logSub) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for i, s := range b.subs {
		if s == target {
			b.subs = append(b.subs[:i], b.subs[i+1:]...)
			return
		}
	}
}

// ── zapcore.Core ──────────────────────────────────────────────────────────────

// Enabled reports true for all levels — dev broadcaster captures everything.
// Enabled 对所有级别返回 true——dev 广播器捕获所有日志。
func (b *LogBroadcaster) Enabled(zapcore.Level) bool { return true }

// With returns a wrapper that prepends ctx fields to every Write call.
// With 返回一个包装器，在每次 Write 调用时前置 ctx 字段。
func (b *LogBroadcaster) With(fields []zapcore.Field) zapcore.Core {
	return &broadcasterWith{
		parent: b,
		ctx:    append(append([]zapcore.Field{}, b.ctx...), fields...),
	}
}

// Check adds b to ce if the entry passes the level check.
func (b *LogBroadcaster) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return ce.AddCore(e, b)
}

// Write encodes the entry + fields as JSON, appends to the ring buffer,
// and fans out to subscribers non-blocking.
//
// Write 把条目 + 字段编码为 JSON，追加到环形缓冲区，
// 并非阻塞地扇出给所有订阅者。
func (b *LogBroadcaster) Write(e zapcore.Entry, fields []zapcore.Field) error {
	return b.write(e, append(b.ctx, fields...))
}

// Sync is a no-op — broadcaster is in-memory.
func (b *LogBroadcaster) Sync() error { return nil }

func (b *LogBroadcaster) write(e zapcore.Entry, fields []zapcore.Field) error {
	enc := zapcore.NewMapObjectEncoder()
	for _, f := range fields {
		f.AddTo(enc)
	}

	entry := LogEntry{
		Time:  e.Time.UTC().Format(time.RFC3339),
		Level: e.Level.String(),
		Msg:   e.Message,
	}
	if len(enc.Fields) > 0 {
		entry.Fields = enc.Fields
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return nil // best-effort; never block the logger
	}

	b.mu.Lock()
	b.ring[b.head] = data
	b.head = (b.head + 1) % ringCap
	b.count++
	snapshot := make([]*logSub, len(b.subs))
	copy(snapshot, b.subs)
	b.mu.Unlock()

	for _, s := range snapshot {
		select {
		case s.ch <- data:
		case <-s.done:
		default:
			// subscriber buffer full — drop silently
		}
	}
	return nil
}

// broadcasterWith is a thin wrapper returned by With; it delegates writes
// to the parent broadcaster with pre-set context fields.
//
// broadcasterWith 是 With 返回的薄包装器，把写入委托给父广播器，
// 并携带预设的 context 字段。
type broadcasterWith struct {
	parent *LogBroadcaster
	ctx    []zapcore.Field
}

func (w *broadcasterWith) Enabled(l zapcore.Level) bool { return w.parent.Enabled(l) }
func (w *broadcasterWith) With(fields []zapcore.Field) zapcore.Core {
	return &broadcasterWith{
		parent: w.parent,
		ctx:    append(append([]zapcore.Field{}, w.ctx...), fields...),
	}
}
func (w *broadcasterWith) Check(e zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return ce.AddCore(e, w)
}
func (w *broadcasterWith) Write(e zapcore.Entry, fields []zapcore.Field) error {
	return w.parent.write(e, append(w.ctx, fields...))
}
func (w *broadcasterWith) Sync() error { return nil }
