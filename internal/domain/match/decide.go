package match

import (
	"slices"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
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
		return playTurn(state, c.By, PhaseDrawing, now, func(r *recorder) error { return draw(r, random) })
	case *DiscardCommand:
		return playTurn(state, c.By, PhaseDiscarding, now, func(r *recorder) error { return discard(r, c.HandIndex) })
	case *UseSpellCommand:
		return playTurn(state, c.By, PhaseRolling, now, func(r *recorder) error {
			return useSpell(r, c.HandIndex, c.Target, c.TargetPosition, random)
		})
	case *RollCommand:
		return playTurn(state, c.By, PhaseRolling, now, func(r *recorder) error { return roll(r, random) })
	case *ChoosePathCommand:
		return playTurn(state, c.By, PhaseChoosingPath, now, func(r *recorder) error { return choosePath(r, c.To) })
	case *PlaceCreatureCommand:
		return playTurn(state, c.By, PhasePlacing, now, func(r *recorder) error { return place(r, c.HandIndex) })
	case *LevelUpTerritoryCommand:
		return playTurn(state, c.By, PhaseTerritoryCommanding, now, func(r *recorder) error { return levelUp(r, c.Position, c.Level) }, PhasePlacing, PhaseInvading)
	case *MoveCreatureCommand:
		return playTurn(state, c.By, PhaseTerritoryCommanding, now, func(r *recorder) error { return moveCreature(r, c.From, c.To) }, PhasePlacing, PhaseInvading)
	case *ChangeLandElementCommand:
		return playTurn(state, c.By, PhaseTerritoryCommanding, now, func(r *recorder) error { return changeLandElement(r, c.Position, c.Element) }, PhasePlacing, PhaseInvading)
	case *ExchangeCreatureCommand:
		return playTurn(state, c.By, PhaseTerritoryCommanding, now, func(r *recorder) error { return exchange(r, c.Position, c.HandIndex) }, PhasePlacing, PhaseInvading)
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
		// 配置の段階でターンを終えるのが「配置しない」、領地コマンドの段階でターンを終えるのが「何もしない」（専用の操作を作らない。2026-11-08・11-09 オーナー）
		return playTurn(state, c.By, PhaseEnding, now, endTurn, PhasePlacing, PhaseTerritoryCommanding)
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

// 山札が空なら、捨て札を混ぜて山札にしてから引く（2026-11-10 オーナー。core#148）
func draw(r *recorder, random Random) error {
	if len(r.current().DrawPile) == 0 {
		pile := slices.Clone(r.current().DiscardPile)
		if len(pile) == 0 {
			return ErrInvalidState
		}
		random.Shuffle(len(pile), func(a, b int) { pile[a], pile[b] = pile[b], pile[a] })
		if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
			return &DiscardPileShuffledEvent{EventMeta: meta, Participant: by, DrawPile: pile}
		}); err != nil {
			return err
		}
	}
	pile := r.current().DrawPile
	return r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &CardDrawnEvent{EventMeta: meta, Participant: by, Card: pile[0]}
	})
}

// 上限まで捨てたら、止めていたターンの終わりを続ける
func discard(r *recorder, handIndex int) error {
	hand := r.current().Hand
	if handIndex < 0 || handIndex >= len(hand) {
		return ErrHandIndexOutOfRange
	}
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &CardDiscardedEvent{EventMeta: meta, Participant: by, HandIndex: handIndex, Card: hand[handIndex]}
	}); err != nil {
		return err
	}
	if r.state.Phase == PhaseDiscarding {
		return nil
	}
	return endTurn(r)
}

// このターンのサイコロの個数だけ同時に振り、合計の分だけ進む（1 個ずつ目の範囲で振る。2026-11-10 オーナー）
func roll(r *recorder, random Random) error {
	// 目を決める呪いは、サイコロを複数振るスペルより優先する（サイコロ 1 個で、その目だけ進む。2026-11-10 オーナー）
	if curse := r.current().Curse; curse != nil && curse.FixedDice != nil {
		value := curse.FixedDice.Value
		if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
			return &DiceRolledEvent{EventMeta: meta, Participant: by, Value: value, FixedByCurse: true}
		}); err != nil {
			return err
		}
		return advance(r)
	}
	rules := r.state.Rules
	var dice []int
	value := 0
	for range max(r.state.DiceCount, 1) {
		face := rules.DiceMin + random.IntN(rules.DiceMax-rules.DiceMin+1)
		dice = append(dice, face)
		value += face
	}
	if len(dice) == 1 {
		dice = nil
	}
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &DiceRolledEvent{EventMeta: meta, Participant: by, Value: value, Dice: dice}
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

