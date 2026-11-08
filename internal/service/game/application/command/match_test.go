package command_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/avatar/avatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board/boardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card/cardtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature/creaturetest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck/decktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules/gamerulestest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match/matchtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar/npcavatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck/npcdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/rules"
)

// 対戦中になった部屋（最後の準備完了まで進めたもの）と、その部屋から対戦を始めるハンドラー
type matchStartFixture struct {
	roomFixture
	room    room.Room
	handler *command.MatchStartHandler
	matches func(t *testing.T) match.State
}

func newMatchStartFixture(t *testing.T) matchStartFixture {
	t.Helper()
	f := matchStartFixture{roomFixture: newRoomFixture(t)}
	current := f.fullyDecided(t)
	published, err := f.roomFixture.handler.Publish(f.owner, current.ID, current.Revision)
	require.NoError(t, err)
	ready, err := f.roomFixture.handler.DeclareReady(f.owner, current.ID, published.Revision)
	require.NoError(t, err)
	f.room = f.load(t, ready.ID)

	matchStore := eventstore.New(f.registry, match.NewID, match.NewEventType, match.InitialState, match.Evolve)
	base := boardtest.Tile(t, 0, 0, "base", "")
	snapshot := match.Board{
		ID: f.room.MatchRules.Board, Size: boardtest.Size(t, 2, 1),
		Tiles:       []board.Tile{base, boardtest.Tile(t, 1, 0, "land", "fire")},
		Connections: []board.Connection{boardtest.Connection(t, 0, 0, 1, 0)},
	}
	f.handler = command.NewMatchStartHandler(
		eventstore.New(f.registry, room.NewID, room.NewEventType, room.InitialState, room.Evolve),
		matchStore,
		func(context.Context) (gamerules.Rules, bool, error) { return gamerules.Defaults(), true, nil },
		func(context.Context, board.ID) (match.Board, error) { return snapshot, nil },
		func(context.Context, deck.ID) ([]master.DeckCard, error) {
			return []master.DeckCard{{Card: cardA, Count: 4}, {Card: cardB, Count: 3}}, nil
		},
		func(context.Context, npcdeck.ID) ([]master.DeckCard, error) {
			return []master.DeckCard{{Card: cardC, Count: 2}}, nil
		},
		func(_ context.Context, refs []card.Ref) (map[card.Ref]master.CardSpec, error) {
			specs := map[card.Ref]master.CardSpec{}
			for _, ref := range refs {
				specs[ref] = cardSpecs[ref]
			}
			return specs, nil
		},
		func() match.Random {
			return matchtest.NewRandom()
		},
		func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	)
	f.matches = func(t *testing.T) match.State {
		t.Helper()
		state, err := matchStore.Load(f.ctx, f.room.Match)
		require.NoError(t, err)
		return state
	}
	return f
}

func TestMatchStart_CreatesMatchWithIDOfRoomEventFromRoomConditions(t *testing.T) {
	f := newMatchStartFixture(t)

	err := f.handler.Handle(f.ctx, &room.MatchStartedEvent{EventMeta: room.EventMeta{EntityID: f.room.ID}, Match: f.room.Match})

	require.NoError(t, err)
	started, ok := f.matches(t).(match.Match)
	require.True(t, ok)
	require.Equal(t, f.room.Match, started.ID)
	require.Len(t, started.Participants, 2)
	require.NotNil(t, started.Participants[0].Participant.Player)
	require.Equal(t, f.room.Owner, started.Participants[0].Participant.Player.Player)
	require.NotNil(t, started.Participants[1].Participant.NPC)
	require.Equal(t, f.room.MatchRules.Board, started.Board.ID)
	require.Equal(t, f.room.MatchRules.TargetMagic, started.Rules.TargetMagic)
	require.Equal(t, gamerules.Defaults().Match.RewardCardsByRank, started.Rules.RewardCardsByRank)
	require.Equal(t, match.DefaultRewardPacks(), started.Rules.RewardPacks)
	// 山札の枚数は、デッキの枚数から配った手札を引いた数
	require.Len(t, started.Participants[0].Hand, 5)
	require.Len(t, started.Participants[0].DrawPile, 2)
	require.Len(t, started.Participants[1].Hand, 2)
	require.Empty(t, started.Participants[1].DrawPile)
}

