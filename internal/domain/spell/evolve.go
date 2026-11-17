package spell

import "gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"

func Evolve(state State, event Event) (State, error) {
	switch e := event.(type) {
	case *RegisteredEvent:
		if _, ok := state.(UnregisteredSpell); !ok {
			return state, ErrInvalidState
		}
		meta := e.GetMeta()
		return Spell{
			ID:       meta.EntityID,
			Version:  meta.AggregateVersion,
			PublicID: e.PublicID,
			Status:   card.StatusDraft,
			Cost:     e.Cost,
			Rarity:   e.Rarity,
			Effect:   e.Effect,
		}, nil
	case *PublishedEvent:
		spell, ok := state.(Spell)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := spell.Status.Publish()
		if err != nil {
			return state, err
		}
		spell.Status = status
		spell.Version = e.GetMeta().AggregateVersion
		return spell, nil
	case *RetiredEvent:
		spell, ok := state.(Spell)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := spell.Status.Retire()
		if err != nil {
			return state, err
		}
		spell.Status = status
		spell.Version = e.GetMeta().AggregateVersion
		return spell, nil
	case *CostChangedEvent:
		spell, ok := state.(Spell)
		if !ok {
			return state, ErrInvalidState
		}
		spell.Cost = e.Cost
		spell.Version = e.GetMeta().AggregateVersion
		return spell, nil
	case *RarityChangedEvent:
		spell, ok := state.(Spell)
		if !ok {
			return state, ErrInvalidState
		}
		spell.Rarity = e.Rarity
		spell.Version = e.GetMeta().AggregateVersion
		return spell, nil
	case *EffectChangedEvent:
		spell, ok := state.(Spell)
		if !ok {
			return state, ErrInvalidState
		}
		spell.Effect = e.Effect
		spell.Version = e.GetMeta().AggregateVersion
		return spell, nil
	default:
		return state, ErrInvalidState
	}
}
