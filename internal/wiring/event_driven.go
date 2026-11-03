package wiring

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/studiorules"
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
		eventstore.NewEventLoader(registry, grantdeck.NewID, grantdeck.NewEventType),
		eventstore.NewEventLoader(registry, player.NewID, player.NewEventType),
		eventstore.NewEventLoader(registry, startergrant.NewID, startergrant.NewEventType),
		eventstore.NewEventLoader(registry, collection.NewID, collection.NewEventType),
		eventstore.NewEventLoader(registry, deck.NewID, deck.NewEventType),
		eventstore.NewEventLoader(registry, gamerules.NewID, gamerules.NewEventType),
		eventstore.NewEventLoader(registry, avatar.NewID, avatar.NewEventType),
		eventstore.NewEventLoader(registry, board.NewID, board.NewEventType),
		eventstore.NewEventLoader(registry, studiorules.NewID, studiorules.NewEventType),
		eventstore.NewEventLoader(registry, npcavatar.NewID, npcavatar.NewEventType),
		eventstore.NewEventLoader(registry, npcdeck.NewID, npcdeck.NewEventType),
	)
}