// 対戦は、始めたときのカードの性能をデッキの写しに持つ（判断と画面が同じ値を使うため）
func TestMatchStart_CopiesCardSpecsIntoParticipantDecks(t *testing.T) {
	f := newMatchStartFixture(t)

	err := f.handler.Handle(f.ctx, &room.MatchStartedEvent{EventMeta: room.EventMeta{EntityID: f.room.ID}, Match: f.room.Match})

	require.NoError(t, err)
	started, ok := f.matches(t).(match.Match)
	require.True(t, ok)
	require.Equal(t, map[card.Ref]match.CardSpec{
		cardA: {Cost: cardtest.Cost(t, 30), Rarity: card.RarityNormal, Creature: &match.CreatureSpec{Elements: creaturetest.Elements(t, "fire"), Attack: creaturetest.Attack(t, 20), HitPoints: creaturetest.HitPoints(t, 30)}},
		cardB: {Cost: cardtest.Cost(t, 0), Rarity: card.RarityStrange, Creature: &match.CreatureSpec{Elements: creaturetest.Elements(t, "water", "earth"), Attack: creaturetest.Attack(t, 0), HitPoints: creaturetest.HitPoints(t, 60)}},
		cardC: {Cost: cardtest.Cost(t, 50), Rarity: card.RarityRare, Creature: &match.CreatureSpec{Elements: creaturetest.Elements(t, "neutral"), Attack: creaturetest.Attack(t, 40), HitPoints: creaturetest.HitPoints(t, 40)}},
	}, started.CardSpecs())
}

func TestMatchStart_FailsWhenTheGameRulesHaveNoRewardCardsByRank(t *testing.T) {
	f := newMatchStartFixture(t)
	old := gamerules.Defaults()
	old.Match.RewardCardsByRank = nil
	handler := command.NewMatchStartHandler(
		eventstore.New(f.registry, room.NewID, room.NewEventType, room.InitialState, room.Evolve),
		eventstore.New(f.registry, match.NewID, match.NewEventType, match.InitialState, match.Evolve),
		func(context.Context) (gamerules.Rules, bool, error) { return old, true, nil },
		nil, nil, nil, nil, nil, nil,
	)

	err := handler.Start(f.ctx, f.room.ID, f.room.Match)

	require.ErrorIs(t, err, rules.ErrNotPublished)
	_, uncreated := f.matches(t).(match.UncreatedMatch)
	require.True(t, uncreated)
}

func TestMatchStart_IsIdempotent(t *testing.T) {
	f := newMatchStartFixture(t)
	event := &room.MatchStartedEvent{EventMeta: room.EventMeta{EntityID: f.room.ID}, Match: f.room.Match}
	require.NoError(t, f.handler.Handle(f.ctx, event))
	before, ok := f.matches(t).(match.Match)
	require.True(t, ok)

	err := f.handler.Handle(f.ctx, event)

	require.NoError(t, err)
	after, ok := f.matches(t).(match.Match)
	require.True(t, ok)
	require.Equal(t, before.Version, after.Version)
}

func TestMatchStart_IgnoresOtherRoomEvents(t *testing.T) {
	f := newMatchStartFixture(t)

	err := f.handler.Handle(f.ctx, &room.PublishedEvent{EventMeta: room.EventMeta{EntityID: f.room.ID}})

	require.NoError(t, err)
	_, uncreated := f.matches(t).(match.UncreatedMatch)
	require.True(t, uncreated)
}

// 混ぜずに席の順のまま並べる（プレイヤーの席が先）乱数の元。サイコロは固定の目、無作為の選びは固定の番号（範囲の中に収める）
type orderedRandom struct {
	// サイコロの目の最小からの差
	roll int
	pick int
}

func (orderedRandom) Shuffle(int, func(i, j int)) {}

func (r orderedRandom) IntN(n int) int {
	if n == 6 {
		return r.roll
	}
	return min(r.pick, n-1)
}

// 分かれ道のあるボード。拠点 (0,0) から (1,0) に進み、(1,0) で (2,0) と (1,1) に分かれる
func forkBoard(t *testing.T) match.Board {
	t.Helper()
	return match.Board{
		ID: boardtest.ID(t, 1), Size: boardtest.Size(t, 3, 2),
		Tiles: []board.Tile{
			boardtest.Tile(t, 0, 0, "base", ""), boardtest.Tile(t, 1, 0, "land", "fire"),
			boardtest.Tile(t, 2, 0, "land", "water"), boardtest.Tile(t, 1, 1, "land", "wind"),
		},
		Connections: []board.Connection{
			boardtest.Connection(t, 0, 0, 1, 0), boardtest.Connection(t, 1, 0, 2, 0), boardtest.Connection(t, 1, 0, 1, 1),
		},
	}
}

