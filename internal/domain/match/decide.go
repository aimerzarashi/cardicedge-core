package match

import (
	"slices"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/effect"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/element"
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
		return playTurn(state, c.By, PhaseDefending, now, func(r *recorder) error { return defend(r, c.By, c.ItemHandIndex, random) })
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
	return r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &CardDrawnEvent{EventMeta: meta, Participant: by}
	})
}

// 上限まで捨てたら、止めていたターンの終わりを続ける
func discard(r *recorder, handIndex int) error {
	hand := r.current().Hand
	if handIndex < 0 || handIndex >= len(hand) {
		return ErrHandIndexOutOfRange
	}
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &CardDiscardedEvent{EventMeta: meta, Participant: by, HandIndex: handIndex}
	}); err != nil {
		return err
	}
	if r.state.Phase == PhaseDiscarding {
		return nil
	}
	return endTurn(r)
}

// サイコロを振り、出た目の合計の分だけ進む。続いているスペルの効果があれば、それに従って振る（目を固定する・複数を同時に振る。1 個ずつ目の範囲で振る。2026-11-10 オーナー）
func roll(r *recorder, random Random) error {
	count := 1
	if lasting := r.current().LastingSpellEffect; lasting != nil {
		switch effect := lasting.Effect; {
		case effect.FixedDice != nil:
			value := effect.FixedDice.Value.Int()
			if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
				return &DiceRolledEvent{EventMeta: meta, Participant: by, Value: value}
			}); err != nil {
				return err
			}
			return advance(r)
		case effect.MultipleDice != nil:
			count = effect.MultipleDice.Count.Int()
		}
	}
	rules := r.state.Rules
	var dice []int
	value := 0
	for range count {
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

// ターンをまたいで効き続ける効果ならそのターン数
func lastingTurns(effect effect.SpellEffect) (int, bool) {
	switch {
	case effect.MultipleDice != nil:
		return effect.MultipleDice.Turns.Int(), true
	case effect.FixedDice != nil:
		return effect.FixedDice.Turns.Int(), true
	}
	return 0, false
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
		return &SpellUsedEvent{EventMeta: meta, Participant: by, HandIndex: handIndex, Cost: spec.Cost}
	}); err != nil {
		return err
	}
	effect := spec.Spell.Effect
	if effect.Damage != nil {
		return damageCreature(r, *targetPosition, effect.Damage.Amount.Int())
	}
	if turns, ok := lastingTurns(effect); ok {
		lasting := LastingSpellEffect{Effect: effect, RemainingTurns: turns}
		return r.record(func(meta EventMeta, by ParticipantIndex) Event {
			return &SpellEffectStartedEvent{EventMeta: meta, Participant: by, Target: *target, Effect: lasting}
		})
	}
	if effect.GainMagic != nil {
		// 周回数はこの対戦で周回を終えた回数（2026-11-10 オーナー。culdcept-lab.com のマナ）
		laps := r.current().Laps
		perLap := effect.GainMagic.PerLap.Int()
		magic := (laps + 1) * perLap
		return r.record(func(meta EventMeta, by ParticipantIndex) Event {
			return &MagicGainedEvent{EventMeta: meta, Participant: by, Magic: magic}
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
		return &CreatureDamagedEvent{EventMeta: meta, Participant: by, Position: position, Damage: damage, RemainingHitPoints: remaining}
	}); err != nil {
		return err
	}
	if remaining.Int() > 0 {
		return nil
	}
	return r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &CreatureDestroyedEvent{EventMeta: meta, Participant: by, Position: position}
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
		return &CreaturePlacedEvent{EventMeta: meta, Participant: by, HandIndex: handIndex, Creature: id, Cost: spec.Cost}
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
	if _, occupied := r.state.TerritoryAt(to); occupied {
		// 他人の領地へは侵略と同じ戦闘をする。アイテムを選ぶところから続ける
		return r.record(func(meta EventMeta, by ParticipantIndex) Event {
			return &TerritoryInvadedByMoveEvent{EventMeta: meta, Participant: by, From: from, Position: to}
		})
	}
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &CreatureMovedEvent{EventMeta: meta, Participant: by, From: from, To: to}
	}); err != nil {
		return err
	}
	return afterTerritoryCommand(r)
}