// コストを払ってスペルを捨て札に移し、効果を当てる。段階は振る段階のまま（使った後もサイコロは振る）
func useSpell(r *recorder, handIndex int, target *ParticipantIndex, targetPosition *board.Position, random Random) error {
	index := slices.IndexFunc(r.state.SpellChoices(), func(choice SpellChoice) bool { return choice.HandIndex == handIndex })
	if index < 0 {
		if handIndex < 0 || handIndex >= len(r.current().Hand) {
			return ErrHandIndexOutOfRange
		}
		return ErrSpellNotUsable
	}
	choice := r.state.SpellChoices()[index]
	targets, positions := choice.Targets, choice.TargetPositions
	switch {
	case len(targets) > 0 && target == nil:
		return ErrSpellTargetRequired
	case len(targets) == 0 && target != nil, target != nil && !slices.Contains(targets, *target):
		return ErrSpellTargetNotAllowed
	case len(positions) > 0 && targetPosition == nil:
		return ErrSpellTargetPositionRequired
	case len(positions) == 0 && targetPosition != nil, targetPosition != nil && !slices.Contains(positions, *targetPosition):
		return ErrSpellTargetPositionNotAllowed
	}
	used := r.current().Hand[handIndex]
	spec := r.state.CardSpecs()[used]
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &SpellUsedEvent{EventMeta: meta, Participant: by, HandIndex: handIndex, Card: used, Cost: spec.Cost}
	}); err != nil {
		return err
	}
	effect := spec.Spell.Effect
	if effect.Damage != nil {
		return damageCreature(r, *targetPosition, effect.Damage.Amount.Int())
	}
	if effect.FixedDiceCurse != nil {
		curse := NewFixedDiceCurse(effect.FixedDiceCurse.Value.Int(), effect.FixedDiceCurse.Turns.Int())
		return r.record(func(meta EventMeta, by ParticipantIndex) Event {
			return &CurseCastEvent{EventMeta: meta, Participant: by, Target: *target, Curse: curse}
		})
	}
	if effect.GainMagic != nil {
		// 周回数はこの対戦で周回を終えた回数（2026-11-10 オーナー。culdcept-lab.com のマナ）
		laps := r.current().Laps
		perLap := effect.GainMagic.PerLap.Int()
		magic := (laps + 1) * perLap
		return r.record(func(meta EventMeta, by ParticipantIndex) Event {
			return &MagicGainedEvent{EventMeta: meta, Participant: by, Magic: magic, Laps: laps, MagicPerLap: perLap}
		})
	}
	if effect.RollDice != nil {
		return r.record(func(meta EventMeta, by ParticipantIndex) Event {
			return &DiceCountChangedEvent{EventMeta: meta, Participant: by, Count: effect.RollDice.Count.Int()}
		})
	}
	if effect.DrawCards != nil {
		// 山札と捨て札の両方が尽きたら、引ける分だけで止める
		for range effect.DrawCards.Count.Int() {
			if len(r.current().DrawPile) == 0 && len(r.current().DiscardPile) == 0 {
				break
			}
			if err := draw(r, random); err != nil {
				return err
			}
		}
	}
	return nil
}

// 今の HP から引き、0 になれば破壊する（2026-11-10 オーナー。減った HP の戻り方は core#114）
func damageCreature(r *recorder, position board.Position, damage int) error {
	territory, _ := r.state.TerritoryAt(position)
	target, _ := r.state.PlacedCreature(territory.Creature)
	remaining := target.HitPoints.Modified(-damage)
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &CreatureDamagedEvent{EventMeta: meta, Participant: by, Position: position, Creature: target.ID, Damage: damage, RemainingHitPoints: remaining}
	}); err != nil {
		return err
	}
	if remaining.Int() > 0 {
		return nil
	}
	return r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &CreatureDestroyedEvent{EventMeta: meta, Participant: by, Position: position, Creature: target.ID}
	})
}

