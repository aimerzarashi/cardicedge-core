package query_test

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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature/creaturetest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck/decktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match/matchtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar/npcavatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck/npcdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
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
func metaAt(version int64, changes query.MatchEventChanges) query.MatchEventMeta {
	return query.MatchEventMeta{Revision: version, OccurredAt: progressBase.Add(time.Duration(version) * time.Second), Changes: changes}
}

func phaseOf(phase query.MatchPhase) *query.MatchPhase {
	return &phase
}

func total(participant, totalMagic int) query.MatchTotalMagicChange {
	return query.MatchTotalMagicChange{Participant: participant, TotalMagic: totalMagic}
}

func changesOf(phase *query.MatchPhase, totals []query.MatchTotalMagicChange, values ...query.MatchTerritoryValue) query.MatchEventChanges {
	if totals == nil {
		totals = []query.MatchTotalMagicChange{}
	}
	if values == nil {
		values = []query.MatchTerritoryValue{}
	}
	return query.MatchEventChanges{Phase: phase, TotalMagics: totals, TerritoryValues: values}
}

func territoryValue(x, value, toll int) query.MatchTerritoryValue {
	return query.MatchTerritoryValue{Position: query.NewBoardPosition(x, 0), Value: value, Toll: toll}
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

// 拠点 (0,0) から火属性の土地が 3 つ続く 1 本道
func lineBoard(t *testing.T) match.Board {
	t.Helper()
	tiles := []board.Tile{boardtest.Tile(t, 0, 0, "base", "")}
	var connections []board.Connection
	for x := 1; x <= 3; x++ {
		tiles = append(tiles, boardtest.Tile(t, x, 0, "land", "fire"))
		connections = append(connections, boardtest.Connection(t, x-1, 0, x, 0))
	}
	return match.Board{ID: boardtest.ID(t, 5), Size: boardtest.Size(t, 4, 1), Tiles: tiles, Connections: connections}
}

func dragonCard(t *testing.T) []match.DeckCard {
	t.Helper()
	spec := match.CardSpec{
		Cost: cardtest.Cost(t, 10), Rarity: card.RarityNormal,
		Creature: &match.CreatureSpec{Elements: creature.DefaultElements(), Attack: creaturetest.Attack(t, 10), HitPoints: creaturetest.HitPoints(t, 20)},
	}
	return []match.DeckCard{{Card: cardtest.CreatureRef(t, 101), Quantity: cardtest.DeckQuantity(t, 3), Spec: spec}}
}

// 対戦で写した性能のドラゴン
func deployedDragon(id uuid.UUID) query.MatchCard {
	return query.MatchCard{
		ID: id, Name: "ドラゴン", Type: query.CardTypeCreature, Cost: 10, Rarity: "normal",
		Creature: &query.CreatureSpec{Elements: []string{"neutral"}, Attack: 10, HitPoints: 20},
	}
}

// NPC（参加者 1）が先の番で、引いて 2 歩進み、止まった土地に配置して終わる対戦
func sampleEvents(t *testing.T) []match.Event {
	t.Helper()
	by := matchtest.Index(t, 1)
	dragon := cardtest.CreatureRef(t, 101)
	return []match.Event{
		&match.CreatedEvent{EventMeta: eventMeta(t, 1), Board: lineBoard(t), Rules: matchtest.Rules(t, 1, 6, 1), Participants: []match.Participant{
			match.NewPlayerParticipant(playertest.ID(t, 10), avatartest.ID(t, 1), decktest.ID(t, 2), dragonCard(t)),
			match.NewNPCParticipant(npcavatartest.ID(t, 7), npcdecktest.ID(t, 8), dragonCard(t)),
		}},
		&match.TurnOrderDecidedEvent{EventMeta: eventMeta(t, 2), Order: []match.ParticipantIndex{by, matchtest.Index(t, 0)}},
		&match.DecksShuffledEvent{EventMeta: eventMeta(t, 3), DrawPiles: [][]card.Ref{{dragon, dragon, dragon}, {dragon, dragon, dragon}}},
		&match.HandsDealtEvent{EventMeta: eventMeta(t, 4), Cards: 1},
		&match.CardDrawnEvent{EventMeta: eventMeta(t, 5), Participant: by, Card: dragon},
		&match.DiceRolledEvent{EventMeta: eventMeta(t, 6), Participant: by, Value: 2},
		&match.MovedEvent{EventMeta: eventMeta(t, 7), Participant: by, To: boardtest.Position(t, 1, 0)},
		&match.MovedEvent{EventMeta: eventMeta(t, 8), Participant: by, To: boardtest.Position(t, 2, 0)},
		&match.CreatureDeployedEvent{EventMeta: eventMeta(t, 9), Participant: by, Card: dragon, Creature: matchtest.DeployedCreatureID(t, 1), Position: boardtest.Position(t, 2, 0), Cost: cardtest.Cost(t, 10)},
		&match.MatchEndedEvent{EventMeta: eventMeta(t, 10), Winners: []match.ParticipantIndex{by}, Ranks: []int{2, 1}, Reason: match.EndReasonRoundLimit},
	}
}

func TestMatchEventsHandler_ReturnsOnlyEventsAfterTheRevisionInOrder(t *testing.T) {
	f := newEventsFixture(t, sampleEvents(t))
	dragon := deployedDragon(f.cardID)

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 4)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchCardDrawnEvent{MatchEventMeta: metaAt(5, changesOf(phaseOf(query.MatchPhaseRolling), nil)), Participant: 1, Card: dragon},
		query.MatchDiceRolledEvent{MatchEventMeta: metaAt(6, changesOf(nil, nil)), Participant: 1, Value: 2, Dice: []int{}},
		query.MatchMovedEvent{MatchEventMeta: metaAt(7, changesOf(nil, nil)), Participant: 1, To: query.NewBoardPosition(1, 0)},
		query.MatchMovedEvent{MatchEventMeta: metaAt(8, changesOf(phaseOf(query.MatchPhaseDeploying), nil)), Participant: 1, To: query.NewBoardPosition(2, 0)},
		query.MatchCreatureDeployedEvent{
			MatchEventMeta: metaAt(9, changesOf(phaseOf(query.MatchPhaseEnding), []query.MatchTotalMagicChange{total(1, 690)}, territoryValue(2, 100, 60))),
			Participant:    1, Card: dragon, Creature: 1, Position: query.NewBoardPosition(2, 0), Cost: 10,
		},
		query.MatchEndedEvent{MatchEventMeta: metaAt(10, changesOf(phaseOf(query.MatchPhaseEnded), nil)), Winners: []int{1}, Ranks: []int{2, 1}, Reason: query.MatchEndReasonRoundLimit},
	}, events)
}

