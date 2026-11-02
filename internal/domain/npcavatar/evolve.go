package npcavatar

func Evolve(state State, event Event) (State, error) {
	meta := event.GetMeta()
	switch e := event.(type) {
	case *RegisteredEvent:
		if _, ok := state.(UnregisteredNPCAvatar); !ok {
			return state, ErrInvalidState
		}
		return NPCAvatar{
			ID:       meta.EntityID,
			Version:  meta.AggregateVersion,
			PublicID: e.PublicID,
			Name:     e.Name,
			Status:   StatusDraft,
		}, nil
	case *RenamedEvent:
		npcAvatar, ok := state.(NPCAvatar)
		if !ok {
			return state, ErrInvalidState
		}
		npcAvatar.Name = e.Name
		return touched(npcAvatar, meta), nil
	case *PublishedEvent:
		npcAvatar, ok := state.(NPCAvatar)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := npcAvatar.Status.Publish()
		if err != nil {
			return state, err
		}
		npcAvatar.Status = status
		return touched(npcAvatar, meta), nil
	case *RetiredEvent:
		npcAvatar, ok := state.(NPCAvatar)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := npcAvatar.Status.Retire()
		if err != nil {
			return state, err
		}
		npcAvatar.Status = status
		return touched(npcAvatar, meta), nil
	case *DeletedEvent:
		npcAvatar, ok := state.(NPCAvatar)
		if !ok {
			return state, ErrInvalidState
		}
		return DeletedNPCAvatar{ID: npcAvatar.ID, Version: meta.AggregateVersion}, nil
	default:
		return state, ErrInvalidState
	}
}

func touched(npcAvatar NPCAvatar, meta EventMeta) NPCAvatar {
	npcAvatar.Version = meta.AggregateVersion
	return npcAvatar
}
