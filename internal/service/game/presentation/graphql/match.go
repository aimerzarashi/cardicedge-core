package graphql

import (
	"errors"
	"fmt"
	"strings"

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
		discardPile := make([]model.CardInfo, len(participant.DiscardPile))
		for j, card := range participant.DiscardPile {
			if discardPile[j], err = fromMatchCard(card); err != nil {
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
			Avatar:     &model.MatchAvatar{ID: participant.Avatar.ID, Name: participant.Avatar.Name},
			Position:   &model.BoardPosition{X: participant.Position.X, Y: participant.Position.Y},
			TotalMagic: participant.TotalMagic,
			Magic:      participant.Magic, Hand: hand, DrawPileCount: participant.DrawPileCount, DiscardPile: discardPile, TurnOrder: participant.TurnOrder,
			PassedCheckpoints:  fromQueryPositions(participant.PassedCheckpoints),
			LastingSpellEffect: fromMatchLastingSpellEffect(participant.LastingSpellEffect),
		}
	}
	rules := match.Rules
	phase, err := fromMatchPhase(match.Phase)
	if err != nil {
		return nil, err
	}
	territories := make([]*model.MatchTerritory, len(match.Territories))
	for i, territory := range match.Territories {
		card, err := fromMatchCard(territory.Creature.Card)
		if err != nil {
			return nil, err
		}
		creatureCard, ok := card.(*model.CreatureCardInfo)
		if !ok {
			return nil, fmt.Errorf("graphql: placed creature %d is not a creature card", territory.Creature.ID)
		}
		territories[i] = &model.MatchTerritory{
			Position: &model.BoardPosition{X: territory.Position.X, Y: territory.Position.Y}, Owner: territory.Owner,
			Creature: &model.MatchPlacedCreature{ID: territory.Creature.ID, Card: creatureCard, HitPoints: territory.Creature.HitPoints},
			Level:    territory.Level, Value: territory.Value, Toll: territory.Toll, RelinquishMagic: territory.RelinquishMagic,
		}
	}
	elementChangeChoices, err := fromQueryElementChangeChoices(match.ElementChangeChoices)
	if err != nil {
		return nil, err
	}
	converted := &model.Match{
		ID: match.ID, Revision: int(match.Revision), Round: match.Round,
		Rules: &model.MatchPlayRules{
			TargetMagic: rules.TargetMagic, StartingMagic: rules.StartingMagic, LapBonus: rules.LapBonus, CheckpointBonus: rules.CheckpointBonus,
			RoundLimit: rules.RoundLimit, DiceMin: rules.DiceMin, DiceMax: rules.DiceMax, BaseLandValue: rules.BaseLandValue,
			MaxHandCards: rules.MaxHandCards, StartingHandCards: rules.StartingHandCards,
			LandLevelMultipliers: rules.LandLevelMultipliers, ChainMultipliers: rules.ChainMultipliers,
			TollPercentage: rules.TollPercentage, RelinquishPercentage: rules.RelinquishPercentage,
		},
		Board:        &model.MatchBoard{Width: match.Board.Width, Height: match.Board.Height, Tiles: tiles, Connections: connections, Pairs: pairs},
		Participants: participants, CurrentParticipant: match.CurrentParticipant, AwaitingParticipant: match.AwaitingParticipant,
		RewardPacks: fromQueryRewardPacks(match.RewardPacks),
		RoomID:      match.RoomID, Number: match.Number,
		Phase: phase, Dice: match.Dice, RemainingSteps: match.RemainingSteps, PathChoices: fromQueryPositions(match.PathChoices),
		SpellChoices: fromSpellChoices(match.SpellChoices), PlaceChoices: match.PlaceChoices, InvadeChoices: match.InvadeChoices, ItemChoices: match.ItemChoices,
		RelinquishChoices: fromQueryPositions(match.RelinquishChoices), Territories: territories,
		LandLevels: fromQueryLandLevels(match.LandLevels), MoveChoices: fromQueryMoveChoices(match.MoveChoices), LevelUpChoices: fromQueryLevelUpChoices(match.LevelUpChoices), ExchangeChoices: fromQueryExchangeChoices(match.ExchangeChoices),
		ElementChangeChoices: elementChangeChoices,
	}
	if invasion := match.Invasion; invasion != nil {
		card, err := fromMatchCard(invasion.Card)
		if err != nil {
			return nil, err
		}
		creatureCard, ok := card.(*model.CreatureCardInfo)
		if !ok {
			return nil, errors.New("graphql: invading card is not a creature card")
		}
		converted.Invasion = &model.MatchInvasion{
			Position: &model.BoardPosition{X: invasion.Position.X, Y: invasion.Position.Y}, Card: creatureCard, HitPoints: invasion.HitPoints,
		}
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

func fromQueryMoveChoices(choices []query.MatchMoveChoice) []*model.MatchMoveChoice {
	converted := make([]*model.MatchMoveChoice, len(choices))
	for i, choice := range choices {
		converted[i] = &model.MatchMoveChoice{From: &model.BoardPosition{X: choice.From.X, Y: choice.From.Y}, To: &model.BoardPosition{X: choice.To.X, Y: choice.To.Y}}
	}
	return converted
}

func fromQueryLandLevels(lands []query.MatchLandLevel) []*model.MatchLandLevel {
	converted := make([]*model.MatchLandLevel, len(lands))
	for i, land := range lands {
		converted[i] = &model.MatchLandLevel{Position: &model.BoardPosition{X: land.Position.X, Y: land.Position.Y}, Level: land.Level}
	}
	return converted
}

func fromQueryLevelUpChoices(choices []query.MatchLevelUpChoice) []*model.MatchLevelUpChoice {
	converted := make([]*model.MatchLevelUpChoice, len(choices))
	for i, choice := range choices {
		converted[i] = &model.MatchLevelUpChoice{Position: &model.BoardPosition{X: choice.Position.X, Y: choice.Position.Y}, Level: choice.Level, Magic: choice.Magic}
	}
	return converted
}

func fromQueryExchangeChoices(choices []query.MatchExchangeChoice) []*model.MatchExchangeChoice {
	converted := make([]*model.MatchExchangeChoice, len(choices))
	for i, choice := range choices {
		converted[i] = &model.MatchExchangeChoice{Position: &model.BoardPosition{X: choice.Position.X, Y: choice.Position.Y}, HandIndex: choice.HandIndex, Magic: choice.Magic}
	}
	return converted
}

// GraphQL の属性（FIRE 等）を、application が受け取る属性の名前（fire 等）にする
func toBoardElementName(element model.BoardTileElement) string {
	return strings.ToLower(string(element))
}

func fromQueryElementChangeChoices(choices []query.MatchElementChangeChoice) ([]*model.MatchElementChangeChoice, error) {
	converted := make([]*model.MatchElementChangeChoice, len(choices))
	for i, choice := range choices {
		element, err := fromBoardTileElement(choice.Element)
		if err != nil {
			return nil, err
		}
		converted[i] = &model.MatchElementChangeChoice{Position: &model.BoardPosition{X: choice.Position.X, Y: choice.Position.Y}, Element: element, Magic: choice.Magic}
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
	case query.MatchPhasePlacing:
		return model.MatchPhasePlacing, nil
	case query.MatchPhaseInvading:
		return model.MatchPhaseInvading, nil
	case query.MatchPhaseAttacking:
		return model.MatchPhaseAttacking, nil
	case query.MatchPhaseDefending:
		return model.MatchPhaseDefending, nil
	case query.MatchPhaseRelinquishing:
		return model.MatchPhaseRelinquishing, nil
	case query.MatchPhaseTerritoryCommanding:
		return model.MatchPhaseTerritoryCommanding, nil
	case query.MatchPhaseEnding:
		return model.MatchPhaseEnding, nil
	case query.MatchPhaseEnded:
		return model.MatchPhaseEnded, nil
	default:
		return "", fmt.Errorf("graphql: unknown match phase: %q", phase)
	}
}

func fromMatchBattleSide(side query.MatchBattleSide) (model.MatchBattleSide, error) {
	switch side {
	case query.MatchBattleSideAttacker:
		return model.MatchBattleSideAttacker, nil
	case query.MatchBattleSideDefender:
		return model.MatchBattleSideDefender, nil
	default:
		return "", fmt.Errorf("graphql: unknown match battle side: %q", side)
	}
}

func fromMatchBattleAbility(ability query.MatchBattleAbility) (model.CreatureAbilityKind, error) {
	switch ability {
	case query.MatchBattleAbilityFirstStrike:
		return model.CreatureAbilityKindFirstStrike, nil
	case query.MatchBattleAbilityLastStrike:
		return model.CreatureAbilityKindLastStrike, nil
	case query.MatchBattleAbilityBattleModifier:
		return model.CreatureAbilityKindBattleModifier, nil
	case query.MatchBattleAbilityBattleRandomStats:
		return model.CreatureAbilityKindBattleRandomStats, nil
	case query.MatchBattleAbilityAttackImmunity:
		return model.CreatureAbilityKindAttackImmunity, nil
	default:
		return "", fmt.Errorf("graphql: unknown match battle ability: %q", ability)
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

func fromMatchEventChanges(changes query.MatchEventChanges) (*model.MatchEventChanges, error) {
	converted := &model.MatchEventChanges{
		TotalMagics:     make([]*model.MatchTotalMagicChange, len(changes.TotalMagics)),
		TerritoryValues: make([]*model.MatchTerritoryValue, len(changes.TerritoryValues)),
	}
	if changes.Phase != nil {
		phase, err := fromMatchPhase(*changes.Phase)
		if err != nil {
			return nil, err
		}
		converted.Phase = &phase
	}
	for i, total := range changes.TotalMagics {
		converted.TotalMagics[i] = &model.MatchTotalMagicChange{Participant: total.Participant, TotalMagic: total.TotalMagic}
	}
	for i, territory := range changes.TerritoryValues {
		converted.TerritoryValues[i] = &model.MatchTerritoryValue{
			Position: &model.BoardPosition{X: territory.Position.X, Y: territory.Position.Y}, Value: territory.Value, Toll: territory.Toll,
			Level: territory.Level, ChainCount: territory.ChainCount, PreviousChainCount: territory.PreviousChainCount,
		}
	}
	return converted, nil
}

func fromQueryMatchEvent(event query.MatchEvent) (model.MatchEvent, error) {
	meta := event.Meta()
	revision := int(meta.Revision)
	changes, err := fromMatchEventChanges(meta.Changes)
	if err != nil {
		return nil, err
	}
	switch e := event.(type) {
	case query.MatchCreatedEvent:
		return &model.MatchCreatedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes}, nil
	case query.MatchTurnOrderDecidedEvent:
		return &model.MatchTurnOrderDecidedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Order: e.Order}, nil
	case query.MatchDecksShuffledEvent:
		return &model.MatchDecksShuffledEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes}, nil
	case query.MatchHandsDealtEvent:
		return &model.MatchHandsDealtEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Cards: e.Cards}, nil
	case query.MatchCardDrawnEvent:
		card, err := fromMatchCard(e.Card)
		if err != nil {
			return nil, err
		}
		return &model.MatchCardDrawnEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant, Card: card}, nil
	case query.MatchDiscardPileShuffledEvent:
		return &model.MatchDiscardPileShuffledEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant, DrawPileCount: e.DrawPileCount}, nil
	case query.MatchHandLimitExceededEvent:
		return &model.MatchHandLimitExceededEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant, HandCount: e.HandCount, MaxHandCards: e.MaxHandCards}, nil
	case query.MatchCardDiscardedEvent:
		card, err := fromMatchCard(e.Card)
		if err != nil {
			return nil, err
		}
		return &model.MatchCardDiscardedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant, HandIndex: e.HandIndex, Card: card}, nil
	case query.MatchCreaturePlacedEvent:
		card, err := fromMatchCard(e.Card)
		if err != nil {
			return nil, err
		}
		return &model.MatchCreaturePlacedEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant, HandIndex: e.HandIndex, Card: card,
			Creature: e.Creature, Position: &model.BoardPosition{X: e.Position.X, Y: e.Position.Y}, Cost: e.Cost,
		}, nil
	case query.MatchTerritoryInvadedEvent:
		card, err := fromMatchCard(e.Card)
		if err != nil {
			return nil, err
		}
		defenderCard, err := fromMatchCard(e.DefendingCreatureCard)
		if err != nil {
			return nil, err
		}
		return &model.MatchTerritoryInvadedEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant, HandIndex: e.HandIndex, Card: card,
			Position: &model.BoardPosition{X: e.Position.X, Y: e.Position.Y}, Cost: e.Cost, DefendingCreatureCard: defenderCard,
		}, nil
	case query.MatchSpellUsedEvent:
		card, err := fromMatchCard(e.Card)
		if err != nil {
			return nil, err
		}
		return &model.MatchSpellUsedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant, HandIndex: e.HandIndex, Card: card, Cost: e.Cost}, nil
	case query.MatchItemUsedEvent:
		card, err := fromMatchCard(e.Card)
		if err != nil {
			return nil, err
		}
		return &model.MatchItemUsedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant, HandIndex: e.HandIndex, Card: card, Cost: e.Cost}, nil
	case query.MatchItemSkippedEvent:
		return &model.MatchItemSkippedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant}, nil
	case query.MatchTerritoryLeveledUpEvent:
		return &model.MatchTerritoryLeveledUpEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant,
			Position: &model.BoardPosition{X: e.Position.X, Y: e.Position.Y}, Level: e.Level, Magic: e.Magic,
		}, nil
	case query.MatchCreatureMovedEvent:
		return &model.MatchCreatureMovedEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant,
			From: &model.BoardPosition{X: e.From.X, Y: e.From.Y}, To: &model.BoardPosition{X: e.To.X, Y: e.To.Y}, Creature: e.Creature,
		}, nil
	case query.MatchTerritoryInvadedByMoveEvent:
		movedCard, err := fromMatchCard(e.Card)
		if err != nil {
			return nil, err
		}
		defenderCard, err := fromMatchCard(e.DefendingCreatureCard)
		if err != nil {
			return nil, err
		}
		return &model.MatchTerritoryInvadedByMoveEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant,
			From: &model.BoardPosition{X: e.From.X, Y: e.From.Y}, Position: &model.BoardPosition{X: e.Position.X, Y: e.Position.Y},
			Creature: e.Creature, Card: movedCard, DefendingCreatureCard: defenderCard,
		}, nil
	case query.MatchLandElementChangedEvent:
		element, err := fromBoardTileElement(e.Element)
		if err != nil {
			return nil, err
		}
		return &model.MatchLandElementChangedEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant,
			Position: &model.BoardPosition{X: e.Position.X, Y: e.Position.Y}, Element: element, Magic: e.Magic, BaseLandValue: e.BaseLandValue, Multiplier: e.Multiplier,
		}, nil
	case query.MatchCreatureExchangedEvent:
		card, err := fromMatchCard(e.Card)
		if err != nil {
			return nil, err
		}
		returned, err := fromMatchCard(e.ReturnedCard)
		if err != nil {
			return nil, err
		}
		return &model.MatchCreatureExchangedEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant,
			Position: &model.BoardPosition{X: e.Position.X, Y: e.Position.Y}, HandIndex: e.HandIndex, Card: card, Creature: e.Creature, Cost: e.Cost, ReturnedCard: returned,
		}, nil
	case query.MatchBattleStartedEvent:
		return &model.MatchBattleStartedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Attacker: fromMatchBattleStats(e.Attacker), Defender: fromMatchBattleStats(e.Defender)}, nil
	case query.MatchBattleSupportEffectActivatedEvent:
		side, err := fromMatchBattleSide(e.Side)
		if err != nil {
			return nil, err
		}
		positions := make([]*model.BoardPosition, len(e.Positions))
		for i, position := range e.Positions {
			positions[i] = &model.BoardPosition{X: position.X, Y: position.Y}
		}
		return &model.MatchBattleSupportEffectActivatedEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Side: side, Positions: positions,
			BonusPerTerritory: e.BonusPerTerritory, Change: fromMatchBattleStatChange(&e.Change),
		}, nil
	case query.MatchBattleTerrainEffectActivatedEvent:
		side, err := fromMatchBattleSide(e.Side)
		if err != nil {
			return nil, err
		}
		element, err := fromBoardTileElement(e.Element)
		if err != nil {
			return nil, err
		}
		return &model.MatchBattleTerrainEffectActivatedEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Side: side, Element: element, Level: e.Level,
			BonusPerLevel: e.BonusPerLevel, Change: fromMatchBattleStatChange(&e.Change),
		}, nil
	case query.MatchBattleCreatureAbilityActivatedEvent:
		side, err := fromMatchBattleSide(e.Side)
		if err != nil {
			return nil, err
		}
		ability, err := fromMatchBattleAbility(e.Ability)
		if err != nil {
			return nil, err
		}
		converted := &model.MatchBattleCreatureAbilityActivatedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Side: side, Ability: ability, Change: fromMatchBattleStatChange(e.Change)}
		if basis := e.BattleModifier; basis != nil {
			converted.BattleModifier = &model.MatchBattleModifierBasis{AttackModifier: basis.AttackModifier, HitPointsModifier: basis.HitPointsModifier}
		}
		if basis := e.BattleRandomStats; basis != nil {
			converted.BattleRandomStats = &model.MatchBattleRandomStatsBasis{Minimum: basis.Minimum, Maximum: basis.Maximum}
		}
		return converted, nil
	case query.MatchBattleItemAbilityActivatedEvent:
		side, err := fromMatchBattleSide(e.Side)
		if err != nil {
			return nil, err
		}
		ability, err := fromMatchBattleAbility(e.Ability)
		if err != nil {
			return nil, err
		}
		card, err := fromMatchCard(e.Card)
		if err != nil {
			return nil, err
		}
		return &model.MatchBattleItemAbilityActivatedEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Side: side, Ability: ability, Card: card,
			AttackModifier: e.AttackModifier, HitPointsModifier: e.HitPointsModifier, Change: fromMatchBattleStatChange(e.Change),
		}, nil
	case query.MatchCreatureStruckEvent:
		striker, err := fromMatchBattleSide(e.Striker)
		if err != nil {
			return nil, err
		}
		return &model.MatchCreatureStruckEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Striker: striker, Attack: e.Attack, Damage: e.Damage, RemainingHitPoints: e.RemainingHitPoints}, nil
	case query.MatchTerritoryConqueredEvent:
		return &model.MatchTerritoryConqueredEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant,
			Position: &model.BoardPosition{X: e.Position.X, Y: e.Position.Y}, Creature: e.Creature, TerritoryHitPoints: e.TerritoryHitPoints,
		}, nil
	case query.MatchInvasionRepelledEvent:
		card, err := fromMatchCard(e.Card)
		if err != nil {
			return nil, err
		}
		var returnedTo *model.BoardPosition
		if e.ReturnedTo != nil {
			returnedTo = &model.BoardPosition{X: e.ReturnedTo.X, Y: e.ReturnedTo.Y}
		}
		return &model.MatchInvasionRepelledEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant,
			Position: &model.BoardPosition{X: e.Position.X, Y: e.Position.Y}, Card: card, ReturnedToHand: e.ReturnedToHand, TerritoryHitPoints: e.TerritoryHitPoints,
			ReturnedTo: returnedTo, ReturnedHitPoints: e.ReturnedHitPoints,
		}, nil
	case query.MatchTollMultiplierActivatedEvent:
		return &model.MatchTollMultiplierActivatedEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes,
			Position: &model.BoardPosition{X: e.Position.X, Y: e.Position.Y}, Percent: e.Percent, Toll: e.Toll, MultipliedToll: e.MultipliedToll,
		}, nil
	case query.MatchTollPaidEvent:
		return &model.MatchTollPaidEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant, Owner: e.Owner,
			Position: &model.BoardPosition{X: e.Position.X, Y: e.Position.Y}, Magic: e.Magic,
		}, nil
	case query.MatchTerritoryRelinquishedEvent:
		return &model.MatchTerritoryRelinquishedEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant,
			Position: &model.BoardPosition{X: e.Position.X, Y: e.Position.Y}, Creature: e.Creature, Magic: e.Magic,
		}, nil
	case query.MatchMagicDepletedEvent:
		return &model.MatchMagicDepletedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant, Magic: e.Magic}, nil
	case query.MatchDiceRolledEvent:
		return &model.MatchDiceRolledEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant, Value: e.Value, Dice: e.Dice, SpellEffect: fromRollingSpellEffect(e.SpellEffect)}, nil
	case query.MatchSpellEffectStartedEvent:
		return &model.MatchSpellEffectStartedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant, Target: e.Target, Effect: fromMatchLastingSpellEffect(&e.Effect)}, nil
	case query.MatchCreatureDamagedEvent:
		return &model.MatchCreatureDamagedEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant,
			Position: &model.BoardPosition{X: e.Position.X, Y: e.Position.Y}, Creature: e.Creature, Damage: e.Damage, RemainingHitPoints: e.RemainingHitPoints,
		}, nil
	case query.MatchCreatureDestroyedEvent:
		return &model.MatchCreatureDestroyedEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant,
			Position: &model.BoardPosition{X: e.Position.X, Y: e.Position.Y}, Creature: e.Creature,
		}, nil
	case query.MatchSpellEffectEndedEvent:
		return &model.MatchSpellEffectEndedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant}, nil
	case query.MatchMagicGainedEvent:
		return &model.MatchMagicGainedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant, Magic: e.Magic, Laps: e.Laps, MagicPerLap: e.MagicPerLap}, nil
	case query.MatchMovedEvent:
		return &model.MatchMovedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant, To: &model.BoardPosition{X: e.To.X, Y: e.To.Y}}, nil
	case query.MatchCheckpointPassedEvent:
		return &model.MatchCheckpointPassedEvent{
			Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant,
			Checkpoint: &model.BoardPosition{X: e.Checkpoint.X, Y: e.Checkpoint.Y}, Magic: e.Magic,
		}, nil
	case query.MatchLapCompletedEvent:
		return &model.MatchLapCompletedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant, Magic: e.Magic}, nil
	case query.MatchTurnEndedEvent:
		return &model.MatchTurnEndedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant}, nil
	case query.MatchEndedEvent:
		reason, err := fromMatchEndReason(e.Reason)
		if err != nil {
			return nil, err
		}
		return &model.MatchEndedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Winners: e.Winners, Ranks: e.Ranks, Reason: reason}, nil
	case query.MatchRewardDrawnEvent:
		cards := make([]model.CardInfo, len(e.Cards))
		for i, drawn := range e.Cards {
			converted, err := fromMatchCard(drawn)
			if err != nil {
				return nil, err
			}
			cards[i] = converted
		}
		return &model.MatchRewardDrawnEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes, Participant: e.Participant, Pack: e.Pack, RewardCards: cards}, nil
	case query.MatchRewardsCompletedEvent:
		return &model.MatchRewardsCompletedEvent{Revision: revision, OccurredAt: meta.OccurredAt, Changes: changes}, nil
	default:
		return nil, fmt.Errorf("graphql: unknown match event: %T", event)
	}
}

