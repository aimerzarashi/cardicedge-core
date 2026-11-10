package graphql

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/presentation/graphql/model"
)

func fromQueryNPCAvatar(avatar query.NPCAvatar) (*model.NpcAvatar, error) {
	decks := make([]*model.NpcDeck, 0, len(avatar.Decks))
	for _, deck := range avatar.Decks {
		cards := make([]*model.NpcDeckCard, 0, len(deck.Cards))
		for _, card := range deck.Cards {
			info, err := fromCard(card.Type, card.CardID, card.Name, card.Cost, card.Rarity, card.Creature, card.Item, card.Spell)
			if err != nil {
				return nil, err
			}
			cards = append(cards, &model.NpcDeckCard{Card: info, Count: card.Count})
		}
		decks = append(decks, &model.NpcDeck{ID: deck.ID, Name: deck.Name, Cards: cards, TotalCount: deck.TotalCount()})
	}
	return &model.NpcAvatar{ID: avatar.ID, Name: avatar.Name, Decks: decks}, nil
}
