package query

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
)

// 対戦の進み。画面は手元の版と比べ、進んでいれば差分を query で読む
type MatchProgress struct {
	ID       uuid.UUID
	Revision int64
	// その版のイベントの起きた時刻
	OccurredAt time.Time
}

// 対戦の最新のイベント。RewardsCompleted は、それが全員の戦利品の受け取りが済んだ（対戦の流れの終わり）か
type MatchHead struct {
	Revision         int64
	OccurredAt       time.Time
	RewardsCompleted bool
}

// DB から対戦の最新のイベントを読み直す。通知を待たずに読むので、通知の取りこぼしを埋められる
type LoadMatchHeadFunc func(ctx context.Context, id match.ID) (MatchHead, error)

// つないでいる購読者に対戦の進みを配る。複数台の core では各台が自分の Hub で通知を受けるので、この台につないでいる購読者だけに配る。
// 通知は起こす合図としてだけ使い、送る値は起きるたびに DB から読み直す。購読者が遅くても版を取りこぼさず、間の版はまとまり、
// 古い版を送り直さない（版ごとに溜めると、遅い購読者の分が増え続けるため）
type MatchProgressBroker struct {
	authorize AuthorizeMatchFunc
	loadHead  LoadMatchHeadFunc
	interval  time.Duration

	mu          sync.Mutex
	closed      bool
	subscribers map[match.ID]map[*matchProgressSubscriber]struct{}
}

// interval は、変わらなくても送り直す間隔。0 以下なら送り直さない（通知の取りこぼしを埋め、何も流れない時間で切る中継にも切られないため）
func NewMatchProgressBroker(authorize AuthorizeMatchFunc, loadHead LoadMatchHeadFunc, interval time.Duration) *MatchProgressBroker {
	return &MatchProgressBroker{
		authorize: authorize, loadHead: loadHead, interval: interval,
		subscribers: make(map[match.ID]map[*matchProgressSubscriber]struct{}),
	}
}

type matchProgressSubscriber struct {
	// 通知を受けた合図。溜めずに 1 つにまとめる
	wake chan struct{}
	// 通知で知った最新の版
	notified atomic.Int64
	// 配り役を閉じた
	closed chan struct{}
}

// 参加していない・無い対戦は ErrMatchNotFound。返したチャネルへ、つないだ直後に今の版を、その後は進むたびと interval ごとに送る。
// 全員が戦利品を受け取り終えたら、その版を送ってから閉じる。ctx が終わるか配り役を閉じても閉じる。
// 受け手は、チャネルが閉じるまで読み続けること（読まなくなると送る側が待つので、ctx を取り消して止める）
func (b *MatchProgressBroker) Subscribe(ctx context.Context, playerID uuid.UUID, matchID uuid.UUID) (<-chan MatchProgress, error) {
	id, err := b.authorize(ctx, playerID, matchID)
	if err != nil {
		return nil, err
	}
	subscriber := &matchProgressSubscriber{wake: make(chan struct{}, 1), closed: make(chan struct{})}
	// 最初に読む前に登録する（読んでから登録すると、その間の通知を逃す）
	if !b.add(id, subscriber) {
		closed := make(chan MatchProgress)
		close(closed)
		return closed, nil
	}
	head, err := b.loadHead(ctx, id)
	if err != nil {
		b.remove(id, subscriber)
		return nil, err
	}
	out := make(chan MatchProgress)
	go b.serve(ctx, id, matchID, subscriber, head, out)
	return out, nil
}

func (b *MatchProgressBroker) serve(ctx context.Context, id match.ID, matchID uuid.UUID, subscriber *matchProgressSubscriber, head MatchHead, out chan<- MatchProgress) {
	defer close(out)
	defer b.remove(id, subscriber)
	timer := time.NewTimer(b.interval)
	defer timer.Stop()
	// 間隔が 0 以下なら送り直さない（0 のままタイマーを回すと、読み直しと送りが止まらなくなる）
	var heartbeats <-chan time.Time
	if b.interval > 0 {
		heartbeats = timer.C
	}
	for {
		// 受け手が受け取るまでの間に版が進んだら、送る値を最新に取り替える（古い版を送らない）
	sending:
		for {
			select {
			case out <- MatchProgress{ID: matchID, Revision: head.Revision, OccurredAt: head.OccurredAt}:
				break sending
			case <-subscriber.wake:
				if subscriber.notified.Load() > head.Revision {
					if latest, err := b.loadHead(ctx, id); err == nil {
						head = latest
					} else {
						slog.WarnContext(ctx, "failed to reload match progress", "error", err)
					}
				}
			case <-ctx.Done():
				return
			case <-subscriber.closed:
				return
			}
		}
		if head.RewardsCompleted {
			return
		}
		timer.Reset(b.interval)
		sent := head.Revision
		for {
			var heartbeat bool
			select {
			case <-subscriber.wake:
				// 送った版以下の通知は、すでに送った分
				if subscriber.notified.Load() <= sent {
					continue
				}
			case <-heartbeats:
				heartbeat = true
			case <-ctx.Done():
				return
			case <-subscriber.closed:
				return
			}
			latest, err := b.loadHead(ctx, id)
			if err != nil {
				// 読めなければ、次の通知か間隔でやり直す（つないだ後の一時的な DB の失敗で購読を落とさない）
				slog.WarnContext(ctx, "failed to reload match progress", "error", err)
				timer.Reset(b.interval)
				continue
			}
			if latest.Revision > sent || heartbeat {
				head = latest
				break
			}
		}
	}
}

// 通知を受けた台で、Hub から呼ぶ。待たずに返す（Listener の受信を止めないため）
func (b *MatchProgressBroker) Notify(id match.ID, revision int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for subscriber := range b.subscribers[id] {
		for {
			current := subscriber.notified.Load()
			if revision <= current || subscriber.notified.CompareAndSwap(current, revision) {
				break
			}
		}
		select {
		case subscriber.wake <- struct{}{}:
		default:
		}
	}
}

// すべての購読を閉じる。止めるとき、つなぎっぱなしの接続でシャットダウンの時間を使い切らないために先に呼ぶ。
// 閉じた後に来た購読は、何も送らずに閉じる
func (b *MatchProgressBroker) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, group := range b.subscribers {
		for subscriber := range group {
			close(subscriber.closed)
		}
	}
}

func (b *MatchProgressBroker) add(id match.ID, subscriber *matchProgressSubscriber) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return false
	}
	if b.subscribers[id] == nil {
		b.subscribers[id] = make(map[*matchProgressSubscriber]struct{})
	}
	b.subscribers[id][subscriber] = struct{}{}
	return true
}

func (b *MatchProgressBroker) remove(id match.ID, subscriber *matchProgressSubscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subscribers[id], subscriber)
	if len(b.subscribers[id]) == 0 {
		delete(b.subscribers, id)
	}
}
