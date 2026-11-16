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
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/effect"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/effect/effecttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/element"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match/matchtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar/npcavatartest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck/npcdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell/spelltest"
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

// 土地のレベルはどれも 1（上げる操作をしない）。前の連鎖の数は無し（この持ち主の連鎖に新しく入った領地）
func territoryValue(x, value, toll, chainCount int) query.MatchTerritoryValue {
	return query.MatchTerritoryValue{Position: query.NewBoardPosition(x, 0), Value: value, Toll: toll, Level: 1, ChainCount: chainCount}
}

func withPreviousChainCount(value query.MatchTerritoryValue, previous int) query.MatchTerritoryValue {
	value.PreviousChainCount = &previous
	return value
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
		Creature: &match.CreatureSpec{Elements: creature.DefaultElements(), Attack: creaturetest.Attack(t, 10), HitPoints: creaturetest.HitPoints(t, 20), ItemRestrictions: creature.DefaultItemRestrictions(), PlacementRestrictions: creature.DefaultPlacementRestrictions(), Effects: effect.DefaultEffects()},
	}
	return []match.DeckCard{{Card: cardtest.CreatureRef(t, 101), Quantity: cardtest.DeckQuantity(t, 3), Spec: spec}}
}

// 対戦で写した性能のドラゴン
func placedDragon(id uuid.UUID) query.MatchCard {
	return query.MatchCard{
		ID: id, Name: "ドラゴン", Type: query.CardTypeCreature, Cost: 10, Rarity: "normal",
		Creature: &query.CreatureSpec{Elements: []string{"neutral"}, Attack: 10, HitPoints: 20, ItemRestrictions: []string{}, PlacementRestrictions: []string{}, Effects: []query.Effect{}},
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
		&match.DrawPilesShuffledEvent{EventMeta: eventMeta(t, 3), DrawPiles: [][]card.Ref{{dragon, dragon, dragon}, {dragon, dragon, dragon}}},
		&match.HandsDealtEvent{EventMeta: eventMeta(t, 4), Cards: 1},
		&match.CardDrawnEvent{EventMeta: eventMeta(t, 5), Participant: by},
		&match.DiceRolledEvent{EventMeta: eventMeta(t, 6), Participant: by, Value: 2},
		&match.PieceMovedEvent{EventMeta: eventMeta(t, 7), Participant: by, To: boardtest.Position(t, 1, 0)},
		&match.PieceMovedEvent{EventMeta: eventMeta(t, 8), Participant: by, To: boardtest.Position(t, 2, 0)},
		&match.CreaturePlacedEvent{EventMeta: eventMeta(t, 9), Participant: by, Creature: matchtest.PlacedCreatureID(t, 1), Cost: cardtest.Cost(t, 10)},
		&match.MatchEndedEvent{EventMeta: eventMeta(t, 10), Winners: []match.ParticipantIndex{by}, Ranks: []int{2, 1}, Reason: match.EndReasonRoundLimit},
	}
}

func TestMatchEventsHandler_ReturnsOnlyEventsAfterTheRevisionInOrder(t *testing.T) {
	f := newEventsFixture(t, sampleEvents(t))
	dragon := placedDragon(f.cardID)

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 4)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchCardDrawnEvent{MatchEventMeta: metaAt(5, changesOf(phaseOf(query.MatchPhaseRolling), nil)), Participant: 1, Card: dragon},
		query.MatchDiceRolledEvent{MatchEventMeta: metaAt(6, changesOf(nil, nil)), Participant: 1, Value: 2, Dice: []int{}},
		query.MatchPieceMovedEvent{MatchEventMeta: metaAt(7, changesOf(nil, nil)), Participant: 1, To: query.NewBoardPosition(1, 0)},
		query.MatchPieceMovedEvent{MatchEventMeta: metaAt(8, changesOf(phaseOf(query.MatchPhasePlacing), nil)), Participant: 1, To: query.NewBoardPosition(2, 0)},
		query.MatchCreaturePlacedEvent{
			MatchEventMeta: metaAt(9, changesOf(phaseOf(query.MatchPhaseEnding), []query.MatchTotalMagicChange{total(1, 690)}, territoryValue(2, 100, 60, 1))),
			Participant:    1, Card: dragon, Creature: 1, Position: query.NewBoardPosition(2, 0), Cost: 10,
		},
		query.MatchEndedEvent{MatchEventMeta: metaAt(10, changesOf(phaseOf(query.MatchPhaseEnded), nil)), Winners: []int{1}, Ranks: []int{2, 1}, Reason: query.MatchEndReasonRoundLimit},
	}, events)
}

