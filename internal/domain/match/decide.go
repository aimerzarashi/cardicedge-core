package match

import (
	"slices"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

// 中で time.Now や乱数を呼ばず受け取るのは、テストで時刻・並びを固定するため。混ぜた並びはイベントに残し、Evolve は乱数に左右されない
func Decide(state State, cmd Command, now time.Time, random Random) ([]Event, error) {
	switch c := cmd.(type) {
	case *StartCommand:
		if _, uncreated := state.(UncreatedMatch); !uncreated {
			return nil, ErrAlreadyCreated
		}
		return start(c, now, random)
	case *DrawCommand:
		return playTurn(state, c.By, PhaseDrawing, now, draw)
	case *DiscardCommand:
		return playTurn(state, c.By, PhaseDiscarding, now, func(r *recorder) error { return discard(r, c.HandIndex) })
	case *RollCommand:
		return playTurn(state, c.By, PhaseRolling, now, func(r *recorder) error { return roll(r, random) })
	case *ChoosePathCommand:
		return playTurn(state, c.By, PhaseChoosingPath, now, func(r *recorder) error { return choosePath(r, c.To) })
	case *EndTurnCommand:
		return playTurn(state, c.By, PhaseEnding, now, endTurn)
	default:
		return nil, ErrInvalidState
	}
}

// 写しを残し、順番を決め、山札を混ぜ、手札を配る。全員が拠点に立ち、魔力は開始時の魔力、ラウンドは 1 から（Evolve が作る）
func start(c *StartCommand, now time.Time, random Random) ([]Event, error) {
	if _, ok := c.Board.base(); !ok {
		return nil, ErrBaseNotFound
	}
	order := make([]ParticipantIndex, len(c.Participants))
	for i := range c.Participants {
		order[i] = ParticipantIndex(i)
	}
	random.Shuffle(len(order), func(a, b int) { order[a], order[b] = order[b], order[a] })
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
		&TurnOrderDecidedEvent{EventMeta: meta(2), Order: order},
		&DecksShuffledEvent{EventMeta: meta(3), DrawPiles: piles},
		&HandsDealtEvent{EventMeta: meta(4), Cards: c.Rules.StartingHandCards.Int()},
	}, nil
}

// 出したイベントを順に Evolve に通して状態を進めながら決める。進む決まり（行き先・チェックポイント・周回・勝ち）を、
// Evolve と二重に書かず、状態を 1 か所で持つため
type recorder struct {
	state  Match
	events []Event
	now    time.Time
}

// 操作した人は今の番の人。イベントごとに引き直さず、ここで入れる
func (r *recorder) record(build func(meta EventMeta, by ParticipantIndex) Event) error {
	by, _ := r.state.CurrentParticipant()
	event := build(EventMeta{EntityID: r.state.ID, AggregateVersion: r.state.Version + 1, OccurredAt: r.now}, by)
	next, err := Evolve(r.state, event)
	if err != nil {
		return err
	}
	r.state = next.(Match)
	r.events = append(r.events, event)
	return nil
}

func (r *recorder) current() ParticipantState {
	index, _ := r.state.CurrentParticipant()
	return r.state.Participants[index]
}

// 終わった後・番でない人・段階の合わない操作を断る。終わった後を先に見るのは、終わった対戦に番を問う意味が無いため
func playTurn(state State, by ParticipantIndex, phase Phase, now time.Time, act func(*recorder) error) ([]Event, error) {
	current, ok := state.(Match)
	if !ok {
		return nil, ErrInvalidState
	}
	if current.Phase == PhaseEnded {
		return nil, ErrPhaseNotAllowed
	}
	if turn, ok := current.CurrentParticipant(); !ok || turn != by {
		return nil, ErrNotYourTurn
	}
	if current.Phase != phase {
		return nil, ErrPhaseNotAllowed
	}
	r := &recorder{state: current, now: now}
	if err := act(r); err != nil {
		return nil, err
	}
	return r.events, nil
}

