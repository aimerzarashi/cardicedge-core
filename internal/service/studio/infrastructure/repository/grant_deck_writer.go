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
type GrantDeckWriter struct {
	entityRefs domain.EntityRefResolver
}

func NewGrantDeckWriter(entityRefs domain.EntityRefResolver) *GrantDeckWriter {
	return &GrantDeckWriter{entityRefs: entityRefs}
}

// すべての列をイベントから作った値で書く。カードは公開 ID の順に並べる（同じ状態から同じ行を作るためだけに並べる）
func (w *GrantDeckWriter) Upsert(ctx context.Context, deck readmodel.GrantDeck) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	refs := make([]domain.EntityRef, 0, len(deck.Cards))
	for card := range deck.Cards {
		refs = append(refs, domain.NewEntityRef(card.EntityType, card.ID))
	}
	registeredBy, updatedBy, publicIDs, err := operatorPublicIDs(ctx, w.entityRefs, deck.RegisteredBy, deck.UpdatedBy, refs...)
	if err != nil {
		return err
	}
	cards := make([]readmodel.DeckCard, 0, len(deck.Cards))
	for card, count := range deck.Cards {
		publicID, ok := publicIDs[domain.NewEntityRef(card.EntityType, card.ID)]
		if !ok {
			return errors.New("repository: card in grant deck has no public id")
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
	row := &models.GrantDeck{
		GrantDeckID: deck.GrantDeckID.String(),
		SortName:    deck.SortName,
		Status:      string(deck.Status),
		TotalCount:  deck.TotalCount(),
		Metadata:    raw,
	}
	return row.Upsert(ctx, exec, true,
		[]string{models.GrantDeckColumns.GrantDeckID},
		boil.Whitelist(models.GrantDeckColumns.SortName, models.GrantDeckColumns.Status, models.GrantDeckColumns.TotalCount, models.GrantDeckColumns.Metadata),
		boil.Infer(),
	)
}

// 行が無くてもエラーにしない（同じ集約を作り直すときに、何度呼ばれてもよいように）
func (w *GrantDeckWriter) Delete(ctx context.Context, deck readmodel.GrantDeck) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = models.GrantDecks(models.GrantDeckWhere.GrantDeckID.EQ(deck.GrantDeckID.String())).DeleteAll(ctx, exec)
	return err
}
