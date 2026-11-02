package graphql

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/presentation/graphql/model"
)

func fromQueryNPCAvatar(npcAvatar query.NPCAvatar) (*model.NpcAvatar, error) {
	status, err := fromNPCAvatarStatus(npcAvatar.Status)
	if err != nil {
		return nil, err
	}
	return &model.NpcAvatar{
		ID: npcAvatar.ID, Name: npcAvatar.Name, Status: status, Revision: int(npcAvatar.Revision),
		RegisteredAt: npcAvatar.RegisteredAt, UpdatedAt: npcAvatar.UpdatedAt,
	}, nil
}

func toNPCAvatarConnection(connection pagination.Connection[query.NPCAvatar]) (*model.NpcAvatarConnection, error) {
	edges := make([]*model.NpcAvatarEdge, 0, len(connection.Edges))
	for _, edge := range connection.Edges {
		node, err := fromQueryNPCAvatar(edge.Node)
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.NpcAvatarEdge{Cursor: edge.Cursor, Node: node})
	}
	pageInfo := &model.PageInfo{
		HasNextPage:     connection.PageInfo.HasNextPage,
		HasPreviousPage: connection.PageInfo.HasPreviousPage,
		StartCursor:     connection.PageInfo.StartCursor,
		EndCursor:       connection.PageInfo.EndCursor,
	}
	return &model.NpcAvatarConnection{Edges: edges, PageInfo: pageInfo}, nil
}

// statuses の null（絞り込まない）と空のリスト（入力の誤り）は、application 層が見分けるため区別して渡す
func toNPCAvatarFilter(filter *model.NpcAvatarFilter) (query.NPCAvatarFilter, error) {
	if filter == nil {
		return query.NPCAvatarFilter{}, nil
	}
	var statuses []readmodel.NPCAvatarStatus
	if filter.Statuses != nil {
		statuses = make([]readmodel.NPCAvatarStatus, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			converted, err := toNPCAvatarStatus(status)
			if err != nil {
				return query.NPCAvatarFilter{}, err
			}
			statuses = append(statuses, converted)
		}
	}
	return query.NPCAvatarFilter{NameContains: filter.NameContains, Statuses: statuses}, nil
}

func toNPCAvatarStatus(status model.NpcAvatarStatus) (readmodel.NPCAvatarStatus, error) {
	switch status {
	case model.NpcAvatarStatusDraft:
		return readmodel.NPCAvatarStatusDraft, nil
	case model.NpcAvatarStatusPublished:
		return readmodel.NPCAvatarStatusPublished, nil
	case model.NpcAvatarStatusRetired:
		return readmodel.NPCAvatarStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown npc avatar status: %q", status)
	}
}

func fromNPCAvatarStatus(status readmodel.NPCAvatarStatus) (model.NpcAvatarStatus, error) {
	switch status {
	case readmodel.NPCAvatarStatusDraft:
		return model.NpcAvatarStatusDraft, nil
	case readmodel.NPCAvatarStatusPublished:
		return model.NpcAvatarStatusPublished, nil
	case readmodel.NPCAvatarStatusRetired:
		return model.NpcAvatarStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown npc avatar status: %q", status)
	}
}
