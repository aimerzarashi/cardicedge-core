package starterset

import (
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
)

func Evolve(state State, event Event) (State, error) {
	meta := event.GetMeta()
	if registered, ok := event.(*RegisteredEvent); ok {
		if _, ok := state.(UnregisteredStarterSet); !ok {
			return state, ErrInvalidState
		}
		return StarterSet{
			ID:       meta.EntityID,
			Version:  meta.AggregateVersion,
			PublicID: registered.PublicID,
			Name:     registered.Name,
			Status:   StatusDraft,
		}, nil
	}
	starterSet, ok := state.(StarterSet)
	if !ok {
		return state, ErrInvalidState
	}
	switch e := event.(type) {
	case *RenamedEvent:
		starterSet.Name = e.Name
	case *GrantDecksAssignedEvent:
		starterSet.GrantDecks = append(slices.Clone(starterSet.GrantDecks), e.GrantDecks...)
	case *GrantDecksUnassignedEvent:
		starterSet.GrantDecks = slices.DeleteFunc(slices.Clone(starterSet.GrantDecks), func(id grantdeck.ID) bool { return slices.Contains(e.GrantDecks, id) })
	case *UnlockBoardsAssignedEvent:
		starterSet.UnlockBoards = append(slices.Clone(starterSet.UnlockBoards), e.UnlockBoards...)
	case *UnlockBoardsUnassignedEvent:
		starterSet.UnlockBoards = slices.DeleteFunc(slices.Clone(starterSet.UnlockBoards), func(id board.ID) bool { return slices.Contains(e.UnlockBoards, id) })
	case *UnlockNPCAvatarsAssignedEvent:
		starterSet.UnlockNPCAvatars = append(slices.Clone(starterSet.UnlockNPCAvatars), e.UnlockNPCAvatars...)
	case *UnlockNPCAvatarsUnassignedEvent:
		starterSet.UnlockNPCAvatars = slices.DeleteFunc(slices.Clone(starterSet.UnlockNPCAvatars), func(id npcavatar.ID) bool { return slices.Contains(e.UnlockNPCAvatars, id) })
	case *PublishedEvent:
		status, _, err := starterSet.Status.Publish()
		if err != nil {
			return state, err
		}
		starterSet.Status = status
	case *RetiredEvent:
		status, _, err := starterSet.Status.Retire()
		if err != nil {
			return state, err
		}
		starterSet.Status = status
	case *DeletedEvent:
		return DeletedStarterSet{ID: starterSet.ID, Version: meta.AggregateVersion}, nil
	default:
		return state, ErrInvalidState
	}
	starterSet.Version = meta.AggregateVersion
	return starterSet, nil
}