// 振るときに効いたスペルの効果は、イベントに持たず、振る直前の状態から作る（core#173）
// 配置が after より前でも、回復したクリーチャーのカードを直前の状態から引ける（開発の画面で、カードが空のまま返って再生が止まった。core#114）
func TestMatchEventsHandler_LapRecoveryShowsTheCardPlacedBeforeTheRevision(t *testing.T) {
	by := matchtest.Index(t, 1)
	events := sampleEvents(t)
	events = append(events[:len(events)-1],
		&match.SpellCreatureDamagedEvent{EventMeta: eventMeta(t, 10), Participant: by, Position: boardtest.Position(t, 2, 0), Damage: 15, RemainingHitPoints: creaturetest.HitPoints(t, 5)},
		&match.LapCreatureHitPointsRecoveredEvent{EventMeta: eventMeta(t, 11), Participant: by, Creature: matchtest.PlacedCreatureID(t, 1), Amount: 10},
	)
	f := newEventsFixture(t, events)

	listed, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 10)

	require.NoError(t, err)
	require.Len(t, listed, 1)
	recovered, ok := listed[0].(query.MatchLapCreatureHitPointsRecoveredEvent)
	require.True(t, ok)
	require.Equal(t, placedDragon(f.cardID), recovered.Card)
	require.Equal(t, query.NewBoardPosition(2, 0), recovered.Position)
	require.Equal(t, []int{1, 1, 10, 15}, []int{recovered.Participant, recovered.Creature, recovered.Amount, recovered.HitPoints})
}

func TestMatchEventsHandler_DiceRolledShowsTheLastingSpellEffectBeforeTheRoll(t *testing.T) {
	by := matchtest.Index(t, 1)
	started := match.LastingSpellEffect{Effect: spelltest.FixedDice(t, 6, 1), RemainingTurns: 1}
	f := newEventsFixture(t, append(sampleEvents(t)[:5],
		&match.SpellEffectStartedEvent{EventMeta: eventMeta(t, 6), Participant: by, Target: by, Effect: started},
		&match.DiceRolledEvent{EventMeta: eventMeta(t, 7), Participant: by, Value: 6},
	))

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 5)

	require.NoError(t, err)
	effect := query.SpellEffect{FixedDice: &query.SpellFixedDice{Value: 6, Turns: 1}}
	lasting := query.MatchLastingSpellEffect{Effect: effect, RemainingTurns: 1}
	require.Equal(t, []query.MatchEvent{
		query.MatchSpellEffectStartedEvent{MatchEventMeta: metaAt(6, changesOf(phaseOf(query.MatchPhaseRolling), nil)), Participant: 1, Target: 1, Effect: lasting},
		query.MatchDiceRolledEvent{MatchEventMeta: metaAt(7, changesOf(nil, nil)), Participant: 1, Value: 6, Dice: []int{}, SpellEffect: &effect},
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
		query.MatchDrawPilesShuffledEvent{MatchEventMeta: metaAt(3, changesOf(nil, nil))},
		query.MatchHandsDealtEvent{MatchEventMeta: metaAt(4, changesOf(phaseOf(query.MatchPhaseDrawing), nil)), Cards: 1},
	}, events)
}

