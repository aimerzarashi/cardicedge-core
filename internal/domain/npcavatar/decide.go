package npcavatar

import "time"

// 変えても同じになる操作（同じ名前・公開中にもう一度公開する等）は、イベントを作らない（版を進めず、画面の版を古くしないため）。
// 名前の変更と削除は準備中だけ。同じ状態への公開・使用不可も、準備中でなくてよい
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredNPCAvatar); !ok {
			return nil, ErrInvalidState
		}
		return []Event{&RegisteredEvent{
			EventMeta:    EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:     c.PublicID,
			Name:         c.Name,
			RegisteredBy: c.RegisteredBy,
		}}, nil
	case *RenameCommand:
		npcAvatar, err := draft(state)
		if err != nil {
			return nil, err
		}
		if npcAvatar.Name == c.Name {
			return nil, nil
		}
		return []Event{&RenamedEvent{EventMeta: nextMeta(npcAvatar, now), Name: c.Name, RenamedBy: c.RenamedBy}}, nil
	case *PublishCommand:
		npcAvatar, ok := state.(NPCAvatar)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := npcAvatar.Status.Publish()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&PublishedEvent{EventMeta: nextMeta(npcAvatar, now), PublishedBy: c.PublishedBy}}, nil
	case *RetireCommand:
		npcAvatar, ok := state.(NPCAvatar)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := npcAvatar.Status.Retire()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&RetiredEvent{EventMeta: nextMeta(npcAvatar, now), RetiredBy: c.RetiredBy}}, nil
	case *DeleteCommand:
		npcAvatar, err := draft(state)
		if err != nil {
			return nil, err
		}
		return []Event{&DeletedEvent{EventMeta: nextMeta(npcAvatar, now), DeletedBy: c.DeletedBy}}, nil
	default:
		return nil, ErrInvalidState
	}
}

// 準備中の NPC のアバターだけを返す。登録していない・削除したものは ErrInvalidState、準備中でなければ ErrNotDraft
func draft(state State) (NPCAvatar, error) {
	npcAvatar, ok := state.(NPCAvatar)
	if !ok {
		return NPCAvatar{}, ErrInvalidState
	}
	if npcAvatar.Status != StatusDraft {
		return NPCAvatar{}, ErrNotDraft
	}
	return npcAvatar, nil
}

func nextMeta(npcAvatar NPCAvatar, now time.Time) EventMeta {
	return EventMeta{EntityID: npcAvatar.ID, AggregateVersion: npcAvatar.Version + 1, OccurredAt: now}
}
