package repository

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/aarondl/sqlboiler/v4/boil"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 書くときにカードの内部 ID を公開 ID に引き直すため、引き方を持つ（PresetDeckWriter と同じ）
type StarterDeckWriter struct {
	entityRefs domain.EntityRefResolver
}

func NewStarterDeckWriter(entityRefs domain.EntityRefResolver) *StarterDeckWriter {
	return &StarterDeckWriter{entityRefs: entityRefs}
}

// すべての列をイベントから作った値で書く。カードは公開 ID の順に並べる（同じ状態から同じ行を作るためだけに並べる）
func (w *StarterDeckWriter) Upsert(ctx context.Context, deck readmodel.StarterDeck) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	refs := make([]domain.EntityRef, 0, len(deck.Cards))
	for card := range deck.Cards {
		refs = append(refs, domain.NewEntityRef(card.EntityType(), card.ID()))
	}
	registeredBy, updatedBy, publicIDs, err := operatorPublicIDs(ctx, w.entityRefs, deck.RegisteredBy, deck.UpdatedBy, refs...)
	if err != nil {
		return err
	}
	cards := make([]readmodel.DeckCard, 0, len(deck.Cards))
	for card, count := range deck.Cards {
		publicID, ok := publicIDs[domain.NewEntityRef(card.EntityType(), card.ID())]
		if !ok {
			return errors.New("repository: card in starter deck has no public id")
		}
		cards = append(cards, readmodel.DeckCard{CardID: publicID, Count: count})
	}
	slices.SortFunc(cards, func(a, b readmodel.DeckCard) int {
		return strings.Compare(a.CardID.String(), b.CardID.String())
	})
	metadata := deck.Metadata
	metadata.Cards = cards
	metadata.RegisteredBy, metadata.UpdatedBy = registeredBy, updatedBy
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	row := &models.StarterDeck{
		StarterDeckID: deck.StarterDeckID.String(),
		SortName:      deck.SortName,
		Status:        string(deck.Status),
		TotalCount:    deck.TotalCount(),
		Metadata:      raw,
	}
	return row.Upsert(ctx, exec, true,
		[]string{models.StarterDeckColumns.StarterDeckID},
		boil.Whitelist(models.StarterDeckColumns.SortName, models.StarterDeckColumns.Status, models.StarterDeckColumns.TotalCount, models.StarterDeckColumns.Metadata),
		boil.Infer(),
	)
}

// 行が無くてもエラーにしない（同じ集約を作り直すときに、何度呼ばれてもよいように）
func (w *StarterDeckWriter) Delete(ctx context.Context, deck readmodel.StarterDeck) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = models.StarterDecks(models.StarterDeckWhere.StarterDeckID.EQ(deck.StarterDeckID.String())).DeleteAll(ctx, exec)
	return err
}
