package match

import "slices"

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
		if len(participant.DrawPile) == 0 || participant.DrawPile[0] != e.Card {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Hand = append(slices.Clone(participant.Hand), e.Card)
		current.Participants[index].DrawPile = participant.DrawPile[1:]
		current.Phase = current.handLimitPhase(index)
	case *CardDiscardedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		hand := current.Participants[index].Hand
		if e.HandIndex < 0 || e.HandIndex >= len(hand) || hand[e.HandIndex] != e.Card {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Hand = slices.Delete(slices.Clone(hand), e.HandIndex, e.HandIndex+1)
		current.Phase = current.handLimitPhase(index)
	case *DiceRolledEvent:
		if _, err := current.actor(e.Participant); err != nil {
			return state, err
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
		current.Participants[index].PassedCheckpoints = append(slices.Clone(participant.PassedCheckpoints), e.Checkpoint)
		current.Participants[index].Magic += e.Magic
	case *LapCompletedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Magic += e.Magic
		current.Participants[index].PassedCheckpoints = nil
	case *CreatureDeployedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		participant := current.Participants[index]
		spec, ok := current.CardSpecs()[e.Card]
		switch {
		case current.Phase != PhaseDeploying, e.Position != participant.Position, !current.isVacantLand(e.Position),
			e.HandIndex < 0, e.HandIndex >= len(participant.Hand), participant.Hand[e.HandIndex] != e.Card,
			!ok, spec.Creature == nil, e.Creature != current.LastDeployedCreatureID+1:
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Hand = slices.Delete(slices.Clone(participant.Hand), e.HandIndex, e.HandIndex+1)
		current.Participants[index].Magic -= e.Cost.Int()
		current.DeployedCreatures = append(slices.Clone(current.DeployedCreatures), DeployedCreature{
			ID: e.Creature, Card: e.Card, Owner: index, Base: *spec.Creature, HitPoints: spec.Creature.HitPoints,
		})
		current.Territories = append(slices.Clone(current.Territories), Territory{Position: e.Position, Creature: e.Creature})
		current.LastDeployedCreatureID = e.Creature
		current.Phase = PhaseEnding
	case *TollPaidEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		if (current.Phase != PhaseInvading && current.Phase != PhaseEnding) || e.Position != current.Participants[index].Position ||
			e.Owner < 0 || int(e.Owner) >= len(current.Participants) || e.Owner == index {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Magic -= e.Magic
		current.Participants[e.Owner].Magic += e.Magic
		current.Phase = current.debtPhase(index)
	case *TerritoryRelinquishedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		territory, ok := current.TerritoryAt(e.Position)
		if !ok || current.Phase != PhaseRelinquishing || territory.Creature != e.Creature {
			return state, ErrInvalidState
		}
		if owner, ok := current.TerritoryOwner(territory); !ok || owner != index {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Magic += e.Magic
		current.Territories = slices.DeleteFunc(slices.Clone(current.Territories), func(t Territory) bool { return t.Position == e.Position })
		current.DeployedCreatures = slices.DeleteFunc(slices.Clone(current.DeployedCreatures), func(d DeployedCreature) bool { return d.ID == e.Creature })
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
		if !ok || from.Creature != e.Creature || !current.acceptsTerritoryCommand() || !current.isVacantLand(e.To) {
			return state, ErrInvalidState
		}
		if owner, ok := current.TerritoryOwner(from); !ok || owner != index {
			return state, ErrInvalidState
		}
		current.Territories = append(slices.DeleteFunc(slices.Clone(current.Territories), func(t Territory) bool { return t.Position == e.From }),
			Territory{Position: e.To, Creature: e.Creature})
		current.Phase = PhaseEnding
	case *TerritoryInvadedByMoveEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		from, ok := current.TerritoryAt(e.From)
		deployed, found := current.DeployedCreature(e.Creature)
		target, occupied := current.TerritoryAt(e.Position)
		if !ok || !found || !occupied || from.Creature != e.Creature || !current.acceptsTerritoryCommand() || deployed.Owner != index {
			return state, ErrInvalidState
		}
		if owner, ok := current.TerritoryOwner(target); !ok || owner == index {
			return state, ErrInvalidState
		}
		origin := e.From
		current.Invasion = &Invasion{Card: deployed.Card, Base: deployed.Base, HitPoints: deployed.HitPoints, Position: e.Position, From: &origin, Creature: e.Creature}
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
		spec, specified := current.CardSpecs()[e.Card]
		territory, ok := current.TerritoryAt(e.Position)
		if !ok {
			return state, ErrInvalidState
		}
		replaced, ok := current.DeployedCreature(territory.Creature)
		switch {
		case !ok, !current.acceptsTerritoryCommand(), replaced.Owner != index, replaced.Card != e.ReturnedCard,
			e.HandIndex < 0, e.HandIndex >= len(participant.Hand), participant.Hand[e.HandIndex] != e.Card,
			!specified, spec.Creature == nil, e.Creature != current.LastDeployedCreatureID+1:
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Hand = append(slices.Delete(slices.Clone(participant.Hand), e.HandIndex, e.HandIndex+1), e.ReturnedCard)
		current.Participants[index].Magic -= e.Cost.Int()
		current.DeployedCreatures = append(
			slices.DeleteFunc(slices.Clone(current.DeployedCreatures), func(d DeployedCreature) bool { return d.ID == replaced.ID }),
			DeployedCreature{ID: e.Creature, Card: e.Card, Owner: index, Base: *spec.Creature, HitPoints: spec.Creature.HitPoints},
		)
		current.Territories = slices.Clone(current.Territories)
		for i := range current.Territories {
			if current.Territories[i].Position == e.Position {
				current.Territories[i].Creature = e.Creature
			}
		}
		current.LastDeployedCreatureID = e.Creature
		current.Phase = PhaseEnding
	case *TerritoryInvadedEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		participant := current.Participants[index]
		spec, ok := current.CardSpecs()[e.Card]
		territory, occupied := current.TerritoryAt(e.Position)
		var owner ParticipantIndex
		if occupied {
			owner, occupied = current.TerritoryOwner(territory)
		}
		switch {
		case current.Phase != PhaseInvading, e.Position != participant.Position, !occupied, owner == index,
			e.HandIndex < 0, e.HandIndex >= len(participant.Hand), participant.Hand[e.HandIndex] != e.Card,
			!ok, spec.Creature == nil:
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[index].Hand = slices.Delete(slices.Clone(participant.Hand), e.HandIndex, e.HandIndex+1)
		current.Participants[index].Magic -= e.Cost.Int()
		current.Invasion = &Invasion{Card: e.Card, Base: *spec.Creature, HitPoints: spec.Creature.HitPoints, Position: e.Position}
		current.Phase = PhaseAttacking
	case *ItemUsedEvent:
		if !current.awaitsItemChoiceOf(e.Participant) {
			return state, ErrInvalidState
		}
		hand := current.Participants[e.Participant].Hand
		if e.HandIndex < 0 || e.HandIndex >= len(hand) || hand[e.HandIndex] != e.Card {
			return state, ErrInvalidState
		}
		current.Participants = slices.Clone(current.Participants)
		current.Participants[e.Participant].Hand = slices.Delete(slices.Clone(hand), e.HandIndex, e.HandIndex+1)
		current.Participants[e.Participant].Magic -= e.Cost.Int()
		invasion := *current.Invasion
		used := e.Card
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
		current.Battle = &Battle{
			Attacker: BattleCreature{Stats: e.Attacker, InitialHitPoints: e.Attacker.HitPoints},
			Defender: BattleCreature{Stats: e.Defender, InitialHitPoints: e.Defender.HitPoints},
		}
		current.Phase = PhaseBattling
	case *BattleStatModifiedEvent:
		if current.Phase != PhaseBattling || current.Battle == nil || !e.Side.valid() {
			return state, ErrInvalidState
		}
		modified := current.Battle.creature(e.Side)
		modified.Stats = BattleStats{Attack: e.Attack, HitPoints: e.HitPoints}
		modified.HitPointsBonus += e.HitPointsDelta
		current.Battle = current.Battle.withCreature(e.Side, modified)
	case *CreatureStruckEvent:
		if current.Phase != PhaseBattling || current.Battle == nil || !e.Striker.valid() {
			return state, ErrInvalidState
		}
		struck := current.Battle.creature(e.Striker.opponent())
		struck.Stats.HitPoints = e.RemainingHitPoints
		current.Battle = current.Battle.withCreature(e.Striker.opponent(), struck)
	case *TerritoryConqueredEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		territory, ok := current.TerritoryAt(e.Position)
		if !ok || current.Phase != PhaseBattling || current.Invasion == nil || current.Battle == nil || territory.Creature != e.DefeatedCreature ||
			e.Position != current.Invasion.Position {
			return state, ErrInvalidState
		}
		invasion := *current.Invasion
		if invasion.From != nil {
			// 移動で侵略したクリーチャーは同じ 1 体のまま移り、元の土地は空き地になる
			if e.Creature != invasion.Creature {
				return state, ErrInvalidState
			}
			current.DeployedCreatures = slices.DeleteFunc(slices.Clone(current.DeployedCreatures), func(d DeployedCreature) bool { return d.ID == e.DefeatedCreature })
			for i := range current.DeployedCreatures {
				if current.DeployedCreatures[i].ID == e.Creature {
					current.DeployedCreatures[i].HitPoints = e.TerritoryHitPoints
				}
			}
			current.Territories = slices.DeleteFunc(slices.Clone(current.Territories), func(t Territory) bool { return t.Position == *invasion.From })
			for i := range current.Territories {
				if current.Territories[i].Position == e.Position {
					current.Territories[i].Creature = e.Creature
				}
			}
			current.Invasion = nil
			current.Battle = nil
			current.Phase = PhaseEnding
			break
		}
		if e.Creature != current.LastDeployedCreatureID+1 {
			return state, ErrInvalidState
		}
		current.DeployedCreatures = append(
			slices.DeleteFunc(slices.Clone(current.DeployedCreatures), func(d DeployedCreature) bool { return d.ID == e.DefeatedCreature }),
			DeployedCreature{ID: e.Creature, Card: invasion.Card, Owner: index, Base: invasion.Base, HitPoints: e.TerritoryHitPoints},
		)
		current.Territories = slices.Clone(current.Territories)
		for i := range current.Territories {
			if current.Territories[i].Position == e.Position {
				current.Territories[i].Creature = e.Creature
			}
		}
		current.LastDeployedCreatureID = e.Creature
		current.Invasion = nil
		current.Battle = nil
		current.Phase = PhaseEnding
	case *InvasionRepelledEvent:
		index, err := current.actor(e.Participant)
		if err != nil {
			return state, err
		}
		participant := current.Participants[index]
		territory, ok := current.TerritoryAt(e.Position)
		if !ok || current.Phase != PhaseBattling || current.Invasion == nil || current.Battle == nil || e.Position != current.Invasion.Position ||
			e.Card != current.Invasion.Card {
			return state, ErrInvalidState
		}
		survived := current.Battle.Attacker.Stats.HitPoints.Int() > 0
		moved := current.Invasion.From != nil
		if e.ReturnedToHand != (survived && !moved) || (e.ReturnedTo != nil) != (survived && moved) || (e.ReturnedTo != nil) != (e.ReturnedHitPoints != nil) {
			return state, ErrInvalidState
		}
		if moved {
			current.DeployedCreatures = slices.Clone(current.DeployedCreatures)
			if survived {
				for i := range current.DeployedCreatures {
					if current.DeployedCreatures[i].ID == current.Invasion.Creature {
						current.DeployedCreatures[i].HitPoints = *e.ReturnedHitPoints
					}
				}
			} else {
				from, mover := *current.Invasion.From, current.Invasion.Creature
				current.DeployedCreatures = slices.DeleteFunc(current.DeployedCreatures, func(d DeployedCreature) bool { return d.ID == mover })
				current.Territories = slices.DeleteFunc(slices.Clone(current.Territories), func(t Territory) bool { return t.Position == from })
			}
		}
		current.DeployedCreatures = slices.Clone(current.DeployedCreatures)
		for i := range current.DeployedCreatures {
			if current.DeployedCreatures[i].ID == territory.Creature {
				current.DeployedCreatures[i].HitPoints = e.TerritoryHitPoints
			}
		}
		if e.ReturnedToHand {
			current.Participants = slices.Clone(current.Participants)
			current.Participants[index].Hand = append(slices.Clone(participant.Hand), e.Card)
		}
		current.Invasion = nil
		current.Battle = nil
		current.Phase = PhaseEnding
	case *TurnEndedEvent:
		if _, err := current.actor(e.Participant); err != nil {
			return state, err
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

// 山札が空なら引く操作は待たない（引けないのに止まらないように）
func (m Match) turnStartPhase() Phase {
	index, err := m.currentIndex()
	if err != nil || len(m.Participants[index].DrawPile) == 0 {
		return PhaseRolling
	}
	return PhaseDrawing
}

// 捨てた後も確かめ直すのは、上限を 2 枚以上超える形（配る枚数や引く枚数を変えたとき）でも 1 枚で振る段階に進まないため
func (m Match) handLimitPhase(index ParticipantIndex) Phase {
	if len(m.Participants[index].Hand) > m.Rules.MaxHandCards.Int() {
		return PhaseDiscarding
	}
	return PhaseRolling
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
		return PhaseDeploying
	}
	if territory, ok := m.invadedTerritory(); ok {
		if owner, ok := m.TerritoryOwner(territory); ok && owner != index {
			return PhaseInvading
		}
	}
	return PhaseEnding
}
