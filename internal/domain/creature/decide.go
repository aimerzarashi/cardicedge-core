package creature

import (
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

// 変わらない操作（公開中にもう一度公開する等）は、イベントを作らない（版を進めず、画面の版を古くしないため）
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredCreature); !ok {
			return nil, ErrInvalidState
		}
		return []Event{&RegisteredEvent{
			EventMeta:             EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:              c.PublicID,
			Name:                  c.Name,
			Cost:                  card.DefaultCost,
			Rarity:                card.DefaultRarity,
			Elements:              DefaultElements(),
			Attack:                DefaultAttack,
			HitPoints:             DefaultHitPoints,
			ItemRestrictions:      DefaultItemRestrictions(),
			PlacementRestrictions: DefaultPlacementRestrictions(),
			Abilities:             DefaultAbilities(),
			RegisteredBy:          c.RegisteredBy,
		}}, nil
	case *PublishCommand:
		creature, ok := state.(Creature)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := creature.Status.Publish()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&PublishedEvent{EventMeta: nextMeta(creature, now), PublishedBy: c.PublishedBy}}, nil
	case *RetireCommand:
		creature, ok := state.(Creature)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := creature.Status.Retire()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&RetiredEvent{EventMeta: nextMeta(creature, now), RetiredBy: c.RetiredBy}}, nil
	case *ChangeCostCommand:
		creature, err := editable(state)
		if err != nil || creature.Cost == c.Cost {
			return nil, err
		}
		return []Event{&CostChangedEvent{EventMeta: nextMeta(creature, now), Cost: c.Cost, ChangedBy: c.ChangedBy}}, nil
	case *ChangeRarityCommand:
		creature, err := editable(state)
		if err != nil || creature.Rarity == c.Rarity {
			return nil, err
		}
		return []Event{&RarityChangedEvent{EventMeta: nextMeta(creature, now), Rarity: c.Rarity, ChangedBy: c.ChangedBy}}, nil
	case *ChangeElementsCommand:
		creature, err := editable(state)
		if err != nil || creature.Elements.Equal(c.Elements) {
			return nil, err
		}
		return []Event{&ElementsChangedEvent{EventMeta: nextMeta(creature, now), Elements: c.Elements, ChangedBy: c.ChangedBy}}, nil
	case *ChangeStatsCommand:
		creature, err := editable(state)
		if err != nil || (creature.Attack == c.Attack && creature.HitPoints == c.HitPoints) {
			return nil, err
		}
		return []Event{&StatsChangedEvent{EventMeta: nextMeta(creature, now), Attack: c.Attack, HitPoints: c.HitPoints, ChangedBy: c.ChangedBy}}, nil
	case *ChangeAbilitiesCommand:
		creature, err := editable(state)
		if err != nil || creature.Abilities.Equal(c.Abilities) {
			return nil, err
		}
		return []Event{&AbilitiesChangedEvent{EventMeta: nextMeta(creature, now), Abilities: c.Abilities, ChangedBy: c.ChangedBy}}, nil
	case *ChangeItemRestrictionsCommand:
		creature, err := editable(state)
		if err != nil || creature.ItemRestrictions.Equal(c.ItemRestrictions) {
			return nil, err
		}
		return []Event{&ItemRestrictionsChangedEvent{EventMeta: nextMeta(creature, now), ItemRestrictions: c.ItemRestrictions, ChangedBy: c.ChangedBy}}, nil
	case *ChangePlacementRestrictionsCommand:
		creature, err := editable(state)
		if err != nil || creature.PlacementRestrictions.Equal(c.PlacementRestrictions) {
			return nil, err
		}
		return []Event{&PlacementRestrictionsChangedEvent{EventMeta: nextMeta(creature, now), PlacementRestrictions: c.PlacementRestrictions, ChangedBy: c.ChangedBy}}, nil
	default:
		return nil, ErrInvalidState
	}
}

// 性能を変えられる（登録済みで準備中の）クリーチャー
func editable(state State) (Creature, error) {
	creature, ok := state.(Creature)
	if !ok {
		return Creature{}, ErrInvalidState
	}
	if !creature.Status.SpecEditable() {
		return Creature{}, card.ErrNotDraft
	}
	return creature, nil
}

func nextMeta(creature Creature, now time.Time) EventMeta {
	return EventMeta{EntityID: creature.ID, AggregateVersion: creature.Version + 1, OccurredAt: now}
}