func changeLandElement(r *recorder, position board.Position, element element.Land) error {
	index := slices.IndexFunc(r.state.ElementChangeChoices(), func(choice ElementChangeChoice) bool { return choice.Position == position && choice.Element == element })
	if index < 0 {
		return ErrLandElementNotChangeable
	}
	choice := r.state.ElementChangeChoices()[index]
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &LandElementChangedEvent{EventMeta: meta, Participant: by, Position: position, Element: element, Magic: choice.Magic}
	}); err != nil {
		return err
	}
	return afterTerritoryCommand(r)
}

func exchange(r *recorder, position board.Position, handIndex int) error {
	if !slices.ContainsFunc(r.state.ExchangeChoices(), func(choice ExchangeChoice) bool { return choice.Position == position && choice.HandIndex == handIndex }) {
		return ErrCreatureNotExchangeable
	}
	ref := r.current().Hand[handIndex]
	cost := r.state.CardSpecs()[ref].Cost
	id := r.state.LastPlacedCreatureID + 1
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &CreatureExchangedEvent{EventMeta: meta, Participant: by, Position: position, HandIndex: handIndex, Creature: id, Cost: cost}
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
	return r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &TerritoryInvadedEvent{EventMeta: meta, Participant: by, HandIndex: handIndex, Cost: cost}
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
		return &ItemUsedEvent{EventMeta: meta, Participant: by, HandIndex: *itemHandIndex, Cost: cost}
	})
}

func defend(r *recorder, by ParticipantIndex, itemHandIndex *int, random Random) error {
	if err := chooseItem(r, by, itemHandIndex); err != nil {
		return err
	}
	return battle(r, random)
}

// 状態から戦闘の入力を組み立て、戦闘を入力と乱数の元だけで決め（resolveBattle）、出来事を順に残してから結果（奪う・退ける）に進む
func battle(r *recorder, random Random) error {
	input, err := r.state.battleInput()
	if err != nil {
		return err
	}
	events, err := resolveBattle(input, random)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := r.record(func(meta EventMeta, _ ParticipantIndex) Event { return event.withMeta(meta) }); err != nil {
			return err
		}
	}
	return settleBattle(r)
}

// 戦闘の入力。状況（領地・隣の領地・土地・ルール）と、侵略に出したクリーチャーと両方が選んだアイテムを、戦闘が始まる時点の状態から集める
func (m Match) battleInput() (BattleInput, error) {
	attacker, ok := m.CurrentParticipant()
	if !ok || m.Invasion == nil {
		return BattleInput{}, ErrInvalidState
	}
	territory, ok := m.battleTerritory()
	if !ok {
		return BattleInput{}, ErrInvalidState
	}
	defending, ok := m.PlacedCreature(territory.Creature)
	if !ok {
		return BattleInput{}, ErrInvalidState
	}
	invasion := *m.Invasion
	attackerItem, err := m.battleItem(invasion.AttackerItem)
	if err != nil {
		return BattleInput{}, err
	}
	defenderItem, err := m.battleItem(invasion.DefenderItem)
	if err != nil {
		return BattleInput{}, err
	}
	// 移動で侵略したときは、動いたクリーチャーがもういない元の領地を数えない（2026-11-10 オーナー）
	adjacent := slices.DeleteFunc(m.adjacentTerritoriesOf(invasion.Position, attacker), func(position board.Position) bool {
		return invasion.From != nil && position == *invasion.From
	})
	return BattleInput{
		Attacker: BattleCombatant{Card: invasion.Card, Spec: invasion.Base, Attack: invasion.Attack, HitPoints: invasion.HitPoints, Item: attackerItem, AdjacentTerritories: adjacent},
		Defender: BattleCombatant{Card: defending.Card, Spec: defending.Base, Attack: defending.Attack, HitPoints: defending.HitPoints, Item: defenderItem, AdjacentTerritories: m.adjacentTerritoriesOf(territory.Position, defending.Owner)},
		Land:     BattleLand{Element: m.landElement(territory.Position), Level: m.LandLevel(territory.Position)},
		Rules:    BattleRules{AdjacentTerritoryBonus: m.Rules.AdjacentTerritoryBonus, TerrainBonusPerLevel: m.Rules.TerrainBonusPerLevel},
	}, nil
}

