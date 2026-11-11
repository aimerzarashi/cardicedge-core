package spell

import (
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

// 変わらない操作（公開中にもう一度公開する等）は、イベントを作らない（版を進めず、画面の版を古くしないため）
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredSpell); !ok {
			return nil, ErrInvalidState
		}
		return []Event{&RegisteredEvent{
			EventMeta:    EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:     c.PublicID,
			Name:         c.Name,
			Cost:         card.DefaultCost,
			Rarity:       card.DefaultRarity,
			Effect:       NoEffect(),
			RegisteredBy: c.RegisteredBy,
		}}, nil
	case *PublishCommand:
		spell, ok := state.(Spell)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := spell.Status.Publish()
		if err != nil || !changed {
			return nil, err
		}
		// 効果の無いスペルは対戦で使えないため、公開できない（6 種類の効果がそろった core#152 から。2026-11-10 オーナー）
		if spell.Effect.IsNone() {
			return nil, ErrNoEffect
		}
		return []Event{&PublishedEvent{EventMeta: nextMeta(spell, now), PublishedBy: c.PublishedBy}}, nil
	case *RetireCommand:
		spell, ok := state.(Spell)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := spell.Status.Retire()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&RetiredEvent{EventMeta: nextMeta(spell, now), RetiredBy: c.RetiredBy}}, nil
	case *ChangeCostCommand:
		spell, err := editable(state)
		if err != nil || spell.Cost == c.Cost {
			return nil, err
		}
		return []Event{&CostChangedEvent{EventMeta: nextMeta(spell, now), Cost: c.Cost, ChangedBy: c.ChangedBy}}, nil
	case *ChangeRarityCommand:
		spell, err := editable(state)
		if err != nil || spell.Rarity == c.Rarity {
			return nil, err
		}
		return []Event{&RarityChangedEvent{EventMeta: nextMeta(spell, now), Rarity: c.Rarity, ChangedBy: c.ChangedBy}}, nil
	case *ChangeEffectCommand:
		spell, err := editable(state)
		if err != nil || spell.Effect.Equal(c.Effect) {
			return nil, err
		}
		return []Event{&EffectChangedEvent{EventMeta: nextMeta(spell, now), Effect: c.Effect, ChangedBy: c.ChangedBy}}, nil
	default:
		return nil, ErrInvalidState
	}
}

// 性能を変えられる（登録済みで準備中の）スペル
func editable(state State) (Spell, error) {
	spell, ok := state.(Spell)
	if !ok {
		return Spell{}, ErrInvalidState
	}
	if !spell.Status.SpecEditable() {
		return Spell{}, card.ErrNotDraft
	}
	return spell, nil
}

func nextMeta(spell Spell, now time.Time) EventMeta {
	return EventMeta{EntityID: spell.ID, AggregateVersion: spell.Version + 1, OccurredAt: now}
}