type matchTurnFixture struct {
	ctx      context.Context
	player   context.Context
	registry *domaindef.Registry
	handler  *command.MatchHandler
	npc      *command.MatchNPCTurnHandler
	store    domain.EventStore[match.ID, match.Event, match.State]
	id       match.ID
	publicID uuid.UUID
}

// プレイヤー（登録済み）と NPC の 2 人の対戦を始めた状態。山札のあるプレイヤーは手札 6 枚で始まり、引くと上限（6）を超える。
// NPC の山札は手札だけで空なので、NPC の番は引かずに振る段階から始まる
func newMatchTurnFixture(t *testing.T, startRandom match.Random) matchTurnFixture {
	t.Helper()
	return newMatchTurnFixtureWithRoundLimit(t, startRandom, 30)
}

func newMatchTurnFixtureWithRoundLimit(t *testing.T, startRandom match.Random, roundLimit int) matchTurnFixture {
	t.Helper()
	ctx := isolatedDatabase(t)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	player := registeredPlayer(t, ctx, "match-player")
	playerID, ok := authentication.Player(player)
	require.True(t, ok)
	store := eventstore.New(registry, match.NewID, match.NewEventType, match.InitialState, match.Evolve)
	id, publicID, err := identifier.New(registry, match.NewID).Issue(ctx)
	require.NoError(t, err)
	rules := match.NewRules(boardtest.DefaultRules(t, 8000, 600, 300, 100, roundLimit, 1, 6, 100), gamerulestest.MaxHandCards(t, 6), gamerulestest.StartingHandCards(t, 6), gamerulestest.RewardCardsByRank(t, 3, 2, 1, 1), match.DefaultRewardPacks(), gamerules.Defaults().Match.LandLevelMultipliers, gamerules.Defaults().Match.ChainMultipliers, gamerules.Defaults().Match.TollPercentage, gamerules.Defaults().Match.RelinquishPercentage)
	events, err := match.Decide(match.InitialState(), &match.StartCommand{
		ID: id, Board: forkBoard(t), Rules: rules,
		Participants: []match.Participant{
			match.NewPlayerParticipant(playerID, avatartest.ID(t, 1), decktest.ID(t, 1), []match.DeckCard{matchDeckCard(t, 1, 4, 10), matchDeckCard(t, 2, 3, 990)}),
			match.NewNPCParticipant(npcavatartest.ID(t, 1), npcdecktest.ID(t, 1), []match.DeckCard{matchDeckCard(t, 3, 2, 30)}),
		},
	}, time.Now().UTC().Truncate(time.Microsecond), startRandom)
	require.NoError(t, err)
	require.NoError(t, store.Save(ctx, events))
	now := func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }
	// サイコロは 1 を引いて目 2 になり、拠点から (1,0) に進んで分かれ道で止まる
	random := func() match.Random { return orderedRandom{roll: 1, pick: 1} }
	return matchTurnFixture{
		ctx: ctx, player: player, registry: registry, store: store, id: id, publicID: publicID,
		handler: command.NewMatchHandler(store, identifier.New(registry, match.NewID), rewardCandidates(t), random, now),
		npc:     command.NewMatchNPCTurnHandler(store, random, now),
	}
}

// コストの付いたクリーチャー（開始時の魔力は 600 なので、990 は配置できない）
func matchDeckCard(t *testing.T, id int64, quantity int, cost int) match.DeckCard {
	t.Helper()
	spec := match.CardSpec{Cost: cardtest.Cost(t, cost), Rarity: card.RarityNormal, Creature: &match.CreatureSpec{
		Elements: creaturetest.Elements(t, "fire"), Attack: creaturetest.Attack(t, 10), HitPoints: creaturetest.HitPoints(t, 20),
	}}
	return match.DeckCard{Card: cardtest.CreatureRef(t, id), Quantity: cardtest.DeckQuantity(t, quantity), Spec: spec}
}

// 配置できる手札（コスト 10 のカード 1）と、配置できない手札（コスト 990 のカード 2）の番号
func handIndexes(t *testing.T, current match.Match) (deployable, notDeployable int) {
	t.Helper()
	deployable, notDeployable = -1, -1
	for i, ref := range current.Participants[0].Hand {
		switch ref {
		case cardtest.CreatureRef(t, 1):
			deployable = i
		case cardtest.CreatureRef(t, 2):
			notDeployable = i
		}
	}
	require.NotEqual(t, -1, deployable)
	require.NotEqual(t, -1, notDeployable)
	return deployable, notDeployable
}