// 配置して、そのままターンを終える
func place(r *recorder, handIndex int) error {
	if !slices.Contains(r.state.PlaceChoices(), handIndex) {
		if handIndex < 0 || handIndex >= len(r.current().Hand) {
			return ErrHandIndexOutOfRange
		}
		return ErrCreatureNotPlaceable
	}
	participant := r.current()
	ref := participant.Hand[handIndex]
	spec := r.state.CardSpecs()[ref]
	id := r.state.LastPlacedCreatureID + 1
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &CreaturePlacedEvent{EventMeta: meta, Participant: by, HandIndex: handIndex, Card: ref, Creature: id, Position: participant.Position, Cost: spec.Cost}
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

func levelUp(r *recorder, position board.Position, level int) error {
	index := slices.IndexFunc(r.state.LevelUpChoices(), func(choice LevelUpChoice) bool { return choice.Position == position && choice.Level == level })
	if index < 0 {
		return ErrTerritoryNotLevelUpable
	}
	magic := r.state.LevelUpChoices()[index].Magic
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &TerritoryLeveledUpEvent{EventMeta: meta, Participant: by, Position: position, Level: level, Magic: magic}
	}); err != nil {
		return err
	}
	return afterTerritoryCommand(r)
}

func moveCreature(r *recorder, from, to board.Position) error {
	if !slices.Contains(r.state.MoveChoices(), MoveChoice{From: from, To: to}) {
		return ErrCreatureNotMovable
	}
	territory, ok := r.state.TerritoryAt(from)
	if !ok {
		return ErrInvalidState
	}
	if defender, occupied := r.state.TerritoryAt(to); occupied {
		// 他人の領地へは侵略と同じ戦闘をする。アイテムを選ぶところから続ける
		return r.record(func(meta EventMeta, by ParticipantIndex) Event {
			return &TerritoryInvadedByMoveEvent{EventMeta: meta, Participant: by, From: from, Position: to, Creature: territory.Creature, DefendingCreature: defender.Creature}
		})
	}
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &CreatureMovedEvent{EventMeta: meta, Participant: by, From: from, To: to, Creature: territory.Creature}
	}); err != nil {
		return err
	}
	return afterTerritoryCommand(r)
}

func changeLandElement(r *recorder, position board.Position, element board.Element) error {
	index := slices.IndexFunc(r.state.ElementChangeChoices(), func(choice ElementChangeChoice) bool { return choice.Position == position && choice.Element == element })
	if index < 0 {
		return ErrLandElementNotChangeable
	}
	choice := r.state.ElementChangeChoices()[index]
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &LandElementChangedEvent{EventMeta: meta, Participant: by, Position: position, Element: element, Magic: choice.Magic, BaseLandValue: r.state.Rules.BaseLandValue, Multiplier: choice.Multiplier}
	}); err != nil {
		return err
	}
	return afterTerritoryCommand(r)
}

func exchange(r *recorder, position board.Position, handIndex int) error {
	if !slices.ContainsFunc(r.state.ExchangeChoices(), func(choice ExchangeChoice) bool { return choice.Position == position && choice.HandIndex == handIndex }) {
		return ErrCreatureNotExchangeable
	}
	territory, ok := r.state.TerritoryAt(position)
	if !ok {
		return ErrInvalidState
	}
	replaced, ok := r.state.PlacedCreature(territory.Creature)
	if !ok {
		return ErrInvalidState
	}
	ref := r.current().Hand[handIndex]
	cost := r.state.CardSpecs()[ref].Cost
	id := r.state.LastPlacedCreatureID + 1
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &CreatureExchangedEvent{EventMeta: meta, Participant: by, Position: position, HandIndex: handIndex, Card: ref, Creature: id, Cost: cost, ReturnedCard: replaced.Card}
	}); err != nil {
		return err
	}
	return afterTerritoryCommand(r)
}

