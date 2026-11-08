package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/aarondl/sqlboiler/v4/boil"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

// 書くときに所持品と持ち主とカードの内部 ID を公開用の ID に引き直すため、引き方を持つ（DeckWriter と同じ）
type CollectionWriter struct {
	entityRefs domain.EntityRefResolver
}

func NewCollectionWriter(entityRefs domain.EntityRefResolver) *CollectionWriter {
	return &CollectionWriter{entityRefs: entityRefs}
}

func (w *CollectionWriter) Upsert(ctx context.Context, state readmodel.Collection) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	collectionRef := domain.NewEntityRef(collection.Types().EntityType, state.Collection.Int64())
	playerRef := domain.NewEntityRef(player.Types().EntityType, state.Player.Int64())
	refs := []domain.EntityRef{collectionRef, playerRef}
	for _, c := range state.Cards {
		refs = append(refs, domain.NewEntityRef(c.Card.EntityType, c.Card.ID))
	}
	publicIDs, err := w.entityRefs.PublicIDs(ctx, refs)
	if err != nil {
		return err
	}
	collectionPublicID, ok := publicIDs[collectionRef]
	if !ok {
		return errors.New("repository: collection has no public id")
	}
	playerPublicID, ok := publicIDs[playerRef]
	if !ok {
		return errors.New("repository: player of collection has no public id")
	}
	metadata := state.Metadata()
	metadata.Cards = make([]readmodel.CollectionCardMetadata, 0, len(state.Cards))
	for _, c := range state.Cards {
		cardPublicID, ok := publicIDs[domain.NewEntityRef(c.Card.EntityType, c.Card.ID)]
		if !ok {
			return errors.New("repository: card of collection has no public id")
		}
		metadata.Cards = append(metadata.Cards, readmodel.CollectionCardMetadata{CardID: cardPublicID, Count: c.Count})
	}
	metadata.ID = collectionPublicID
	metadata.RegisteredBy, metadata.UpdatedBy = playerPublicID, playerPublicID
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	row := &models.Collection{
		CollectionID: collectionPublicID.String(),
		PlayerID:     playerPublicID.String(),
		Metadata:     raw,
	}
	return row.Upsert(ctx, exec, true,
		[]string{models.CollectionColumns.CollectionID},
		boil.Whitelist(models.CollectionColumns.PlayerID, models.CollectionColumns.Metadata),
		boil.Infer(),
	)
}

// 所持品を消すイベントは無いため呼ばれない（投影の宣言に要るので置く）。行が無くてもエラーにしない
func (w *CollectionWriter) Delete(ctx context.Context, state readmodel.Collection) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	ref := domain.NewEntityRef(collection.Types().EntityType, state.Collection.Int64())
	publicIDs, err := w.entityRefs.PublicIDs(ctx, []domain.EntityRef{ref})
	if err != nil {
		return err
	}
	publicID, ok := publicIDs[ref]
	if !ok {
		return nil
	}
	_, err = models.Collections(models.CollectionWhere.CollectionID.EQ(publicID.String())).DeleteAll(ctx, exec)
	return err
}
