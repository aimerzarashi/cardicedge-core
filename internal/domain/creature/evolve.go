package creature

import "gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"

func Evolve(state State, event Event) (State, error) {
	switch e := event.(type) {
	case *RegisteredEvent:
		if _, ok := state.(UnregisteredCreature); !ok {
			return state, ErrInvalidState
		}
		meta := e.GetMeta()
		return Creature{
			ID:                    meta.EntityID,
			Version:               meta.AggregateVersion,
			PublicID:              e.PublicID,
			Status:                card.StatusDraft,
			Cost:                  e.Cost,
			Rarity:                e.Rarity,
			Elements:              e.Elements,
			Attack:                e.Attack,
			HitPoints:             e.HitPoints,
			ItemRestrictions:      e.ItemRestrictions,
			PlacementRestrictions: e.PlacementRestrictions,
			Effects:               e.Effects,
		}, nil
	case *PublishedEvent:
		creature, ok := state.(Creature)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := creature.Status.Publish()
		if err != nil {
			return state, err
		}
		creature.Status = status
		creature.Version = e.GetMeta().AggregateVersion
		return creature, nil
	case *RetiredEvent:
		creature, ok := state.(Creature)
		if !ok {
			return state, ErrInvalidState
		}
		status, _, err := creature.Status.Retire()
		if err != nil {
			return state, err
		}
		creature.Status = status
		creature.Version = e.GetMeta().AggregateVersion
		return creature, nil
	case *CostChangedEvent:
		creature, ok := state.(Creature)
		if !ok {
			return state, ErrInvalidState
		}
		creature.Cost = e.Cost
		creature.Version = e.GetMeta().AggregateVersion
		return creature, nil
	case *RarityChangedEvent:
		creature, ok := state.(Creature)
		if !ok {
			return state, ErrInvalidState
		}
		creature.Rarity = e.Rarity
		creature.Version = e.GetMeta().AggregateVersion
		return creature, nil
	case *ElementsChangedEvent:
		creature, ok := state.(Creature)
		if !ok {
			return state, ErrInvalidState
		}
		creature.Elements = e.Elements
		creature.Version = e.GetMeta().AggregateVersion
		return creature, nil
	case *StatsChangedEvent:
		creature, ok := state.(Creature)
		if !ok {
			return state, ErrInvalidState
		}
		creature.Attack = e.Attack
		creature.HitPoints = e.HitPoints
		creature.Version = e.GetMeta().AggregateVersion
		return creature, nil
	case *EffectsChangedEvent:
		creature, ok := state.(Creature)
		if !ok {
			return state, ErrInvalidState
		}
		creature.Effects = e.Effects
		creature.Version = e.GetMeta().AggregateVersion
		return creature, nil
	case *ItemRestrictionsChangedEvent:
		creature, ok := state.(Creature)
		if !ok {
			return state, ErrInvalidState
		}
		creature.ItemRestrictions = e.ItemRestrictions
		creature.Version = e.GetMeta().AggregateVersion
		return creature, nil
	case *PlacementRestrictionsChangedEvent:
		creature, ok := state.(Creature)
		if !ok {
			return state, ErrInvalidState
		}
		creature.PlacementRestrictions = e.PlacementRestrictions
		creature.Version = e.GetMeta().AggregateVersion
		return creature, nil
	default:
		return state, ErrInvalidState
	}
}
