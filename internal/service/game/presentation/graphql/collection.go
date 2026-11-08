package graphql

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/presentation/graphql/model"
)

// 指定が無い（null）と空のリストを区別して渡す（空のリストは application 層が入力の誤りにする）
func toCollectionFilter(filter *model.CollectionFilter) (query.CollectionFilter, error) {
	if filter == nil || filter.Types == nil {
		return query.CollectionFilter{}, nil
	}
	types := make([]query.CardType, 0, len(filter.Types))
	for _, cardType := range filter.Types {
		converted, err := toCardType(cardType)
		if err != nil {
			return query.CollectionFilter{}, err
		}
		types = append(types, converted)
	}
	return query.CollectionFilter{Types: types}, nil
}

// 値の対応を明示する（enum を足して写し忘れたら、黙って別の種類にせずエラーにする）
func toCardType(cardType model.CardType) (query.CardType, error) {
	switch cardType {
	case model.CardTypeCreature:
		return query.CardTypeCreature, nil
	case model.CardTypeItem:
		return query.CardTypeItem, nil
	case model.CardTypeSpell:
		return query.CardTypeSpell, nil
	default:
		return 0, fmt.Errorf("graphql: unknown card type: %q", cardType)
	}
}

func toCollectionCardConnection(connection pagination.Connection[query.CollectionCard]) (*model.CollectionCardConnection, error) {
	edges := make([]*model.CollectionCardEdge, 0, len(connection.Edges))
	for _, edge := range connection.Edges {
		card := edge.Node.Card
		info, err := fromCard(card.Type, card.ID, card.Name, card.Cost, card.Rarity, card.Creature)
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.CollectionCardEdge{Cursor: edge.Cursor, Node: &model.CollectionCard{Card: info, Count: edge.Node.Count}})
	}
	pageInfo := &model.PageInfo{
		HasNextPage:     connection.PageInfo.HasNextPage,
		HasPreviousPage: connection.PageInfo.HasPreviousPage,
		StartCursor:     connection.PageInfo.StartCursor,
		EndCursor:       connection.PageInfo.EndCursor,
	}
	return &model.CollectionCardConnection{Edges: edges, PageInfo: pageInfo}, nil
}
