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
	case *DeployCreatureCommand:
		return playTurn(state, c.By, PhaseDeploying, now, func(r *recorder) error { return deploy(r, c.HandIndex) })
	case *InvadeCommand:
		return playTurn(state, c.By, PhaseInvading, now, func(r *recorder) error { return invade(r, c.HandIndex) })
	case *AttackCommand:
		return playTurn(state, c.By, PhaseAttacking, now, func(r *recorder) error { return chooseItem(r, c.By, c.ItemHandIndex) })
	case *PayTollCommand:
		return playTurn(state, c.By, PhaseInvading, now, payToll)
	case *DefendCommand:
		return playTurn(state, c.By, PhaseDefending, now, func(r *recorder) error { return defend(r, c.By, c.ItemHandIndex) })
	case *RelinquishTerritoryCommand:
		return playTurn(state, c.By, PhaseRelinquishing, now, func(r *recorder) error { return relinquish(r, c.Position) })
	case *EndTurnCommand:
		// 配置の段階でターンを終えるのが「配置しない」（専用の操作を作らない。2026-11-08 オーナー）
		return playTurn(state, c.By, PhaseEnding, now, endTurn, PhaseDeploying)
	case *ChooseRewardPackCommand:
		return chooseRewardPack(state, c, now, random)
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

// 終わった後・待っている人でない人・段階の合わない操作を断る。終わった後を先に見るのは、終わった対戦に番を問う意味が無いため
func playTurn(state State, by ParticipantIndex, phase Phase, now time.Time, act func(*recorder) error, otherPhases ...Phase) ([]Event, error) {
	current, ok := state.(Match)
	if !ok {
		return nil, ErrInvalidState
	}
	if current.Phase == PhaseEnded {
		return nil, ErrPhaseNotAllowed
	}
	if awaiting, ok := current.AwaitingParticipant(); !ok || awaiting != by {
		return nil, ErrNotYourTurn
	}
	if current.Phase != phase && !slices.Contains(otherPhases, current.Phase) {
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

// 配置して、そのままターンを終える
func deploy(r *recorder, handIndex int) error {
	if !slices.Contains(r.state.DeployChoices(), handIndex) {
		if handIndex < 0 || handIndex >= len(r.current().Hand) {
			return ErrHandIndexOutOfRange
		}
		return ErrCreatureNotDeployable
	}
	participant := r.current()
	ref := participant.Hand[handIndex]
	spec := r.state.CardSpecs()[ref]
	id := r.state.LastDeployedCreatureID + 1
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &CreatureDeployedEvent{EventMeta: meta, Participant: by, HandIndex: handIndex, Card: ref, Creature: id, Position: participant.Position, Cost: spec.Cost}
	}); err != nil {
		return err
	}
	return endTurn(r)
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

// 手札のクリーチャーを出して確定する。アイテムは攻める側 → 守る側の順に、別の操作で選ぶ
func invade(r *recorder, handIndex int) error {
	if !slices.Contains(r.state.InvadeChoices(), handIndex) {
		if handIndex < 0 || handIndex >= len(r.current().Hand) {
			return ErrHandIndexOutOfRange
		}
		return ErrCreatureNotInvadable
	}
	participant := r.current()
	ref := participant.Hand[handIndex]
	cost := r.state.CardSpecs()[ref].Cost
	return r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &TerritoryInvadedEvent{EventMeta: meta, Participant: by, HandIndex: handIndex, Card: ref, Position: participant.Position, Cost: cost}
	})
}

// アイテムを出すか、出さなかったことを残す。出さないときもイベントを出すのは、段階を事実から進めるためと、選びの結果を描くため
func chooseItem(r *recorder, by ParticipantIndex, itemHandIndex *int) error {
	if itemHandIndex == nil {
		return r.record(func(meta EventMeta, _ ParticipantIndex) Event {
			return &ItemSkippedEvent{EventMeta: meta, Participant: by}
		})
	}
	if !slices.Contains(r.state.ItemChoices(), *itemHandIndex) {
		return ErrItemNotUsable
	}
	ref := r.state.Participants[by].Hand[*itemHandIndex]
	cost := r.state.CardSpecs()[ref].Cost
	return r.record(func(meta EventMeta, _ ParticipantIndex) Event {
		return &ItemUsedEvent{EventMeta: meta, Participant: by, HandIndex: *itemHandIndex, Card: ref, Cost: cost}
	})
}

