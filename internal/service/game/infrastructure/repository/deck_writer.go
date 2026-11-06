package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/aarondl/sqlboiler/v4/boil"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

// 書くときにデッキと持ち主の内部 ID を公開用の ID に引き直すため、引き方を持つ（AvatarWriter と同じ）
type DeckWriter struct {
	entityRefs domain.EntityRefResolver
}

func NewDeckWriter(entityRefs domain.EntityRefResolver) *DeckWriter {
	return &DeckWriter{entityRefs: entityRefs}
}

func (w *DeckWriter) Upsert(ctx context.Context, state readmodel.Deck) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	deckRef := domain.NewEntityRef(deck.Types().EntityType, state.Deck.Int64())
	playerRef := domain.NewEntityRef(player.Types().EntityType, state.Player.Int64())
	refs := []domain.EntityRef{deckRef, playerRef}
	for _, c := range state.Cards {
		refs = append(refs, domain.NewEntityRef(c.Card.EntityType, c.Card.ID))
	}
	publicIDs, err := w.entityRefs.PublicIDs(ctx, refs)
	if err != nil {
		return err
	}
	deckPublicID, ok := publicIDs[deckRef]
	if !ok {
		return errors.New("repository: deck has no public id")
	}
	playerPublicID, ok := publicIDs[playerRef]
	if !ok {
		return errors.New("repository: player of deck has no public id")
	}
	metadata := state.Metadata()
	metadata.Cards = make([]readmodel.DeckCard, 0, len(state.Cards))
	for _, c := range state.Cards {
		cardPublicID, ok := publicIDs[domain.NewEntityRef(c.Card.EntityType, c.Card.ID)]
		if !ok {
			return errors.New("repository: card of deck has no public id")
		}
		metadata.Cards = append(metadata.Cards, readmodel.DeckCard{CardID: cardPublicID, Count: c.Quantity.Int()})
	}
	metadata.ID = deckPublicID
	metadata.RegisteredBy, metadata.UpdatedBy = playerPublicID, playerPublicID
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	row := &models.Deck{
		DeckID:       deckPublicID.String(),
		PlayerID:     playerPublicID.String(),
		RegisteredAt: state.RegisteredAt,
		Metadata:     raw,
	}
	return row.Upsert(ctx, exec, true,
		[]string{models.DeckColumns.DeckID},
		boil.Whitelist(models.DeckColumns.PlayerID, models.DeckColumns.RegisteredAt, models.DeckColumns.Metadata),
		boil.Infer(),
	)
}

// デッキを消すイベントは無いため呼ばれない（投影の宣言に要るので置く）。行が無くてもエラーにしない
func (w *DeckWriter) Delete(ctx context.Context, state readmodel.Deck) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	ref := domain.NewEntityRef(deck.Types().EntityType, state.Deck.Int64())
	publicIDs, err := w.entityRefs.PublicIDs(ctx, []domain.EntityRef{ref})
	if err != nil {
		return err
	}
	publicID, ok := publicIDs[ref]
	if !ok {
		return nil
	}
	_, err = models.Decks(models.DeckWhere.DeckID.EQ(publicID.String())).DeleteAll(ctx, exec)
	return err
}