func (f matchTurnFixture) load(t *testing.T) match.Match {
	t.Helper()
	state, err := f.store.Load(f.ctx, f.id)
	require.NoError(t, err)
	current, ok := state.(match.Match)
	require.True(t, ok)
	return current
}

// プレイヤーの番で、引いて捨てて振り、分かれ道で止まるところまで進める
func (f matchTurnFixture) playUntilChoosingPath(t *testing.T) command.ChangedMatch {
	t.Helper()
	changed, err := f.handler.Draw(f.player, f.publicID, f.load(t).Version)
	require.NoError(t, err)
	changed, err = f.handler.Discard(f.player, f.publicID, changed.Revision, 0)
	require.NoError(t, err)
	changed, err = f.handler.Roll(f.player, f.publicID, changed.Revision)
	require.NoError(t, err)
	require.Equal(t, match.PhaseChoosingPath, f.load(t).Phase)
	return changed
}

func TestMatchHandler_PlaysTheTurnOfThePlayerThroughEveryPhase(t *testing.T) {
	f := newMatchTurnFixture(t, orderedRandom{})
	start := f.load(t)
	require.Equal(t, match.PhaseDrawing, start.Phase)

	changed, err := f.handler.Draw(f.player, f.publicID, start.Version)
	require.NoError(t, err)
	require.Equal(t, command.ChangedMatch{ID: f.publicID, Revision: start.Version + 1}, changed)
	require.Equal(t, match.PhaseDiscarding, f.load(t).Phase)

	changed, err = f.handler.Discard(f.player, f.publicID, changed.Revision, 0)
	require.NoError(t, err)
	require.Equal(t, match.PhaseRolling, f.load(t).Phase)

	changed, err = f.handler.Roll(f.player, f.publicID, changed.Revision)
	require.NoError(t, err)
	rolled := f.load(t)
	require.Equal(t, match.PhaseChoosingPath, rolled.Phase)
	require.Equal(t, 2, rolled.Dice)
	require.Equal(t, rolled.Version, changed.Revision)

	changed, err = f.handler.ChoosePath(f.player, f.publicID, changed.Revision, 2, 0)
	require.NoError(t, err)
	// 止まった (2,0) は空いている土地
	stopped := f.load(t)
	require.Equal(t, match.PhaseDeploying, stopped.Phase)
	deployable, _ := handIndexes(t, stopped)

	_, err = f.handler.Deploy(f.player, f.publicID, changed.Revision, deployable)
	require.NoError(t, err)
	next := f.load(t)
	require.Equal(t, []match.Territory{{Position: boardtest.Position(t, 2, 0), Creature: matchtest.DeployedCreatureID(t, 1), Level: 1}}, next.Territories)
	require.Equal(t, 600-10, next.Participants[0].Magic)
	// 配置したらターンが終わり、NPC の番になる
	require.Equal(t, match.PhaseRolling, next.Phase)
	current, _ := next.CurrentParticipant()
	require.Equal(t, matchtest.Index(t, 1), current)
}

func TestMatchHandler_EndTurnWhileDeployingLeavesTheLandVacant(t *testing.T) {
	f := newMatchTurnFixture(t, orderedRandom{})
	rolled := f.playUntilChoosingPath(t)
	changed, err := f.handler.ChoosePath(f.player, f.publicID, rolled.Revision, 2, 0)
	require.NoError(t, err)

	_, err = f.handler.EndTurn(f.player, f.publicID, changed.Revision)

	require.NoError(t, err)
	next := f.load(t)
	require.Empty(t, next.Territories)
	require.Equal(t, 600, next.Participants[0].Magic)
}

