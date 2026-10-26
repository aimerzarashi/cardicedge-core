package graphql

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/presentation/graphql/model"
)

func fromQueryStarterDeck(deck query.StarterDeck) (*model.StarterDeck, error) {
	status, err := fromStarterDeckStatus(deck.Status)
	if err != nil {
		return nil, err
	}
	counts := make([]model.DeckCardCount, 0, len(deck.Cards))
	for _, card := range deck.Cards {
		counts = append(counts, model.DeckCardCount{CardID: card.CardID, Count: card.Count})
	}
	var source *model.StarterDeckSource
	if deck.Source != nil {
		source = &model.StarterDeckSource{
			PresetDeckID:       deck.Source.PresetDeckID,
			Revision:           int(deck.Source.Revision),
			ChangedSinceCopied: deck.Source.ChangedSinceCopied,
			PresetDeckDeleted:  deck.Source.PresetDeckDeleted,
		}
	}
	return &model.StarterDeck{
		ID: deck.ID, Name: deck.Name, Status: status, TotalCount: deck.TotalCount, Source: source, Revision: int(deck.Revision),
		RegisteredAt: deck.RegisteredAt, UpdatedAt: deck.UpdatedAt, CardCounts: counts,
	}, nil
}

func toStarterDeckConnection(connection pagination.Connection[query.StarterDeck]) (*model.StarterDeckConnection, error) {
	edges := make([]*model.StarterDeckEdge, 0, len(connection.Edges))
	for _, edge := range connection.Edges {
		node, err := fromQueryStarterDeck(edge.Node)
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.StarterDeckEdge{Cursor: edge.Cursor, Node: node})
	}
	pageInfo := &model.PageInfo{
		HasNextPage:     connection.PageInfo.HasNextPage,
		HasPreviousPage: connection.PageInfo.HasPreviousPage,
		StartCursor:     connection.PageInfo.StartCursor,
		EndCursor:       connection.PageInfo.EndCursor,
	}
	return &model.StarterDeckConnection{Edges: edges, PageInfo: pageInfo}, nil
}

// statuses の null（絞り込まない）と空のリスト（入力の誤り）は、application 層が見分けるため区別して渡す
func toStarterDeckFilter(filter *model.StarterDeckFilter) (query.StarterDeckFilter, error) {
	if filter == nil {
		return query.StarterDeckFilter{}, nil
	}
	var statuses []readmodel.StarterDeckStatus
	if filter.Statuses != nil {
		statuses = make([]readmodel.StarterDeckStatus, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			converted, err := toStarterDeckStatus(status)
			if err != nil {
				return query.StarterDeckFilter{}, err
			}
			statuses = append(statuses, converted)
		}
	}
	return query.StarterDeckFilter{NameContains: filter.NameContains, Statuses: statuses}, nil
}

// enum の値と状態の対応を明示する（toCardStatus と同じ）
func toStarterDeckStatus(status model.StarterDeckStatus) (readmodel.StarterDeckStatus, error) {
	switch status {
	case model.StarterDeckStatusDraft:
		return readmodel.StarterDeckStatusDraft, nil
	case model.StarterDeckStatusPublished:
		return readmodel.StarterDeckStatusPublished, nil
	case model.StarterDeckStatusRetired:
		return readmodel.StarterDeckStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown starter deck status: %s", status)
	}
}

func fromStarterDeckStatus(status readmodel.StarterDeckStatus) (model.StarterDeckStatus, error) {
	switch status {
	case readmodel.StarterDeckStatusDraft:
		return model.StarterDeckStatusDraft, nil
	case readmodel.StarterDeckStatusPublished:
		return model.StarterDeckStatusPublished, nil
	case readmodel.StarterDeckStatusRetired:
		return model.StarterDeckStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown starter deck status: %q", status)
	}
}
