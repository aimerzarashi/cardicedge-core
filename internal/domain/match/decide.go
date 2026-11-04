package match

import (
	"slices"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

// 中で time.Now や乱数を呼ばず受け取るのは、テストで時刻・目・並びを固定するため。出た目と並びはイベントに残し、Evolve は乱数に左右されない
func Decide(state State, cmd Command, now time.Time, random Random) ([]Event, error) {
	switch c := cmd.(type) {
	case *StartCommand:
		if _, uncreated := state.(UncreatedMatch); !uncreated {
			return nil, ErrAlreadyCreated
		}
		return start(c, now, random)
	default:
		return nil, ErrInvalidState
	}
}

// 写しを残し、順番を決め、山札を混ぜ、手札を配る。全員が拠点に立ち、魔力は開始時の魔力、ラウンドは 1 から（Evolve が作る）
func start(c *StartCommand, now time.Time, random Random) ([]Event, error) {
	if _, ok := c.Board.base(); !ok {
		return nil, ErrBaseNotFound
	}
	// 目が 1 種類だと振り直しても順番が決まらない。board.DefaultRules が確かめているが、ここで止めないと終わらなくなるので見る
	if c.Rules.DiceMax <= c.Rules.DiceMin {
		return nil, ErrDiceRangeTooNarrow
	}
	indexes := make([]ParticipantIndex, len(c.Participants))
	for i := range c.Participants {
		indexes[i] = ParticipantIndex(i)
	}
	var rolls []TurnOrderRoll
	order := decideTurnOrder(indexes, 1, func(participant ParticipantIndex, attempt int) int {
		value := c.Rules.rollDie(random)
		rolls = append(rolls, TurnOrderRoll{Participant: participant, Attempt: attempt, Value: value})
		return value
	})
	piles := make([][]card.Ref, len(c.Participants))
	for i, participant := range c.Participants {
		pile := participant.deckCards()
		random.Shuffle(len(pile), func(a, b int) { pile[a], pile[b] = pile[b], pile[a] })
		piles[i] = pile
	}
	meta := func(version int64) EventMeta {
		return EventMeta{EntityID: c.ID, AggregateVersion: version, OccurredAt: now}
	}
	return []Event{
		&CreatedEvent{EventMeta: meta(1), Participants: c.Participants, Board: c.Board, Rules: c.Rules},
		&TurnOrderDecidedEvent{EventMeta: meta(2), Rolls: rolls, Order: order},
		&DecksShuffledEvent{EventMeta: meta(3), DrawPiles: piles},
		&HandsDealtEvent{EventMeta: meta(4), Cards: c.Rules.StartingHandCards.Int()},
	}, nil
}

// 全員が振り、大きい順に並べる。同じ目の人どうしは、その人たちだけでもう一度振って順を決める（決まるまで繰り返す）。
// 同じ目の組が複数あれば、上の順位の組から振り直す
func decideTurnOrder(group []ParticipantIndex, attempt int, roll func(participant ParticipantIndex, attempt int) int) []ParticipantIndex {
	if len(group) <= 1 {
		return group
	}
	values := make(map[ParticipantIndex]int, len(group))
	for _, participant := range group {
		values[participant] = roll(participant, attempt)
	}
	sorted := slices.Clone(group)
	slices.SortStableFunc(sorted, func(a, b ParticipantIndex) int { return values[b] - values[a] })
	order := make([]ParticipantIndex, 0, len(group))
	for start := 0; start < len(sorted); {
		end := start + 1
		for end < len(sorted) && values[sorted[end]] == values[sorted[start]] {
			end++
		}
		order = append(order, decideTurnOrder(sorted[start:end], attempt+1, roll)...)
		start = end
	}
	return order
}
