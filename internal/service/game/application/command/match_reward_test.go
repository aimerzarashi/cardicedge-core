package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection/collectiontest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match/matchtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant/startergranttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
)

// 公開中のカードの代わりに返す 5 枚（クリーチャー 11〜15）
func rewardCandidates(t *testing.T) master.GetPublishedCardsFunc {
	t.Helper()
	candidates := []card.Ref{
		cardtest.CreatureRef(t, 11), cardtest.CreatureRef(t, 12), cardtest.CreatureRef(t, 13), cardtest.CreatureRef(t, 14), cardtest.CreatureRef(t, 15),
	}
	return func(context.Context) ([]card.Ref, error) { return candidates, nil }
}

// ラウンドの上限を 1 にして、プレイヤー（先に動く）と NPC が 1 ターンずつ終えた、終わった対戦。魔力は同じなので、どちらも 1 位
func newEndedMatchFixture(t *testing.T) matchTurnFixture {
	t.Helper()
	f := newMatchTurnFixtureWithRoundLimit(t, orderedRandom{}, 1)
	changed := f.playUntilChoosingPath(t)
	changed, err := f.handler.ChoosePath(f.player, f.publicID, changed.Revision, 2, 0)
	require.NoError(t, err)
	changed, err = f.handler.EndTurn(f.player, f.publicID, changed.Revision)
	require.NoError(t, err)
	// 引いた分で手札が上限を超えているので、ターンの終わりに 1 枚捨てる
	if f.load(t).Phase == match.PhaseDiscarding {
		_, err = f.handler.Discard(f.player, f.publicID, changed.Revision, 0)
		require.NoError(t, err)
	}
	// NPC の番が進まなくなったときに、時間切れまで回り続けないように回数を区切る
	for i := 0; f.load(t).Phase != match.PhaseEnded; i++ {
		require.Less(t, i, 100)
		require.NoError(t, f.npc.Play(f.ctx, f.id, f.load(t).Version))
	}
	return f
}

func TestMatchHandler_ChooseRewardPackDrawsTheCardsOfTheRankFromTheCandidates(t *testing.T) {
	f := newEndedMatchFixture(t)
	ended := f.load(t)

	changed, err := f.handler.ChooseRewardPack(f.player, f.publicID, ended.Version, string(match.DefaultPackKey))

	require.NoError(t, err)
	after := f.load(t)
	require.Equal(t, after.Version, changed.Revision)
	reward := after.Participants[0].Reward
	require.NotNil(t, reward)
	require.Equal(t, match.DefaultPackKey, reward.Pack)
	// NPC が配置した領地の価値の分、総魔力で NPC が上になり、プレイヤーは 2 位の枚数を引く
	require.Equal(t, []int{2, 1}, ended.Result.Ranks)
	require.Len(t, reward.Cards, 2)
	candidates, err := rewardCandidates(t)(f.ctx)
	require.NoError(t, err)
	for _, drawn := range reward.Cards {
		require.Contains(t, candidates, drawn)
	}
	require.True(t, after.RewardsCompleted)
	require.Equal(t, match.PhaseEnded, after.Phase)
}

func TestMatchHandler_ChooseRewardPackRejectsWhatCannotBeDone(t *testing.T) {
	t.Run("before_the_match_ended", func(t *testing.T) {
		f := newMatchTurnFixture(t, orderedRandom{})

		_, err := f.handler.ChooseRewardPack(f.player, f.publicID, f.load(t).Version, string(match.DefaultPackKey))

		require.ErrorIs(t, err, command.ErrMatchPhaseNotAllowed)
	})
	t.Run("already_received", func(t *testing.T) {
		f := newEndedMatchFixture(t)
		first, err := f.handler.ChooseRewardPack(f.player, f.publicID, f.load(t).Version, string(match.DefaultPackKey))
		require.NoError(t, err)

		_, err = f.handler.ChooseRewardPack(f.player, f.publicID, first.Revision, string(match.DefaultPackKey))

		require.ErrorIs(t, err, command.ErrMatchRewardAlreadyReceived)
	})
	t.Run("pack_that_cannot_be_chosen", func(t *testing.T) {
		f := newEndedMatchFixture(t)

		_, err := f.handler.ChooseRewardPack(f.player, f.publicID, f.load(t).Version, "unknown")

		invalid, ok := input.AsErrors(err)
		require.True(t, ok)
		require.Equal(t, input.Errors{&input.NotFound{Field: "pack"}}, invalid)
		require.Nil(t, f.load(t).Participants[0].Reward)
	})
	t.Run("blank_pack", func(t *testing.T) {
		f := newEndedMatchFixture(t)

		_, err := f.handler.ChooseRewardPack(f.player, f.publicID, f.load(t).Version, "")

		invalid, ok := input.AsErrors(err)
		require.True(t, ok)
		require.Equal(t, input.Errors{&input.NotFound{Field: "pack"}}, invalid)
	})
	t.Run("stale_revision", func(t *testing.T) {
		f := newEndedMatchFixture(t)

		_, err := f.handler.ChooseRewardPack(f.player, f.publicID, f.load(t).Version-1, string(match.DefaultPackKey))

		require.ErrorIs(t, err, command.ErrRevisionConflict)
	})
	t.Run("not_participating", func(t *testing.T) {
		f := newEndedMatchFixture(t)
		stranger := registeredPlayer(t, f.ctx, "match-stranger")

		_, err := f.handler.ChooseRewardPack(stranger, f.publicID, f.load(t).Version, string(match.DefaultPackKey))

		require.ErrorIs(t, err, command.ErrMatchNotFound)
	})
}

