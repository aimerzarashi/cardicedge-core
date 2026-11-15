package item

import (
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

// 変わらない操作（公開中にもう一度公開する等）は、イベントを作らない（版を進めず、画面の版を古くしないため）
func Decide(state State, cmd Command, now time.Time) ([]Event, error) {
	switch c := cmd.(type) {
	case *RegisterCommand:
		if _, ok := state.(UnregisteredItem); !ok {
			return nil, ErrInvalidState
		}
		return []Event{&RegisteredEvent{
			EventMeta:    EventMeta{EntityID: c.ID, AggregateVersion: 1, OccurredAt: now},
			PublicID:     c.PublicID,
			Name:         c.Name,
			Cost:         card.DefaultCost,
			Rarity:       card.DefaultRarity,
			Category:     DefaultCategory,
			Effects:      DefaultEffects(),
			RegisteredBy: c.RegisteredBy,
		}}, nil
	case *PublishCommand:
		item, ok := state.(Item)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := item.Status.Publish()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&PublishedEvent{EventMeta: nextMeta(item, now), PublishedBy: c.PublishedBy}}, nil
	case *RetireCommand:
		item, ok := state.(Item)
		if !ok {
			return nil, ErrInvalidState
		}
		_, changed, err := item.Status.Retire()
		if err != nil || !changed {
			return nil, err
		}
		return []Event{&RetiredEvent{EventMeta: nextMeta(item, now), RetiredBy: c.RetiredBy}}, nil
	case *ChangeCostCommand:
		item, err := editable(state)
		if err != nil || item.Cost == c.Cost {
			return nil, err
		}
		return []Event{&CostChangedEvent{EventMeta: nextMeta(item, now), Cost: c.Cost, ChangedBy: c.ChangedBy}}, nil
	case *ChangeRarityCommand:
		item, err := editable(state)
		if err != nil || item.Rarity == c.Rarity {
			return nil, err
		}
		return []Event{&RarityChangedEvent{EventMeta: nextMeta(item, now), Rarity: c.Rarity, ChangedBy: c.ChangedBy}}, nil
	case *ChangeCategoryCommand:
		item, err := editable(state)
		if err != nil || item.Category == c.Category {
			return nil, err
		}
		if _, found := item.Effects.NotAllowedIn(c.Category); found {
			return nil, ErrEffectNotAllowed
		}
		return []Event{&CategoryChangedEvent{EventMeta: nextMeta(item, now), Category: c.Category, ChangedBy: c.ChangedBy}}, nil
	case *ChangeEffectsCommand:
		item, err := editable(state)
		if err != nil || item.Effects.Equal(c.Effects) {
			return nil, err
		}
		if _, found := c.Effects.NotAllowedIn(item.Category); found {
			return nil, ErrEffectNotAllowed
		}
		return []Event{&EffectsChangedEvent{EventMeta: nextMeta(item, now), Effects: c.Effects, ChangedBy: c.ChangedBy}}, nil
	default:
		return nil, ErrInvalidState
	}
}

// 性能を変えられる（登録済みで準備中の）アイテム
func editable(state State) (Item, error) {
	item, ok := state.(Item)
	if !ok {
		return Item{}, ErrInvalidState
	}
	if !item.Status.SpecEditable() {
		return Item{}, card.ErrNotDraft
	}
	return item, nil
}

func nextMeta(item Item, now time.Time) EventMeta {
	return EventMeta{EntityID: item.ID, AggregateVersion: item.Version + 1, OccurredAt: now}
}
