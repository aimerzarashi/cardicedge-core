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
		hand := make([]*model.DeckCardInfo, len(participant.Hand))
		for j, card := range participant.Hand {
			cardType, err := fromCardType(card.Type)
			if err != nil {
				return nil, err
			}
			hand[j] = &model.DeckCardInfo{ID: card.ID, Name: card.Name, Type: cardType}
		}
		participants[i] = &model.MatchParticipant{
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
		Phase: phase, Dice: match.Dice, RemainingSteps: match.RemainingSteps, PathChoices: fromQueryPositions(match.PathChoices),
	}
	if outcome := match.Outcome; outcome != nil {
		reason, err := fromMatchEndReason(outcome.Reason)
		if err != nil {
			return nil, err
		}
		converted.Outcome = &model.MatchOutcome{Winners: outcome.Winners, Reason: reason}
	}
	return converted, nil
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
