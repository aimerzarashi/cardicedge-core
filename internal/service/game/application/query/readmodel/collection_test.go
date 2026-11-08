package readmodel_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection/collectiontest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match/matchtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant/startergranttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

func TestCollection_EvolvesFromCreated(t *testing.T) {
	cardA, cardB := cardtest.CreatureRef(t, 11), cardtest.Ref(t, card.ItemEntityType, 12)
	occurredAt := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	event := &collection.CreatedEvent{
		EventMeta:    collection.EventMeta{EntityID: collectiontest.ID(t, 3), AggregateVersion: 1, OccurredAt: occurredAt},
		Player:       playertest.ID(t, 7),
		Cards:        []collection.Card{collectiontest.Card(t, cardA, 4), collectiontest.Card(t, cardB, 1)},
		StarterGrant: startergranttest.ID(t, 5),
	}

	state := readmodel.Collection{}.Evolve(event)

	require.Equal(t, readmodel.Collection{
		Collection:   collectiontest.ID(t, 3),
		Player:       playertest.ID(t, 7),
		Cards:        []readmodel.CollectionCard{{Card: cardA, Count: 4}, {Card: cardB, Count: 1}},
		RegisteredAt: occurredAt,
		UpdatedAt:    occurredAt,
	}, state)
	// 公開 ID・カード・操作した人は、イベントが内部 ID で持つため空のまま（書くときに repository が入れる）
	require.Equal(t, readmodel.CollectionMetadata{RegisteredAt: occurredAt, UpdatedAt: occurredAt}, state.Metadata())
	require.False(t, state.IsTerminal())
}

// 同じカードは 1 つにまとめて枚数を足し、新しいカードは後ろに足す（初めて持った順）
func TestCollection_AcquiredMergesCountsOfSameCard(t *testing.T) {
	cardA, cardB, cardC := cardtest.CreatureRef(t, 11), cardtest.Ref(t, card.ItemEntityType, 12), cardtest.Ref(t, card.SpellEntityType, 13)
	occurredAt := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	laterAt := occurredAt.Add(time.Hour)
	created := &collection.CreatedEvent{
		EventMeta:    collection.EventMeta{EntityID: collectiontest.ID(t, 3), AggregateVersion: 1, OccurredAt: occurredAt},
		Player:       playertest.ID(t, 7),
		Cards:        []collection.Card{collectiontest.Card(t, cardA, 4), collectiontest.Card(t, cardB, 1)},
		StarterGrant: startergranttest.ID(t, 5),
	}
	acquired := &collection.CardsAcquiredEvent{
		EventMeta: collection.EventMeta{EntityID: collectiontest.ID(t, 3), AggregateVersion: 2, OccurredAt: laterAt},
		Cards:     []collection.Card{collectiontest.Card(t, cardB, 2), collectiontest.Card(t, cardC, 1), collectiontest.Card(t, cardB, 1)},
		Source:    collection.NewMatchRewardSource(matchtest.ID(t, 9)),
	}
	before := readmodel.Collection{}.Evolve(created)

	state := before.Evolve(acquired)

	require.Equal(t, []readmodel.CollectionCard{{Card: cardA, Count: 4}, {Card: cardB, Count: 4}, {Card: cardC, Count: 1}}, state.Cards)
	require.Equal(t, occurredAt, state.RegisteredAt)
	require.Equal(t, laterAt, state.UpdatedAt)
	// 渡した状態の枚数は変わらない
	require.Equal(t, []readmodel.CollectionCard{{Card: cardA, Count: 4}, {Card: cardB, Count: 1}}, before.Cards)
}

// 購読していない集約のイベントは状態を変えない
func TestCollection_IgnoresOtherAggregateEvents(t *testing.T) {
	state := readmodel.Collection{UpdatedAt: time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)}

	evolved := state.Evolve(&player.RegisteredEvent{EventMeta: player.EventMeta{EntityID: playertest.ID(t, 1), AggregateVersion: 1}})

	require.Equal(t, state, evolved)
}
