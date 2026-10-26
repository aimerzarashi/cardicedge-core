package eventdriven

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// 複数台の core で、1 つの処理の単位（購読者 × 集約）を 1 台だけが処理するためのロック（core#19）。
// PostgreSQL のアドバイザリーロックを、待たずに取る（pg_try_advisory_lock）。取れなければ、ほかの台が処理中なので飛ばす。
// やることの表に取った台と期限の列を持つ案は、落ちた台のための期限の延長と、デッドロックを避ける決まりが要るため採らなかった
// （アドバイザリーロックは接続が切れると PostgreSQL が外し、待たないのでデッドロックも起きない）。
//
// ロックは Hub ごとに 1 本の接続に持つ（1 つの接続でいくつものロックを持て、取る・外すの文はすぐ終わるため、
// ワーカーが順番に使っても詰まらない）。接続が切れる・外せないときは接続を捨て、その接続のロックをまとめて外す。
// その間はほかの台も同じ単位を処理しうるが、投影はチェックポイント、ポリシーはコマンドの一意な key・状態で、結果は 1 回に保たれる
type unitLocks struct {
	mu   sync.Mutex
	conn *sql.Conn
	// つなぎ直すたびに増やす。捨てた接続のロックは外れているので、取った後に接続が変わっていれば外さない
	// （新しい接続で外そうとすると「持っていない」になり、新しい接続まで捨ててしまうため）
	generation uint64
}

// 取ったロックの印。外すときに、取ったときと同じ接続かを確かめる
type unitLock struct {
	id         int64
	generation uint64
}

// 番号は購読者の id を上位に、集約の内部 ID を下位 48 ビットに置く。起動時の種別の同期のロック（hashtext の 32 ビットの値）とは、
// 購読者の id が 1 以上で番号が 2^48 以上になるため重ならない
func unitLockID(key workKey) (int64, error) {
	const entityBits = 48
	if key.entityID < 0 || key.entityID >= 1<<entityBits {
		return 0, fmt.Errorf("eventdriven: entity id does not fit in the lock id: %d", key.entityID)
	}
	return int64(key.subscriberID)<<entityBits | key.entityID, nil
}

// 取れたら ok が true。接続の誤りのときは接続を捨ててエラーを返す（次に呼ばれたときにつなぎ直す）
func (l *unitLocks) tryLock(ctx context.Context, key workKey) (lock unitLock, ok bool, err error) {
	id, err := unitLockID(key)
	if err != nil {
		return unitLock{}, false, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		conn, err := rdb.Conn(ctx)
		if err != nil {
			return unitLock{}, false, err
		}
		l.conn = conn
		l.generation++
	}
	var acquired bool
	if err := l.conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, id).Scan(&acquired); err != nil {
		l.discard()
		return unitLock{}, false, err
	}
	return unitLock{id: id, generation: l.generation}, acquired, nil
}

// 取ったときの接続をもう捨てていれば、ロックは外れているので何もしない。外せなかった（接続の誤り、または持っていなかった）
// ときは接続を捨てる。捨てると、この接続のロックがすべて外れる（外し忘れたロックを残すと、その単位を誰も処理できなくなるため）
func (l *unitLocks) unlock(ctx context.Context, lock unitLock) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil || l.generation != lock.generation {
		return nil
	}
	var released bool
	if err := l.conn.QueryRowContext(ctx, `SELECT pg_advisory_unlock($1)`, lock.id).Scan(&released); err != nil {
		l.discard()
		return err
	}
	if !released {
		l.discard()
		return errors.New("eventdriven: unit lock was not held")
	}
	return nil
}

func (l *unitLocks) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn != nil {
		l.discard()
	}
}

// mu を持って呼ぶ。Close だけではプールに返るだけで、ロックを持った接続が使い回されるため、壊れた接続として閉じさせる
// （database/sql は Raw の関数が driver.ErrBadConn を返すと、その接続をプールに戻さず閉じる）
func (l *unitLocks) discard() {
	_ = l.conn.Raw(func(any) error { return driver.ErrBadConn })
	_ = l.conn.Close()
	l.conn = nil
}