func fromMatchCard(card query.MatchCard) (model.CardInfo, error) {
	return fromCard(card.Type, card.ID, card.Name, card.Cost, card.Rarity, card.Creature, card.Item, card.Spell)
}

func fromMatchBattleStats(stats query.MatchBattleStats) *model.MatchBattleStats {
	return &model.MatchBattleStats{Attack: stats.Attack, HitPoints: stats.HitPoints}
}

func fromMatchBattleStatChange(change *query.MatchBattleStatChange) *model.MatchBattleStatChange {
	if change == nil {
		return nil
	}
	return &model.MatchBattleStatChange{AttackDelta: change.AttackDelta, HitPointsDelta: change.HitPointsDelta, Attack: change.Attack, HitPoints: change.HitPoints}
}

func fromSpellChoices(choices []query.MatchSpellChoice) []*model.MatchSpellChoice {
	converted := make([]*model.MatchSpellChoice, len(choices))
	for i, choice := range choices {
		converted[i] = &model.MatchSpellChoice{HandIndex: choice.HandIndex, Targets: choice.Targets, TargetPositions: fromQueryPositions(choice.TargetPositions)}
	}
	return converted
}

func fromMatchLastingSpellEffect(lasting *query.MatchLastingSpellEffect) *model.MatchLastingSpellEffect {
	if lasting == nil {
		return nil
	}
	return &model.MatchLastingSpellEffect{Effect: fromSpellEffect(lasting.Effect), RemainingTurns: lasting.RemainingTurns}
}

func fromRollingSpellEffect(effect *query.SpellEffect) model.SpellEffect {
	if effect == nil {
		return nil
	}
	return fromSpellEffect(*effect)
}
