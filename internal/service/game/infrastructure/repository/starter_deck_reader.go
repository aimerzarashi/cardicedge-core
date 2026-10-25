package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
)

// core_game の starter_decks（make publish-master が studio の read model から全行を状態つきで写した表）のうち、公開中だけを読む。
// 準備中・使用不可も写してあるのは、配り済みのデッキの元を後で表示したくなったとき、表を作り直さずに済むようにするため
type StarterDeckReader struct{}

func NewStarterDeckReader() *StarterDeckReader {
	return &StarterDeckReader{}
}

// 写した表の metadata のうち、配布に要る項目。studio の read model の型を import せず、この JSON の形だけを契約にする
// （公開した版と game の版が一致しなくても、読む項目が変わらない限り読める）
type starterDeckMetadata struct {
	Name  string `json:"name"`
	Cards []struct {
		CardID uuid.UUID `json:"card_id"`
		Count  int       `json:"count"`
	} `json:"cards"`
}

// 並びは索引（sort_name, starter_deck_id）と同じ。sort_name は COLLATE "C" のため、OS の照合順序に左右されない
func (r *StarterDeckReader) ListPublished(ctx context.Context) ([]master.StarterDeck, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := models.StarterDecks(
		models.StarterDeckWhere.Status.EQ("published"),
		qm.OrderBy(models.StarterDeckColumns.SortName+", "+models.StarterDeckColumns.StarterDeckID),
	).All(ctx, exec)
	if err != nil {
		return nil, err
	}
	decks := make([]master.StarterDeck, 0, len(rows))
	for _, row := range rows {
		var metadata starterDeckMetadata
		if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
			return nil, fmt.Errorf("starter_decks.metadata of %s: %w", row.StarterDeckID, err)
		}
		cards := make([]master.DeckCard, 0, len(metadata.Cards))
		for _, c := range metadata.Cards {
			cards = append(cards, master.DeckCard{CardID: c.CardID, Count: c.Count})
		}
		decks = append(decks, master.StarterDeck{Name: metadata.Name, Cards: cards})
	}
	return decks, nil
}