// 山札の並びを持たず、種類だけが残る。段階が決まるのは配り終えてから
func TestMatchEventsHandler_ShowsTheStartWithoutTheDeckOrder(t *testing.T) {
	f := newEventsFixture(t, sampleEvents(t)[:4])

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 0)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchCreatedEvent{MatchEventMeta: metaAt(1, changesOf(nil, []query.MatchTotalMagicChange{total(0, 600), total(1, 600)}))},
		query.MatchTurnOrderDecidedEvent{MatchEventMeta: metaAt(2, changesOf(nil, nil)), Order: []int{1, 0}},
		query.MatchDecksShuffledEvent{MatchEventMeta: metaAt(3, changesOf(nil, nil))},
		query.MatchHandsDealtEvent{MatchEventMeta: metaAt(4, changesOf(phaseOf(query.MatchPhaseDrawing), nil)), Cards: 1},
	}, events)
}

// 配置で連鎖が増えると、前からある領地の価値と通行料も、その版の変わった値に入る
func TestMatchEventsHandler_ChangesIncludeEarlierTerritoriesWhoseChainGrew(t *testing.T) {
	by := matchtest.Index(t, 1)
	dragon := cardtest.CreatureRef(t, 101)
	f := newEventsFixture(t, append(sampleEvents(t)[:9],
		&match.TurnEndedEvent{EventMeta: eventMeta(t, 10), Participant: by},
		&match.TurnEndedEvent{EventMeta: eventMeta(t, 11), Participant: matchtest.Index(t, 0)},
		&match.DiceRolledEvent{EventMeta: eventMeta(t, 12), Participant: by, Value: 1},
		&match.MovedEvent{EventMeta: eventMeta(t, 13), Participant: by, To: boardtest.Position(t, 3, 0)},
		&match.CreatureDeployedEvent{EventMeta: eventMeta(t, 14), Participant: by, Card: dragon, Creature: matchtest.DeployedCreatureID(t, 2), Position: boardtest.Position(t, 3, 0), Cost: cardtest.Cost(t, 10)},
	))

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 13)

	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, changesOf(phaseOf(query.MatchPhaseEnding), []query.MatchTotalMagicChange{total(1, 880)}, territoryValue(2, 150, 90), territoryValue(3, 150, 90)), events[0].Meta().Changes)
}