// 配置で連鎖が増えると、前からある領地の価値と通行料も、その版の変わった値に入る
func TestMatchEventsHandler_ChangesIncludeEarlierTerritoriesWhoseChainGrew(t *testing.T) {
	by := matchtest.Index(t, 1)
	f := newEventsFixture(t, append(sampleEvents(t)[:9],
		&match.TurnEndedEvent{EventMeta: eventMeta(t, 10), Participant: by},
		&match.TurnEndedEvent{EventMeta: eventMeta(t, 11), Participant: matchtest.Index(t, 0)},
		&match.DiceRolledEvent{EventMeta: eventMeta(t, 12), Participant: by, Value: 1},
		&match.PieceMovedEvent{EventMeta: eventMeta(t, 13), Participant: by, To: boardtest.Position(t, 3, 0)},
		&match.CreaturePlacedEvent{EventMeta: eventMeta(t, 14), Participant: by, Creature: matchtest.PlacedCreatureID(t, 2), Cost: cardtest.Cost(t, 10)},
	))

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 13)

	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, changesOf(phaseOf(query.MatchPhaseEnding), []query.MatchTotalMagicChange{total(1, 880)}, withPreviousChainCount(territoryValue(2, 150, 90, 2), 1), territoryValue(3, 150, 90, 2)), events[0].Meta().Changes)
}

// 別の属性の領地ができても、連鎖が変わらない領地は入らない
func TestMatchEventsHandler_ChangesLeaveOutTerritoriesThatDidNotChange(t *testing.T) {
	by := matchtest.Index(t, 1)
	events := sampleEvents(t)[:9]
	lined := lineBoard(t)
	lined.Tiles[3] = boardtest.Tile(t, 3, 0, "land", "water")
	events[0].(*match.CreatedEvent).Board = lined
	f := newEventsFixture(t, append(events,
		&match.TurnEndedEvent{EventMeta: eventMeta(t, 10), Participant: by},
		&match.TurnEndedEvent{EventMeta: eventMeta(t, 11), Participant: matchtest.Index(t, 0)},
		&match.DiceRolledEvent{EventMeta: eventMeta(t, 12), Participant: by, Value: 1},
		&match.PieceMovedEvent{EventMeta: eventMeta(t, 13), Participant: by, To: boardtest.Position(t, 3, 0)},
		&match.CreaturePlacedEvent{EventMeta: eventMeta(t, 14), Participant: by, Creature: matchtest.PlacedCreatureID(t, 2), Cost: cardtest.Cost(t, 10)},
	))

	found, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 13)

	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, []query.MatchTerritoryValue{territoryValue(3, 100, 60, 1)}, found[0].Meta().Changes.TerritoryValues)
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
		&match.AllRewardsReceivedEvent{EventMeta: eventMeta(t, 12)},
	))
	dragon := query.MatchCard{ID: f.cardID, Name: "ドラゴン", Type: query.CardTypeCreature}
	ended := changesOf(phaseOf(query.MatchPhaseEnded), nil)

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 10)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchRewardDrawnEvent{MatchEventMeta: metaAt(11, ended), Participant: 0, Pack: "default", Cards: []query.MatchCard{dragon}},
		query.MatchAllRewardsReceivedEvent{MatchEventMeta: metaAt(12, ended)},
	}, events)
}

// NPC が (2,0) に配置した領地に、プレイヤーが止まって侵略する前までのイベント
func invadingEvents(t *testing.T) []match.Event {
	t.Helper()
	player := matchtest.Index(t, 0)
	return append(sampleEvents(t)[:9],
		&match.TurnEndedEvent{EventMeta: eventMeta(t, 10), Participant: matchtest.Index(t, 1)},
		&match.DiceRolledEvent{EventMeta: eventMeta(t, 11), Participant: player, Value: 2},
		&match.PieceMovedEvent{EventMeta: eventMeta(t, 12), Participant: player, To: boardtest.Position(t, 1, 0)},
		&match.PieceMovedEvent{EventMeta: eventMeta(t, 13), Participant: player, To: boardtest.Position(t, 2, 0)},
		&match.TerritoryInvadedEvent{EventMeta: eventMeta(t, 14), Participant: player, Cost: cardtest.Cost(t, 10)},
		&match.ItemSkippedEvent{EventMeta: eventMeta(t, 15), Participant: player},
	)
}

