package graphql

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/presentation/graphql/model"
)

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
		}
	}
	rolls := make([]*model.MatchTurnOrderRoll, len(match.TurnOrderRolls))
	for i, roll := range match.TurnOrderRolls {
		rolls[i] = &model.MatchTurnOrderRoll{Participant: roll.Participant, Attempt: roll.Attempt, Value: roll.Value}
	}
	rules := match.Rules
	return &model.Match{
		ID: match.ID, Revision: int(match.Revision), Round: match.Round,
		Rules: &model.MatchPlayRules{
			TargetMagic: rules.TargetMagic, StartingMagic: rules.StartingMagic, LapBonus: rules.LapBonus, CheckpointBonus: rules.CheckpointBonus,
			RoundLimit: rules.RoundLimit, DiceMin: rules.DiceMin, DiceMax: rules.DiceMax,
			MaxHandCards: rules.MaxHandCards, StartingHandCards: rules.StartingHandCards,
		},
		Board:        &model.MatchBoard{Width: match.Board.Width, Height: match.Board.Height, Tiles: tiles, Connections: connections, Pairs: pairs},
		Participants: participants, TurnOrderRolls: rolls, CurrentParticipant: match.CurrentParticipant,
	}, nil
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
