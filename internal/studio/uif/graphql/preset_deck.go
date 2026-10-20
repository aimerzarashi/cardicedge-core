package graphql

import (
	"context"
	"slices"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/uif/graphql/model"
)

func fromQueryPresetDeck(deck query.PresetDeck) *model.PresetDeck {
	counts := make([]model.DeckCardCount, 0, len(deck.Cards))
	for _, card := range deck.Cards {
		counts = append(counts, model.DeckCardCount{CardID: card.CardID, Count: card.Count})
	}
	return &model.PresetDeck{
		ID: deck.ID, Name: deck.Name, TotalCount: deck.TotalCount, Revision: int(deck.Revision),
		RegisteredAt: deck.RegisteredAt, UpdatedAt: deck.UpdatedAt, CardCounts: counts,
	}
}

func toPresetDeckConnection(connection pagination.Connection[query.PresetDeck]) *model.PresetDeckConnection {
	edges := make([]*model.PresetDeckEdge, 0, len(connection.Edges))
	for _, edge := range connection.Edges {
		edges = append(edges, &model.PresetDeckEdge{Cursor: edge.Cursor, Node: fromQueryPresetDeck(edge.Node)})
	}
	pageInfo := &model.PageInfo{
		HasNextPage:     connection.PageInfo.HasNextPage,
		HasPreviousPage: connection.PageInfo.HasPreviousPage,
		StartCursor:     connection.PageInfo.StartCursor,
		EndCursor:       connection.PageInfo.EndCursor,
	}
	return &model.PresetDeckConnection{Edges: edges, PageInfo: pageInfo}
}

func toCardCountInputs(cards []*model.PresetDeckCardCountInput) []command.CardCountInput {
	inputs := make([]command.CardCountInput, 0, len(cards))
	for _, card := range cards {
		inputs = append(inputs, command.CardCountInput{CardID: card.CardID, Count: card.Count})
	}
	return inputs
}

// カードはローダーでまとめて引いてから、カードの一覧と同じ並びにする（並べるにはカード名・種類が要り、
// read model はそれを持たないため。core#24）。一覧の各プリセットデッキからの呼び出しは、ローダーが 1 回の読み出しに束ねる
func loadDeckCards(ctx context.Context, counts []model.DeckCardCount) ([]*model.DeckCard, error) {
	ids := make([]uuid.UUID, 0, len(counts))
	for _, count := range counts {
		ids = append(ids, count.CardID)
	}
	cards, err := cardLoader(ctx).LoadAll(ctx, ids)
	if err != nil {
		return nil, err
	}
	type loaded struct {
		card  query.Card
		count int
	}
	loadedCards := make([]loaded, 0, len(cards))
	for i, card := range cards {
		loadedCards = append(loadedCards, loaded{card: card, count: counts[i].Count})
	}
	slices.SortFunc(loadedCards, func(a, b loaded) int {
		return query.CompareCards(a.card, b.card)
	})
	result := make([]*model.DeckCard, 0, len(loadedCards))
	for _, l := range loadedCards {
		card, err := toCard(l.card)
		if err != nil {
			return nil, err
		}
		result = append(result, &model.DeckCard{Card: card, Count: l.count})
	}
	return result, nil
}
