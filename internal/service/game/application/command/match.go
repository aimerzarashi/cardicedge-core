package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/room"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/master"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/rules"
)

// 部屋の MatchStartedEvent を受けて、対戦を始めるポリシー。部屋の命令の中で作らず、イベントを受けて別のトランザクションで作る
// （1 つのトランザクションで変える集約を 1 つにするため）
type MatchStartHandler struct {
	rooms           domain.EventStore[room.ID, room.Event, room.State]
	matches         domain.EventStore[match.ID, match.Event, match.State]
	getRules        rules.GetRulesFunc
	getBoard        master.GetMatchBoardFunc
	getDeckCards    master.GetPlayerDeckCardsFunc
	getNPCDeckCards master.GetNPCDeckCardsFunc
	// 乱数の元は対戦ごとに新しく作る（再実行で同じ並びになるとは限らないが、保存されるのは Decide が出したイベントだけで、
	// 二重に動いても対戦は 1 つしかできない）。テストは固定した乱数を返す関数を渡す
	newRandom func() match.Random
	now       func() time.Time
}

func NewMatchStartHandler(
	rooms domain.EventStore[room.ID, room.Event, room.State],
	matches domain.EventStore[match.ID, match.Event, match.State],
	getRules rules.GetRulesFunc,
	getBoard master.GetMatchBoardFunc,
	getDeckCards master.GetPlayerDeckCardsFunc,
	getNPCDeckCards master.GetNPCDeckCardsFunc,
	newRandom func() match.Random,
	now func() time.Time,
) *MatchStartHandler {
	return &MatchStartHandler{
		rooms: rooms, matches: matches, getRules: getRules, getBoard: getBoard, getDeckCards: getDeckCards,
		getNPCDeckCards: getNPCDeckCards, newRandom: newRandom, now: now,
	}
}

// 購読する部屋のほかのイベントは何もしない
func (c *MatchStartHandler) Handle(ctx context.Context, event any) error {
	if e, ok := event.(*room.MatchStartedEvent); ok {
		return c.Start(ctx, e.GetMeta().EntityID, e.Match)
	}
	return nil
}

// 部屋の条件の写し（席の順の参加者・デッキのカード・ボードの形・ルール）から対戦を始める。ID は部屋の命令が取ったものを使う
// （部屋から対戦をすぐ引けるようにするため）。もう始まっていれば何もしない（ポリシーは二度動くことがある）。
// 部屋の条件が欠けている・写していないものがある失敗は、待てば直るもの（写しの遅れ）か、起きないはずの誤りとして返す
func (c *MatchStartHandler) Start(ctx context.Context, roomID room.ID, matchID match.ID) error {
	state, err := c.rooms.Load(ctx, roomID)
	if err != nil {
		return err
	}
	current, ok := state.(room.Room)
	if !ok || current.MatchRules == nil {
		return fmt.Errorf("match start: room %d has no conditions: %w", roomID, room.ErrInvalidState)
	}
	gameRules, found, err := c.getRules(ctx)
	if err != nil {
		return err
	}
	if !found {
		return rules.ErrNotPublished
	}
	participants := make([]match.Participant, len(current.Seats))
	for i, seat := range current.Seats {
		if participants[i], err = c.participant(ctx, seat, gameRules.Deck.MaxCardDuplicates); err != nil {
			return fmt.Errorf("match start: seat %d: %w", i, err)
		}
	}
	snapshot, err := c.getBoard(ctx, current.MatchRules.Board)
	if err != nil {
		return err
	}
	values := current.MatchRules
	defaultRules, err := board.NewDefaultRules(values.TargetMagic, values.StartingMagic, values.LapBonus, values.CheckpointBonus,
		values.RoundLimit, values.DiceMin, values.DiceMax)
	if err != nil {
		return err
	}
	cmd := &match.StartCommand{
		ID: matchID, Participants: participants, Board: snapshot,
		Rules: match.NewRules(defaultRules, gameRules.Match.MaxHandCards, gameRules.Match.StartingHandCards),
	}
	random := c.newRandom()
	_, err = rdb.Transact(ctx, func(ctx context.Context) (struct{}, error) {
		existing, err := c.matches.Load(ctx, matchID)
		if err != nil {
			return struct{}{}, err
		}
		events, err := match.Decide(existing, cmd, c.now(), random)
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, c.matches.Save(ctx, events)
	})
	if errors.Is(err, match.ErrAlreadyCreated) {
		return nil
	}
	return err
}

func (c *MatchStartHandler) participant(ctx context.Context, seat room.SeatState, maxDuplicates card.MaxDeckQuantity) (match.Participant, error) {
	switch {
	case seat.Seat.Kind == room.SeatKindPlayer && seat.Choice != nil && seat.Choice.Player != nil:
		source, err := c.getDeckCards(ctx, seat.Choice.Player.Deck)
		if err != nil {
			return match.Participant{}, err
		}
		cards, err := deckCards(source, maxDuplicates)
		if err != nil {
			return match.Participant{}, err
		}
		return match.NewPlayerParticipant(seat.Seat.Player, seat.Choice.Player.Avatar, seat.Choice.Player.Deck, cards), nil
	case seat.Seat.Kind == room.SeatKindNPC && seat.Choice != nil && seat.Choice.NPC != nil:
		source, err := c.getNPCDeckCards(ctx, seat.Choice.NPC.Deck)
		if err != nil {
			return match.Participant{}, err
		}
		cards, err := deckCards(source, maxDuplicates)
		if err != nil {
			return match.Participant{}, err
		}
		return match.NewNPCParticipant(seat.Choice.NPC.Avatar, seat.Choice.NPC.Deck, cards), nil
	default:
		return match.Participant{}, room.ErrConditionsNotDecided
	}
}

func deckCards(source []master.DeckCard, maxDuplicates card.MaxDeckQuantity) ([]match.DeckCard, error) {
	cards := make([]match.DeckCard, len(source))
	for i, entry := range source {
		quantity, err := card.NewDeckQuantity(entry.Count, maxDuplicates)
		if err != nil {
			return nil, fmt.Errorf("card %s/%d: %w", entry.Card.EntityType, entry.Card.ID, err)
		}
		cards[i] = match.DeckCard{Card: entry.Card, Quantity: quantity}
	}
	return cards, nil
}
