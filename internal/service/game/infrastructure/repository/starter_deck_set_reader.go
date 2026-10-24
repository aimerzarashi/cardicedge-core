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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
)

// core_game の starter_deck_sets（make publish-master が studio の read model から写した表）を読む
type StarterDeckSetReader struct{}

func NewStarterDeckSetReader() *StarterDeckSetReader {
	return &StarterDeckSetReader{}
}

// 写した表の metadata のうち、配布に要る項目。studio の read model の型を import せず、この JSON の形だけを契約にする
// （公開した版と game の版が一致しなくても、読む項目が変わらない限り読める）
type starterDeckSetMetadata struct {
	Decks []struct {
		Name  string `json:"name"`
		Cards []struct {
			CardID uuid.UUID `json:"card_id"`
			Count  int       `json:"count"`
		} `json:"cards"`
	} `json:"decks"`
}

// 行はゲーム全体で 1 行（公開されていなければ無い）なので、条件を付けずに 1 行読む
func (r *StarterDeckSetReader) Get(ctx context.Context) (master.StarterDeckSet, bool, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return master.StarterDeckSet{}, false, err
	}
	row, err := models.StarterDeckSets().One(ctx, exec)
	if errors.Is(err, sql.ErrNoRows) {
		return master.StarterDeckSet{}, false, nil
	}
	if err != nil {
		return master.StarterDeckSet{}, false, err
	}
	var metadata starterDeckSetMetadata
	if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
		return master.StarterDeckSet{}, false, fmt.Errorf("starter_deck_sets.metadata of %s: %w", row.StarterDeckSetID, err)
	}
	decks := make([]master.StarterDeck, 0, len(metadata.Decks))
	for _, deck := range metadata.Decks {
		cards := make([]master.DeckCard, 0, len(deck.Cards))
		for _, c := range deck.Cards {
			cards = append(cards, master.DeckCard{CardID: c.CardID, Count: c.Count})
		}
		decks = append(decks, master.StarterDeck{Name: deck.Name, Cards: cards})
	}
	return master.StarterDeckSet{Decks: decks}, true, nil
}