func (m Match) battleItem(used *card.Ref) (*BattleItem, error) {
	if used == nil {
		return nil, nil
	}
	spec := m.CardSpecs()[*used].Item
	if spec == nil {
		return nil, ErrInvalidState
	}
	return &BattleItem{Card: *used, Spec: *spec}, nil
}

// 戦闘を、入力と乱数の元だけで決める。効果を順に当て、残っていれば先に攻撃する側（strikeOrder）が攻撃し、相手が生きていれば反撃する。
// どこかで体力が 0 になれば、その時点で戦闘は終わる（残りの効果も攻撃も無い。2026-11-09 オーナー）。
// 状態を見ないのは、戦闘を「意思決定と状況を入力にして出来事の並びを出す独立した処理」にし、入力と出力だけで確かめられるようにするため
// （2026-11-13 オーナー。core#171）。外に出すのは、決まりを入力と出来事の並びの表で確かめるため（core#172）と、
// NPC の考え方で侵略・アイテムを見積もるときに同じ決まりを使えるようにするため。返す出来事は版を持たない
func ResolveBattle(input BattleInput, random Random) ([]Event, error) {
	resolved, err := resolveBattle(input, random)
	if err != nil {
		return nil, err
	}
	events := make([]Event, len(resolved))
	for i, event := range resolved {
		events[i] = event
	}
	return events, nil
}

// 版を入れる前の出来事（recorder が withMeta で版を入れる）
func resolveBattle(input BattleInput, random Random) ([]battleEvent, error) {
	b := &battleResolver{input: input}
	if err := b.emit(&BattleStartedEvent{Input: input}); err != nil {
		return nil, err
	}
	for _, apply := range b.effects(random) {
		if err := apply(); err != nil {
			return nil, err
		}
		if b.battle.decided() {
			return b.events, nil
		}
	}
	if err := b.activateStrikeEffects(); err != nil {
		return nil, err
	}
	for _, striker := range b.strikeOrder() {
		if err := b.strike(striker); err != nil {
			return nil, err
		}
		if b.battle.decided() {
			break
		}
	}
	return b.events, nil
}

// 戦闘を決める間の値。出した出来事を evolveBattle で当てて、今の AT・HP を進める（Evolve と同じ決まりで進めるため）
type battleResolver struct {
	input  BattleInput
	battle *Battle
	events []battleEvent
}

func (b *battleResolver) emit(event battleEvent) error {
	next, err := evolveBattle(b.battle, event)
	if err != nil {
		return err
	}
	b.battle = next
	b.events = append(b.events, event)
	return nil
}

// 効果を当てる順はここだけで決める。攻める側を全部当ててから守る側で、各側の中は スペルの効果 → クリーチャーの効果 → 場の効果 → アイテム（2026-11-09 オーナー）。
// 場の効果は、攻める側は支援効果、守る側は支援効果 → 地形効果（AT と HP なので順で値は変わらないが、攻める側と並びをそろえる。2026-11-15 オーナー。core#178）。
// スペルの効果は作るときに前へ足す。倍率は、両方の側の増減・上書きがすべて済んだ後の倍率の段階で掛ける（2026-11-14 オーナー。core#177）
func (b *battleResolver) effects(random Random) []func() error {
	return []func() error{
		func() error { return b.applyCreatureEffects(BattleSideAttacker, random) },
		func() error { return b.applySupportEffect(BattleSideAttacker) },
		func() error { return b.applyItem(BattleSideAttacker, random) },
		func() error { return b.applyCreatureEffects(BattleSideDefender, random) },
		func() error { return b.applySupportEffect(BattleSideDefender) },
		b.applyTerrain,
		func() error { return b.applyItem(BattleSideDefender, random) },
		func() error { return b.applyAttackMultiplier(BattleSideAttacker) },
		func() error { return b.applyAttackMultiplier(BattleSideDefender) },
	}
}