// 両方がカード 101（AT 10・戦闘の前の HP 20）で、アイテムも効果も無い戦闘の入力
func battleInput(t *testing.T) match.BattleInput {
	t.Helper()
	spec := match.CreatureSpec{
		Elements: creature.DefaultElements(), Attack: creaturetest.Attack(t, 10), HitPoints: creaturetest.HitPoints(t, 30),
		ItemRestrictions: creature.DefaultItemRestrictions(), PlacementRestrictions: creature.DefaultPlacementRestrictions(), Effects: effect.DefaultEffects(),
	}
	combatant := match.BattleCombatant{Card: cardtest.CreatureRef(t, 101), Spec: spec, Attack: spec.Attack, HitPoints: creaturetest.HitPoints(t, 20), AdjacentTerritories: []board.Position{}}
	land := match.BattleLand{Element: element.LandFire, Level: 1}
	return match.BattleInput{Attacker: combatant, Defender: combatant, Land: land}
}

func battleStarted(t *testing.T, revision int64) *match.BattleStartedEvent {
	t.Helper()
	return battleStartedWith(t, revision, battleInput(t))
}

func battleStartedWith(t *testing.T, revision int64, input match.BattleInput) *match.BattleStartedEvent {
	t.Helper()
	return &match.BattleStartedEvent{EventMeta: eventMeta(t, revision), Input: input}
}

// 発動のイベントは増減だけを持ち、根拠の値は戦闘の開始の入力から、当てた後の値は直前の戦闘の中の値から添える（core#171・core#175）
func TestMatchEventsHandler_ReturnsTheBasisAndTheStatsAfterEachActivation(t *testing.T) {
	input := battleInput(t)
	// アイテムの名前は写しのあるカードで引く（この見本の写しはクリーチャー 101 だけ）
	weapon := match.ItemSpec{Category: card.ItemCategoryWeapon, Effects: effecttest.Effects(t, effect.NewAttackModifierEffect(effect.NewAttackModifier(25)))}
	input.Attacker.Item = &match.BattleItem{Card: cardtest.CreatureRef(t, 101), Spec: weapon}
	input.Defender.Spec.Effects = effecttest.Effects(t, effecttest.HitPointsModifier(5))
	input.Land = match.BattleLand{Element: element.LandFire, Level: 2}
	input.Attacker.AdjacentTerritories = []board.Position{boardtest.Position(t, 1, 0)}
	input.Defender.AdjacentTerritories = []board.Position{boardtest.Position(t, 3, 0), boardtest.Position(t, 2, 1)}
	rules := matchtest.Rules(t, 1, 6, 5)
	input.Rules = match.BattleRules{AdjacentTerritoryBonus: rules.AdjacentTerritoryBonus, TerrainBonusPerLevel: rules.TerrainBonusPerLevel}
	f := newEventsFixture(t, append(invadingEvents(t),
		battleStartedWith(t, 16, input),
		&match.BattleSupportEffectActivatedEvent{EventMeta: eventMeta(t, 17), Side: match.BattleSideAttacker, AttackDelta: 10},
		&match.BattleItemEffectActivatedEvent{EventMeta: eventMeta(t, 18), Side: match.BattleSideAttacker, Effect: effect.KindAttackModifier, Change: &match.BattleStatChange{AttackDelta: 25}},
		&match.BattleTerrainEffectActivatedEvent{EventMeta: eventMeta(t, 19), HitPointsDelta: 20},
		&match.BattleCreatureEffectActivatedEvent{EventMeta: eventMeta(t, 20), Side: match.BattleSideDefender, Effect: effect.KindHitPointsModifier, Change: &match.BattleStatChange{HitPointsDelta: 5}},
		&match.BattleSupportEffectActivatedEvent{EventMeta: eventMeta(t, 21), Side: match.BattleSideDefender, AttackDelta: 20},
	))
	dragon := placedDragon(f.cardID)
	weaponEffect := query.Effect{AttackModifier: new(25)}
	armorEffect := query.Effect{HitPointsModifier: new(5)}
	supported := query.MatchBattleStatChange{AttackDelta: 10, Attack: 20, HitPoints: 20}
	terrain := query.MatchBattleStatChange{HitPointsDelta: 20, Attack: 10, HitPoints: 40}
	defenderSupported := query.MatchBattleStatChange{AttackDelta: 20, Attack: 30, HitPoints: 45}

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 16)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchBattleSupportEffectActivatedEvent{
			MatchEventMeta: metaAt(17, changesOf(nil, nil)), Side: query.MatchBattleSideAttacker, Positions: []query.BoardPosition{query.NewBoardPosition(1, 0)}, BonusPerTerritory: 10,
			Change: supported,
		},
		query.MatchBattleItemEffectActivatedEvent{
			MatchEventMeta: metaAt(18, changesOf(nil, nil)), Side: query.MatchBattleSideAttacker, Effect: query.EffectKindAttackModifier, Card: dragon,
			ActivatedEffect: weaponEffect,
			Change:          &query.MatchBattleStatChange{AttackDelta: 25, Attack: 45, HitPoints: 20},
		},
		query.MatchBattleTerrainEffectActivatedEvent{
			MatchEventMeta: metaAt(19, changesOf(nil, nil)), Side: query.MatchBattleSideDefender, Element: query.BoardTileElementFire, Level: 2, BonusPerLevel: 10,
			Change: terrain,
		},
		query.MatchBattleCreatureEffectActivatedEvent{
			MatchEventMeta: metaAt(20, changesOf(nil, nil)), Side: query.MatchBattleSideDefender, Effect: query.EffectKindHitPointsModifier, Card: dragon,
			ActivatedEffect: armorEffect, Change: &query.MatchBattleStatChange{HitPointsDelta: 5, Attack: 10, HitPoints: 45},
		},
		// 守る側の支援効果は、守る側の隣の領地と、守る側の値に当てた後の値を添える
		query.MatchBattleSupportEffectActivatedEvent{
			MatchEventMeta: metaAt(21, changesOf(nil, nil)), Side: query.MatchBattleSideDefender,
			Positions: []query.BoardPosition{query.NewBoardPosition(3, 0), query.NewBoardPosition(2, 1)}, BonusPerTerritory: 10,
			Change: defenderSupported,
		},
	}, events)
}

