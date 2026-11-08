package graphql

import (
	"errors"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/presentation/graphql/model"
)

// 対戦の操作に共通の、application 層のエラーから GraphQL のエラー型への写し。使い方は roomError と同じ。写せなければ nil
func matchError(err error) any {
	switch {
	case errors.Is(err, command.ErrMatchNotFound):
		return &model.MatchNotFoundError{Message: "match not found"}
	case errors.Is(err, command.ErrMatchNotYourTurn):
		return &model.MatchNotYourTurnError{Message: "not your turn"}
	case errors.Is(err, command.ErrMatchPhaseNotAllowed):
		return &model.MatchPhaseNotAllowedError{Message: "match phase not allowed"}
	case errors.Is(err, command.ErrMatchRewardAlreadyReceived):
		return &model.MatchRewardAlreadyReceivedError{Message: "match reward already received"}
	case errors.Is(err, command.ErrRevisionConflict):
		return &model.RevisionConflictError{Message: "revision conflict"}
	case errors.Is(err, command.ErrPlayerNotRegistered):
		return &model.PlayerNotRegisteredError{Message: "player not registered"}
	}
	return nil
}

func fromQueryMatch(match query.Match) (*model.Match, error) {
	tiles := make([]model.BoardTile, 0, len(match.Board.Tiles))
	for _, tile := range match.Board.Tiles {
		converted, err := fromBoardTile(tile)
		if err != nil {
			return nil, err
		}
		tiles = append(tiles, converted)
	}
	connections := make([]*model.BoardConnection, 0, len(match.Board.Connections))
	for _, connection := range match.Board.Connections {
		connections = append(connections, &model.BoardConnection{
			A: &model.BoardPosition{X: connection.A.X, Y: connection.A.Y},
			B: &model.BoardPosition{X: connection.B.X, Y: connection.B.Y},
		})
	}
	pairs := make([]*model.BoardTilePair, 0, len(match.Board.Pairs))
	for _, pair := range match.Board.Pairs {
		pairs = append(pairs, &model.BoardTilePair{
			A: &model.BoardPosition{X: pair.A.X, Y: pair.A.Y},
			B: &model.BoardPosition{X: pair.B.X, Y: pair.B.Y},
		})
	}
	participants := make([]*model.MatchParticipant, len(match.Participants))
	for i, participant := range match.Participants {
		kind, err := fromMatchParticipantKind(participant.Kind)
		if err != nil {
			return nil, err
		}
		hand := make([]model.CardInfo, len(participant.Hand))
		for j, card := range participant.Hand {
			if hand[j], err = fromMatchCard(card); err != nil {
				return nil, err
			}
		}
		rewardCards := make([]model.CardInfo, len(participant.RewardCards))
		for j, card := range participant.RewardCards {
			if rewardCards[j], err = fromMatchCard(card); err != nil {
				return nil, err
			}
		}
		participants[i] = &model.MatchParticipant{
			RewardReceived: participant.RewardReceived, RewardCards: rewardCards,
			Kind: kind, PlayerID: participant.Player,
			Avatar:   &model.MatchAvatar{ID: participant.Avatar.ID, Name: participant.Avatar.Name},
			Position: &model.BoardPosition{X: participant.Position.X, Y: participant.Position.Y},
			Magic:    participant.Magic, Hand: hand, DrawPileCount: participant.DrawPileCount, TurnOrder: participant.TurnOrder,
			PassedCheckpoints: fromQueryPositions(participant.PassedCheckpoints),
		}
	}
	rules := match.Rules
	phase, err := fromMatchPhase(match.Phase)
	if err != nil {
		return nil, err
	}
	converted := &model.Match{
		ID: match.ID, Revision: int(match.Revision), Round: match.Round,
		Rules: &model.MatchPlayRules{
			TargetMagic: rules.TargetMagic, StartingMagic: rules.StartingMagic, LapBonus: rules.LapBonus, CheckpointBonus: rules.CheckpointBonus,
			RoundLimit: rules.RoundLimit, DiceMin: rules.DiceMin, DiceMax: rules.DiceMax,
			MaxHandCards: rules.MaxHandCards, StartingHandCards: rules.StartingHandCards,
		},
		Board:        &model.MatchBoard{Width: match.Board.Width, Height: match.Board.Height, Tiles: tiles, Connections: connections, Pairs: pairs},
		Participants: participants, CurrentParticipant: match.CurrentParticipant,
		RewardPacks: fromQueryRewardPacks(match.RewardPacks),
		RoomID:      match.RoomID, Number: match.Number,
		Phase: phase, Dice: match.Dice, RemainingSteps: match.RemainingSteps, PathChoices: fromQueryPositions(match.PathChoices),
	}
	if outcome := match.Outcome; outcome != nil {
		reason, err := fromMatchEndReason(outcome.Reason)
		if err != nil {
			return nil, err
		}
		converted.Outcome = &model.MatchOutcome{Winners: outcome.Winners, Ranks: outcome.Ranks, Reason: reason}
	}
	return converted, nil
}

func fromQueryRewardPacks(packs []query.MatchRewardPack) []*model.MatchRewardPack {
	converted := make([]*model.MatchRewardPack, len(packs))
	for i, pack := range packs {
		converted[i] = &model.MatchRewardPack{Key: pack.Key, Name: pack.Name}
	}
	return converted
}

func fromQueryPositions(positions []query.BoardPosition) []*model.BoardPosition {
	converted := make([]*model.BoardPosition, len(positions))
	for i, position := range positions {
		converted[i] = &model.BoardPosition{X: position.X, Y: position.Y}
	}
	return converted
}