// 別の属性の領地ができても、連鎖が変わらない領地は入らない
func TestMatchEventsHandler_ChangesLeaveOutTerritoriesThatDidNotChange(t *testing.T) {
	by := matchtest.Index(t, 1)
	dragon := cardtest.CreatureRef(t, 101)
	events := sampleEvents(t)[:9]
	lined := lineBoard(t)
	lined.Tiles[3] = boardtest.Tile(t, 3, 0, "land", "water")
	events[0].(*match.CreatedEvent).Board = lined
	f := newEventsFixture(t, append(events,
		&match.TurnEndedEvent{EventMeta: eventMeta(t, 10), Participant: by},
		&match.TurnEndedEvent{EventMeta: eventMeta(t, 11), Participant: matchtest.Index(t, 0)},
		&match.DiceRolledEvent{EventMeta: eventMeta(t, 12), Participant: by, Value: 1},
		&match.MovedEvent{EventMeta: eventMeta(t, 13), Participant: by, To: boardtest.Position(t, 3, 0)},
		&match.CreatureDeployedEvent{EventMeta: eventMeta(t, 14), Participant: by, Card: dragon, Creature: matchtest.DeployedCreatureID(t, 2), Position: boardtest.Position(t, 3, 0), Cost: cardtest.Cost(t, 10)},
	))

	found, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 13)

	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, []query.MatchTerritoryValue{territoryValue(3, 100, 60)}, found[0].Meta().Changes.TerritoryValues)
}

func TestMatchEventsHandler_ReturnsNothingWhenTheViewerIsUpToDate(t *testing.T) {
	f := newEventsFixture(t, sampleEvents(t))

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 10)

	require.NoError(t, err)
	require.Empty(t, events)
}

func TestMatchEventsHandler_RejectsViewersWhoDoNotParticipate(t *testing.T) {
	f := newEventsFixture(t, sampleEvents(t))

	_, err := f.handler.ListAfter(t.Context(), uuid.New(), f.matchID, 0)

	require.ErrorIs(t, err, query.ErrMatchNotFound)
}

func TestMatchEventsHandler_ShowsTheRewardDrawnWithTheCardsAndTheCompletion(t *testing.T) {
	f := newEventsFixture(t, append(sampleEvents(t),
		&match.RewardDrawnEvent{EventMeta: eventMeta(t, 11), Participant: matchtest.Index(t, 0), Pack: match.DefaultPackKey, Cards: []card.Ref{cardtest.CreatureRef(t, 101)}},
		&match.RewardsCompletedEvent{EventMeta: eventMeta(t, 12)},
	))
	dragon := query.MatchCard{ID: f.cardID, Name: "ドラゴン", Type: query.CardTypeCreature}
	ended := changesOf(phaseOf(query.MatchPhaseEnded), nil)

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 10)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchRewardDrawnEvent{MatchEventMeta: metaAt(11, ended), Participant: 0, Pack: "default", Cards: []query.MatchCard{dragon}},
		query.MatchRewardsCompletedEvent{MatchEventMeta: metaAt(12, ended)},
	}, events)
}