func defend(r *recorder, by ParticipantIndex, itemHandIndex *int) error {
	if err := chooseItem(r, by, itemHandIndex); err != nil {
		return err
	}
	return battle(r)
}

// 攻める側が先に攻撃し、守る側が生きていれば反撃する。能力とアイテムの効果はまだ無い（core#114）
func battle(r *recorder) error {
	defeated, err := strike(r, BattleSideAttacker)
	if err != nil {
		return err
	}
	if defeated {
		return conquer(r)
	}
	if _, err := strike(r, BattleSideDefender); err != nil {
		return err
	}
	return repel(r)
}

// 受けた側が倒れたかを返す
func strike(r *recorder, striker BattleSide) (bool, error) {
	territory, ok := r.state.invadedTerritory()
	if !ok || r.state.Invasion == nil {
		return false, ErrInvalidState
	}
	defending, ok := r.state.DeployedCreature(territory.Creature)
	if !ok {
		return false, ErrInvalidState
	}
	invasion := *r.state.Invasion
	damage, remaining := invasion.Base.Attack, defending.HitPoints.Damaged(invasion.Base.Attack)
	if striker == BattleSideDefender {
		damage, remaining = defending.Base.Attack, invasion.HitPoints.Damaged(defending.Base.Attack)
	}
	err := r.record(func(meta EventMeta, _ ParticipantIndex) Event {
		return &CreatureStruckEvent{EventMeta: meta, Striker: striker, Damage: damage, RemainingHitPoints: remaining}
	})
	return remaining.Int() == 0, err
}

// 奪ったら、配置と同じくそのままターンを終える
func conquer(r *recorder) error {
	territory, ok := r.state.invadedTerritory()
	if !ok {
		return ErrInvalidState
	}
	id := r.state.LastDeployedCreatureID + 1
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &TerritoryConqueredEvent{EventMeta: meta, Participant: by, Position: territory.Position, DefeatedCreature: territory.Creature, Creature: id}
	}); err != nil {
		return err
	}
	return endTurn(r)
}

func repel(r *recorder) error {
	if r.state.Invasion == nil {
		return ErrInvalidState
	}
	invasion := *r.state.Invasion
	position := r.current().Position
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &InvasionRepelledEvent{EventMeta: meta, Participant: by, Position: position, Card: invasion.Card, ReturnedToHand: invasion.HitPoints.Int() > 0}
	}); err != nil {
		return err
	}
	return payToll(r)
}

// 止まっている他人の領地の通行料を払う。侵略を選ばなかったときと、侵略で倒せなかったときに通る
func payToll(r *recorder) error {
	territory, ok := r.state.invadedTerritory()
	if !ok {
		return ErrInvalidState
	}
	owner, ok := r.state.TerritoryOwner(territory)
	if !ok {
		return ErrInvalidState
	}
	toll := r.state.TerritoryToll(territory)
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &TollPaidEvent{EventMeta: meta, Participant: by, Owner: owner, Position: territory.Position, Magic: toll}
	}); err != nil {
		return err
	}
	return settleDebt(r)
}

// 手放す領地も無く負のままなら、魔力が枯渇して拠点からやり直し、ターンを終える。周回のボーナスも勝ちの判定も無い（2026-11-08 オーナー）
func settleDebt(r *recorder) error {
	if r.state.Phase != PhaseEnding || r.current().Magic >= 0 {
		return nil
	}
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &MagicDepletedEvent{EventMeta: meta, Participant: by, Magic: r.state.Rules.StartingMagic}
	}); err != nil {
		return err
	}
	return endTurn(r)
}