func TestMatchHandler_RejectsOperationsOfOtherThanTheParticipantsInTurn(t *testing.T) {
	t.Run("not_your_turn", func(t *testing.T) {
		f := newMatchTurnFixture(t, matchtest.NewRandom())

		_, err := f.handler.Draw(f.player, f.publicID, f.load(t).Version)

		require.ErrorIs(t, err, command.ErrMatchNotYourTurn)
	})
	t.Run("phase_not_allowed", func(t *testing.T) {
		f := newMatchTurnFixture(t, orderedRandom{})

		_, err := f.handler.Roll(f.player, f.publicID, f.load(t).Version)

		require.ErrorIs(t, err, command.ErrMatchPhaseNotAllowed)
	})
	t.Run("stale_revision", func(t *testing.T) {
		f := newMatchTurnFixture(t, orderedRandom{})

		_, err := f.handler.Draw(f.player, f.publicID, f.load(t).Version-1)

		require.ErrorIs(t, err, command.ErrRevisionConflict)
		require.Equal(t, match.PhaseDrawing, f.load(t).Phase)
	})
	t.Run("not_participating", func(t *testing.T) {
		f := newMatchTurnFixture(t, orderedRandom{})
		stranger := registeredPlayer(t, f.ctx, "match-stranger")

		_, err := f.handler.Draw(stranger, f.publicID, f.load(t).Version)

		require.ErrorIs(t, err, command.ErrMatchNotFound)
	})
	t.Run("unknown_match", func(t *testing.T) {
		f := newMatchTurnFixture(t, orderedRandom{})

		_, err := f.handler.Draw(f.player, uuid.New(), 1)

		require.ErrorIs(t, err, command.ErrMatchNotFound)
	})
	t.Run("player_not_registered", func(t *testing.T) {
		f := newMatchTurnFixture(t, orderedRandom{})

		_, err := f.handler.Draw(f.ctx, f.publicID, f.load(t).Version)

		require.ErrorIs(t, err, command.ErrPlayerNotRegistered)
	})
}

func TestMatchHandler_ReportsInputThatIsNotInTheRange(t *testing.T) {
	t.Run("hand_index_out_of_range", func(t *testing.T) {
		f := newMatchTurnFixture(t, orderedRandom{})
		drawn, err := f.handler.Draw(f.player, f.publicID, f.load(t).Version)
		require.NoError(t, err)

		_, err = f.handler.Discard(f.player, f.publicID, drawn.Revision, 7)

		invalid, ok := input.AsErrors(err)
		require.True(t, ok)
		require.Equal(t, input.Errors{&input.OutOfRange{Field: "handIndex", Min: 0, Max: 6}}, invalid)
		require.Len(t, f.load(t).Participants[0].Hand, 7)
	})
	t.Run("path_not_selectable", func(t *testing.T) {
		f := newMatchTurnFixture(t, orderedRandom{})
		rolled := f.playUntilChoosingPath(t)

		// (0,0) は来たマスなので選べない
		_, err := f.handler.ChoosePath(f.player, f.publicID, rolled.Revision, 0, 0)

		invalid, ok := input.AsErrors(err)
		require.True(t, ok)
		require.Equal(t, input.Errors{&input.NotFound{Field: "to"}}, invalid)
		require.Equal(t, match.PhaseChoosingPath, f.load(t).Phase)
	})
	t.Run("creature_not_deployable", func(t *testing.T) {
		f := newMatchTurnFixture(t, orderedRandom{})
		rolled := f.playUntilChoosingPath(t)
		changed, err := f.handler.ChoosePath(f.player, f.publicID, rolled.Revision, 2, 0)
		require.NoError(t, err)
		_, notDeployable := handIndexes(t, f.load(t))

		_, err = f.handler.Deploy(f.player, f.publicID, changed.Revision, notDeployable)

		invalid, ok := input.AsErrors(err)
		require.True(t, ok)
		require.Equal(t, input.Errors{&input.NotFound{Field: "handIndex"}}, invalid)
		require.Empty(t, f.load(t).Territories)
	})
	t.Run("deploy_hand_index_out_of_range", func(t *testing.T) {
		f := newMatchTurnFixture(t, orderedRandom{})
		rolled := f.playUntilChoosingPath(t)
		changed, err := f.handler.ChoosePath(f.player, f.publicID, rolled.Revision, 2, 0)
		require.NoError(t, err)

		_, err = f.handler.Deploy(f.player, f.publicID, changed.Revision, 99)

		invalid, ok := input.AsErrors(err)
		require.True(t, ok)
		require.Equal(t, input.Errors{&input.OutOfRange{Field: "handIndex", Min: 0, Max: 5}}, invalid)
	})
	t.Run("not_your_turn_comes_before_range", func(t *testing.T) {
		f := newMatchTurnFixture(t, matchtest.NewRandom())

		_, err := f.handler.Discard(f.player, f.publicID, f.load(t).Version, 99)

		require.ErrorIs(t, err, command.ErrMatchNotYourTurn)
	})
}