func draw(r *recorder) error {
	pile := r.current().DrawPile
	if len(pile) == 0 {
		return ErrInvalidState
	}
	return r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &CardDrawnEvent{EventMeta: meta, Participant: by, Card: pile[0]}
	})
}

func discard(r *recorder, handIndex int) error {
	hand := r.current().Hand
	if handIndex < 0 || handIndex >= len(hand) {
		return ErrHandIndexOutOfRange
	}
	return r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &CardDiscardedEvent{EventMeta: meta, Participant: by, HandIndex: handIndex, Card: hand[handIndex]}
	})
}

func roll(r *recorder, random Random) error {
	rules := r.state.Rules
	value := rules.DiceMin + random.IntN(rules.DiceMax-rules.DiceMin+1)
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &DiceRolledEvent{EventMeta: meta, Participant: by, Value: value}
	}); err != nil {
		return err
	}
	return advance(r)
}

func choosePath(r *recorder, to board.Position) error {
	if !slices.Contains(r.state.PathChoices(), to) {
		return ErrPathNotAllowed
	}
	if err := step(r, to); err != nil {
		return err
	}
	return advance(r)
}

// 行き先が 1 つの間は進み続け、分かれ道・歩数切れ・勝ちで止まる
func advance(r *recorder) error {
	for r.state.Phase == PhaseMoving {
		if err := step(r, r.state.PathChoices()[0]); err != nil {
			return err
		}
	}
	return nil
}

// 1 歩進み、入ったマスで起きること（通過でも停止でも同じ。拠点を出るときは何も起きない）を続ける
func step(r *recorder, to board.Position) error {
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &MovedEvent{EventMeta: meta, Participant: by, To: to}
	}); err != nil {
		return err
	}
	rules := r.state.Rules
	for _, tile := range r.state.Board.Tiles {
		if tile.Position != to {
			continue
		}
		switch tile.Kind {
		case board.TileKindCheckpoint:
			if slices.Contains(r.current().PassedCheckpoints, to) {
				return nil
			}
			return r.record(func(meta EventMeta, by ParticipantIndex) Event {
				return &CheckpointPassedEvent{EventMeta: meta, Participant: by, Checkpoint: to, Magic: rules.CheckpointBonus}
			})
		case board.TileKindBase:
			return enterBase(r)
		}
	}
	return nil
}

func enterBase(r *recorder) error {
	if r.state.allCheckpointsPassed(r.current()) {
		if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
			return &LapCompletedEvent{EventMeta: meta, Participant: by, Magic: r.state.Rules.LapBonus}
		}); err != nil {
			return err
		}
	}
	if r.current().Magic < r.state.Rules.TargetMagic {
		return nil
	}
	return r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &MatchEndedEvent{EventMeta: meta, Winners: []ParticipantIndex{by}, Reason: EndReasonTargetMagic}
	})
}

func (m Match) allCheckpointsPassed(participant ParticipantState) bool {
	for _, tile := range m.Board.Tiles {
		if tile.Kind == board.TileKindCheckpoint && !slices.Contains(participant.PassedCheckpoints, tile.Position) {
			return false
		}
	}
	return true
}

// 上限のラウンドの最後の人が終えるときは、ラウンドを進めず（上限 +1 に見えないように）対戦を終える
func endTurn(r *recorder) error {
	lastInRound := r.state.Turn+1 >= len(r.state.TurnOrder)
	if lastInRound && r.state.Round >= r.state.Rules.RoundLimit {
		var winners []ParticipantIndex
		best := -1
		for i, participant := range r.state.Participants {
			switch {
			case participant.Magic > best:
				best = participant.Magic
				winners = []ParticipantIndex{ParticipantIndex(i)}
			case participant.Magic == best:
				winners = append(winners, ParticipantIndex(i))
			}
		}
		return r.record(func(meta EventMeta, by ParticipantIndex) Event {
			return &MatchEndedEvent{EventMeta: meta, Winners: winners, Reason: EndReasonRoundLimit}
		})
	}
	return r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &TurnEndedEvent{EventMeta: meta, Participant: by}
	})
}
