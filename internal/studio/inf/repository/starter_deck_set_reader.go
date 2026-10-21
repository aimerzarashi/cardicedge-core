package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query/readmodel"
)

// starter_deck_sets を読む
type StarterDeckSetReader struct{}

func NewStarterDeckSetReader() *StarterDeckSetReader {
	return &StarterDeckSetReader{}
}

// 行はゲーム全体で 1 行（未設定なら無い）なので、条件を付けずに 1 行読む
func (r *StarterDeckSetReader) Get(ctx context.Context) (query.StarterDeckSet, bool, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.StarterDeckSet{}, false, err
	}
	row, err := models.StarterDeckSets().One(ctx, exec)
	if errors.Is(err, sql.ErrNoRows) {
		return query.StarterDeckSet{}, false, nil
	}
	if err != nil {
		return query.StarterDeckSet{}, false, err
	}
	var metadata readmodel.StarterDeckSetMetadata
	if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
		return query.StarterDeckSet{}, false, fmt.Errorf("starter_deck_sets.metadata of %s: %w", row.StarterDeckSetID, err)
	}
	decks := make([]query.StarterDeck, 0, len(metadata.Decks))
	for _, deck := range metadata.Decks {
		cards := make([]query.PresetDeckCard, 0, len(deck.Cards))
		for _, card := range deck.Cards {
			cards = append(cards, query.PresetDeckCard{CardID: card.CardID, Count: card.Count})
		}
		source := query.StarterDeckSource{PresetDeckID: deck.Source.PresetDeckID, Revision: deck.Source.Revision}
		decks = append(decks, query.StarterDeck{Name: deck.Name, Cards: cards, Source: source})
	}
	updatedAt := metadata.UpdatedAt
	return query.StarterDeckSet{Decks: decks, Revision: metadata.Revision, UpdatedAt: &updatedAt}, true, nil
}
