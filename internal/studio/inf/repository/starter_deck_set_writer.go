package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/aarondl/sqlboiler/v4/boil"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query/readmodel"
)

// 書くときにカードの内部 ID を公開 ID に引き直すため、引き方を持つ（プリセットデッキと同じ）
type StarterDeckSetWriter struct {
	entityRefs domain.EntityRefResolver
}

func NewStarterDeckSetWriter(entityRefs domain.EntityRefResolver) *StarterDeckSetWriter {
	return &StarterDeckSetWriter{entityRefs: entityRefs}
}

// すべての列をイベントから作った値で書く。カードの並びは写しの並び（種類・内部 ID の順）のまま
func (w *StarterDeckSetWriter) Upsert(ctx context.Context, set readmodel.StarterDeckSet) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	var refs []domain.EntityRef
	for _, deck := range set.Decks {
		for _, card := range deck.Cards() {
			refs = append(refs, domain.NewEntityRef(card.Card().EntityType(), card.Card().ID()))
		}
	}
	publicIDs, err := w.entityRefs.PublicIDs(ctx, refs)
	if err != nil {
		return err
	}
	decks := make([]readmodel.StarterDeckSetDeck, 0, len(set.Decks))
	for _, deck := range set.Decks {
		cards := make([]readmodel.PresetDeckListCard, 0, len(deck.Cards()))
		for _, card := range deck.Cards() {
			publicID, ok := publicIDs[domain.NewEntityRef(card.Card().EntityType(), card.Card().ID())]
			if !ok {
				return errors.New("repository: card in starter deck has no public id")
			}
			cards = append(cards, readmodel.PresetDeckListCard{CardID: publicID, Count: card.Count().Int()})
		}
		source := readmodel.StarterDeckSetSource{PresetDeckID: deck.Source().PresetDeckID(), Revision: deck.Source().Revision()}
		decks = append(decks, readmodel.StarterDeckSetDeck{Name: deck.Name().String(), Cards: cards, Source: source})
	}
	metadata := readmodel.StarterDeckSetMetadata{ID: set.StarterDeckSetID, Revision: set.Revision, UpdatedAt: set.UpdatedAt, Decks: decks}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	row := &models.StarterDeckSet{StarterDeckSetID: set.StarterDeckSetID.String(), Metadata: raw}
	return row.Upsert(ctx, exec, true,
		[]string{models.StarterDeckSetColumns.StarterDeckSetID},
		boil.Whitelist(models.StarterDeckSetColumns.Metadata),
		boil.Infer(),
	)
}

// 削除のイベントは無いため呼ばれない（投影の宣言に要るので置く）
func (w *StarterDeckSetWriter) Delete(ctx context.Context, set readmodel.StarterDeckSet) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = models.StarterDeckSets(models.StarterDeckSetWhere.StarterDeckSetID.EQ(set.StarterDeckSetID.String())).DeleteAll(ctx, exec)
	return err
}
