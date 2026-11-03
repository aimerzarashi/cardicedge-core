package graphql

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/presentation/graphql/model"
)

func fromQueryDeck(deck query.Deck) (*model.Deck, error) {
	cards := make([]*model.DeckCard, 0, len(deck.Cards))
	for _, card := range deck.Cards {
		cardType, err := fromCardType(card.Type)
		if err != nil {
			return nil, err
		}
		cards = append(cards, &model.DeckCard{
			Card:  &model.DeckCardInfo{ID: card.CardID, Name: card.Name, Type: cardType},
			Count: card.Count,
		})
	}
	counts := deck.TypeCounts()
	typeCounts := &model.DeckTypeCounts{Creature: counts.Creature, Item: counts.Item, Spell: counts.Spell}
	return &model.Deck{ID: deck.ID, Name: deck.Name, Cards: cards, TotalCount: deck.TotalCount(), TypeCounts: typeCounts}, nil
}