// クリーチャーの効果の位置。効果を並びの順に、AT・HP を変える効果（増減・上書き）だけ当てる（先に・後に攻撃するは攻撃の前に、
// 受けるダメージを 0 にするは攻撃を受ける直前に別に発動する）。種類ごとに決まった順にしないのは、効果の並びが入力のまま残る決まりで、
// 当てる順・乱数を引く順になるため（core#127・core#178）
func (b *battleResolver) applyCreatureEffects(side BattleSide, random Random) error {
	for _, candidate := range b.input.Combatant(side).Spec.Effects {
		change, ok := b.statChange(side, candidate, random)
		if !ok {
			continue
		}
		if err := b.emit(&BattleCreatureEffectActivatedEvent{Side: side, Effect: candidate.Kind(), Change: &change}); err != nil {
			return err
		}
		if b.battle.decided() {
			return nil
		}
	}
	return nil
}

// AT・HP を変える効果（増減・上書き）の実際の増減。ほかの種類は false。クリーチャーとアイテムの位置で同じ決まりを使う。
// 上書きの増減は、引いた値と今の値の差（増減が 0 でも発動を表すので出す）
func (b *battleResolver) statChange(side BattleSide, candidate effect.Effect, random Random) (BattleStatChange, bool) {
	before := b.battle.Creature(side).Stats()
	switch {
	case candidate.AttackModifier != nil:
		return b.actualChange(side, candidate.AttackModifier.Value.Int(), 0), true
	case candidate.HitPointsModifier != nil:
		return b.actualChange(side, 0, candidate.HitPointsModifier.Value.Int()), true
	case candidate.AttackOverride != nil:
		attack := overrideValue(candidate.AttackOverride.Minimum.Int(), candidate.AttackOverride.Maximum.Int(), random)
		return b.actualChange(side, attack-before.Attack.Int(), 0), true
	case candidate.HitPointsOverride != nil:
		hitPoints := overrideValue(candidate.HitPointsOverride.Minimum.Int(), candidate.HitPointsOverride.Maximum.Int(), random)
		return b.actualChange(side, 0, hitPoints-before.HitPoints.Int()), true
	}
	return BattleStatChange{}, false
}

// 上書きの値。最小〜最大から 1 刻みで引く。固定（最小と最大が同じ）は引かない（乱数の元を使わず、固定の上書きだけの戦闘の再生を変えないため）
func overrideValue(minimum, maximum int, random Random) int {
	if minimum == maximum {
		return minimum
	}
	return minimum + random.IntN(maximum-minimum+1)
}

// 隣の自分の領地が無ければ発動しない
func (b *battleResolver) applySupportEffect(side BattleSide) error {
	adjacent := b.input.Combatant(side).AdjacentTerritories
	if len(adjacent) == 0 {
		return nil
	}
	bonus := len(adjacent) * b.input.Rules.AdjacentTerritoryBonus.Int()
	return b.emit(&BattleSupportEffectActivatedEvent{Side: side, AttackDelta: b.actualChange(side, bonus, 0).AttackDelta})
}

func (b *battleResolver) applyTerrain() error {
	if !terrainMatches(b.input.Land.Element, b.input.Defender.Spec.Elements) {
		return nil
	}
	bonus := b.input.Land.Level * b.input.Rules.TerrainBonusPerLevel.Int()
	return b.emit(&BattleTerrainEffectActivatedEvent{HitPointsDelta: b.actualChange(BattleSideDefender, 0, bonus).HitPointsDelta})
}

// 複の土地は、無属性以外のクリーチャーならどれでも効く（2026-11-09 オーナー）
func terrainMatches(land element.Land, elements creature.Elements) bool {
	if land == element.LandMulti {
		return !slices.Contains(elements, element.CreatureNeutral)
	}
	return slices.ContainsFunc(elements, func(element element.Creature) bool { return element.String() == land.String() })
}

