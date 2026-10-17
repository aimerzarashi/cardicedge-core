package eventdriven

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/lib/pq"
)

// migrations の notify_domain_event が通知するチャネル
const channel = "domain_events_channel"

// 通知を受け取る側（Hub）
type ListenerHandler interface {
	HandleEvent(ref EventRef)
	// DB とつながり直した。その間の通知は失われているため、取りこぼしを探す（Poller を動かす）
	HandleReconnect()
}

// domain_events の通知を受け取る。DB との再接続は pq.Listener に任せる（つながり直すと LISTEN を張り直し、
// nil の通知で知らせる。参照の api は外側にも再接続のループを持っていたが、二重になるため持たない）
type Listener struct {
	dsn       string
	ready     chan struct{}
	readyOnce sync.Once
}

func NewListener(dsn string) *Listener {
	return &Listener{dsn: dsn, ready: make(chan struct{})}
}

// LISTEN を張り終えたら閉じる。張る前の通知は届かないため、確実に受け取りたい呼び出し側（起動・テスト）が待つ
func (l *Listener) Ready() <-chan struct{} {
	return l.ready
}

// ctx が終わるまで通知を受け取り、handler に渡す。DB につながるまで待つ
func (l *Listener) Run(ctx context.Context, handler ListenerHandler) {
	listener := pq.NewListener(l.dsn, 100*time.Millisecond, 10*time.Second, logListenerEvent)
	go func() {
		<-ctx.Done()
		// つながるまで待っている Listen を解き、Notify を閉じさせる
		_ = listener.Close()
	}()

	if err := listener.Listen(channel); err != nil {
		if ctx.Err() == nil {
			slog.ErrorContext(ctx, "failed to listen domain events", "error", err)
		}
		return
	}
	l.readyOnce.Do(func() { close(l.ready) })

	for notification := range listener.Notify {
		if notification == nil {
			handler.HandleReconnect()
			continue
		}
		ref, err := ParseEventRef(notification.Extra)
		if err != nil {
			slog.ErrorContext(ctx, "ignored invalid domain event notification", "error", err)
			continue
		}
		handler.HandleEvent(ref)
	}
}

// DB との接続が切れること自体は起こりうる（再起動等）ため ERROR にしない。つながらないままなら Poller のほうで気づける
func logListenerEvent(event pq.ListenerEventType, err error) {
	switch event {
	case pq.ListenerEventConnected:
		slog.Info("domain events listener connected")
	case pq.ListenerEventReconnected:
		slog.Info("domain events listener reconnected")
	default:
		slog.Warn("domain events listener disconnected", "event", event, "error", err)
	}
}
