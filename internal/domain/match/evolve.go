package match

import (
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
)

// 状態は値で返し、渡された状態を書き換えない
func Evolve(state State, event Event) (State, error) {
	meta := event.GetMeta()
	if created, ok := event.(*CreatedEvent); ok {
		if _, uncreated := state.(UncreatedMatch); !uncreated {
			return state, ErrInvalidState
		}
		base, _ := created.Board.base()
		participants := make([]ParticipantState, len(created.Participants))
		for i, participant := range created.Participants {
			participants[i] = ParticipantState{Participant: participant, Position: base, Magic: created.Rules.StartingMagic}
		}
		return Match{
			ID: meta.EntityID, Version: meta.AggregateVersion, Board: created.Board, Rules: created.Rules,
			Participants: participants, Round: 1,
		}, nil
	}
	current, ok := state.(Match)
	if !ok {
		return state, ErrInvalidState
	}
	current.Version = meta.AggregateVersion
	switch e := event.(type) {
	case *TurnOrderDecidedEvent:
		current.TurnOrder = e.Order
		current.Turn = 0
	case *DecksShuffledEvent:
		if len(e.DrawPiles) != len(current.Participants) {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		for i := range current.Participants {
			current.Participants[i].DrawPile = e.DrawPiles[i]
		}
	case *HandsDealtEvent:
		current.Participants = slices.Clone(current.Participants)
		for i, participant := range current.Participants {
			count := min(e.Cards, len(participant.DrawPile))
			current.Participants[i].Hand = append(slices.Clone(participant.Hand), participant.DrawPile[:count]...)
			current.Participants[i].DrawPile = participant.DrawPile[count:]
		}
		current.Phase = current.turnStartPhase()
	case *CardDrawnEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		participant := current.Participants[index]
		if len(participant.DrawPile) == 0 {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Hand = append(slices.Clone(participant.Hand), participant.DrawPile[0])
		current.Participants[index].DrawPile = participant.DrawPile[1:]
		current.Phase = PhaseRolling
	case *DiscardPileShuffledEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		participant := current.Participants[index]
		if len(participant.DrawPile) > 0 || !sameCards(participant.DiscardPile, e.DrawPile) {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].DrawPile = e.DrawPile
		current.Participants[index].DiscardPile = nil
	case *SpellUsedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		participant := current.Participants[index]
		used, inHand := participant.handCard(e.HandIndex)
		spec, ok := current.CardSpecs()[used]
		switch {
		case current.Phase != PhaseRolling, current.SpellUsed, !inHand, !ok, spec.Spell == nil:
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Hand = slices.Delete(slices.Clone(participant.Hand), e.HandIndex, e.HandIndex+1)
		current.Participants[index].Magic -= e.Cost.Int()
		current.Participants[index].DiscardPile = append(slices.Clone(participant.DiscardPile), used)
		current.SpellUsed = true
	case *HandLimitExceededEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		if !current.exceedsHandLimit(index) {
			return state, ErrInvalidState
		}
		current.Phase = PhaseDiscarding
	case *CardDiscardedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		hand := current.Participants[index].Hand
		discarded, ok := current.Participants[index].handCard(e.HandIndex)
		if !ok {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Hand = slices.Delete(slices.Clone(hand), e.HandIndex, e.HandIndex+1)
		current.Participants[index].DiscardPile = append(slices.Clone(current.Participants[index].DiscardPile), discarded)
		current.Phase = current.handLimitPhase(index)
	case *DiceRolledEvent:
		if _, err := current.actor(e.Participant); err != nil {
			return state, err
		}
		if len(e.Dice) > 0 && sum(e.Dice) != e.Value {
			return state, ErrInvalidState
		}
		current.Dice = e.Value
		current.RemainingSteps = e.Value
		current.Phase = current.settle()
	case *MovedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		from := current.Participants[index].Position
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Previous = &from
		current.Participants[index].Position = e.To
		current.PassedPositions = append(slices.Clone(current.PassedPositions), e.To)
		current.RemainingSteps--
		current.Phase = current.settle()
	case *CheckpointPassedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		participant := current.Participants[index]
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].PassedCheckpoints = append(slices.Clone(participant.PassedCheckpoints), participant.Position)
		current.Participants[index].Magic += e.Magic
	case *LapCompletedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Magic += e.Magic
		current.Participants[index].PassedCheckpoints = nil
		current.Participants[index].Laps++
	case *MagicGainedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Magic += e.Magic
	case *SpellEffectStartedEvent:
		if _, err := current.actor(e.Participant); err != nil {
			return state, err
		}
		if e.Target < 0 || int(e.Target) >= len(current.Participants) || e.Effect.RemainingTurns < 1 {
			return state, ErrInvalidState
		}
		lasting := e.Effect
		current.Participants = slices.Clone(current.Participants)
		current.Participants[e.Target].LastingSpellEffect = &lasting
	case *CreatureDamagedEvent:
		if _, err := current.actor(e.Participant); err != nil {
			return state, err
		}
		territory, ok := current.TerritoryAt(e.Position)
		if !ok {
			return state, ErrInvalidState
		}
		current.PlacedCreatures = slices.Clone(current.PlacedCreatures)
		for i := range current.PlacedCreatures {
			if current.PlacedCreatures[i].ID == territory.Creature {
				current.PlacedCreatures[i].HitPoints = e.RemainingHitPoints
			}
		}
	case *CreatureDestroyedEvent:
		if _, err := current.actor(e.Participant); err != nil {
			return state, err
		}
		territory, ok := current.TerritoryAt(e.Position)
		destroyed, found := current.PlacedCreature(territory.Creature)
		if !ok || !found || destroyed.HitPoints.Int() != 0 {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[destroyed.Owner].DiscardPile = append(slices.Clone(current.Participants[destroyed.Owner].DiscardPile), destroyed.Card)
		current.Territories = slices.DeleteFunc(slices.Clone(current.Territories), func(t Territory) bool { return t.Position == e.Position })
		current.PlacedCreatures = slices.DeleteFunc(slices.Clone(current.PlacedCreatures), func(d PlacedCreature) bool { return d.ID == destroyed.ID })
	case *SpellEffectEndedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		if current.Participants[index].LastingSpellEffect == nil {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].LastingSpellEffect = nil
	case *CreaturePlacedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		participant := current.Participants[index]
		placed, inHand := participant.handCard(e.HandIndex)
		spec, ok := current.CardSpecs()[placed]
		switch {
		case current.Phase != PhasePlacing, !current.isVacantLand(participant.Position),
			!inHand, !ok, spec.Creature == nil, e.Creature != current.LastPlacedCreatureID+1:
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Hand = slices.Delete(slices.Clone(participant.Hand), e.HandIndex, e.HandIndex+1)
		current.Participants[index].Magic -= e.Cost.Int()
		current.PlacedCreatures = append(slices.Clone(current.PlacedCreatures), PlacedCreature{
			ID: e.Creature, Card: placed, Owner: index, Base: *spec.Creature, Attack: spec.Creature.Attack, HitPoints: spec.Creature.HitPoints,
		})
		current.Territories = append(slices.Clone(current.Territories), Territory{Position: participant.Position, Creature: e.Creature})
		current.LastPlacedCreatureID = e.Creature
		current.Phase = PhaseEnding
	case *TollPaidEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		owner, ok := current.tollOwner(index)
		if (current.Phase != PhaseInvading && current.Phase != PhaseEnding) || !ok {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Magic -= e.Magic
		current.Participants[owner].Magic += e.Magic
		current.Phase = current.debtPhase(index)
	case *TerritoryRelinquishedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		territory, ok := current.TerritoryAt(e.Position)
		if !ok || current.Phase != PhaseRelinquishing {
			return state, ErrInvalidState
		}
		if owner, ok := current.TerritoryOwner(territory); !ok || owner != index {
			return state, ErrInvalidState
		}
		relinquished, found := current.PlacedCreature(territory.Creature)
		if !found {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Magic += e.Magic
		current.Participants[index].DiscardPile = append(slices.Clone(current.Participants[index].DiscardPile), relinquished.Card)
		current.Territories = slices.DeleteFunc(slices.Clone(current.Territories), func(t Territory) bool { return t.Position == e.Position })
		current.PlacedCreatures = slices.DeleteFunc(slices.Clone(current.PlacedCreatures), func(d PlacedCreature) bool { return d.ID == relinquished.ID })
		current.Phase = current.debtPhase(index)
	case *MagicDepletedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		base, ok := current.Board.base()
		if !ok || current.Phase != PhaseEnding || current.Participants[index].Magic >= 0 || len(current.territoriesOf(index)) > 0 {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Magic = e.Magic
		current.Participants[index].Position = base
		current.Participants[index].Previous = nil
		current.Participants[index].PassedCheckpoints = nil
	case *TerritoryLeveledUpEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		territory, ok := current.TerritoryAt(e.Position)
		if !ok || !current.acceptsTerritoryCommand() || e.Level <= current.LandLevel(e.Position) {
			return state, ErrInvalidState
		}
		if owner, ok := current.TerritoryOwner(territory); !ok || owner != index {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Magic -= e.Magic
		current.LandLevels = append(slices.DeleteFunc(slices.Clone(current.LandLevels), func(l LandLevel) bool { return l.Position == e.Position }),
			LandLevel{Position: e.Position, Level: e.Level})
		current.Phase = PhaseEnding
	case *CreatureMovedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		from, ok := current.TerritoryAt(e.From)
		if !ok || !current.acceptsTerritoryCommand() || !current.isVacantLand(e.To) {
			return state, ErrInvalidState
		}
		if owner, ok := current.TerritoryOwner(from); !ok || owner != index {
			return state, ErrInvalidState
		}
		current.Territories = append(slices.DeleteFunc(slices.Clone(current.Territories), func(t Territory) bool { return t.Position == e.From }),
			Territory{Position: e.To, Creature: from.Creature})
		current.Phase = PhaseEnding
	case *TerritoryInvadedByMoveEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		from, ok := current.TerritoryAt(e.From)
		placed, found := current.PlacedCreature(from.Creature)
		target, occupied := current.TerritoryAt(e.Position)
		if !ok || !found || !occupied || !current.acceptsTerritoryCommand() || placed.Owner != index {
			return state, ErrInvalidState
		}
		if owner, ok := current.TerritoryOwner(target); !ok || owner == index {
			return state, ErrInvalidState
		}
		origin := e.From
		current.Invasion = &Invasion{Card: placed.Card, Base: placed.Base, Attack: placed.Attack, HitPoints: placed.HitPoints, Position: e.Position, From: &origin, Creature: placed.ID}
		current.Phase = PhaseAttacking
	case *LandElementChangedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		territory, ok := current.TerritoryAt(e.Position)
		if !ok || !current.acceptsTerritoryCommand() || !slices.Contains(changeableLandElements, e.Element) || current.landElement(e.Position) == e.Element {
			return state, ErrInvalidState
		}
		if owner, ok := current.TerritoryOwner(territory); !ok || owner != index {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Magic -= e.Magic
		// 盤面の写しのマスを変える（連鎖・地形効果・画面がマスの属性を 1 か所で読むため）
		current.Board.Tiles = slices.Clone(current.Board.Tiles)
		for i := range current.Board.Tiles {
			if current.Board.Tiles[i].Position == e.Position {
				current.Board.Tiles[i].Element = e.Element
			}
		}
		current.Phase = PhaseEnding
	case *CreatureExchangedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		participant := current.Participants[index]
		exchanged, inHand := participant.handCard(e.HandIndex)
		spec, specified := current.CardSpecs()[exchanged]
		territory, ok := current.TerritoryAt(e.Position)
		if !ok {
			return state, ErrInvalidState
		}
		replaced, ok := current.PlacedCreature(territory.Creature)
		switch {
		case !ok, !current.acceptsTerritoryCommand(), replaced.Owner != index,
			!inHand, !specified, spec.Creature == nil, e.Creature != current.LastPlacedCreatureID+1:
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Hand = append(slices.Delete(slices.Clone(participant.Hand), e.HandIndex, e.HandIndex+1), replaced.Card)
		current.Participants[index].Magic -= e.Cost.Int()
		current.PlacedCreatures = append(
			slices.DeleteFunc(slices.Clone(current.PlacedCreatures), func(d PlacedCreature) bool { return d.ID == replaced.ID }),
			PlacedCreature{ID: e.Creature, Card: exchanged, Owner: index, Base: *spec.Creature, Attack: spec.Creature.Attack, HitPoints: spec.Creature.HitPoints},
		)
		current.Territories = slices.Clone(current.Territories)
		for i := range current.Territories {
			if current.Territories[i].Position == e.Position {
				current.Territories[i].Creature = e.Creature
			}
		}
		current.LastPlacedCreatureID = e.Creature
		current.Phase = PhaseEnding
	case *TerritoryInvadedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		participant := current.Participants[index]
		invading, inHand := participant.handCard(e.HandIndex)
		spec, ok := current.CardSpecs()[invading]
		territory, occupied := current.TerritoryAt(participant.Position)
		var owner ParticipantIndex
		if occupied {
			owner, occupied = current.TerritoryOwner(territory)
		}
		switch {
		case current.Phase != PhaseInvading, !occupied, owner == index, !inHand, !ok, spec.Creature == nil:
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Hand = slices.Delete(slices.Clone(participant.Hand), e.HandIndex, e.HandIndex+1)
		current.Participants[index].Magic -= e.Cost.Int()
		current.Invasion = &Invasion{Card: invading, Base: *spec.Creature, Attack: spec.Creature.Attack, HitPoints: spec.Creature.HitPoints, Position: participant.Position}
		current.Phase = PhaseAttacking
	case *ItemUsedEvent:
		if !current.awaitsItemChoiceOf(e.Participant) {
			return state, ErrInvalidState
		}
		hand := current.Participants[e.Participant].Hand
		used, ok := current.Participants[e.Participant].handCard(e.HandIndex)
		if !ok {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[e.Participant].Hand = slices.Delete(slices.Clone(hand), e.HandIndex, e.HandIndex+1)
		current.Participants[e.Participant].DiscardPile = append(slices.Clone(current.Participants[e.Participant].DiscardPile), used)
		current.Participants[e.Participant].Magic -= e.Cost.Int()
		invasion := *current.Invasion
		if current.Phase == PhaseAttacking {
			invasion.AttackerItem = &used
		} else {
			invasion.DefenderItem = &used
		}
		current.Invasion = &invasion
		current.Phase = current.afterItemChoice()
	case *ItemSkippedEvent:
		if !current.awaitsItemChoiceOf(e.Participant) {
			return state, ErrInvalidState
		}
		current.Phase = current.afterItemChoice()
	case *BattleStartedEvent:
		if current.Phase != PhaseDefending || current.Invasion == nil {
			return state, ErrInvalidState
		}
		battle, err := evolveBattle(nil, e)
		if err != nil {
			return state, err
		}
		current.Battle = battle
		current.Phase = PhaseBattling
	case *BattleSupportEffectActivatedEvent, *BattleTerrainEffectActivatedEvent, *BattleCreatureAbilityActivatedEvent, *BattleItemEffectActivatedEvent, *CreatureStruckEvent:
		if current.Phase != PhaseBattling {
			return state, ErrInvalidState
		}
		battle, err := evolveBattle(current.Battle, e)
		if err != nil {
			return state, err
		}
		current.Battle = battle
	case *TollMultiplierActivatedEvent:
		territory, ok := current.invadedTerritory()
		if !ok {
			return state, ErrInvalidState
		}
		if _, ok := current.tollMultiplier(territory); !ok {
			return state, ErrInvalidState
		}
	case *TerritoryConqueredEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		if current.Phase != PhaseBattling || current.Invasion == nil || current.Battle == nil {
			return state, ErrInvalidState
		}
		territory, ok := current.TerritoryAt(current.Invasion.Position)
		if !ok {
			return state, ErrInvalidState
		}
		defeated, found := current.PlacedCreature(territory.Creature)
		if !found {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[defeated.Owner].DiscardPile = append(slices.Clone(current.Participants[defeated.Owner].DiscardPile), defeated.Card)
		invasion := *current.Invasion
		if invasion.From != nil {
			// 移動で侵略したクリーチャーは同じ 1 体のまま移り、元の土地は空き地になる
			if e.Creature != invasion.Creature {
				return state, ErrInvalidState
			}
			current.PlacedCreatures = slices.DeleteFunc(slices.Clone(current.PlacedCreatures), func(d PlacedCreature) bool { return d.ID == defeated.ID })
			for i := range current.PlacedCreatures {
				if current.PlacedCreatures[i].ID == e.Creature {
					current.PlacedCreatures[i].HitPoints = e.TerritoryHitPoints
				}
			}
			current.Territories = slices.DeleteFunc(slices.Clone(current.Territories), func(t Territory) bool { return t.Position == *invasion.From })
			for i := range current.Territories {
				if current.Territories[i].Position == invasion.Position {
					current.Territories[i].Creature = e.Creature
				}
			}
			current.Invasion = nil
			current.Battle = nil
			current.Phase = PhaseEnding
			break
		}
		if e.Creature != current.LastPlacedCreatureID+1 {
			return state, ErrInvalidState
		}
		current.PlacedCreatures = append(
			slices.DeleteFunc(slices.Clone(current.PlacedCreatures), func(d PlacedCreature) bool { return d.ID == defeated.ID }),
			PlacedCreature{ID: e.Creature, Card: invasion.Card, Owner: index, Base: invasion.Base, Attack: invasion.Attack, HitPoints: e.TerritoryHitPoints},
		)
		current.Territories = slices.Clone(current.Territories)
		for i := range current.Territories {
			if current.Territories[i].Position == invasion.Position {
				current.Territories[i].Creature = e.Creature
			}
		}
		current.LastPlacedCreatureID = e.Creature
		current.Invasion = nil
		current.Battle = nil
		current.Phase = PhaseEnding
	case *InvasionRepelledEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		participant := current.Participants[index]
		if current.Phase != PhaseBattling || current.Invasion == nil || current.Battle == nil {
			return state, ErrInvalidState
		}
		territory, ok := current.TerritoryAt(current.Invasion.Position)
		survived := current.Battle.Attacker.Stats().HitPoints.Int() > 0
		moved := current.Invasion.From != nil
		if !ok || (e.ReturnedHitPoints != nil) != (survived && moved) {
			return state, ErrInvalidState
		}
		if moved {
			current.PlacedCreatures = slices.Clone(current.PlacedCreatures)
			if survived {
				for i := range current.PlacedCreatures {
					if current.PlacedCreatures[i].ID == current.Invasion.Creature {
						current.PlacedCreatures[i].HitPoints = *e.ReturnedHitPoints
					}
				}
			} else {
				from, mover := *current.Invasion.From, current.Invasion.Creature
				current.PlacedCreatures = slices.DeleteFunc(current.PlacedCreatures, func(d PlacedCreature) bool { return d.ID == mover })
				current.Territories = slices.DeleteFunc(slices.Clone(current.Territories), func(t Territory) bool { return t.Position == from })
			}
		}
		current.PlacedCreatures = slices.Clone(current.PlacedCreatures)
		for i := range current.PlacedCreatures {
			if current.PlacedCreatures[i].ID == territory.Creature {
				current.PlacedCreatures[i].HitPoints = e.TerritoryHitPoints
			}
		}
		current.Participants = slices.Clone(current.Participants)
		switch {
		case survived && !moved:
			current.Participants[index].Hand = append(slices.Clone(participant.Hand), current.Invasion.Card)
		case !survived:
			current.Participants[index].DiscardPile = append(slices.Clone(participant.DiscardPile), current.Invasion.Card)
		}
		current.Invasion = nil
		current.Battle = nil
		current.Phase = PhaseEnding
	case *TurnEndedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		// 終わるのは SpellEffectEndedEvent。残るターンがあれば 1 減らす
		if lasting := current.Participants[index].LastingSpellEffect; lasting != nil {
			remaining := *lasting
			remaining.RemainingTurns--
			current.Participants = slices.Clone(current.Participants)
			current.Participants[index].LastingSpellEffect = &remaining
		}
		if current.Turn+1 < len(current.TurnOrder) {
			current.Turn++
		} else {
			current.Turn = 0
			current.Round++
		}
		current.RemainingSteps = 0
		current.Dice = 0
		current.PassedPositions = nil
		current.SpellUsed = false
		current.Phase = current.turnStartPhase()
	case *MatchEndedEvent:
		current.Result = &Result{Winners: e.Winners, Ranks: e.Ranks, Reason: e.Reason}
		current.Phase = PhaseEnded
	case *RewardDrawnEvent:
		if current.Phase != PhaseEnded || int(e.Participant) >= len(current.Participants) || !current.CanChooseRewardPack(e.Participant) {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[e.Participant].Reward = &Reward{Pack: e.Pack, Cards: e.Cards}
	case *RewardsCompletedEvent:
		if current.Phase != PhaseEnded || current.RewardsCompleted || !current.allPlayersRewarded() {
			return state, ErrInvalidState
		}
		current.RewardsCompleted = true
	default:
		return state, ErrInvalidState
	}
	return current, nil
}

// 攻める段階・守る段階で、この参加者のアイテムの選びを待っているか
func (m Match) awaitsItemChoiceOf(participant ParticipantIndex) bool {
	if (m.Phase != PhaseAttacking && m.Phase != PhaseDefending) || m.Invasion == nil {
		return false
	}
	awaiting, ok := m.AwaitingParticipant()
	return ok && awaiting == participant && int(participant) < len(m.Participants)
}

// 攻める側が選んだら守る側へ。守る側が選んだ後は、続く戦闘のイベントが段階を進める
func (m Match) afterItemChoice() Phase {
	if m.Phase == PhaseAttacking {
		return PhaseDefending
	}
	return m.Phase
}

// 読み直しで不整合に気づけるよう、イベントの参加者が今の番と違えば断る
func (m Match) actor(by ParticipantIndex) (ParticipantIndex, error) {
	index, err := m.currentIndex()
	if err != nil || index != by {
		return 0, ErrInvalidState
	}
	return index, nil
}

func (m Match) currentIndex() (ParticipantIndex, error) {
	index, ok := m.CurrentParticipant()
	if !ok || int(index) >= len(m.Participants) {
		return 0, ErrInvalidState
	}
	return index, nil
}

// 山札も捨て札も空なら引く操作は待たない（引けないのに止まらないように）。山札だけが空なら、引くときに捨て札を混ぜて山札にする
func (m Match) turnStartPhase() Phase {
	index, err := m.currentIndex()
	if err != nil || (len(m.Participants[index].DrawPile) == 0 && len(m.Participants[index].DiscardPile) == 0) {
		return PhaseRolling
	}
	return PhaseDrawing
}

func (m Match) exceedsHandLimit(index ParticipantIndex) bool {
	return len(m.Participants[index].Hand) > m.Rules.MaxHandCards.Int()
}

// 捨てた後も確かめ直すのは、上限を 2 枚以上超えたとき（引くスペル等）に 1 枚でターンを終えないため。上限に収まれば Decide がターンを終える
func (m Match) handLimitPhase(index ParticipantIndex) Phase {
	if m.exceedsHandLimit(index) {
		return PhaseDiscarding
	}
	return PhaseEnding
}

func sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

// 混ぜ直した山札が捨て札と同じカードの組か（並びは問わない）
func sameCards(a, b []card.Ref) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[card.Ref]int, len(a))
	for _, ref := range a {
		counts[ref]++
	}
	for _, ref := range b {
		counts[ref]--
		if counts[ref] < 0 {
			return false
		}
	}
	return true
}

// 進んだ直後の段階。行き先が無いのに歩数が残る形（つながりの無い拠点）は、止まって終えるしかない
func (m Match) settle() Phase {
	if m.RemainingSteps <= 0 {
		return m.stopPhase()
	}
	switch choices := m.PathChoices(); {
	case len(choices) == 0:
		return m.stopPhase()
	case len(choices) > 1:
		return PhaseChoosingPath
	default:
		return PhaseMoving
	}
}

// 払った・手放した後の段階。負のままで手放せる領地があれば手放す。領地が無く負のままなら Decide が魔力の枯渇を続ける
func (m Match) debtPhase(index ParticipantIndex) Phase {
	if m.Participants[index].Magic < 0 && len(m.territoriesOf(index)) > 0 {
		return PhaseRelinquishing
	}
	return PhaseEnding
}

// 止まった後の段階。止まったマスに行動が無くても、通った自分の領地があれば領地コマンドを選ぶ（止まったマスの行動と同じ選択肢に並ぶ。2026-11-09 オーナー）
func (m Match) stopPhase() Phase {
	phase := m.stopActionPhase()
	if phase == PhaseEnding && len(m.passedOwnTerritories()) > 0 {
		return PhaseTerritoryCommanding
	}
	return phase
}

// 止まったマスでの行動。空いている土地なら配置を選び（配置できる手札が無くても、配置しないを選ぶ段階に入る。2026-11-08 オーナー）、
// 他人の領地なら通行料か侵略かを選ぶ
func (m Match) stopActionPhase() Phase {
	index, err := m.currentIndex()
	if err != nil {
		return PhaseEnding
	}
	if m.isVacantLand(m.Participants[index].Position) {
		return PhasePlacing
	}
	if territory, ok := m.invadedTerritory(); ok {
		if owner, ok := m.TerritoryOwner(territory); ok && owner != index {
			return PhaseInvading
		}
	}
	return PhaseEnding
}

// 戦闘の出来事を戦闘の中の値に当てる。Evolve と resolveBattle が同じ 1 か所で値を進めるため（戦闘を入力と乱数の元だけで決めても、
// 保存した後に当て直した値とずれないように。core#171）
func evolveBattle(battle *Battle, event Event) (*Battle, error) {
	switch e := event.(type) {
	case *BattleStartedEvent:
		attacker, defender := e.Input.Attacker, e.Input.Defender
		return &Battle{
			Attacker: BattleCreature{Base: BattleStats{Attack: attacker.Spec.Attack, HitPoints: attacker.Spec.HitPoints}, InitialAttack: attacker.Attack, InitialHitPoints: attacker.HitPoints, ReceivedDamagePercent: 100},
			Defender: BattleCreature{Base: BattleStats{Attack: defender.Spec.Attack, HitPoints: defender.Spec.HitPoints}, InitialAttack: defender.Attack, InitialHitPoints: defender.HitPoints, ReceivedDamagePercent: 100},
		}, nil
	case *BattleSupportEffectActivatedEvent:
		if battle == nil {
			return nil, ErrInvalidState
		}
		return battle.withModifier(BattleSideAttacker, BattleModifierSource{Kind: BattleModifierSourceSupportEffect}, BattleStatChange{AttackDelta: e.AttackDelta}), nil
	case *BattleTerrainEffectActivatedEvent:
		if battle == nil {
			return nil, ErrInvalidState
		}
		return battle.withModifier(BattleSideDefender, BattleModifierSource{Kind: BattleModifierSourceTerrainEffect}, BattleStatChange{HitPointsDelta: e.HitPointsDelta}), nil
	case *BattleItemEffectActivatedEvent:
		if battle == nil || !e.Side.valid() {
			return nil, ErrInvalidState
		}
		switch e.Effect {
		case item.EffectKindAttackModifier, item.EffectKindHitPointsModifier, item.EffectKindAttackOverride, item.EffectKindAttackMultiplier:
			if e.Change == nil {
				return nil, ErrInvalidState
			}
			return battle.withModifier(e.Side, BattleModifierSource{Kind: BattleModifierSourceItemEffect, ItemEffect: e.Effect}, *e.Change), nil
		case item.EffectKindIgnoreTerrainEffect:
			// 無視されるのは相手の地形効果なので、相手の側に持たせる
			if e.Change != nil {
				return nil, ErrInvalidState
			}
			opponent := battle.Creature(e.Side.opponent())
			opponent.TerrainEffectIgnored = true
			return battle.withCreature(e.Side.opponent(), opponent), nil
		}
		return nil, ErrInvalidState
	// 発動で得た状態を戦闘の中の値に持たせ、後の攻撃の順とダメージはこの値だけで決める（入力の性能を見直すと、
	// イベントの並びだけでは「なぜその順か」「なぜ 0 か」が決まらないため。2026-11-14 オーナー。core#175）
	case *BattleCreatureAbilityActivatedEvent:
		if battle == nil || !e.Side.valid() {
			return nil, ErrInvalidState
		}
		switch e.Ability {
		case BattleAbilityBattleModifier, BattleAbilityBattleRandomStats:
			if e.Change == nil {
				return nil, ErrInvalidState
			}
			return battle.withModifier(e.Side, BattleModifierSource{Kind: BattleModifierSourceCreatureAbility, Ability: e.Ability}, *e.Change), nil
		}
		if e.Change != nil {
			return nil, ErrInvalidState
		}
		activated := battle.Creature(e.Side)
		switch e.Ability {
		case BattleAbilityFirstStrike:
			activated.StrikePriority = 1
		case BattleAbilityLastStrike:
			activated.StrikePriority = -1
		case BattleAbilityAttackImmunity:
			activated.ReceivedDamagePercent = min(activated.ReceivedDamagePercent, 0)
		default:
			return nil, ErrInvalidState
		}
		return battle.withCreature(e.Side, activated), nil
	case *CreatureStruckEvent:
		if battle == nil || !e.Striker.valid() {
			return nil, ErrInvalidState
		}
		struck := battle.Creature(e.Striker.opponent())
		struck.Damage += struck.Stats().HitPoints.Int() - e.RemainingHitPoints.Int()
		return battle.withCreature(e.Striker.opponent(), struck), nil
	}
	return nil, ErrInvalidState
}

// 手札のその番号のカード。番号が手札の外なら false
func (p ParticipantState) handCard(handIndex int) (card.Ref, bool) {
	if handIndex < 0 || handIndex >= len(p.Hand) {
		return card.Ref{}, false
	}
	return p.Hand[handIndex], true
}

// 通行料を受け取る人（止まっているマスの他人の領地の持ち主）
func (m Match) tollOwner(payer ParticipantIndex) (ParticipantIndex, bool) {
	territory, ok := m.invadedTerritory()
	if !ok {
		return 0, false
	}
	owner, ok := m.TerritoryOwner(territory)
	if !ok || owner == payer {
		return 0, false
	}
	return owner, true
}
