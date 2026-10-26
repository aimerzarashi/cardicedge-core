package repository

import (
	"context"
	"errors"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

// players の配布の状態の列だけを書く。書くときにプレイヤーの内部 ID を公開用の ID に引き直すため、引き方を持つ
type PlayerStarterGrantWriter struct {
	entityRefs domain.EntityRefResolver
}

func NewPlayerStarterGrantWriter(entityRefs domain.EntityRefResolver) *PlayerStarterGrantWriter {
	return &PlayerStarterGrantWriter{entityRefs: entityRefs}
}

// 行が無ければ作り、あれば metadata に配布の状態の項目だけを重ねる（players の投影が先に動いても後に動いても同じ行になる）。
// metadata の中の自分の項目だけを書くのは、sqlboiler の Upsert では表せない（列ごとの上書きしかできない）ため、SQL を直接書く
const upsertPlayerStarterGrantQuery = `
	INSERT INTO players (player_id, metadata) VALUES ($1, jsonb_build_object('starter_grant', $2::text))
	ON CONFLICT (player_id) DO UPDATE SET metadata = players.metadata || EXCLUDED.metadata
`

func (w *PlayerStarterGrantWriter) Upsert(ctx context.Context, grant readmodel.PlayerStarterGrant) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	publicID, err := w.publicID(ctx, grant)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, upsertPlayerStarterGrantQuery, publicID, string(grant.Status))
	return err
}

// 配布の状態を消すイベントは無いため呼ばれない（投影の宣言に要るので置く）。行は players の投影のものなので消さず、
// 自分の項目だけを外す（未開始に戻す）
func (w *PlayerStarterGrantWriter) Delete(ctx context.Context, grant readmodel.PlayerStarterGrant) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	publicID, err := w.publicID(ctx, grant)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, `UPDATE players SET metadata = metadata - 'starter_grant' WHERE player_id = $1`, publicID)
	return err
}

func (w *PlayerStarterGrantWriter) publicID(ctx context.Context, grant readmodel.PlayerStarterGrant) (string, error) {
	ref := domain.NewEntityRef(player.Types().EntityType, grant.Player.Int64())
	publicIDs, err := w.entityRefs.PublicIDs(ctx, []domain.EntityRef{ref})
	if err != nil {
		return "", err
	}
	publicID, ok := publicIDs[ref]
	if !ok {
		return "", errors.New("repository: player of starter grant has no public id")
	}
	return publicID.String(), nil
}
