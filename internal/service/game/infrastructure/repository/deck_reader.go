package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

type DeckReader struct{}

func NewDeckReader() *DeckReader {
	return &DeckReader{}
}

// プレイヤーのデッキを作った順に読む（同じ日時は公開 ID の順）。1 人の数に上限があるため、ページ分けしない。
// カードの名前・種類は、デッキの数によらず 1 回で写した cards から引く。写していないカードはエラー（NPC のデッキと同じ）
func (r *DeckReader) ListByPlayer(ctx context.Context, playerID uuid.UUID) ([]query.Deck, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := models.Decks(
		models.DeckWhere.PlayerID.EQ(playerID.String()),
		qm.OrderBy(models.DeckColumns.RegisteredAt+", "+models.DeckColumns.DeckID),
	).All(ctx, exec)
	if err != nil {
		return nil, err
	}
	metadatas := make([]readmodel.DeckMetadata, len(rows))
	cardIDs := []uuid.UUID{}
	for i, row := range rows {
		if err := json.Unmarshal(row.Metadata, &metadatas[i]); err != nil {
			return nil, fmt.Errorf("decks.metadata of %s: %w", row.DeckID, err)
		}
		for _, card := range metadatas[i].Cards {
			cardIDs = append(cardIDs, card.CardID)
		}
	}
	cards, err := loadCards(ctx, exec, cardIDs)
	if err != nil {
		return nil, err
	}
	decks := make([]query.Deck, len(rows))
	for i, row := range rows {
		decks[i], err = buildDeck(row.DeckID, metadatas[i], cards)
		if err != nil {
			return nil, err
		}
	}
	return decks, nil
}

// 公開 ID と持ち主の両方で絞る。持ち主が違う行は無いのと同じ ErrDeckNotFound にする（ほかのプレイヤーのデッキの存在を明かさない）
func (r *DeckReader) Get(ctx context.Context, playerID uuid.UUID, deckID uuid.UUID) (query.Deck, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.Deck{}, err
	}
	row, err := models.Decks(
		models.DeckWhere.DeckID.EQ(deckID.String()),
		models.DeckWhere.PlayerID.EQ(playerID.String()),
	).One(ctx, exec)
	if errors.Is(err, sql.ErrNoRows) {
		return query.Deck{}, query.ErrDeckNotFound
	}
	if err != nil {
		return query.Deck{}, err
	}
	var metadata readmodel.DeckMetadata
	if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
		return query.Deck{}, fmt.Errorf("decks.metadata of %s: %w", row.DeckID, err)
	}
	cardIDs := make([]uuid.UUID, len(metadata.Cards))
	for i, card := range metadata.Cards {
		cardIDs[i] = card.CardID
	}
	cards, err := loadCards(ctx, exec, cardIDs)
	if err != nil {
		return query.Deck{}, err
	}
	return buildDeck(row.DeckID, metadata, cards)
}

func buildDeck(rowID string, metadata readmodel.DeckMetadata, cards map[uuid.UUID]cardRow) (query.Deck, error) {
	counts := make([]cardCount, len(metadata.Cards))
	for i, c := range metadata.Cards {
		counts[i] = cardCount{CardID: c.CardID, Count: c.Count}
	}
	deckCards, err := sortedDeckCards(counts, cards)
	if err != nil {
		return query.Deck{}, fmt.Errorf("decks.metadata of %s: %w", rowID, err)
	}
	return query.Deck{ID: metadata.ID, Name: metadata.Name, Cards: deckCards}, nil
}
