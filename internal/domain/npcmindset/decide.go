package npcmindset

import (
	"maps"
	"time"
)

// 変えても同じになる操作（同じ名前・同じ重み・公開中にもう一度公開する等）は、イベントを作らない（版を進めず、画面の版を古くしないため）。
// 名前・重みの変更と削除は準備中だけ。同じ状態への公開・使用不可も、準備中でなくてよい
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredNPCMindset); !ok {
			return nil, ErrInvalidState
		}
		return []Event{&RegisteredEvent{
			EventMeta:        EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:         c.PublicID,
			Name:             c.Name,
			ActionWeights:    DefaultActionWeights(),
			CriterionWeights: DefaultCriterionWeights(),
			RegisteredBy:     c.RegisteredBy,
		}}, nil
	case *RenameCommand:
		npcMindset, err := draft(state)
		if err != nil {
			return nil, err
		}
		if npcMindset.Name == c.Name {
			return nil, nil
		}
		return []Event{&RenamedEvent{EventMeta: nextMeta(npcMindset, now), Name: c.Name, RenamedBy: c.RenamedBy}}, nil
	case *SetActionWeightsCommand:
		npcMindset, err := draft(state)
		if err != nil {
			return nil, err
		}
		if !c.Weights.complete() {
			return nil, ErrActionWeightsMismatch
		}
		if maps.Equal(npcMindset.ActionWeights, c.Weights) {
			return nil, nil
		}
		return []Event{&ActionWeightsChangedEvent{EventMeta: nextMeta(npcMindset, now), Weights: maps.Clone(c.Weights), ChangedBy: c.ChangedBy}}, nil
	case *SetCriterionWeightsCommand:
		npcMindset, err := draft(state)
		if err != nil {
			return nil, err
		}
		if !c.Weights.complete() {
			return nil, ErrCriterionWeightsMismatch
		}
		if maps.Equal(npcMindset.CriterionWeights, c.Weights) {
			return nil, nil
		}
		return []Event{&CriterionWeightsChangedEvent{EventMeta: nextMeta(npcMindset, now), Weights: maps.Clone(c.Weights), ChangedBy: c.ChangedBy}}, nil
	case *PublishCommand:
		npcMindset, ok := state.(NPCMindset)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := npcMindset.Status.Publish()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&PublishedEvent{EventMeta: nextMeta(npcMindset, now), PublishedBy: c.PublishedBy}}, nil
	case *RetireCommand:
		npcMindset, ok := state.(NPCMindset)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := npcMindset.Status.Retire()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&RetiredEvent{EventMeta: nextMeta(npcMindset, now), RetiredBy: c.RetiredBy}}, nil
	case *DeleteCommand:
		npcMindset, err := draft(state)
		if err != nil {
			return nil, err
		}
		return []Event{&DeletedEvent{EventMeta: nextMeta(npcMindset, now), DeletedBy: c.DeletedBy}}, nil
	default:
		return nil, ErrInvalidState
	}
}

// 準備中の思考パターンだけを返す。登録していない・削除したものは ErrInvalidState、準備中でなければ ErrNotDraft
func draft(state State) (NPCMindset, error) {
	npcMindset, ok := state.(NPCMindset)
	if !ok {
		return NPCMindset{}, ErrInvalidState
	}
	if npcMindset.Status != StatusDraft {
		return NPCMindset{}, ErrNotDraft
	}
	return npcMindset, nil
}

func nextMeta(npcMindset NPCMindset, now time.Time) EventMeta {
	return EventMeta{EntityID: npcMindset.ID, AggregateVersion: npcMindset.Version + 1, OccurredAt: now}
}