func relinquish(r *recorder, position board.Position) error {
	if !slices.Contains(r.state.RelinquishChoices(), position) {
		return ErrTerritoryNotRelinquishable
	}
	territory, _ := r.state.TerritoryAt(position)
	magic := r.state.TerritoryRelinquishMagic(territory)
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &TerritoryRelinquishedEvent{EventMeta: meta, Participant: by, Position: position, Creature: territory.Creature, Magic: magic}
	}); err != nil {
		return err
	}
	return settleDebt(r)
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
	winner, _ := r.state.CurrentParticipant()
	if r.state.TotalMagics()[winner] < r.state.Rules.TargetMagic {
		return nil
	}
	return r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &MatchEndedEvent{EventMeta: meta, Winners: []ParticipantIndex{by}, Ranks: ranksWithWinner(r.state.TotalMagics(), by), Reason: EndReasonTargetMagic}
	})
}

// 競技の順位（同じ総魔力は同じ順位で、次は人数分あく。1, 2, 2, 4）。順位の決まりはここだけ
func ranksByMagic(totals []int) []int {
	return ranksFrom(totals, 1, -1)
}

// 目標の魔力で終わったときは、勝った人が 1 位で、ほかの人は勝った人を除いて 2 位から
// （勝った人と同じ総魔力の人も 2 位以下にするため、ranksByMagic は使えない）
func ranksWithWinner(totals []int, winner ParticipantIndex) []int {
	ranks := ranksFrom(totals, 2, winner.Int())
	ranks[winner.Int()] = 1
	return ranks
}

func ranksFrom(totals []int, first int, skip int) []int {
	ranks := make([]int, len(totals))
	for i, total := range totals {
		rank := first
		for j, other := range totals {
			if j != skip && other > total {
				rank++
			}
		}
		ranks[i] = rank
	}
	return ranks
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
		totals := r.state.TotalMagics()
		best := -1
		for i, total := range totals {
			switch {
			case total > best:
				best = total
				winners = []ParticipantIndex{ParticipantIndex(i)}
			case total == best:
				winners = append(winners, ParticipantIndex(i))
			}
		}
		return r.record(func(meta EventMeta, by ParticipantIndex) Event {
			return &MatchEndedEvent{EventMeta: meta, Winners: winners, Ranks: ranksByMagic(totals), Reason: EndReasonRoundLimit}
		})
	}
	return r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &TurnEndedEvent{EventMeta: meta, Participant: by}
	})
}

// 終わった後のプレイヤーが 1 回だけ、順位の枚数を候補から重ならずに引く。受け取り済みを先に断るのは、引き直しで結果を変えられないため。
// 引き方はパックのルールで決める。今は既定のパック（等しい出やすさ）だけで、種類・属性ごとの出やすさは、パックのマスタを作るときにここへ足す
func chooseRewardPack(state State, c *ChooseRewardPackCommand, now time.Time, random Random) ([]Event, error) {
	current, ok := state.(Match)
	if !ok {
		return nil, ErrInvalidState
	}
	if current.Phase != PhaseEnded {
		return nil, ErrPhaseNotAllowed
	}
	if current.Result == nil || len(current.Result.Ranks) != len(current.Participants) || len(current.Rules.RewardCardsByRank) == 0 || int(c.By) >= len(current.Participants) {
		return nil, ErrInvalidState
	}
	participant := current.Participants[c.By]
	switch {
	case participant.Participant.Player == nil:
		return nil, ErrRewardNotForNPC
	case participant.Reward != nil:
		return nil, ErrRewardAlreadyReceived
	case !current.Rules.hasRewardPack(c.Pack):
		return nil, ErrRewardPackNotAvailable
	}
	pool := slices.Clone(c.Candidates)
	slices.SortFunc(pool, card.Ref.Compare)
	pool = slices.Compact(pool)
	random.Shuffle(len(pool), func(a, b int) { pool[a], pool[b] = pool[b], pool[a] })
	drawn := make([]card.Ref, min(current.Rules.rewardCards(current.Result.Ranks[c.By]), len(pool)))
	copy(drawn, pool)
	meta := func(version int64) EventMeta {
		return EventMeta{EntityID: current.ID, AggregateVersion: version, OccurredAt: now}
	}
	events := []Event{&RewardDrawnEvent{EventMeta: meta(current.Version + 1), Participant: c.By, Pack: c.Pack, Cards: drawn}}
	next, err := Evolve(current, events[0])
	if err != nil {
		return nil, err
	}
	if next.(Match).allPlayersRewarded() {
		events = append(events, &RewardsCompletedEvent{EventMeta: meta(current.Version + 2)})
	}
	return events, nil
}
