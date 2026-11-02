package graphql

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/presentation/graphql/model"
)

func fromQueryNPCAvatar(avatar query.NPCAvatar) (*model.NpcAvatar, error) {
	decks := make([]*model.NpcDeck, 0, len(avatar.Decks))
	for _, deck := range avatar.Decks {
		cards := make([]*model.NpcDeckCard, 0, len(deck.Cards))
		for _, card := range deck.Cards {
			cardType, err := fromCardType(card.Type)
			if err != nil {
				return nil, err
			}
			cards = append(cards, &model.NpcDeckCard{
				Card:  &model.NpcDeckCardInfo{ID: card.CardID, Name: card.Name, Type: cardType},
				Count: card.Count,
			})
		}
		decks = append(decks, &model.NpcDeck{ID: deck.ID, Name: deck.Name, Cards: cards, TotalCount: deck.TotalCount()})
	}
	return &model.NpcAvatar{ID: avatar.ID, Name: avatar.Name, Decks: decks}, nil
}

func toNPCAvatarsConnection(connection pagination.Connection[query.NPCAvatar]) (*model.NpcAvatarsConnection, error) {
	edges := make([]*model.NpcAvatarsEdge, 0, len(connection.Edges))
	for _, edge := range connection.Edges {
		node, err := fromQueryNPCAvatar(edge.Node)
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.NpcAvatarsEdge{Cursor: edge.Cursor, Node: node})
	}
	pageInfo := &model.PageInfo{
		HasNextPage:     connection.PageInfo.HasNextPage,
		HasPreviousPage: connection.PageInfo.HasPreviousPage,
		StartCursor:     connection.PageInfo.StartCursor,
		EndCursor:       connection.PageInfo.EndCursor,
	}
	return &model.NpcAvatarsConnection{Edges: edges, PageInfo: pageInfo}, nil
}

// 種類の対応を明示する（種類が増えて写し忘れたら、黙って落とさずエラーにする）
func fromCardType(cardType query.CardType) (model.CardType, error) {
	switch cardType {
	case query.CardTypeCreature:
		return model.CardTypeCreature, nil
	case query.CardTypeItem:
		return model.CardTypeItem, nil
	case query.CardTypeSpell:
		return model.CardTypeSpell, nil
	default:
		return "", fmt.Errorf("graphql: unknown card type: %d", cardType)
	}
}
