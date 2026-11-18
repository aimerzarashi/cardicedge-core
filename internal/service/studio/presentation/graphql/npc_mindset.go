package graphql

import (
	"fmt"
	"strings"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/presentation/graphql/model"
)

func fromQueryNPCMindset(npcMindset query.NPCMindset) (*model.NpcMindset, error) {
	status, err := fromNPCMindsetStatus(npcMindset.Status)
	if err != nil {
		return nil, err
	}
	actionWeights := make([]*model.NpcMindsetActionWeight, 0, len(npcMindset.ActionWeights))
	for _, weight := range npcMindset.ActionWeights {
		action := model.MatchAction(strings.ToUpper(weight.Action))
		if !action.IsValid() {
			return nil, fmt.Errorf("graphql: unknown match action: %q", weight.Action)
		}
		actionWeights = append(actionWeights, &model.NpcMindsetActionWeight{Action: action, Weight: weight.Weight})
	}
	criterionWeights := make([]*model.NpcMindsetCriterionWeight, 0, len(npcMindset.CriterionWeights))
	for _, weight := range npcMindset.CriterionWeights {
		criterion := model.NpcMindsetCriterion(strings.ToUpper(weight.Criterion))
		if !criterion.IsValid() {
			return nil, fmt.Errorf("graphql: unknown npc mindset criterion: %q", weight.Criterion)
		}
		criterionWeights = append(criterionWeights, &model.NpcMindsetCriterionWeight{Criterion: criterion, Weight: weight.Weight})
	}
	return &model.NpcMindset{
		ID: npcMindset.ID, Name: npcMindset.Name, Status: status, Revision: int(npcMindset.Revision),
		RegisteredAt: npcMindset.RegisteredAt, UpdatedAt: npcMindset.UpdatedAt, ActionWeights: actionWeights, CriterionWeights: criterionWeights,
	}, nil
}

func toNPCMindsetConnection(connection pagination.Connection[query.NPCMindset]) (*model.NpcMindsetConnection, error) {
	edges := make([]*model.NpcMindsetEdge, 0, len(connection.Edges))
	for _, edge := range connection.Edges {
		node, err := fromQueryNPCMindset(edge.Node)
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.NpcMindsetEdge{Cursor: edge.Cursor, Node: node})
	}
	pageInfo := &model.PageInfo{
		HasNextPage:     connection.PageInfo.HasNextPage,
		HasPreviousPage: connection.PageInfo.HasPreviousPage,
		StartCursor:     connection.PageInfo.StartCursor,
		EndCursor:       connection.PageInfo.EndCursor,
	}
	return &model.NpcMindsetConnection{Edges: edges, PageInfo: pageInfo}, nil
}

// statuses の null（絞り込まない）と空のリスト（入力の誤り）は、application 層が見分けるため区別して渡す
func toNPCMindsetFilter(filter *model.NpcMindsetFilter) (query.NPCMindsetFilter, error) {
	if filter == nil {
		return query.NPCMindsetFilter{}, nil
	}
	var statuses []readmodel.NPCMindsetStatus
	if filter.Statuses != nil {
		statuses = make([]readmodel.NPCMindsetStatus, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			converted, err := toNPCMindsetStatus(status)
			if err != nil {
				return query.NPCMindsetFilter{}, err
			}
			statuses = append(statuses, converted)
		}
	}
	return query.NPCMindsetFilter{NameContains: filter.NameContains, Statuses: statuses}, nil
}

func toNPCMindsetStatus(status model.NpcMindsetStatus) (readmodel.NPCMindsetStatus, error) {
	switch status {
	case model.NpcMindsetStatusDraft:
		return readmodel.NPCMindsetStatusDraft, nil
	case model.NpcMindsetStatusPublished:
		return readmodel.NPCMindsetStatusPublished, nil
	case model.NpcMindsetStatusRetired:
		return readmodel.NPCMindsetStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown npc mindset status: %q", status)
	}
}

func fromNPCMindsetStatus(status readmodel.NPCMindsetStatus) (model.NpcMindsetStatus, error) {
	switch status {
	case readmodel.NPCMindsetStatusDraft:
		return model.NpcMindsetStatusDraft, nil
	case readmodel.NPCMindsetStatusPublished:
		return model.NpcMindsetStatusPublished, nil
	case readmodel.NPCMindsetStatusRetired:
		return model.NpcMindsetStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown npc mindset status: %q", status)
	}
}

func fromActionWeightInputs(weights []*model.NpcMindsetActionWeightInput) []command.ActionWeightInput {
	inputs := make([]command.ActionWeightInput, 0, len(weights))
	for _, weight := range weights {
		inputs = append(inputs, command.ActionWeightInput{Action: strings.ToLower(string(weight.Action)), Weight: weight.Weight})
	}
	return inputs
}

func fromCriterionWeightInputs(weights []*model.NpcMindsetCriterionWeightInput) []command.CriterionWeightInput {
	inputs := make([]command.CriterionWeightInput, 0, len(weights))
	for _, weight := range weights {
		inputs = append(inputs, command.CriterionWeightInput{Criterion: strings.ToLower(string(weight.Criterion)), Weight: weight.Weight})
	}
	return inputs
}
