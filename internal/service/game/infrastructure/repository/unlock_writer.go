package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/unlock"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

// 書くときに解放と持ち主の内部 ID を公開用の ID に引き直すため、引き方を持つ（AvatarWriter と同じ）
type UnlockWriter struct {
	entityRefs domain.EntityRefResolver
}

func NewUnlockWriter(entityRefs domain.EntityRefResolver) *UnlockWriter {
	return &UnlockWriter{entityRefs: entityRefs}
}

func (w *UnlockWriter) Upsert(ctx context.Context, state readmodel.Unlock) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	unlockRef := domain.NewEntityRef(unlock.Types().EntityType, state.Unlock.Int64())
	playerRef := domain.NewEntityRef(player.Types().EntityType, state.Player.Int64())
	refs := []domain.EntityRef{unlockRef, playerRef}
	for _, id := range state.Boards {
		refs = append(refs, domain.NewEntityRef(board.Types().EntityType, id.Int64()))
	}
	for _, id := range state.NPCAvatars {
		refs = append(refs, domain.NewEntityRef(npcavatar.Types().EntityType, id.Int64()))
	}
	publicIDs, err := w.entityRefs.PublicIDs(ctx, refs)
	if err != nil {
		return err
	}
	unlockPublicID, ok := publicIDs[unlockRef]
	if !ok {
		return errors.New("repository: unlock has no public id")
	}
	playerPublicID, ok := publicIDs[playerRef]
	if !ok {
		return errors.New("repository: player of unlock has no public id")
	}
	metadata := state.Metadata()
	metadata.ID = unlockPublicID
	// read model は公開 ID だけを持つ。game の内部 ID は、publish-master が domain_entities に作った行から引き直す
	metadata.NPCAvatars = make([]uuid.UUID, 0, len(state.NPCAvatars))
	for _, id := range state.NPCAvatars {
		publicID, ok := publicIDs[domain.NewEntityRef(npcavatar.Types().EntityType, id.Int64())]
		if !ok {
			return errors.New("repository: npc avatar of unlock has no public id")
		}
		metadata.NPCAvatars = append(metadata.NPCAvatars, publicID)
	}
	metadata.Boards = make([]uuid.UUID, 0, len(state.Boards))
	for _, id := range state.Boards {
		publicID, ok := publicIDs[domain.NewEntityRef(board.Types().EntityType, id.Int64())]
		if !ok {
			return errors.New("repository: board of unlock has no public id")
		}
		metadata.Boards = append(metadata.Boards, publicID)
	}
	metadata.RegisteredBy, metadata.UpdatedBy = playerPublicID, playerPublicID
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	row := &models.Unlock{
		UnlockID:     unlockPublicID.String(),
		PlayerID:     playerPublicID.String(),
		RegisteredAt: state.RegisteredAt,
		Metadata:     raw,
	}
	return row.Upsert(ctx, exec, true,
		[]string{models.UnlockColumns.UnlockID},
		boil.Whitelist(models.UnlockColumns.PlayerID, models.UnlockColumns.RegisteredAt, models.UnlockColumns.Metadata),
		boil.Infer(),
	)
}

// デッキを消すイベントは無いため呼ばれない（投影の宣言に要るので置く）。行が無くてもエラーにしない
func (w *UnlockWriter) Delete(ctx context.Context, state readmodel.Unlock) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	ref := domain.NewEntityRef(unlock.Types().EntityType, state.Unlock.Int64())
	publicIDs, err := w.entityRefs.PublicIDs(ctx, []domain.EntityRef{ref})
	if err != nil {
		return err
	}
	publicID, ok := publicIDs[ref]
	if !ok {
		return nil
	}
	_, err = models.Unlocks(models.UnlockWhere.UnlockID.EQ(publicID.String())).DeleteAll(ctx, exec)
	return err
}
