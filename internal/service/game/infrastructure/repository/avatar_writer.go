package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/aarondl/sqlboiler/v4/boil"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

// 書くときに持ち主の内部 ID を公開用の ID に引き直すため、引き方を持つ（PlayerStarterGrantWriter と同じ）
type AvatarWriter struct {
	entityRefs domain.EntityRefResolver
}

func NewAvatarWriter(entityRefs domain.EntityRefResolver) *AvatarWriter {
	return &AvatarWriter{entityRefs: entityRefs}
}

func (w *AvatarWriter) Upsert(ctx context.Context, avatar readmodel.Avatar) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	ref := domain.NewEntityRef(player.Types().EntityType, avatar.Player.Int64())
	publicIDs, err := w.entityRefs.PublicIDs(ctx, []domain.EntityRef{ref})
	if err != nil {
		return err
	}
	playerPublicID, ok := publicIDs[ref]
	if !ok {
		return errors.New("repository: player of avatar has no public id")
	}
	metadata := avatar.Metadata()
	metadata.RegisteredBy, metadata.UpdatedBy = playerPublicID, playerPublicID
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	row := &models.Avatar{
		AvatarID:     avatar.AvatarID.String(),
		PlayerID:     playerPublicID.String(),
		RegisteredAt: avatar.RegisteredAt,
		Metadata:     raw,
	}
	return row.Upsert(ctx, exec, true,
		[]string{models.AvatarColumns.AvatarID},
		boil.Whitelist(models.AvatarColumns.PlayerID, models.AvatarColumns.RegisteredAt, models.AvatarColumns.Metadata),
		boil.Infer(),
	)
}

// アバターを消すイベントは無いため呼ばれない（投影の宣言に要るので置く）。行が無くてもエラーにしない
func (w *AvatarWriter) Delete(ctx context.Context, avatar readmodel.Avatar) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = models.Avatars(models.AvatarWhere.AvatarID.EQ(avatar.AvatarID.String())).DeleteAll(ctx, exec)
	return err
}