// アイテムの位置。効果を並びの順に、増減・上書きだけ当てる（増減が 0 でも発動を表すので出す。倍率は倍率の段階、
// 地形効果を無視するは攻撃の直前に発動する）
func (b *battleResolver) applyItem(side BattleSide, random Random) error {
	used := b.input.Combatant(side).Item
	if used == nil {
		return nil
	}
	for _, candidate := range used.Spec.Effects {
		change, ok := b.statChange(side, candidate, random)
		if !ok {
			continue
		}
		if err := b.emit(&BattleItemEffectActivatedEvent{Side: side, Effect: candidate.Kind(), Change: &change}); err != nil {
			return err
		}
		if b.battle.decided() {
			return nil
		}
	}
	return nil
}

// 倍率の段階。制約を満たす AT の倍率のうち一番高いものを 1 回だけ掛け（切り捨て）、掛けたものだけが発動を出す。採られなかった
// 倍率は何も出さない（2026-11-14 オーナー。重なるカードを作るときに見直す）。同じ倍率が並べば先のものを採る
func (b *battleResolver) applyAttackMultiplier(side BattleSide) error {
	used := b.input.Combatant(side).Item
	if used == nil {
		return nil
	}
	opponentElements := b.input.Combatant(side.opponent()).Spec.Elements
	highest := 0
	for _, effect := range used.Spec.Effects {
		if effect.AttackMultiplier == nil || !effect.AttackMultiplier.Constraints.SatisfiedByOpponentCreatureElements(opponentElements) {
			continue
		}
		highest = max(highest, effect.AttackMultiplier.Percent.Int())
	}
	if highest == 0 {
		return nil
	}
	attack := b.battle.Creature(side).Stats().Attack.Int()
	change := b.actualChange(side, attack*highest/100-attack, 0)
	return b.emit(&BattleItemEffectActivatedEvent{Side: side, Effect: effect.KindAttackMultiplier, Change: &change})
}

// 地形効果を無視する効果を持つ側が攻撃する直前に、相手に地形効果の層があれば発動する（無視するものが無ければ状態が変わらないので
// 出さない。2026-11-14 オーナー）
func (b *battleResolver) ignoreTerrainEffect(striker BattleSide) error {
	used := b.input.Combatant(striker).Item
	if used == nil || !b.battle.Creature(striker.opponent()).hasTerrainEffect() {
		return nil
	}
	if !slices.ContainsFunc(used.Spec.Effects, func(effect effect.Effect) bool { return effect.IgnoreTerrainEffect != nil }) {
		return nil
	}
	return b.emit(&BattleItemEffectActivatedEvent{Side: striker, Effect: effect.KindIgnoreTerrainEffect})
}

// 今の値に当てて下限（0）で止まった後の、実際の増減
func (b *battleResolver) actualChange(side BattleSide, attackDelta, hitPointsDelta int) BattleStatChange {
	before := b.battle.Creature(side).Stats()
	return BattleStatChange{
		AttackDelta:    before.Attack.Modified(attackDelta).Int() - before.Attack.Int(),
		HitPointsDelta: before.HitPoints.Modified(hitPointsDelta).Int() - before.HitPoints.Int(),
	}
}

// 攻撃の順に効く効果（先に・後に攻撃する）を、持っている側ごとに攻める側 → 守る側の順で発動させる。効き目が無くても持っていれば出す
func (b *battleResolver) activateStrikeEffects() error {
	for _, side := range []BattleSide{BattleSideAttacker, BattleSideDefender} {
		kind, ok := b.input.Combatant(side).Spec.strikeEffect()
		if !ok {
			continue
		}
		if err := b.emit(&BattleCreatureEffectActivatedEvent{Side: side, Effect: kind}); err != nil {
			return err
		}
	}
	return nil
}

// 攻撃する順。先に攻撃する > どちらも無し > 後に攻撃する の強い方が先で、同じなら攻める側が先（culdcept-lab.com の戦闘のクイズの基礎編。2026-11-11 オーナー。core#158）。
// 強さは発動で得た戦闘の中の値で比べる（core#175）
func (b *battleResolver) strikeOrder() []BattleSide {
	if b.battle.Defender.StrikePriority > b.battle.Attacker.StrikePriority {
		return []BattleSide{BattleSideDefender, BattleSideAttacker}
	}
	return []BattleSide{BattleSideAttacker, BattleSideDefender}
}

