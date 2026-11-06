package readmodel_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck/decktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant/startergranttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

func TestDeck_EvolvesFromCreated(t *testing.T) {
	cardA, cardB := cardtest.CreatureRef(t, 11), cardtest.Ref(t, card.ItemEntityType, 12)
	occurredAt := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	event := &deck.CreatedEvent{
		EventMeta:    deck.EventMeta{EntityID: decktest.ID(t, 3), AggregateVersion: 1, OccurredAt: occurredAt},
		Player:       playertest.ID(t, 7),
		Name:         decktest.Name(t, "火"),
		Cards:        []deck.Card{decktest.Card(t, cardA, 4), decktest.Card(t, cardB, 1)},
		StarterGrant: startergranttest.ID(t, 5),
		Number:       startergranttest.DeckNumber(t, 1),
	}

	state := readmodel.Deck{}.Evolve(event)

	require.Equal(t, readmodel.Deck{Deck: decktest.ID(t, 3), Player: playertest.ID(t, 7), Name: "火", Cards: event.Cards, RegisteredAt: occurredAt}, state)
	// 公開 ID・カード・操作した人は、イベントが内部 ID で持つため空のまま（書くときに repository が入れる）
	require.Equal(t, readmodel.DeckMetadata{Name: "火", RegisteredAt: occurredAt, UpdatedAt: occurredAt}, state.Metadata())
	require.False(t, state.IsTerminal())
}

// 購読していない集約のイベントは状態を変えない
func TestDeck_IgnoresOtherAggregateEvents(t *testing.T) {
	state := readmodel.Deck{Name: "火"}

	evolved := state.Evolve(&player.RegisteredEvent{EventMeta: player.EventMeta{EntityID: playertest.ID(t, 1), AggregateVersion: 1}})

	require.Equal(t, state, evolved)
}
