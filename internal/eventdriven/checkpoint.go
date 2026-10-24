package eventdriven

import (
	"context"
	"database/sql"
	"errors"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// 今より大きい版のときだけ書き、書けたかを返す。read model を書く前に、同じトランザクションで呼ぶ。
// 単純な上書きにしないのは、複数台で同じ集約を同時に作り直したとき、古い版を読んだほうが後から commit して
// read model を古い状態に戻さないため（参照の api は 1 台で動かす前提で上書きしている）。書けた行の鍵は commit まで持つので、
// 同じキーのほかのトランザクションは commit を待ってから版を比べ直す
const advanceCheckpointQuery = `
	INSERT INTO event_subscriber_checkpoints (subscriber_id, entity_type_id, entity_id, last_version, updated_at)
	VALUES ($1, $2, $3, $4, now())
	ON CONFLICT (subscriber_id, entity_type_id, entity_id) DO UPDATE
	SET last_version = EXCLUDED.last_version, updated_at = EXCLUDED.updated_at
	WHERE event_subscriber_checkpoints.last_version < EXCLUDED.last_version
	RETURNING last_version
`

func advanceCheckpoint(ctx context.Context, key workKey, version int64) (bool, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return false, err
	}
	var written int64
	err = exec.QueryRowContext(ctx, advanceCheckpointQuery, key.subscriberID, key.entityTypeID.Int16(), key.entityID, version).Scan(&written)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// 処理した版以下の行だけを消す。処理中に新しいイベントが入っていれば、トリガーが版を上げているため残る。
// 最後に消すのは、イベントを保存するコマンドのトリガーと同じ行の鍵を取り合う時間を、commit までの短い間にするため
const removePendingQuery = `
	DELETE FROM event_subscriber_pending
	WHERE subscriber_id = $1 AND entity_type_id = $2 AND entity_id = $3 AND version <= $4
`

func removePending(ctx context.Context, key workKey, version int64) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, removePendingQuery, key.subscriberID, key.entityTypeID.Int16(), key.entityID, version)
	return err
}
