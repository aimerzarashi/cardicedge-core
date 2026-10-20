package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query/readmodel"
)

// 行はゲーム全体で 1 行（未設定なら無い）なので、条件を付けずに 1 行読む
func GetStarterDeckSet(ctx context.Context) (query.StarterDeckSet, bool, error) {
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

// 公開 ID で引く。無い（削除した）プリセットデッキは結果に含めない
func GetPresetDeckRevisions(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]int64, error) {
	revisions := make(map[uuid.UUID]int64, len(ids))
	if len(ids) == 0 {
		return revisions, nil
	}
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		keys = append(keys, id.String())
	}
	rows, err := models.PresetDeckLists(models.PresetDeckListWhere.PresetDeckID.IN(keys)).All(ctx, exec)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		var metadata readmodel.PresetDeckListMetadata
		if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
			return nil, fmt.Errorf("preset_deck_lists.metadata of %s: %w", row.PresetDeckID, err)
		}
		revisions[metadata.ID] = metadata.Revision
	}
	return revisions, nil
}