// 受けるダメージを 0 にする（攻撃無効）は相手の AT を下げるのではないので、発動を攻撃の前に別のイベントで出す
// （攻撃のイベントに埋めると、ダメージが 0 になった理由がイベントの並びに出ないため。2026-11-13 オーナー。core#174）。
// 発動するか（制約を満たすか）は入力の性能で判定し、ダメージを 0 にするかは発動で得た戦闘の中の値で決める（core#175）
func (b *battleResolver) strike(striker BattleSide) error {
	receiver := striker.opponent()
	// 地形効果を無視する（攻める側の効果）→ 受けるダメージを 0 にする（受ける側の効果）→ 攻撃の順（2026-11-14 オーナー）
	if err := b.ignoreTerrainEffect(striker); err != nil {
		return err
	}
	if b.input.Combatant(receiver).Spec.nullifiesDamageFrom(b.input.Combatant(striker).Spec) {
		if err := b.emit(&BattleCreatureEffectActivatedEvent{Side: receiver, Effect: effect.KindNullifyReceivedDamage}); err != nil {
			return err
		}
	}
	attack := b.battle.Creature(striker).Stats().Attack
	// 受ける割合を掛けて切り捨てる（受けるダメージを 0 にするは 0）
	damage := attack.Modified(attack.Int()*b.battle.Creature(receiver).ReceivedDamagePercent/100 - attack.Int())
	remaining := b.battle.Creature(receiver).Stats().HitPoints.Damaged(damage)
	return b.emit(&CreatureStruckEvent{Striker: striker, Damage: damage, RemainingHitPoints: remaining})
}

func settleBattle(r *recorder) error {
	if r.state.Battle.Defender.Stats().HitPoints.Int() == 0 {
		return conquer(r)
	}
	return repel(r)
}

// 奪ったら、配置と同じくそのままターンを終える
func conquer(r *recorder) error {
	_, ok := r.state.battleTerritory()
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
		return &TerritoryConqueredEvent{EventMeta: meta, Participant: by, Creature: id, TerritoryHitPoints: hitPoints}
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
	survived := r.state.Battle.Attacker.Stats().HitPoints.Int() > 0
	hitPoints := r.state.Battle.Defender.territoryHitPoints()
	repelled := &InvasionRepelledEvent{Participant: 0, TerritoryHitPoints: hitPoints}
	if survived && invasion.From != nil {
		// 移動で侵略して生き残ったクリーチャーは、元の領地に戻る（傷は残る）
		returnedHitPoints := r.state.Battle.Attacker.territoryHitPoints()
		repelled.ReturnedHitPoints = &returnedHitPoints
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
	if _, ok := r.state.TerritoryOwner(territory); !ok {
		return ErrInvalidState
	}
	toll := r.state.TerritoryToll(territory)
	// 倍率は盤面の通行料（切り捨てた後の値）に掛けて、もう一度切り捨てる。画面に見せた通行料から計算が合うようにするため
	// （2026-11-11 オーナー。core#157 の「最後に 1 回だけ切り捨てる」から変えた）
	if percent, ok := r.state.tollMultiplier(territory); ok {
		multiplied := toll * percent.Int() / 100
		if err := r.record(func(meta EventMeta, _ ParticipantIndex) Event {
			return &TollMultiplierActivatedEvent{EventMeta: meta}
		}); err != nil {
			return err
		}
		toll = multiplied
	}
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &TollPaidEvent{EventMeta: meta, Participant: by, Magic: toll}
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
		return &TerritoryRelinquishedEvent{EventMeta: meta, Participant: by, Position: position, Magic: magic}
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
				return &CheckpointPassedEvent{EventMeta: meta, Participant: by, Magic: rules.CheckpointBonus}
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
			return &HandLimitExceededEvent{EventMeta: meta, Participant: by}
		})
	}
	// 最後のターンを終えると続いていたスペルの効果が終わる（残るターンがあれば TurnEndedEvent で 1 減る）
	if lasting := r.current().LastingSpellEffect; lasting != nil && lasting.RemainingTurns <= 1 {
		if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
			return &SpellEffectEndedEvent{EventMeta: meta, Participant: by}
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