// 領地コマンドを実行した後の止まったマスの行動は、他人の領地なら通行料を払う一択で、ほかは何もせず終わる（2026-11-09 オーナー。施設は作るときにここへ足す）
func afterTerritoryCommand(r *recorder) error {
	index, _ := r.state.CurrentParticipant()
	if territory, ok := r.state.invadedTerritory(); ok {
		if owner, ok := r.state.TerritoryOwner(territory); ok && owner != index {
			return payToll(r)
		}
	}
	return endTurn(r)
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
	defender, ok := r.state.TerritoryAt(participant.Position)
	if !ok {
		return ErrInvalidState
	}
	return r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &TerritoryInvadedEvent{EventMeta: meta, Participant: by, HandIndex: handIndex, Card: ref, Position: participant.Position, Cost: cost, DefendingCreature: defender.Creature}
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

// 効果を順に当て、残っていれば先に攻撃する側（strikeOrder）が攻撃し、相手が生きていれば反撃する。
// どこかで体力が 0 になれば、その時点で戦闘は終わる（残りの効果も攻撃も無い。2026-11-09 オーナー）
func battle(r *recorder) error {
	if err := startBattle(r); err != nil {
		return err
	}
	for _, apply := range battleEffects() {
		if err := apply(r); err != nil {
			return err
		}
		if r.state.Battle.decided() {
			return settleBattle(r)
		}
	}
	if err := activateStrikeAbilities(r); err != nil {
		return err
	}
	order, err := strikeOrder(r.state)
	if err != nil {
		return err
	}
	for _, striker := range order {
		if err := strike(r, striker); err != nil {
			return err
		}
		if r.state.Battle.decided() {
			break
		}
	}
	return settleBattle(r)
}

// 攻撃の順に効く能力（先制・後手）を、持っている側ごとに攻める側 → 守る側の順で発動させる。効き目が無くても持っていれば出す
func activateStrikeAbilities(r *recorder) error {
	defending, err := r.state.defendingCreature()
	if err != nil {
		return err
	}
	sides := []struct {
		side BattleSide
		spec CreatureSpec
	}{{BattleSideAttacker, r.state.Invasion.Base}, {BattleSideDefender, defending.Base}}
	for _, s := range sides {
		ability, ok := s.spec.strikeAbility()
		if !ok {
			continue
		}
		if err := r.record(func(meta EventMeta, _ ParticipantIndex) Event {
			return &BattleAbilityActivatedEvent{EventMeta: meta, Side: s.side, Ability: ability}
		}); err != nil {
			return err
		}
	}
	return nil
}

// 攻撃する順。先制 > どちらも無し > 後手 の強い方が先で、同じなら攻める側が先（culdcept-lab.com の戦闘のクイズの基礎編。2026-11-11 オーナー。core#158）
func strikeOrder(state Match) ([]BattleSide, error) {
	defending, err := state.defendingCreature()
	if err != nil {
		return nil, err
	}
	if defending.Base.strikePriority() > state.Invasion.Base.strikePriority() {
		return []BattleSide{BattleSideDefender, BattleSideAttacker}, nil
	}
	return []BattleSide{BattleSideAttacker, BattleSideDefender}, nil
}

func startBattle(r *recorder) error {
	territory, ok := r.state.battleTerritory()
	if !ok || r.state.Invasion == nil {
		return ErrInvalidState
	}
	defending, ok := r.state.PlacedCreature(territory.Creature)
	if !ok {
		return ErrInvalidState
	}
	invasion := *r.state.Invasion
	return r.record(func(meta EventMeta, _ ParticipantIndex) Event {
		return &BattleStartedEvent{
			EventMeta: meta,
			Attacker:  BattleStats{Attack: invasion.Base.Attack, HitPoints: invasion.HitPoints},
			Defender:  BattleStats{Attack: defending.Base.Attack, HitPoints: defending.HitPoints},
		}
	})
}

// 効果を当てる順はここだけで決める。攻める側を全部当ててから守る側で、各側の中は 呪い → 能力 → 場の効果 → アイテム（2026-11-09 オーナー）。
// 呪いは作るときに前へ足し、倍率の効果は足し算がすべて終わった後に足す
func battleEffects() []func(*recorder) error {
	return []func(*recorder) error{
		func(r *recorder) error { return applyBattleModifier(r, BattleSideAttacker) },
		applyAdjacentTerritories,
		func(r *recorder) error { return applyItem(r, BattleSideAttacker, r.state.Invasion.AttackerItem) },
		func(r *recorder) error { return applyBattleModifier(r, BattleSideDefender) },
		applyTerrain,
		func(r *recorder) error { return applyItem(r, BattleSideDefender, r.state.Invasion.DefenderItem) },
	}
}

func applyBattleModifier(r *recorder, side BattleSide) error {
	if r.state.Invasion == nil {
		return ErrInvalidState
	}
	spec := r.state.Invasion.Base
	if side == BattleSideDefender {
		defending, err := r.state.defendingCreature()
		if err != nil {
			return err
		}
		spec = defending.Base
	}
	modifier, ok := spec.battleModifier()
	if !ok {
		return nil
	}
	return modifyBattleStats(r, side, NewAbilitySource(BattleAbilityBattleModifier, modifier.AttackModifier, modifier.HitPointsModifier), modifier.AttackModifier.Int(), modifier.HitPointsModifier.Int())
}

func applyAdjacentTerritories(r *recorder) error {
	attacker, ok := r.state.CurrentParticipant()
	if !ok {
		return ErrInvalidState
	}
	if r.state.Invasion == nil {
		return ErrInvalidState
	}
	// 移動で侵略したときは、動いたクリーチャーがもういない元の領地を数えない（2026-11-10 オーナー）
	positions := slices.DeleteFunc(r.state.adjacentTerritoriesOf(r.state.Invasion.Position, attacker), func(position board.Position) bool {
		return r.state.Invasion.From != nil && position == *r.state.Invasion.From
	})
	return modifyBattleStats(r, BattleSideAttacker, NewAdjacentTerritoriesSource(positions, r.state.Rules.AdjacentTerritoryBonus.Int()), len(positions)*r.state.Rules.AdjacentTerritoryBonus.Int(), 0)
}

func applyTerrain(r *recorder) error {
	territory, ok := r.state.battleTerritory()
	if !ok {
		return ErrInvalidState
	}
	defending, ok := r.state.PlacedCreature(territory.Creature)
	if !ok {
		return ErrInvalidState
	}
	element := r.state.landElement(territory.Position)
	if !terrainMatches(element, defending.Base.Elements) {
		return nil
	}
	level := r.state.LandLevel(territory.Position)
	return modifyBattleStats(r, BattleSideDefender, NewTerrainSource(element, level, r.state.Rules.TerrainBonusPerLevel.Int()), 0, level*r.state.Rules.TerrainBonusPerLevel.Int())
}

// 複の土地は、無属性以外のクリーチャーならどれでも効く（2026-11-09 オーナー）
func terrainMatches(land board.Element, elements creature.Elements) bool {
	if land == board.ElementMulti {
		return !slices.Contains(elements, creature.ElementNeutral)
	}
	return slices.ContainsFunc(elements, func(element creature.Element) bool { return element.String() == land.String() })
}

// 巻物は分類だけで、増減を持っていても効果は無い（2026-11-09 オーナー）
func applyItem(r *recorder, side BattleSide, used *card.Ref) error {
	if used == nil {
		return nil
	}
	spec := r.state.CardSpecs()[*used].Item
	if spec == nil || spec.Category == item.CategoryScroll {
		return nil
	}
	return modifyBattleStats(r, side, NewItemSource(*used, spec.AttackModifier, spec.HitPointsModifier), spec.AttackModifier.Int(), spec.HitPointsModifier.Int())
}

// 下限で止まった後の実際の増減を残す。何も変わらなければイベントを作らない。ただし能力は、このイベントが発動を表すので
// （BattleAbilityActivatedEvent を別に出すと、再生で 1 つの発動が 2 行に割れるため。2026-11-12 オーナー）、
// 持っていれば効き目が無くても出す決まり（2026-11-11 オーナー）に合わせて増減が 0 でも作る
func modifyBattleStats(r *recorder, side BattleSide, source BattleStatSource, attackDelta, hitPointsDelta int) error {
	before := r.state.Battle.creature(side).Stats
	attack, hitPoints := before.Attack.Modified(attackDelta), before.HitPoints.Modified(hitPointsDelta)
	actualAttack, actualHitPoints := attack.Int()-before.Attack.Int(), hitPoints.Int()-before.HitPoints.Int()
	if actualAttack == 0 && actualHitPoints == 0 && source.Ability == nil {
		return nil
	}
	return r.record(func(meta EventMeta, _ ParticipantIndex) Event {
		return &BattleStatModifiedEvent{EventMeta: meta, Side: side, Source: source, AttackDelta: actualAttack, HitPointsDelta: actualHitPoints, Attack: attack, HitPoints: hitPoints}
	})
}

func strike(r *recorder, striker BattleSide) error {
	damage := r.state.Battle.creature(striker).Stats.Attack
	remaining := r.state.Battle.creature(striker.opponent()).Stats.HitPoints.Damaged(damage)
	return r.record(func(meta EventMeta, _ ParticipantIndex) Event {
		return &CreatureStruckEvent{EventMeta: meta, Striker: striker, Damage: damage, RemainingHitPoints: remaining}
	})
}

func settleBattle(r *recorder) error {
	if r.state.Battle.Defender.Stats.HitPoints.Int() == 0 {
		return conquer(r)
	}
	return repel(r)
}

// 奪ったら、配置と同じくそのままターンを終える
func conquer(r *recorder) error {
	territory, ok := r.state.battleTerritory()
	if !ok || r.state.Invasion == nil {
		return ErrInvalidState
	}
	moved := r.state.Invasion.From != nil
	id := r.state.LastPlacedCreatureID + 1
	if moved {
		id = r.state.Invasion.Creature
	}
	hitPoints := r.state.Battle.Attacker.territoryHitPoints()
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &TerritoryConqueredEvent{EventMeta: meta, Participant: by, Position: territory.Position, DefeatedCreature: territory.Creature, Creature: id, TerritoryHitPoints: hitPoints}
	}); err != nil {
		return err
	}
	if moved {
		return afterTerritoryCommand(r)
	}
	return endTurn(r)
}

