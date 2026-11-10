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
	case *RollCommand:
		return playTurn(state, c.By, PhaseRolling, now, func(r *recorder) error { return roll(r, random) })
	case *ChoosePathCommand:
		return playTurn(state, c.By, PhaseChoosingPath, now, func(r *recorder) error { return choosePath(r, c.To) })
	case *DeployCreatureCommand:
		return playTurn(state, c.By, PhaseDeploying, now, func(r *recorder) error { return deploy(r, c.HandIndex) })
	case *LevelUpTerritoryCommand:
		return playTurn(state, c.By, PhaseTerritoryCommanding, now, func(r *recorder) error { return levelUp(r, c.Position, c.Level) }, PhaseDeploying, PhaseInvading)
	case *MoveCreatureCommand:
		return playTurn(state, c.By, PhaseTerritoryCommanding, now, func(r *recorder) error { return moveCreature(r, c.From, c.To) }, PhaseDeploying, PhaseInvading)
	case *ChangeLandElementCommand:
		return playTurn(state, c.By, PhaseTerritoryCommanding, now, func(r *recorder) error { return changeLandElement(r, c.Position, c.Element) }, PhaseDeploying, PhaseInvading)
	case *ExchangeCreatureCommand:
		return playTurn(state, c.By, PhaseTerritoryCommanding, now, func(r *recorder) error { return exchange(r, c.Position, c.HandIndex) }, PhaseDeploying, PhaseInvading)
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
		return playTurn(state, c.By, PhaseEnding, now, endTurn, PhaseDeploying, PhaseTerritoryCommanding)
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
	if _, occupied := r.state.TerritoryAt(to); occupied {
		// 他人の領地へは侵略と同じ戦闘をする。アイテムを選ぶところから続ける
		return r.record(func(meta EventMeta, by ParticipantIndex) Event {
			return &TerritoryInvadedByMoveEvent{EventMeta: meta, Participant: by, From: from, Position: to, Creature: territory.Creature}
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
	magic := r.state.ElementChangeChoices()[index].Magic
	if err := r.record(func(meta EventMeta, by ParticipantIndex) Event {
		return &LandElementChangedEvent{EventMeta: meta, Participant: by, Position: position, Element: element, Magic: magic}
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
	replaced, ok := r.state.DeployedCreature(territory.Creature)
	if !ok {
		return ErrInvalidState
	}
	ref := r.current().Hand[handIndex]
	cost := r.state.CardSpecs()[ref].Cost
	id := r.state.LastDeployedCreatureID + 1
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

// 効果を順に当て、残っていれば攻める側が先に攻撃し、守る側が生きていれば反撃する。
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
	for _, striker := range []BattleSide{BattleSideAttacker, BattleSideDefender} {
		if err := strike(r, striker); err != nil {
			return err
		}
		if r.state.Battle.decided() {
			break
		}
	}
	return settleBattle(r)
}

func startBattle(r *recorder) error {
	territory, ok := r.state.battleTerritory()
	if !ok || r.state.Invasion == nil {
		return ErrInvalidState
	}
	defending, ok := r.state.DeployedCreature(territory.Creature)
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
// 呪い・能力は作るときに前へ足し、倍率の効果は足し算がすべて終わった後に足す
func battleEffects() []func(*recorder) error {
	return []func(*recorder) error{
		applyAdjacentTerritories,
		func(r *recorder) error { return applyItem(r, BattleSideAttacker, r.state.Invasion.AttackerItem) },
		applyTerrain,
		func(r *recorder) error { return applyItem(r, BattleSideDefender, r.state.Invasion.DefenderItem) },
	}
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
	return modifyBattleStats(r, BattleSideAttacker, NewAdjacentTerritoriesSource(positions), len(positions)*r.state.Rules.AdjacentTerritoryBonus.Int(), 0)
}

func applyTerrain(r *recorder) error {
	territory, ok := r.state.battleTerritory()
	if !ok {
		return ErrInvalidState
	}
	defending, ok := r.state.DeployedCreature(territory.Creature)
	if !ok {
		return ErrInvalidState
	}
	element := r.state.landElement(territory.Position)
	if !terrainMatches(element, defending.Base.Elements) {
		return nil
	}
	level := r.state.LandLevel(territory.Position)
	return modifyBattleStats(r, BattleSideDefender, NewTerrainSource(element, level), 0, level*r.state.Rules.TerrainBonusPerLevel.Int())
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
	return modifyBattleStats(r, side, NewItemSource(*used), spec.AttackBonus.Int(), spec.HitPointsBonus.Int())
}

// 下限で止まった後の実際の増減を残す。何も変わらなければイベントを作らない
func modifyBattleStats(r *recorder, side BattleSide, source BattleStatSource, attackDelta, hitPointsDelta int) error {
	before := r.state.Battle.creature(side).Stats
	attack, hitPoints := before.Attack.Modified(attackDelta), before.HitPoints.Modified(hitPointsDelta)
	actualAttack, actualHitPoints := attack.Int()-before.Attack.Int(), hitPoints.Int()-before.HitPoints.Int()
	if actualAttack == 0 && actualHitPoints == 0 {
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
	id := r.state.LastDeployedCreatureID + 1
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
			return &HandLimitExceededEvent{EventMeta: meta, Participant: by}
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
