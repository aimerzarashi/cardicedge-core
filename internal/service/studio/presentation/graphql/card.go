package graphql

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/presentation/graphql/model"
)

// types の null（絞り込まない）と空のリスト（入力の誤り）は、application 層が見分けるため区別して渡す
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
	var statuses []readmodel.CardStatus
	if filter.Statuses != nil {
		statuses = make([]readmodel.CardStatus, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			converted, err := toCardStatus(status)
			if err != nil {
				return query.CardFilter{}, err
			}
			statuses = append(statuses, converted)
		}
	}
	return query.CardFilter{NameContains: filter.NameContains, Types: types, Statuses: statuses}, nil
}

// toCardType と同じく、enum の値と状態の対応を明示する
func toCardStatus(status model.CardStatus) (readmodel.CardStatus, error) {
	switch status {
	case model.CardStatusDraft:
		return readmodel.CardStatusDraft, nil
	case model.CardStatusPublished:
		return readmodel.CardStatusPublished, nil
	case model.CardStatusRetired:
		return readmodel.CardStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown card status: %s", status)
	}
}

func fromCardStatus(status readmodel.CardStatus) (model.CardStatus, error) {
	switch status {
	case readmodel.CardStatusDraft:
		return model.CardStatusDraft, nil
	case readmodel.CardStatusPublished:
		return model.CardStatusPublished, nil
	case readmodel.CardStatusRetired:
		return model.CardStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown card status: %q", status)
	}
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

// 3 種類すべてに GraphQL の型があるが、CardType は整数で、switch の漏れをコンパイラが見つけない。
// default を黙って nil にすると、種類を足したときの足し忘れが一覧から消えるだけで気づけないため、エラーにする
func toCard(card query.Card) (model.Card, error) {
	status, err := fromCardStatus(card.Status)
	if err != nil {
		return nil, err
	}
	revision := int(card.Revision)
	switch card.Type {
	case readmodel.CardTypeCreature:
		return &model.Creature{ID: card.ID, Name: card.Name, Status: status, Revision: revision, RegisteredAt: card.RegisteredAt, UpdatedAt: card.UpdatedAt}, nil
	case readmodel.CardTypeItem:
		return &model.Item{ID: card.ID, Name: card.Name, Status: status, Revision: revision, RegisteredAt: card.RegisteredAt, UpdatedAt: card.UpdatedAt}, nil
	case readmodel.CardTypeSpell:
		return &model.Spell{ID: card.ID, Name: card.Name, Status: status, Revision: revision, RegisteredAt: card.RegisteredAt, UpdatedAt: card.UpdatedAt}, nil
	default:
		return nil, fmt.Errorf("graphql: card type %d has no GraphQL type", card.Type)
	}
}