func repel(r *recorder) error {
	if r.state.Invasion == nil {
		return ErrInvalidState
	}
	invasion := *r.state.Invasion
	survived := r.state.Battle.Attacker.Stats.HitPoints.Int() > 0
	hitPoints := r.state.Battle.Defender.territoryHitPoints()
	repelled := &InvasionRepelledEvent{Participant: 0, Position: invasion.Position, Card: invasion.Card, ReturnedToHand: survived && invasion.From == nil, TerritoryHitPoints: hitPoints}
	if survived && invasion.From != nil {
		// 移動で侵略して生き残ったクリーチャーは、元の領地に戻る（傷は残る）
		from, returnedHitPoints := *invasion.From, r.state.Battle.Attacker.territoryHitPoints()
		repelled.ReturnedTo, repelled.ReturnedHitPoints = &from, &returnedHitPoints
	}
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		event := *repelled
		event.EventMeta, event.Participant = meta, by
		return &event
	}); err != nil {
		return err
	}
	if invasion.From != nil {
		return afterTerritoryCommand(r)
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
	// 倍率は盤面の通行料（切り捨てた後の値）に掛けて、もう一度切り捨てる。画面に見せた通行料から計算が合うようにするため
	// （2026-11-11 オーナー。core#157 の「最後に 1 回だけ切り捨てる」から変えた）
	if percent, ok := r.state.tollMultiplier(territory); ok {
		multiplied := toll * percent.Int() / 100
		if err := r.record(func(meta EventMeta, _ ParticipantIndex) Event {
			return &TollMultiplierActivatedEvent{EventMeta: meta, Position: territory.Position, Percent: percent, Toll: toll, MultipliedToll: multiplied}
		}); err != nil {
			return err
		}
		toll = multiplied
	}
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &TollPaidEvent{EventMeta: meta, Participant: by, Owner: owner, Position: territory.Position, Magic: toll}
	}); err != nil {
		return err
	}
	return settleDebt(r)
}