// 倍率は制約を満たす一番高い効果を、地形効果を無視するは相手の HP が地形効果の分を除いた値になる変化を添える（core#177）
func TestMatchEventsHandler_ReturnsTheActivatedCardEffectAndTheIgnoredTerrainEffect(t *testing.T) {
	input := battleInput(t)
	scroll := match.ItemSpec{Category: card.ItemCategoryScroll, Effects: effecttest.Effects(t,
		effecttest.AttackMultiplier(t, 200, "water"), effecttest.AttackMultiplier(t, 150, "neutral"), effecttest.AttackMultiplier(t, 120, "neutral"), effect.NewIgnoreTerrainEffect(),
	)}
	input.Attacker.Item = &match.BattleItem{Card: cardtest.CreatureRef(t, 101), Spec: scroll}
	input.Land = match.BattleLand{Element: element.LandFire, Level: 2}
	f := newEventsFixture(t, append(invadingEvents(t),
		battleStartedWith(t, 16, input),
		&match.BattleTerrainEffectActivatedEvent{EventMeta: eventMeta(t, 17), HitPointsDelta: 20},
		&match.BattleItemEffectActivatedEvent{EventMeta: eventMeta(t, 18), Side: match.BattleSideAttacker, Effect: effect.KindAttackMultiplier, Change: &match.BattleStatChange{AttackDelta: 5}},
		&match.BattleItemEffectActivatedEvent{EventMeta: eventMeta(t, 19), Side: match.BattleSideAttacker, Effect: effect.KindIgnoreTerrainEffect},
	))
	dragon := placedDragon(f.cardID)
	strike := query.Effect{AttackMultiplier: &query.EffectAttackMultiplier{Percent: 150, Constraints: []query.EffectConstraint{{OpponentCreatureElements: []string{"neutral"}}}}}
	ignore := query.Effect{IgnoreTerrainEffect: true}

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 17)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchBattleItemEffectActivatedEvent{
			MatchEventMeta: metaAt(18, changesOf(nil, nil)), Side: query.MatchBattleSideAttacker, Effect: query.EffectKindAttackMultiplier, Card: dragon,
			ActivatedEffect: strike,
			Change:          &query.MatchBattleStatChange{AttackDelta: 5, Attack: 15, HitPoints: 20},
		},
		query.MatchBattleItemEffectActivatedEvent{
			MatchEventMeta: metaAt(19, changesOf(nil, nil)), Side: query.MatchBattleSideAttacker, Effect: query.EffectKindIgnoreTerrainEffect, Card: dragon,
			ActivatedEffect: ignore,
			OpponentChange:  &query.MatchBattleStatChange{HitPointsDelta: -20, Attack: 10, HitPoints: 20},
		},
	}, events)
}