// NPC が (2,0) に配置した領地に、プレイヤーが止まって侵略する前までのイベント
func invadingEvents(t *testing.T) []match.Event {
	t.Helper()
	player := matchtest.Index(t, 0)
	return append(sampleEvents(t)[:9],
		&match.TurnEndedEvent{EventMeta: eventMeta(t, 10), Participant: matchtest.Index(t, 1)},
		&match.DiceRolledEvent{EventMeta: eventMeta(t, 11), Participant: player, Value: 2},
		&match.MovedEvent{EventMeta: eventMeta(t, 12), Participant: player, To: boardtest.Position(t, 1, 0)},
		&match.MovedEvent{EventMeta: eventMeta(t, 13), Participant: player, To: boardtest.Position(t, 2, 0)},
		&match.TerritoryInvadedEvent{EventMeta: eventMeta(t, 14), Participant: player, Card: cardtest.CreatureRef(t, 101), Position: boardtest.Position(t, 2, 0), Cost: cardtest.Cost(t, 10)},
		&match.ItemSkippedEvent{EventMeta: eventMeta(t, 15), Participant: player},
	)
}

func battleStarted(t *testing.T, revision int64) *match.BattleStartedEvent {
	t.Helper()
	stats := match.BattleStats{Attack: creaturetest.Attack(t, 10), HitPoints: creaturetest.HitPoints(t, 20)}
	return &match.BattleStartedEvent{EventMeta: eventMeta(t, revision), Attacker: stats, Defender: stats}
}

func TestMatchEventsHandler_ReturnsTheSourceOfEachBattleEffect(t *testing.T) {
	modified := func(revision int64, side match.BattleSide, source match.BattleStatSource, attack, hitPoints int) *match.BattleStatModifiedEvent {
		return &match.BattleStatModifiedEvent{EventMeta: eventMeta(t, revision), Side: side, Source: source, AttackDelta: attack, HitPointsDelta: hitPoints, Attack: creaturetest.Attack(t, 10+attack), HitPoints: creaturetest.HitPoints(t, 20+hitPoints)}
	}
	f := newEventsFixture(t, append(invadingEvents(t),
		battleStarted(t, 16),
		modified(17, match.BattleSideAttacker, match.NewAdjacentTerritoriesSource([]board.Position{boardtest.Position(t, 1, 0)}), 10, 0),
		modified(18, match.BattleSideAttacker, match.NewItemSource(cardtest.CreatureRef(t, 101)), 25, 0),
		modified(19, match.BattleSideDefender, match.NewTerrainSource(board.ElementFire, 2), 0, 20),
	))
	dragon := deployedDragon(f.cardID)
	adjacent := query.MatchBattleStatSource{AdjacentTerritories: []query.BoardPosition{query.NewBoardPosition(1, 0)}}
	usedItem := query.MatchBattleStatSource{Item: &dragon}
	terrain := query.MatchBattleStatSource{Terrain: &query.MatchTerrainSource{Element: query.BoardTileElementFire, Level: 2}}

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 16)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchBattleStatModifiedEvent{MatchEventMeta: metaAt(17, changesOf(nil, nil)), Side: query.MatchBattleSideAttacker, Source: adjacent, AttackDelta: 10, Attack: 20, HitPoints: 20},
		query.MatchBattleStatModifiedEvent{MatchEventMeta: metaAt(18, changesOf(nil, nil)), Side: query.MatchBattleSideAttacker, Source: usedItem, AttackDelta: 25, Attack: 35, HitPoints: 20},
		query.MatchBattleStatModifiedEvent{MatchEventMeta: metaAt(19, changesOf(nil, nil)), Side: query.MatchBattleSideDefender, Source: terrain, HitPointsDelta: 20, Attack: 10, HitPoints: 40},
	}, events)
}

