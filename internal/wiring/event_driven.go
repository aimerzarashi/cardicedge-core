package wiring

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeckset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventdriven"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
)

func newHub(registry *domaindef.Registry, options eventdriven.Options, subscribers []eventdriven.SubscriberDef) (*eventdriven.Hub, error) {
	// 購読される集約のイベントの読み方。購読する集約を足したらここにも足す（足し忘れると投影の登録で起動が止まる）
	return eventdriven.NewHub(subscribers, registry,
		options,
		eventstore.NewEventLoader(registry, creature.NewID, creature.NewEventType),
		eventstore.NewEventLoader(registry, item.NewID, item.NewEventType),
		eventstore.NewEventLoader(registry, spell.NewID, spell.NewEventType),
		eventstore.NewEventLoader(registry, presetdeck.NewID, presetdeck.NewEventType),
		eventstore.NewEventLoader(registry, starterdeckset.NewID, starterdeckset.NewEventType),
	)
}
