package graphql

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/presentation/graphql/model"
)

func fromQueryGrantDeck(deck query.GrantDeck) (*model.GrantDeck, error) {
	status, err := fromGrantDeckStatus(deck.Status)
	if err != nil {
		return nil, err
	}
	counts := make([]model.DeckCardCount, 0, len(deck.Cards))
	for _, card := range deck.Cards {
		counts = append(counts, model.DeckCardCount{CardID: card.CardID, Count: card.Count})
	}
	return &model.GrantDeck{
		ID: deck.ID, Name: deck.Name, Status: status, TotalCount: deck.TotalCount, Revision: int(deck.Revision),
		RegisteredAt: deck.RegisteredAt, UpdatedAt: deck.UpdatedAt, CardCounts: counts,
	}, nil
}

func toGrantDeckConnection(connection pagination.Connection[query.GrantDeck]) (*model.GrantDeckConnection, error) {
	edges := make([]*model.GrantDeckEdge, 0, len(connection.Edges))
	for _, edge := range connection.Edges {
		node, err := fromQueryGrantDeck(edge.Node)
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.GrantDeckEdge{Cursor: edge.Cursor, Node: node})
	}
	pageInfo := &model.PageInfo{
		HasNextPage:     connection.PageInfo.HasNextPage,
		HasPreviousPage: connection.PageInfo.HasPreviousPage,
		StartCursor:     connection.PageInfo.StartCursor,
		EndCursor:       connection.PageInfo.EndCursor,
	}
	return &model.GrantDeckConnection{Edges: edges, PageInfo: pageInfo}, nil
}

// statuses の null（絞り込まない）と空のリスト（入力の誤り）は、application 層が見分けるため区別して渡す
func toGrantDeckFilter(filter *model.GrantDeckFilter) (query.GrantDeckFilter, error) {
	if filter == nil {
		return query.GrantDeckFilter{}, nil
	}
	var statuses []readmodel.GrantDeckStatus
	if filter.Statuses != nil {
		statuses = make([]readmodel.GrantDeckStatus, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			converted, err := toGrantDeckStatus(status)
			if err != nil {
				return query.GrantDeckFilter{}, err
			}
			statuses = append(statuses, converted)
		}
	}
	return query.GrantDeckFilter{NameContains: filter.NameContains, Statuses: statuses}, nil
}

// enum の値と状態の対応を明示する（toCardStatus と同じ）
func toGrantDeckStatus(status model.GrantDeckStatus) (readmodel.GrantDeckStatus, error) {
	switch status {
	case model.GrantDeckStatusDraft:
		return readmodel.GrantDeckStatusDraft, nil
	case model.GrantDeckStatusPublished:
		return readmodel.GrantDeckStatusPublished, nil
	case model.GrantDeckStatusRetired:
		return readmodel.GrantDeckStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown grant deck status: %s", status)
	}
}

func fromGrantDeckStatus(status readmodel.GrantDeckStatus) (model.GrantDeckStatus, error) {
	switch status {
	case readmodel.GrantDeckStatusDraft:
		return model.GrantDeckStatusDraft, nil
	case readmodel.GrantDeckStatusPublished:
		return model.GrantDeckStatusPublished, nil
	case readmodel.GrantDeckStatusRetired:
		return model.GrantDeckStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown grant deck status: %q", status)
	}
}