func TestMatchEventsHandler_ReturnsAFailedInvasionWithTheChangesItMade(t *testing.T) {
	player := matchtest.Index(t, 0)
	f := newEventsFixture(t, append(invadingEvents(t),
		battleStarted(t, 16),
		&match.CreatureStruckEvent{EventMeta: eventMeta(t, 17), Striker: match.BattleSideAttacker, Damage: creaturetest.Attack(t, 10), RemainingHitPoints: creaturetest.HitPoints(t, 10)},
		&match.CreatureStruckEvent{EventMeta: eventMeta(t, 18), Striker: match.BattleSideDefender, Damage: creaturetest.Attack(t, 10), RemainingHitPoints: creaturetest.HitPoints(t, 10)},
		&match.InvasionRepelledEvent{EventMeta: eventMeta(t, 19), Participant: player, Position: boardtest.Position(t, 2, 0), Card: cardtest.CreatureRef(t, 101), ReturnedToHand: true, TerritoryHitPoints: creaturetest.HitPoints(t, 10)},
	))
	dragon := deployedDragon(f.cardID)
	stats := query.MatchBattleStats{Attack: 10, HitPoints: 20}

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 13)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchTerritoryInvadedEvent{
			MatchEventMeta: metaAt(14, changesOf(phaseOf(query.MatchPhaseAttacking), []query.MatchTotalMagicChange{total(0, 590)})),
			Participant:    0, Card: dragon, Position: query.NewBoardPosition(2, 0), Cost: 10,
		},
		query.MatchItemSkippedEvent{MatchEventMeta: metaAt(15, changesOf(phaseOf(query.MatchPhaseDefending), nil)), Participant: 0},
		query.MatchBattleStartedEvent{MatchEventMeta: metaAt(16, changesOf(nil, nil)), Attacker: stats, Defender: stats},
		query.MatchCreatureStruckEvent{MatchEventMeta: metaAt(17, changesOf(nil, nil)), Striker: query.MatchBattleSideAttacker, Damage: 10, RemainingHitPoints: 10},
		query.MatchCreatureStruckEvent{MatchEventMeta: metaAt(18, changesOf(nil, nil)), Striker: query.MatchBattleSideDefender, Damage: 10, RemainingHitPoints: 10},
		query.MatchInvasionRepelledEvent{MatchEventMeta: metaAt(19, changesOf(phaseOf(query.MatchPhaseEnding), nil)), Participant: 0, Position: query.NewBoardPosition(2, 0), Card: dragon, ReturnedToHand: true, TerritoryHitPoints: 10},
	}, events)
}

// 持ち主が変わると両方の総魔力が動く。土地の価値と通行料は変わらないので入らない
func TestMatchEventsHandler_ConqueringMovesTheTotalMagicOfBothParticipants(t *testing.T) {
	player := matchtest.Index(t, 0)
	f := newEventsFixture(t, append(invadingEvents(t),
		battleStarted(t, 16),
		&match.CreatureStruckEvent{EventMeta: eventMeta(t, 17), Striker: match.BattleSideAttacker, Damage: creaturetest.Attack(t, 10), RemainingHitPoints: creaturetest.HitPoints(t, 1)},
		&match.TerritoryConqueredEvent{EventMeta: eventMeta(t, 18), Participant: player, Position: boardtest.Position(t, 2, 0), DefeatedCreature: matchtest.DeployedCreatureID(t, 1), Creature: matchtest.DeployedCreatureID(t, 2), TerritoryHitPoints: creaturetest.HitPoints(t, 20)},
	))

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 17)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchTerritoryConqueredEvent{
			MatchEventMeta: metaAt(18, changesOf(phaseOf(query.MatchPhaseEnding), []query.MatchTotalMagicChange{total(0, 690), total(1, 590)})),
			Participant:    0, Position: query.NewBoardPosition(2, 0), DefeatedCreature: 1, Creature: 2, TerritoryHitPoints: 20,
		},
	}, events)
}
