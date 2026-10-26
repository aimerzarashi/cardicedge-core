package repository

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

// players の行を作る。metadata の配布の状態は書かない（player_starter_grants の投影の項目）
type PlayerWriter struct{}

func NewPlayerWriter() *PlayerWriter {
	return &PlayerWriter{}
}

// 行があれば何もしない。配布の投影が先に動いて書いた metadata を、消さないため
func (w *PlayerWriter) Upsert(ctx context.Context, player readmodel.Player) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx,
		`INSERT INTO players (player_id) VALUES ($1) ON CONFLICT (player_id) DO NOTHING`,
		player.PlayerID)
	return err
}

// プレイヤーを消すイベントは無いため呼ばれない（投影の宣言に要るので置く）
func (w *PlayerWriter) Delete(ctx context.Context, player readmodel.Player) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, `DELETE FROM players WHERE player_id = $1`, player.PlayerID)
	return err
}
