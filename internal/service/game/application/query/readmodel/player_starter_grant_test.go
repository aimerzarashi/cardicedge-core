package readmodel_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant/startergranttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

func TestPlayerStarterGrant_EvolvesStatusThroughGrant(t *testing.T) {
	playerID := playertest.ID(t, 7)
	meta := func(version int64) startergrant.EventMeta {
		return startergrant.EventMeta{EntityID: startergranttest.ID(t, 1), AggregateVersion: version}
	}
	started := &startergrant.StartedEvent{EventMeta: meta(1), Player: playerID, Cards: []startergrant.CollectionCard{}, Decks: []startergrant.Deck{}}
	collection := &startergrant.CollectionRecordedEvent{EventMeta: meta(2)}
	deck := &startergrant.DeckRecordedEvent{EventMeta: meta(3), Number: startergranttest.DeckNumber(t, 1)}
	completed := &startergrant.CompletedEvent{EventMeta: meta(4)}

	tests := []struct {
		name       string
		events     []any
		wantStatus readmodel.StarterGrantStatus
	}{
		{name: "started", events: []any{started}, wantStatus: readmodel.StarterGrantInProgress},
		{name: "recorded", events: []any{started, collection, deck}, wantStatus: readmodel.StarterGrantInProgress},
		{name: "completed", events: []any{started, collection, deck, completed}, wantStatus: readmodel.StarterGrantCompleted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var state readmodel.PlayerStarterGrant
			for _, event := range tt.events {
				state = state.Evolve(event)
			}

			require.Equal(t, readmodel.PlayerStarterGrant{Player: playerID, Status: tt.wantStatus}, state)
			require.False(t, state.IsTerminal())
		})
	}
}