// 上書きの範囲は戦闘の入力のカードの効果から添える（引いた値は Change の当てた後の値）
func TestMatchEventsHandler_ReturnsTheRangeOfTheOverrideFromTheBattleInput(t *testing.T) {
	input := battleInput(t)
	input.Defender.Spec.Effects = effecttest.Effects(t, effecttest.AttackOverrideRange(t, 10, 70))
	f := newEventsFixture(t, append(invadingEvents(t),
		battleStartedWith(t, 16, input),
		&match.BattleCreatureEffectActivatedEvent{EventMeta: eventMeta(t, 17), Side: match.BattleSideDefender, Effect: effect.KindAttackOverride, Change: &match.BattleStatChange{AttackDelta: 40}},
	))

	override := query.Effect{AttackOverride: &query.EffectRange{Minimum: 10, Maximum: 70}}

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 16)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchBattleCreatureEffectActivatedEvent{
			MatchEventMeta: metaAt(17, changesOf(nil, nil)), Side: query.MatchBattleSideDefender, Effect: query.EffectKindAttackOverride, Card: placedDragon(f.cardID),
			ActivatedEffect: override, Change: &query.MatchBattleStatChange{AttackDelta: 40, Attack: 50, HitPoints: 20},
		},
	}, events)
}

func TestMatchEventsHandler_ReturnsTheActivatedStrikeEffects(t *testing.T) {
	input := battleInput(t)
	input.Attacker.Spec.Effects = effecttest.Effects(t, effect.NewStrikeLastEffect())
	input.Defender.Spec.Effects = effecttest.Effects(t, effect.NewStrikeFirstEffect())
	f := newEventsFixture(t, append(invadingEvents(t),
		battleStartedWith(t, 16, input),
		&match.BattleCreatureEffectActivatedEvent{EventMeta: eventMeta(t, 17), Side: match.BattleSideAttacker, Effect: effect.KindStrikeLast},
		&match.BattleCreatureEffectActivatedEvent{EventMeta: eventMeta(t, 18), Side: match.BattleSideDefender, Effect: effect.KindStrikeFirst},
	))
	dragon := placedDragon(f.cardID)
	last, first := query.Effect{StrikeLast: true}, query.Effect{StrikeFirst: true}

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 16)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchBattleCreatureEffectActivatedEvent{MatchEventMeta: metaAt(17, changesOf(nil, nil)), Side: query.MatchBattleSideAttacker, Effect: query.EffectKindStrikeLast, Card: dragon, ActivatedEffect: last},
		query.MatchBattleCreatureEffectActivatedEvent{MatchEventMeta: metaAt(18, changesOf(nil, nil)), Side: query.MatchBattleSideDefender, Effect: query.EffectKindStrikeFirst, Card: dragon, ActivatedEffect: first},
	}, events)
}

