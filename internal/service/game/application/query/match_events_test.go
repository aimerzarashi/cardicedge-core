package query_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match/matchtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

// 公開 ID を引く部分だけ動くリゾルバー
type fakeEntityRefs struct {
	domain.EntityRefResolver
	publicIDs map[domain.EntityRef]uuid.UUID
}

func (f fakeEntityRefs) PublicIDs(_ context.Context, refs []domain.EntityRef) (map[domain.EntityRef]uuid.UUID, error) {
	found := map[domain.EntityRef]uuid.UUID{}
	for _, ref := range refs {
		if id, ok := f.publicIDs[ref]; ok {
			found[ref] = id
		}
	}
	return found, nil
}

// 版 n のイベントの、見せる側の共通の項目
func metaAt(version int64) query.MatchEventMeta {
	return query.MatchEventMeta{Revision: version, OccurredAt: progressBase.Add(time.Duration(version) * time.Second)}
}

func eventMeta(t *testing.T, version int64) match.EventMeta {
	return match.EventMeta{EntityID: matchtest.ID(t, 7), AggregateVersion: version, OccurredAt: progressBase.Add(time.Duration(version) * time.Second)}
}

type eventsFixture struct {
	handler *query.MatchEventsHandler
	matchID uuid.UUID
	cardID  uuid.UUID
	viewer  uuid.UUID
}

func newEventsFixture(t *testing.T, events []match.Event) *eventsFixture {
	t.Helper()
	f := &eventsFixture{matchID: uuid.New(), cardID: uuid.New(), viewer: uuid.New()}
	creature := cardtest.CreatureRef(t, 101)
	refs := fakeEntityRefs{publicIDs: map[domain.EntityRef]uuid.UUID{domain.NewEntityRef(creature.EntityType, creature.ID): f.cardID}}
	authorize := func(_ context.Context, playerID uuid.UUID, matchID uuid.UUID) (match.ID, error) {
		if playerID != f.viewer || matchID != f.matchID {
			return 0, query.ErrMatchNotFound
		}
		return matchtest.ID(t, 7), nil
	}
	loadNames := func(_ context.Context, _, _, cardIDs []uuid.UUID) (query.MatchNames, error) {
		require.Equal(t, []uuid.UUID{f.cardID}, cardIDs)
		return query.MatchNames{Cards: map[uuid.UUID]query.MatchCard{f.cardID: {ID: f.cardID, Name: "ドラゴン", Type: query.CardTypeCreature}}}, nil
	}
	f.handler = query.NewMatchEventsHandler(authorize, func(context.Context, match.ID) ([]match.Event, error) { return events, nil }, refs, loadNames)
	return f
}

func sampleEvents(t *testing.T) []match.Event {
	by := matchtest.Index(t, 1)
	return []match.Event{
		&match.CreatedEvent{EventMeta: eventMeta(t, 1)},
		&match.TurnOrderDecidedEvent{EventMeta: eventMeta(t, 2), Order: []match.ParticipantIndex{by, matchtest.Index(t, 0)}},
		&match.DecksShuffledEvent{EventMeta: eventMeta(t, 3), DrawPiles: [][]card.Ref{{cardtest.CreatureRef(t, 101)}}},
		&match.HandsDealtEvent{EventMeta: eventMeta(t, 4), Cards: 5},
		&match.CardDrawnEvent{EventMeta: eventMeta(t, 5), Participant: by, Card: cardtest.CreatureRef(t, 101)},
		&match.MovedEvent{EventMeta: eventMeta(t, 6), Participant: by, To: boardtest.Position(t, 2, 3)},
		&match.MatchEndedEvent{EventMeta: eventMeta(t, 7), Winners: []match.ParticipantIndex{by}, Reason: match.EndReasonRoundLimit},
	}
}

func TestMatchEventsHandler_ReturnsOnlyEventsAfterTheRevisionInOrder(t *testing.T) {
	f := newEventsFixture(t, sampleEvents(t))
	dragon := query.MatchCard{ID: f.cardID, Name: "ドラゴン", Type: query.CardTypeCreature}

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 4)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchCardDrawnEvent{MatchEventMeta: metaAt(5), Participant: 1,
			Card: dragon},
		query.MatchMovedEvent{MatchEventMeta: metaAt(6), Participant: 1, To: query.NewBoardPosition(2, 3)},
		query.MatchEndedEvent{MatchEventMeta: metaAt(7), Winners: []int{1}, Reason: query.MatchEndReasonRoundLimit},
	}, events)
}

// 山札の並びを持たず、種類だけが残る
func TestMatchEventsHandler_ShowsTheStartWithoutTheDeckOrder(t *testing.T) {
	f := newEventsFixture(t, sampleEvents(t)[:4])

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 0)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchCreatedEvent{MatchEventMeta: metaAt(1)},
		query.MatchTurnOrderDecidedEvent{MatchEventMeta: metaAt(2), Order: []int{1, 0}},
		query.MatchDecksShuffledEvent{MatchEventMeta: metaAt(3)},
		query.MatchHandsDealtEvent{MatchEventMeta: metaAt(4), Cards: 5},
	}, events)
}

func TestMatchEventsHandler_ReturnsNothingWhenTheViewerIsUpToDate(t *testing.T) {
	f := newEventsFixture(t, sampleEvents(t))

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 7)

	require.NoError(t, err)
	require.Empty(t, events)
}

func TestMatchEventsHandler_RejectsViewersWhoDoNotParticipate(t *testing.T) {
	f := newEventsFixture(t, sampleEvents(t))

	_, err := f.handler.ListAfter(t.Context(), uuid.New(), f.matchID, 0)

	require.ErrorIs(t, err, query.ErrMatchNotFound)
}
