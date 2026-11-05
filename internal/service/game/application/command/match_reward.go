package command

import (
	"context"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/collection"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
)

// 対戦の RewardDrawnEvent を受けて、引いたカードをそのプレイヤーの所持品に足すポリシー。
// 対戦の操作の中で足さない（1 つのトランザクションで変える集約を 1 つにするため）
type MatchRewardHandler struct {
	matches     domain.EventStore[match.ID, match.Event, match.State]
	collections *CollectionHandler
}

func NewMatchRewardHandler(matches domain.EventStore[match.ID, match.Event, match.State], collections *CollectionHandler) *MatchRewardHandler {
	return &MatchRewardHandler{matches: matches, collections: collections}
}

// 購読する対戦のほかのイベントは何もしない
func (c *MatchRewardHandler) Handle(ctx context.Context, event any) error {
	if e, ok := event.(*match.RewardDrawnEvent); ok {
		return c.Grant(ctx, e)
	}
	return nil
}

// イベントの参加者のプレイヤーの所持品に、引いたカードを 1 枚ずつ足す。どの対戦の戦利品かを出どころに残し、二度動いても 1 回しか足さない。
// イベントの値でなく対戦を読み直すのは、プレイヤーを対戦の参加者から引くため（イベントは参加者の番号しか持たない）
func (c *MatchRewardHandler) Grant(ctx context.Context, event *match.RewardDrawnEvent) error {
	matchID := event.GetMeta().EntityID
	state, err := c.matches.Load(ctx, matchID)
	if err != nil {
		return err
	}
	current, ok := state.(match.Match)
	if !ok || int(event.Participant) >= len(current.Participants) || current.Participants[event.Participant].Participant.Player == nil {
		return fmt.Errorf("match reward: participant %d of match %d is not a player: %w", event.Participant, matchID, match.ErrInvalidState)
	}
	owned := make([]collection.Card, len(event.Cards))
	for i, ref := range event.Cards {
		quantity, err := card.NewOwnedQuantity(1)
		if err != nil {
			return err
		}
		owned[i] = collection.Card{Card: ref, Quantity: quantity}
	}
	return c.collections.Acquire(ctx, current.Participants[event.Participant].Participant.Player.Player, owned, collection.NewMatchRewardSource(matchID))
}