// 攻撃の値は、攻撃した側の直前の戦闘中の AT（増減の後）。受けるダメージを 0 にするで受けた量が 0 でも、攻撃の値はそのまま
func TestMatchEventsHandler_StruckShowsTheAttackOfTheStrikerBeforeTheStrike(t *testing.T) {
	input := battleInput(t)
	// 受けるダメージを 0 にする効果のうち、相手（無属性）が制約を満たす最初のものを添える
	input.Defender.Spec.Effects = effecttest.Effects(t, effecttest.NullifyReceivedDamage(t, "fire"), effecttest.NullifyReceivedDamage(t, "neutral"))
	f := newEventsFixture(t, append(invadingEvents(t),
		battleStartedWith(t, 16, input),
		&match.BattleItemEffectActivatedEvent{EventMeta: eventMeta(t, 17), Side: match.BattleSideAttacker, Effect: effect.KindAttackModifier, Change: &match.BattleStatChange{AttackDelta: 30}},
		&match.BattleCreatureEffectActivatedEvent{EventMeta: eventMeta(t, 18), Side: match.BattleSideDefender, Effect: effect.KindNullifyReceivedDamage},
		&match.CreatureStruckEvent{EventMeta: eventMeta(t, 19), Striker: match.BattleSideAttacker, Damage: creaturetest.Attack(t, 0), RemainingHitPoints: creaturetest.HitPoints(t, 20)},
		&match.CreatureStruckEvent{EventMeta: eventMeta(t, 20), Striker: match.BattleSideDefender, Damage: creaturetest.Attack(t, 10), RemainingHitPoints: creaturetest.HitPoints(t, 10)},
	))

	nullify := query.Effect{NullifyReceivedDamage: &query.EffectNullifyReceivedDamage{Constraints: []query.EffectConstraint{{OpponentCreatureElements: []string{"neutral"}}}}}

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 17)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchBattleCreatureEffectActivatedEvent{
			MatchEventMeta: metaAt(18, changesOf(nil, nil)), Side: query.MatchBattleSideDefender, Effect: query.EffectKindNullifyReceivedDamage, Card: placedDragon(f.cardID),
			ActivatedEffect: nullify,
		},
		query.MatchCreatureStruckEvent{MatchEventMeta: metaAt(19, changesOf(nil, nil)), Striker: query.MatchBattleSideAttacker, Attack: 40, Damage: 0, RemainingHitPoints: 20},
		query.MatchCreatureStruckEvent{MatchEventMeta: metaAt(20, changesOf(nil, nil)), Striker: query.MatchBattleSideDefender, Attack: 10, Damage: 10, RemainingHitPoints: 10},
	}, events)
}

func TestMatchEventsHandler_ReturnsTheActivatedTollMultiplier(t *testing.T) {
	effects := effecttest.Effects(t, effecttest.TollMultiplier(t, 150))
	loaded := invadingEvents(t)
	// 両方の参加者が同じカード 101 を持つので、どちらの写しにも倍率を持たせる
	for _, participant := range loaded[0].(*match.CreatedEvent).Participants {
		participant.Cards[0].Spec.Creature.Effects = effects
	}
	f := newEventsFixture(t, append(loaded,
		&match.TollCreatureEffectActivatedEvent{EventMeta: eventMeta(t, 16)},
	))

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 15)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchTollCreatureEffectActivatedEvent{MatchEventMeta: metaAt(16, changesOf(phaseOf(query.MatchPhaseDefending), nil)), Position: query.NewBoardPosition(2, 0), Percent: 150, Toll: 60, MultipliedToll: 90},
	}, events)
}

