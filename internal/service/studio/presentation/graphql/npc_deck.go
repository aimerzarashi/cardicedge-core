package graphql

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/presentation/graphql/model"
)

func fromQueryNPCDeck(npcDeck query.NPCDeck) (*model.NpcDeck, error) {
	status, err := fromNPCDeckStatus(npcDeck.Status)
	if err != nil {
		return nil, err
	}
	counts := make([]model.DeckCardCount, 0, len(npcDeck.Cards))
	for _, card := range npcDeck.Cards {
		counts = append(counts, model.DeckCardCount{CardID: card.CardID, Count: card.Count})
	}
	var source *model.NpcDeckSource
	if npcDeck.Source != nil {
		source = &model.NpcDeckSource{
			PresetDeckID:       npcDeck.Source.PresetDeckID,
			Revision:           int(npcDeck.Source.Revision),
			ChangedSinceCopied: npcDeck.Source.ChangedSinceCopied,
			PresetDeckDeleted:  npcDeck.Source.PresetDeckDeleted,
		}
	}
	return &model.NpcDeck{
		ID: npcDeck.ID, Name: npcDeck.Name, Status: status, TotalCount: npcDeck.TotalCount, Source: source, Revision: int(npcDeck.Revision),
		RegisteredAt: npcDeck.RegisteredAt, UpdatedAt: npcDeck.UpdatedAt, CardCounts: counts,
	}, nil
}

func toNPCDeckConnection(connection pagination.Connection[query.NPCDeck]) (*model.NpcDeckConnection, error) {
	edges := make([]*model.NpcDeckEdge, 0, len(connection.Edges))
	for _, edge := range connection.Edges {
		node, err := fromQueryNPCDeck(edge.Node)
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.NpcDeckEdge{Cursor: edge.Cursor, Node: node})
	}
	pageInfo := &model.PageInfo{
		HasNextPage:     connection.PageInfo.HasNextPage,
		HasPreviousPage: connection.PageInfo.HasPreviousPage,
		StartCursor:     connection.PageInfo.StartCursor,
		EndCursor:       connection.PageInfo.EndCursor,
	}
	return &model.NpcDeckConnection{Edges: edges, PageInfo: pageInfo}, nil
}

// statuses の null（絞り込まない）と空のリスト（入力の誤り）は、application 層が見分けるため区別して渡す
func toNPCDeckFilter(filter *model.NpcDeckFilter) (query.NPCDeckFilter, error) {
	if filter == nil {
		return query.NPCDeckFilter{}, nil
	}
	var statuses []readmodel.NPCDeckStatus
	if filter.Statuses != nil {
		statuses = make([]readmodel.NPCDeckStatus, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			converted, err := toNPCDeckStatus(status)
			if err != nil {
				return query.NPCDeckFilter{}, err
			}
			statuses = append(statuses, converted)
		}
	}
	return query.NPCDeckFilter{NameContains: filter.NameContains, Statuses: statuses}, nil
}

// enum の値と状態の対応を明示する（toCardStatus と同じ）
func toNPCDeckStatus(status model.NpcDeckStatus) (readmodel.NPCDeckStatus, error) {
	switch status {
	case model.NpcDeckStatusDraft:
		return readmodel.NPCDeckStatusDraft, nil
	case model.NpcDeckStatusPublished:
		return readmodel.NPCDeckStatusPublished, nil
	case model.NpcDeckStatusRetired:
		return readmodel.NPCDeckStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown npc deck status: %s", status)
	}
}

func fromNPCDeckStatus(status readmodel.NPCDeckStatus) (model.NpcDeckStatus, error) {
	switch status {
	case readmodel.NPCDeckStatusDraft:
		return model.NpcDeckStatusDraft, nil
	case readmodel.NPCDeckStatusPublished:
		return model.NpcDeckStatusPublished, nil
	case readmodel.NPCDeckStatusRetired:
		return model.NpcDeckStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown npc deck status: %q", status)
	}
}
