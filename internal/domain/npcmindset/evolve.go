package npcmindset

import "maps"

// 状態を書き換えないよう、重みは写してから持つ
func Evolve(state State, event Event) (State, error) {
	meta := event.GetMeta()
	switch e := event.(type) {
	case *RegisteredEvent:
		if _, ok := state.(UnregisteredNPCMindset); !ok {
			return state, ErrInvalidState
		}
		return NPCMindset{
			ID:               meta.EntityID,
			Version:          meta.AggregateVersion,
			PublicID:         e.PublicID,
			Name:             e.Name,
			Status:           StatusDraft,
			ActionWeights:    maps.Clone(e.ActionWeights),
			CriterionWeights: maps.Clone(e.CriterionWeights),
		}, nil
	case *RenamedEvent:
		npcMindset, ok := state.(NPCMindset)
		if !ok {
			return state, ErrInvalidState
		}
		npcMindset.Name = e.Name
		return touched(npcMindset, meta), nil
	case *ActionWeightsChangedEvent:
		npcMindset, ok := state.(NPCMindset)
		if !ok {
			return state, ErrInvalidState
		}
		npcMindset.ActionWeights = maps.Clone(e.Weights)
		return touched(npcMindset, meta), nil
	case *CriterionWeightsChangedEvent:
		npcMindset, ok := state.(NPCMindset)
		if !ok {
			return state, ErrInvalidState
		}
		npcMindset.CriterionWeights = maps.Clone(e.Weights)
		return touched(npcMindset, meta), nil
	case *PublishedEvent:
		npcMindset, ok := state.(NPCMindset)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := npcMindset.Status.Publish()
		if err != nil {
			return state, err
		}
		npcMindset.Status = status
		return touched(npcMindset, meta), nil
	case *RetiredEvent:
		npcMindset, ok := state.(NPCMindset)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := npcMindset.Status.Retire()
		if err != nil {
			return state, err
		}
		npcMindset.Status = status
		return touched(npcMindset, meta), nil
	case *DeletedEvent:
		npcMindset, ok := state.(NPCMindset)
		if !ok {
			return state, ErrInvalidState
		}
		return DeletedNPCMindset{ID: npcMindset.ID, Version: meta.AggregateVersion}, nil
	default:
		return state, ErrInvalidState
	}
}

func touched(npcMindset NPCMindset, meta EventMeta) NPCMindset {
	npcMindset.Version = meta.AggregateVersion
	return npcMindset
}