func TestMatchNPCTurn_PlaysOneOperationPerPhaseOfTheNPCTurn(t *testing.T) {
	f := newMatchTurnFixture(t, matchtest.NewRandom())
	start := f.load(t)
	require.Equal(t, match.PhaseRolling, start.Phase)

	require.NoError(t, f.npc.Play(f.ctx, f.id, start.Version))
	rolled := f.load(t)
	require.Equal(t, match.PhaseChoosingPath, rolled.Phase)
	require.Equal(t, 2, rolled.Dice)

	require.NoError(t, f.npc.Play(f.ctx, f.id, rolled.Version))
	chosen := f.load(t)
	// 無作為の選びは固定の番号（1）なので、選べる行き先の 2 番目。止まった (1,1) は空いている土地
	require.Equal(t, boardtest.Position(t, 1, 1), chosen.Participants[1].Position)
	require.Equal(t, match.PhaseDeploying, chosen.Phase)

	// 払える最初のクリーチャーを配置し、そのままターンを終える
	require.NoError(t, f.npc.Play(f.ctx, f.id, chosen.Version))
	ended := f.load(t)
	require.Len(t, ended.Territories, 1)
	require.Equal(t, boardtest.Position(t, 1, 1), ended.Territories[0].Position)
	require.Equal(t, matchtest.Index(t, 1), ended.DeployedCreatures[0].Owner)
	require.Equal(t, match.PhaseDrawing, ended.Phase)
	current, _ := ended.CurrentParticipant()
	require.Equal(t, matchtest.Index(t, 0), current)
}

func TestMatchNPCTurn_DoesNothingWhenItIsNotTheTurnOfAnNPCOrTheEventIsOld(t *testing.T) {
	t.Run("turn_of_player", func(t *testing.T) {
		f := newMatchTurnFixture(t, orderedRandom{})
		start := f.load(t)

		require.NoError(t, f.npc.Play(f.ctx, f.id, start.Version))

		require.Equal(t, start.Version, f.load(t).Version)
	})
	t.Run("old_event", func(t *testing.T) {
		f := newMatchTurnFixture(t, matchtest.NewRandom())
		start := f.load(t)

		require.NoError(t, f.npc.Play(f.ctx, f.id, start.Version-1))

		require.Equal(t, start.Version, f.load(t).Version)
	})
	t.Run("ended_match", func(t *testing.T) {
		f := newMatchTurnFixture(t, matchtest.NewRandom())
		start := f.load(t)
		meta := match.EventMeta{EntityID: f.id, AggregateVersion: start.Version + 1, OccurredAt: time.Now().UTC().Truncate(time.Microsecond)}
		ended := &match.MatchEndedEvent{EventMeta: meta, Winners: []match.ParticipantIndex{matchtest.Index(t, 0)}, Ranks: []int{1, 2}, Reason: match.EndReasonRoundLimit}
		require.NoError(t, f.store.Save(f.ctx, []match.Event{ended}))

		require.NoError(t, f.npc.Play(f.ctx, f.id, start.Version+1))

		require.Equal(t, start.Version+1, f.load(t).Version)
	})
	t.Run("event_of_another_aggregate", func(t *testing.T) {
		f := newMatchTurnFixture(t, matchtest.NewRandom())
		start := f.load(t)

		require.NoError(t, f.npc.Handle(f.ctx, struct{}{}))

		require.Equal(t, start.Version, f.load(t).Version)
	})
}

// 保存の版の衝突に当たる = ほかが先に進めた、として成功で返す
type conflictingStore struct {
	domain.EventStore[match.ID, match.Event, match.State]
}

func (conflictingStore) Save(context.Context, []match.Event) error {
	return domain.ErrVersionConflict
}

func TestMatchNPCTurn_TreatsVersionConflictOfSaveAsSomeoneElseMovedOn(t *testing.T) {
	f := newMatchTurnFixture(t, matchtest.NewRandom())
	start := f.load(t)
	handler := command.NewMatchNPCTurnHandler(conflictingStore{f.store}, func() match.Random { return orderedRandom{roll: 1} }, time.Now)

	require.NoError(t, handler.Play(f.ctx, f.id, start.Version))

	require.Equal(t, start.Version, f.load(t).Version)
}

func TestMatchNPCTurn_HandleReadsEntityAndVersionFromTheEvent(t *testing.T) {
	f := newMatchTurnFixture(t, matchtest.NewRandom())
	start := f.load(t)

	event := &match.HandsDealtEvent{EventMeta: match.EventMeta{EntityID: f.id, AggregateVersion: start.Version}}
	require.NoError(t, f.npc.Handle(f.ctx, event))

	require.Equal(t, match.PhaseChoosingPath, f.load(t).Phase)
}
