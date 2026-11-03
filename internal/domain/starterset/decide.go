package starterset

import (
	"slices"
	"time"
)

// 変えても同じになる操作（同じ名前・公開中にもう一度公開する等）は、イベントを作らない（版を進めず、画面の版を古くしないため）。
// 名前・配るデッキ・解放するボードの変更と削除は準備中だけ（公開した後は中身を変えられない）。同じ状態への公開・使用不可も、準備中でなくてよい
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredStarterSet); !ok {
			return nil, ErrInvalidState
		}
		return []Event{&RegisteredEvent{
			EventMeta:    EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:     c.PublicID,
			Name:         c.Name,
			RegisteredBy: c.RegisteredBy,
		}}, nil
	case *RenameCommand:
		starterSet, err := draft(state)
		if err != nil {
			return nil, err
		}
		if starterSet.Name == c.Name {
			return nil, nil
		}
		return []Event{&RenamedEvent{EventMeta: nextMeta(starterSet, now), Name: c.Name, RenamedBy: c.RenamedBy}}, nil
	case *SetGrantDecksCommand:
		starterSet, err := draft(state)
		if err != nil {
			return nil, err
		}
		// 外す → 割り当てるの順に出す（版は出したイベントの数だけ進む）。差が無ければイベントを作らない
		var events []Event
		version := starterSet.Version
		if unassigned := missingFrom(c.GrantDecks, starterSet.GrantDecks); len(unassigned) > 0 {
			version++
			events = append(events, &GrantDecksUnassignedEvent{EventMeta: metaAt(starterSet, version, now), GrantDecks: unassigned, UnassignedBy: c.ChangedBy})
		}
		if assigned := missingFrom(starterSet.GrantDecks, c.GrantDecks); len(assigned) > 0 {
			version++
			events = append(events, &GrantDecksAssignedEvent{EventMeta: metaAt(starterSet, version, now), GrantDecks: assigned, AssignedBy: c.ChangedBy})
		}
		return events, nil
	case *SetUnlockBoardsCommand:
		starterSet, err := draft(state)
		if err != nil {
			return nil, err
		}
		var events []Event
		version := starterSet.Version
		if unassigned := missingFrom(c.UnlockBoards, starterSet.UnlockBoards); len(unassigned) > 0 {
			version++
			events = append(events, &UnlockBoardsUnassignedEvent{EventMeta: metaAt(starterSet, version, now), UnlockBoards: unassigned, UnassignedBy: c.ChangedBy})
		}
		if assigned := missingFrom(starterSet.UnlockBoards, c.UnlockBoards); len(assigned) > 0 {
			version++
			events = append(events, &UnlockBoardsAssignedEvent{EventMeta: metaAt(starterSet, version, now), UnlockBoards: assigned, AssignedBy: c.ChangedBy})
		}
		return events, nil
	case *SetUnlockNPCAvatarsCommand:
		starterSet, err := draft(state)
		if err != nil {
			return nil, err
		}
		var events []Event
		version := starterSet.Version
		if unassigned := missingFrom(c.UnlockNPCAvatars, starterSet.UnlockNPCAvatars); len(unassigned) > 0 {
			version++
			events = append(events, &UnlockNPCAvatarsUnassignedEvent{EventMeta: metaAt(starterSet, version, now), UnlockNPCAvatars: unassigned, UnassignedBy: c.ChangedBy})
		}
		if assigned := missingFrom(starterSet.UnlockNPCAvatars, c.UnlockNPCAvatars); len(assigned) > 0 {
			version++
			events = append(events, &UnlockNPCAvatarsAssignedEvent{EventMeta: metaAt(starterSet, version, now), UnlockNPCAvatars: assigned, AssignedBy: c.ChangedBy})
		}
		return events, nil
	case *PublishCommand:
		starterSet, ok := state.(StarterSet)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := starterSet.Status.Publish()
		if err != nil || !changed {
			return nil, err
		}
		// 解放するボードは 0 件でもよい。配るデッキが 0 件では配布を始められない（startergrant.ErrInvalidDeckCount）
		if len(starterSet.GrantDecks) == 0 {
			return nil, ErrNoGrantDecks
		}
		return []Event{&PublishedEvent{EventMeta: nextMeta(starterSet, now), PublishedBy: c.PublishedBy}}, nil
	case *RetireCommand:
		starterSet, ok := state.(StarterSet)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := starterSet.Status.Retire()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&RetiredEvent{EventMeta: nextMeta(starterSet, now), RetiredBy: c.RetiredBy}}, nil
	case *DeleteCommand:
		starterSet, err := draft(state)
		if err != nil {
			return nil, err
		}
		return []Event{&DeletedEvent{EventMeta: nextMeta(starterSet, now), DeletedBy: c.DeletedBy}}, nil
	default:
		return nil, ErrInvalidState
	}
}

// 準備中のスターターセットだけを返す。登録していない・削除したものは ErrInvalidState、準備中でなければ ErrNotDraft
func draft(state State) (StarterSet, error) {
	starterSet, ok := state.(StarterSet)
	if !ok {
		return StarterSet{}, ErrInvalidState
	}
	if starterSet.Status != StatusDraft {
		return StarterSet{}, ErrNotDraft
	}
	return starterSet, nil
}

// items のうち base に無いものを、items の順で返す（並べ替えだけでは差が出ない。一覧の順は割り当てた順のまま）
func missingFrom[T comparable](base, items []T) []T {
	var missing []T
	for _, item := range items {
		if !slices.Contains(base, item) {
			missing = append(missing, item)
		}
	}
	return missing
}

func nextMeta(starterSet StarterSet, now time.Time) EventMeta {
	return metaAt(starterSet, starterSet.Version+1, now)
}

func metaAt(starterSet StarterSet, version int64, now time.Time) EventMeta {
	return EventMeta{EntityID: starterSet.ID, AggregateVersion: version, OccurredAt: now}
}
