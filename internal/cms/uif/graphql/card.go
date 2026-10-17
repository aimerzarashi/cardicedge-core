package graphql

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/uif/graphql/model"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
)

// types の null（絞り込まない）と空のリスト（入力の誤り）は、app 層が見分けるため区別して渡す
func toCardFilter(filter *model.CardFilter) (query.CardFilter, error) {
	if filter == nil {
		return query.CardFilter{}, nil
	}
	var types []readmodel.CardType
	if filter.Types != nil {
		types = make([]readmodel.CardType, 0, len(filter.Types))
		for _, cardType := range filter.Types {
			converted, err := toCardType(cardType)
			if err != nil {
				return query.CardFilter{}, err
			}
			types = append(types, converted)
		}
	}
	return query.CardFilter{NameContains: filter.NameContains, Types: types}, nil
}

// enum の値と種類の対応を明示する（文字列から型を変換すると、対応が名前の一致に隠れるため）
func toCardType(cardType model.CardType) (readmodel.CardType, error) {
	switch cardType {
	case model.CardTypeCreature:
		return readmodel.CardTypeCreature, nil
	case model.CardTypeItem:
		return readmodel.CardTypeItem, nil
	case model.CardTypeSpell:
		return readmodel.CardTypeSpell, nil
	default:
		return 0, fmt.Errorf("graphql: unknown card type: %s", cardType)
	}
}

func toCardConnection(connection pagination.Connection[query.Card]) (*model.CardConnection, error) {
	edges := make([]*model.CardEdge, 0, len(connection.Edges))
	for _, edge := range connection.Edges {
		node, err := toCard(edge.Node)
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.CardEdge{Cursor: edge.Cursor, Node: node})
	}
	pageInfo := &model.PageInfo{
		HasNextPage:     connection.PageInfo.HasNextPage,
		HasPreviousPage: connection.PageInfo.HasPreviousPage,
		StartCursor:     connection.PageInfo.StartCursor,
		EndCursor:       connection.PageInfo.EndCursor,
	}
	return &model.CardConnection{Edges: edges, PageInfo: pageInfo}, nil
}

// Item・Spell は GraphQL の型をまだ作っていない（read model にも入らない）。来たら黙って飛ばさずエラーにする
// （飛ばすと、一覧から消えたことに気づけない）
func toCard(card query.Card) (model.Card, error) {
	switch card.Type {
	case readmodel.CardTypeCreature:
		return &model.Creature{ID: card.ID, Name: card.Name, RegisteredAt: card.RegisteredAt, UpdatedAt: card.UpdatedAt}, nil
	default:
		return nil, fmt.Errorf("graphql: card type %d has no GraphQL type", card.Type)
	}
}