// 支払いと手放しの後の始末。手放せる領地が残っていて負のままなら手放す段階を続け、0 以上ならターンを終える（2026-11-10 オーナー。選ぶものが無いのにターン終了を待たせないため）。
// 手放す領地も無く負のままなら、魔力が枯渇して拠点からやり直し、ターンを終える。周回のボーナスも勝ちの判定も無い（2026-11-08 オーナー）
func settleDebt(r *recorder) error {
	if r.state.Phase != PhaseEnding {
		return nil
	}
	if r.current().Magic >= 0 {
		return endTurn(r)
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

// 上限のラウンドの最後の人が終えるときは、ラウンドを進めず（上限 +1 に見えないように）対戦を終える。
// 対戦が続くときだけ、手札が上限を超えていれば捨てる段階に入り、捨て終えてから終える（2026-11-10 オーナー。引いたターンのうちに手札を使えるよう、ターンの始めから移した。core#148）
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
	if index, _ := r.state.CurrentParticipant(); r.state.exceedsHandLimit(index) {
		return r.record(func(meta EventMeta, by ParticipantIndex) Event {
			return &HandLimitExceededEvent{EventMeta: meta, Participant: by, HandCount: len(r.state.Participants[index].Hand), MaxHandCards: r.state.Rules.MaxHandCards.Int()}
		})
	}
	// 最後のターンを終えると呪いが解ける（残るターンがあれば TurnEndedEvent で 1 減る）
	if curse := r.current().Curse; curse != nil && curse.Expiry.RemainingTurns <= 1 {
		if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
			return &CurseRemovedEvent{EventMeta: meta, Participant: by}
		}); err != nil {
			return err
		}
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