func fromMatchPhase(phase query.MatchPhase) (model.MatchPhase, error) {
	switch phase {
	case query.MatchPhaseDrawing:
		return model.MatchPhaseDrawing, nil
	case query.MatchPhaseDiscarding:
		return model.MatchPhaseDiscarding, nil
	case query.MatchPhaseRolling:
		return model.MatchPhaseRolling, nil
	case query.MatchPhaseChoosingPath:
		return model.MatchPhaseChoosingPath, nil
	case query.MatchPhaseEnding:
		return model.MatchPhaseEnding, nil
	case query.MatchPhaseEnded:
		return model.MatchPhaseEnded, nil
	default:
		return "", fmt.Errorf("graphql: unknown match phase: %q", phase)
	}
}

func fromMatchEndReason(reason query.MatchEndReason) (model.MatchEndReason, error) {
	switch reason {
	case query.MatchEndReasonTargetMagic:
		return model.MatchEndReasonTargetMagic, nil
	case query.MatchEndReasonRoundLimit:
		return model.MatchEndReasonRoundLimit, nil
	default:
		return "", fmt.Errorf("graphql: unknown match end reason: %q", reason)
	}
}

func fromMatchParticipantKind(kind query.MatchParticipantKind) (model.MatchParticipantKind, error) {
	switch kind {
	case query.MatchParticipantKindPlayer:
		return model.MatchParticipantKindPlayer, nil
	case query.MatchParticipantKindNPC:
		return model.MatchParticipantKindNpc, nil
	default:
		return "", fmt.Errorf("graphql: unknown match participant kind: %q", kind)
	}
}

// 1 つだけ送って閉じるチャネル（結果の型で返す誤りを、Subscription では 1 回送って閉じる）
func singleResult[T any](result T) <-chan T {
	results := make(chan T, 1)
	results <- result
	close(results)
	return results
}

func fromQueryMatchEvents(events []query.MatchEvent) (*model.MatchEventList, error) {
	converted := make([]model.MatchEvent, 0, len(events))
	for _, event := range events {
		item, err := fromQueryMatchEvent(event)
		if err != nil {
			return nil, err
		}
		converted = append(converted, item)
	}
	return &model.MatchEventList{Events: converted}, nil
}

func fromQueryMatchEvent(event query.MatchEvent) (model.MatchEvent, error) {
	meta := event.Meta()
	revision := int(meta.Revision)
	switch e := event.(type) {
	case query.MatchCreatedEvent:
		return &model.MatchCreatedEvent{Revision: revision, OccurredAt: meta.OccurredAt}, nil
	case query.MatchTurnOrderDecidedEvent:
		return &model.MatchTurnOrderDecidedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Order: e.Order}, nil
	case query.MatchDecksShuffledEvent:
		return &model.MatchDecksShuffledEvent{Revision: revision, OccurredAt: meta.OccurredAt}, nil
	case query.MatchHandsDealtEvent:
		return &model.MatchHandsDealtEvent{Revision: revision, OccurredAt: meta.OccurredAt, Cards: e.Cards}, nil
	case query.MatchCardDrawnEvent:
		card, err := fromMatchCard(e.Card)
		if err != nil {
			return nil, err
		}
		return &model.MatchCardDrawnEvent{Revision: revision, OccurredAt: meta.OccurredAt, Participant: e.Participant, Card: card}, nil
	case query.MatchCardDiscardedEvent:
		card, err := fromMatchCard(e.Card)
		if err != nil {
			return nil, err
		}
		return &model.MatchCardDiscardedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Participant: e.Participant, HandIndex: e.HandIndex, Card: card}, nil
	case query.MatchDiceRolledEvent:
		return &model.MatchDiceRolledEvent{Revision: revision, OccurredAt: meta.OccurredAt, Participant: e.Participant, Value: e.Value}, nil
	case query.MatchMovedEvent:
		return &model.MatchMovedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Participant: e.Participant, To: &model.BoardPosition{X: e.To.X, Y: e.To.Y}}, nil
	case query.MatchCheckpointPassedEvent:
		return &model.MatchCheckpointPassedEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Participant: e.Participant,
			Checkpoint: &model.BoardPosition{X: e.Checkpoint.X, Y: e.Checkpoint.Y}, Magic: e.Magic,
		}, nil
	case query.MatchLapCompletedEvent:
		return &model.MatchLapCompletedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Participant: e.Participant, Magic: e.Magic}, nil
	case query.MatchTurnEndedEvent:
		return &model.MatchTurnEndedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Participant: e.Participant}, nil
	case query.MatchEndedEvent:
		reason, err := fromMatchEndReason(e.Reason)
		if err != nil {
			return nil, err
		}
		return &model.MatchEndedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Winners: e.Winners, Ranks: e.Ranks, Reason: reason}, nil
	case query.MatchRewardDrawnEvent:
		cards := make([]model.CardInfo, len(e.Cards))
		for i, drawn := range e.Cards {
			converted, err := fromMatchCard(drawn)
			if err != nil {
				return nil, err
			}
			cards[i] = converted
		}
		return &model.MatchRewardDrawnEvent{Revision: revision, OccurredAt: meta.OccurredAt, Participant: e.Participant, Pack: e.Pack, RewardCards: cards}, nil
	case query.MatchRewardsCompletedEvent:
		return &model.MatchRewardsCompletedEvent{Revision: revision, OccurredAt: meta.OccurredAt}, nil
	default:
		return nil, fmt.Errorf("graphql: unknown match event: %T", event)
	}
}

func fromMatchCard(card query.MatchCard) (model.CardInfo, error) {
	return fromCard(card.Type, card.ID, card.Name, card.Cost, card.Rarity, card.Creature)
}
