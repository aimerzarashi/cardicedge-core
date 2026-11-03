package graphql

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/presentation/graphql/model"
)

func fromQueryStarterSet(starterSet query.StarterSet) (*model.StarterSet, error) {
	status, err := fromStarterSetStatus(starterSet.Status)
	if err != nil {
		return nil, err
	}
	return &model.StarterSet{
		ID: starterSet.ID, Name: starterSet.Name, Status: status, Revision: int(starterSet.Revision),
		RegisteredAt: starterSet.RegisteredAt, UpdatedAt: starterSet.UpdatedAt, GrantDeckIDs: starterSet.GrantDecks, UnlockBoardIDs: starterSet.UnlockBoards, UnlockNPCAvatarIDs: starterSet.UnlockNPCAvatars,
	}, nil
}

func toStarterSetConnection(connection pagination.Connection[query.StarterSet]) (*model.StarterSetConnection, error) {
	edges := make([]*model.StarterSetEdge, 0, len(connection.Edges))
	for _, edge := range connection.Edges {
		node, err := fromQueryStarterSet(edge.Node)
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.StarterSetEdge{Cursor: edge.Cursor, Node: node})
	}
	pageInfo := &model.PageInfo{
		HasNextPage:     connection.PageInfo.HasNextPage,
		HasPreviousPage: connection.PageInfo.HasPreviousPage,
		StartCursor:     connection.PageInfo.StartCursor,
		EndCursor:       connection.PageInfo.EndCursor,
	}
	return &model.StarterSetConnection{Edges: edges, PageInfo: pageInfo}, nil
}

// statuses の null（絞り込まない）と空のリスト（入力の誤り）は、application 層が見分けるため区別して渡す
func toStarterSetFilter(filter *model.StarterSetFilter) (query.StarterSetFilter, error) {
	if filter == nil {
		return query.StarterSetFilter{}, nil
	}
	var statuses []readmodel.StarterSetStatus
	if filter.Statuses != nil {
		statuses = make([]readmodel.StarterSetStatus, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			converted, err := toStarterSetStatus(status)
			if err != nil {
				return query.StarterSetFilter{}, err
			}
			statuses = append(statuses, converted)
		}
	}
	return query.StarterSetFilter{NameContains: filter.NameContains, Statuses: statuses}, nil
}

func toStarterSetStatus(status model.StarterSetStatus) (readmodel.StarterSetStatus, error) {
	switch status {
	case model.StarterSetStatusDraft:
		return readmodel.StarterSetStatusDraft, nil
	case model.StarterSetStatusPublished:
		return readmodel.StarterSetStatusPublished, nil
	case model.StarterSetStatusRetired:
		return readmodel.StarterSetStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown npc avatar status: %q", status)
	}
}

func fromStarterSetStatus(status readmodel.StarterSetStatus) (model.StarterSetStatus, error) {
	switch status {
	case readmodel.StarterSetStatusDraft:
		return model.StarterSetStatusDraft, nil
	case readmodel.StarterSetStatusPublished:
		return model.StarterSetStatusPublished, nil
	case readmodel.StarterSetStatusRetired:
		return model.StarterSetStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown npc avatar status: %q", status)
	}
}
