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
		return PhaseEnding
	}
	switch choices := m.PathChoices(); {
	case len(choices) == 0:
		return PhaseEnding
	case len(choices) > 1:
		return PhaseChoosingPath
	default:
		return PhaseMoving
	}
}