func TestMatchEventsHandler_ReturnsAFailedInvasionWithTheChangesItMade(t *testing.T) {
	player := matchtest.Index(t, 0)
	f := newEventsFixture(t, append(invadingEvents(t),
		battleStarted(t, 16),
		&match.CreatureStruckEvent{EventMeta: eventMeta(t, 17), Striker: match.BattleSideAttacker, Damage: creaturetest.Attack(t, 10), RemainingHitPoints: creaturetest.HitPoints(t, 10)},
		&match.CreatureStruckEvent{EventMeta: eventMeta(t, 18), Striker: match.BattleSideDefender, Damage: creaturetest.Attack(t, 10), RemainingHitPoints: creaturetest.HitPoints(t, 10)},
		&match.InvasionFailedEvent{EventMeta: eventMeta(t, 19), Participant: player, TerritoryHitPoints: creaturetest.HitPoints(t, 10)},
	))
	dragon := placedDragon(f.cardID)
	stats := query.MatchBattleStats{Attack: 10, HitPoints: 20}

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 13)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		query.MatchTerritoryInvadedEvent{
			MatchEventMeta: metaAt(14, changesOf(phaseOf(query.MatchPhaseAttacking), []query.MatchTotalMagicChange{total(0, 590)})),
			Participant:    0, Card: dragon, Position: query.NewBoardPosition(2, 0), Cost: 10, DefendingCreatureCard: dragon,
		},
		query.MatchItemSkippedEvent{MatchEventMeta: metaAt(15, changesOf(phaseOf(query.MatchPhaseDefending), nil)), Participant: 0},
		query.MatchBattleStartedEvent{MatchEventMeta: metaAt(16, changesOf(nil, nil)), Attacker: stats, Defender: stats},
		query.MatchCreatureStruckEvent{MatchEventMeta: metaAt(17, changesOf(nil, nil)), Striker: query.MatchBattleSideAttacker, Attack: 10, Damage: 10, RemainingHitPoints: 10},
		query.MatchCreatureStruckEvent{MatchEventMeta: metaAt(18, changesOf(nil, nil)), Striker: query.MatchBattleSideDefender, Attack: 10, Damage: 10, RemainingHitPoints: 10},
		query.MatchInvasionFailedEvent{MatchEventMeta: metaAt(19, changesOf(phaseOf(query.MatchPhaseEnding), nil)), Participant: 0, Position: query.NewBoardPosition(2, 0), Card: dragon, ReturnedToHand: true, TerritoryHitPoints: 10},
	}, events)
}

// 持ち主が変わると両方の総魔力が動く。土地の価値と通行料は変わらないので入らない
func TestMatchEventsHandler_ConqueringMovesTheTotalMagicAndTheTerritoryIntoTheConquerorsChain(t *testing.T) {
	player := matchtest.Index(t, 0)
	f := newEventsFixture(t, append(invadingEvents(t),
		battleStarted(t, 16),
		&match.CreatureStruckEvent{EventMeta: eventMeta(t, 17), Striker: match.BattleSideAttacker, Damage: creaturetest.Attack(t, 20), RemainingHitPoints: creaturetest.HitPoints(t, 20).Damaged(creaturetest.Attack(t, 20))},
		&match.DefendingCreatureDestroyedEvent{EventMeta: eventMeta(t, 18), Participant: player},
		&match.InvasionSucceededEvent{EventMeta: eventMeta(t, 19), Participant: player, Creature: matchtest.PlacedCreatureID(t, 2), TerritoryHitPoints: creaturetest.HitPoints(t, 20)},
	))

	events, err := f.handler.ListAfter(t.Context(), f.viewer, f.matchID, 17)

	require.NoError(t, err)
	require.Equal(t, []query.MatchEvent{
		// 破壊で守る側の総魔力から領地が抜け、侵略の成功で攻めた側に入る（core#181）
		query.MatchDefendingCreatureDestroyedEvent{
			MatchEventMeta: metaAt(18, changesOf(nil, []query.MatchTotalMagicChange{total(1, 590)})),
			Participant:    0, Card: placedDragon(f.cardID), Position: query.NewBoardPosition(2, 0),
		},
		query.MatchInvasionSucceededEvent{
			// 奪った領地は価値が同じでも、奪った側の連鎖に新しく入るので入る（前の連鎖の数は無し）
			MatchEventMeta: metaAt(19, changesOf(phaseOf(query.MatchPhaseEnding), []query.MatchTotalMagicChange{total(0, 690)}, territoryValue(2, 100, 60, 1))),
			Participant:    0, Position: query.NewBoardPosition(2, 0), Creature: 2, TerritoryHitPoints: 20,
		},
	}, events)
}