type matchRewardFixture struct {
	matchTurnFixture
	collections *command.CollectionHandler
	policy      *command.MatchRewardHandler
	playerID    int64
}

func newMatchRewardFixture(t *testing.T) matchRewardFixture {
	t.Helper()
	f := newEndedMatchFixture(t)
	collections := command.NewCollectionHandler(
		eventstore.New(f.registry, collection.NewID, collection.NewEventType, collection.InitialState, collection.Evolve),
		identifier.New(f.registry, collection.NewID),
		constraint.UpdateCollectionPlayer,
		constraint.FindCollectionByPlayer,
		func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	)
	playerID, ok := authentication.Player(f.player)
	require.True(t, ok)
	return matchRewardFixture{matchTurnFixture: f, collections: collections, policy: command.NewMatchRewardHandler(f.store, collections), playerID: playerID.Int64()}
}

func (f matchRewardFixture) drawn(t *testing.T) *match.RewardDrawnEvent {
	t.Helper()
	return &match.RewardDrawnEvent{
		EventMeta:   match.EventMeta{EntityID: f.id, AggregateVersion: f.load(t).Version + 1},
		Participant: matchtest.Index(t, 0), Pack: match.DefaultPackKey, Cards: []card.Ref{cardtest.CreatureRef(t, 11), cardtest.CreatureRef(t, 12)},
	}
}

func (f matchRewardFixture) loadCollection(t *testing.T) collection.Collection {
	t.Helper()
	playerID, ok := authentication.Player(f.player)
	require.True(t, ok)
	id, found, err := constraint.FindCollectionByPlayer(f.ctx, playerID)
	require.NoError(t, err)
	require.True(t, found)
	state, err := eventstore.New(f.registry, collection.NewID, collection.NewEventType, collection.InitialState, collection.Evolve).Load(f.ctx, id)
	require.NoError(t, err)
	return state.(collection.Collection)
}

func (f matchRewardFixture) createCollection(t *testing.T) {
	t.Helper()
	playerID, ok := authentication.Player(f.player)
	require.True(t, ok)
	require.NoError(t, f.collections.Create(f.ctx, playerID, startergranttest.ID(t, 1), []collection.Card{collectiontest.Card(t, cardtest.CreatureRef(t, 11), 1)}))
}

func TestMatchReward_AddsTheDrawnCardsToTheCollectionOfThePlayerWithItsSource(t *testing.T) {
	f := newMatchRewardFixture(t)
	f.createCollection(t)

	require.NoError(t, f.policy.Handle(f.ctx, f.drawn(t)))

	got := f.loadCollection(t)
	require.Equal(t, int64(2), got.Version)
	require.Equal(t, []match.ID{f.id}, got.RewardedMatches)
}

func TestMatchReward_AddsOnlyOnceEvenIfItRunsTwice(t *testing.T) {
	f := newMatchRewardFixture(t)
	f.createCollection(t)
	event := f.drawn(t)
	require.NoError(t, f.policy.Handle(f.ctx, event))

	require.NoError(t, f.policy.Handle(f.ctx, event))

	require.Equal(t, int64(2), f.loadCollection(t).Version)
}

func TestMatchReward_WaitsForTheCollectionToBeCreated(t *testing.T) {
	f := newMatchRewardFixture(t)

	err := f.policy.Handle(f.ctx, f.drawn(t))

	require.ErrorIs(t, err, command.ErrCollectionNotCreated)
}

func TestMatchReward_DoesNothingForOtherMatchEvents(t *testing.T) {
	f := newMatchRewardFixture(t)
	f.createCollection(t)

	require.NoError(t, f.policy.Handle(f.ctx, &match.AllRewardsReceivedEvent{EventMeta: match.EventMeta{EntityID: f.id}}))

	require.Equal(t, int64(1), f.loadCollection(t).Version)
}

func TestMatchReward_RejectsParticipantThatIsNotAPlayer(t *testing.T) {
	f := newMatchRewardFixture(t)
	event := f.drawn(t)
	event.Participant = matchtest.Index(t, 1)

	err := f.policy.Handle(f.ctx, event)

	require.ErrorIs(t, err, match.ErrInvalidState)
}
