// NPC の判断。対戦の状態と選べるものから、NPC が送るコマンドを 1 つ選ぶ（選ぶ重みは、NPC の参加者に写した思考パターン npcmindset から読む）。
// 決まり（できるか）は match の Decide と選択肢（ActionChoices 等）だけが持ち、ここは選べるものの中から選ぶだけにする
// （決まりを NPC の側に書き写さないため。設計メモ「NPC の考え方」）
package npcdecision

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/matchaction"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
)

// 引く・捨てる・振る前は、山札の順とサイコロ（隠れた情報）で結果が変わり、後の状態を当てると先に見ることになるので、採点せず
// 種類ごとの重みで選ぶ。それ以外（配置・侵略・攻める・守る・手放す・領地コマンド・終わり）は、候補ごとに採点基準で点を付けて選ぶ（scoreCandidates）。
// 分かれ道の行き先は目が出た後で隠れた情報を見ないので、止まった後の一番よい行動まで進めた点で選ぶ（choosePath。core#191）。
// 段階で分けるのは、点の付く段階と付かない段階が混ざらないようにするため
func scored(phase match.Phase) bool {
	switch phase {
	case match.PhaseDrawing, match.PhaseDiscarding, match.PhaseRolling:
		return false
	default:
		return true
	}
}

// 点の付かない段階では、操作を待っている参加者が今選べる行動の種類から、重みに比例して 1 つを選び、その種類の対象は均等に選ぶ（2026-11-17 オーナー。
// core#179・core#187）。選べるものを全部 1 列に並べて引かないのは、候補の多い種類（レベルアップ・移動等）ばかりが選ばれないようにするため。
// 重みがどれも同じなら完全ランダム。
// 重みは引数で受けず、NPC の参加者に写した思考パターンから読む（studio のマスタを直接読むと、記録したイベントだけで同じ対戦を再生できなくなるため）
func Choose(current match.Match, by match.ParticipantIndex, random match.Random) (match.Command, error) {
	if by.Int() >= len(current.Participants) || current.Participants[by].Participant.NPC == nil {
		return nil, fmt.Errorf("npcdecision: participant %d: %w", by, ErrNotNPC)
	}
	mindset := current.Participants[by].Participant.NPC.Mindset
	actions := current.ActionChoices()
	if len(actions) == 0 {
		return nil, fmt.Errorf("npcdecision: no action choices: %w", match.ErrInvalidState)
	}
	if current.Phase == match.PhaseChoosingPath {
		return choosePath(current, by, mindset.CriterionWeights, random)
	}
	if scored(current.Phase) {
		candidates, err := candidatesOf(current, by, actions)
		if err != nil {
			return nil, err
		}
		scores, err := scoreCandidates(current, current, by, candidates, mindset.CriterionWeights, random)
		if err != nil {
			return nil, err
		}
		return pickBest(candidates, scores, random), nil
	}
	action := chooseAction(actions, mindset.ActionWeights, random)
	cmd, ok := command(current, by, action, random)
	if !ok {
		return nil, fmt.Errorf("npcdecision: no targets for action %q: %w", action, match.ErrInvalidState)
	}
	return cmd, nil
}

// 重みの合計から 1 つ引き、並びの順に重みを積んで当たった種類を選ぶ。選べる種類の重みがすべて 0 なら均等に選ぶ
// （通行料を払うしか選べない場面で払うの重みが 0 でも、止まらないようにするため）
func chooseAction(actions []matchaction.Action, weights npcmindset.ActionWeights, random match.Random) matchaction.Action {
	total := 0
	for _, action := range actions {
		total += weights[action].Int()
	}
	if total == 0 {
		return actions[random.IntN(len(actions))]
	}
	drawn := random.IntN(total)
	for _, action := range actions {
		drawn -= weights[action].Int()
		if drawn < 0 {
			return action
		}
	}
	return actions[len(actions)-1]
}

func pick[T any](random match.Random, choices []T) (T, bool) {
	if len(choices) == 0 {
		var zero T
		return zero, false
	}
	return choices[random.IntN(len(choices))], true
}

// 種類ごとに対象を選んでコマンドを作る。一覧にある種類なら対象は空でない（match の ActionChoices の決まり）ので、
// 空なら false を返して誤りにする
func command(current match.Match, by match.ParticipantIndex, action matchaction.Action, random match.Random) (match.Command, bool) {
	switch action {
	case matchaction.ActionDrawCard:
		return &match.DrawCommand{By: by}, true
	case matchaction.ActionDiscardCard:
		// 捨てる手札には選択肢の関数が無い（手札のどれでも捨てられる）
		hand := current.Participants[by].Hand
		if len(hand) == 0 {
			return nil, false
		}
		return &match.DiscardCommand{By: by, HandIndex: random.IntN(len(hand))}, true
	case matchaction.ActionUseSpell:
		choice, ok := pick(random, current.SpellChoices())
		if !ok {
			return nil, false
		}
		cmd := &match.UseSpellCommand{By: by, HandIndex: choice.HandIndex}
		if target, ok := pick(random, choice.Targets); ok {
			cmd.Target = &target
		}
		if position, ok := pick(random, choice.TargetPositions); ok {
			cmd.TargetPosition = &position
		}
		return cmd, true
	case matchaction.ActionRollDice:
		return &match.RollCommand{By: by}, true
	case matchaction.ActionChoosePath:
		to, ok := pick(random, current.PathChoices())
		return &match.ChoosePathCommand{By: by, To: to}, ok
	case matchaction.ActionPlaceCreature:
		handIndex, ok := pick(random, current.PlaceChoices())
		return &match.PlaceCreatureCommand{By: by, HandIndex: handIndex}, ok
	case matchaction.ActionInvadeTerritory:
		handIndex, ok := pick(random, current.InvadeChoices())
		return &match.InvadeCommand{By: by, HandIndex: handIndex}, ok
	case matchaction.ActionPayToll:
		return &match.PayTollCommand{By: by}, true
	case matchaction.ActionLevelUpLand:
		choice, ok := pick(random, current.LevelUpChoices())
		return &match.LevelUpLandCommand{By: by, Position: choice.Position, Level: choice.Level}, ok
	case matchaction.ActionExchangeCreature:
		choice, ok := pick(random, current.ExchangeChoices())
		return &match.ExchangeCreatureCommand{By: by, Position: choice.Position, HandIndex: choice.HandIndex}, ok
	case matchaction.ActionChangeLandElement:
		choice, ok := pick(random, current.ElementChangeChoices())
		return &match.ChangeLandElementCommand{By: by, Position: choice.Position, Element: choice.Element}, ok
	case matchaction.ActionMoveCreature:
		choice, ok := pick(random, current.MoveChoices())
		return &match.MoveCreatureCommand{By: by, From: choice.From, To: choice.To}, ok
	case matchaction.ActionAttackWithItem:
		handIndex, ok := pick(random, current.ItemChoices())
		return &match.AttackCommand{By: by, ItemHandIndex: &handIndex}, ok
	case matchaction.ActionAttackWithoutItem:
		return &match.AttackCommand{By: by}, true
	case matchaction.ActionDefendWithItem:
		handIndex, ok := pick(random, current.ItemChoices())
		return &match.DefendCommand{By: by, ItemHandIndex: &handIndex}, ok
	case matchaction.ActionDefendWithoutItem:
		return &match.DefendCommand{By: by}, true
	case matchaction.ActionUseCreatureAsItem:
		handIndex, ok := pick(random, current.CreatureAsItemChoices())
		return &match.UseCreatureAsItemCommand{By: by, HandIndex: handIndex}, ok
	case matchaction.ActionRelinquishTerritory:
		position, ok := pick(random, current.RelinquishChoices())
		return &match.RelinquishTerritoryCommand{By: by, Position: position}, ok
	case matchaction.ActionEndTurn:
		return &match.EndTurnCommand{By: by}, true
	default:
		return nil, false
	}
}

// 戦闘が決まるまでの先読みを、乱数のぶれをならすために何回当てるか。戦闘にならない候補は 1 回。値は全員 NPC の対戦で見て決める
const battleSamples = 8

// 候補を当てた後の状態の値。採点基準の 3 つと、全員に同じ重みで損に数える上手さの 2 つ（性格で変えないので思考パターンの重みに持たない。core#191）
type outcome struct {
	ownTotalMagic   float64
	rivalTotalMagic float64 // 相手の平均
	territoryCount  float64
	// クリーチャーの消耗（attrition）。自分の領地のクリーチャーが、判断を始めたときより弱った割合 × その領地の価値。今は HP の減りだけ（AT の低下・副作用は、
	// 起こす効果を作るときに足す）。これが無いと、負けた移動の侵略は費用も無く元の領地に戻るので点の上でただになり、何もしないと同点のくじで選ばれていた
	attrition float64
	// 空けた土地。判断を始めたときにレベル 2 以上だった自分の領地が空いたら、その価値（レベルが土地に残り、次に置いた人がただで引き継ぐため）
	vacated float64
	// 守備評価（defense）。自分の領地のうち、相手の手札で一番強い攻めに耐えきれないものの価値の合計。消耗と空けた土地だけでは、
	// 高い土地を守るクリーチャーを交換する・強いクリーチャーを価値の高い土地に置く良し悪しが点に出なかったため（core#194）
	defense float64
}

func (o outcome) plus(other outcome, rate float64) outcome {
	return outcome{
		ownTotalMagic:   o.ownTotalMagic + other.ownTotalMagic*rate,
		rivalTotalMagic: o.rivalTotalMagic + other.rivalTotalMagic*rate,
		territoryCount:  o.territoryCount + other.territoryCount*rate,
		attrition:       o.attrition + other.attrition*rate,
		vacated:         o.vacated + other.vacated*rate,
		defense:         o.defense + other.defense*rate,
	}
}

// origin は判断を始めたときの状態（消耗と空けた土地は、そこから変わった分を数える）
func outcomeOf(origin, state match.Match, by match.ParticipantIndex) outcome {
	var result outcome
	totals := state.TotalMagics()
	for i, total := range totals {
		if i == by.Int() {
			result.ownTotalMagic = float64(total)
			continue
		}
		result.rivalTotalMagic += float64(total) / float64(len(totals)-1)
	}
	result.territoryCount = float64(state.TerritoryCount(by))
	for _, territory := range state.Territories {
		if owner, ok := state.TerritoryOwner(territory); !ok || owner != by {
			continue
		}
		now, ok := state.PlacedCreature(territory.Creature)
		if !ok {
			continue
		}
		before, ok := origin.PlacedCreature(territory.Creature)
		if !ok {
			continue
		}
		if lost := before.HitPoints.Int() - now.HitPoints.Int(); lost > 0 {
			result.attrition += float64(lost) / float64(now.Base.HitPoints.Int()) * float64(state.TerritoryValue(territory))
		}
	}
	for _, territory := range origin.Territories {
		if owner, ok := origin.TerritoryOwner(territory); !ok || owner != by || origin.LandLevel(territory.Position) < 2 {
			continue
		}
		if _, still := state.TerritoryAt(territory.Position); !still {
			result.vacated += float64(origin.TerritoryValue(territory))
		}
	}
	result.defense = defenseLoss(state, by)
	return result
}

// 守る強さは、今の HP ＋ 地形効果（属性が合えば土地のレベルの分）で見る。手札の防具・援護は数えない（全員 NPC の対戦で、
// 手札に防具があっても守った戦闘の 8 割以上を取られていて、数えると守れない土地を守れると見誤り、数えないほうが強かった。
// 侵略する側は勝てると見込んだときにしか攻めないため）。攻めの強さは、相手ごとの手札のクリーチャーの AT ＋ その相手の手札で
// 一番強い武器の AT の増減の、相手全員での一番大きい値。サイコロの届く範囲で相手を絞らない（絞る形はまだ試していない）。
// 取られる見込みを割合にせず価値をまるごと損に数えるのは、割合（0.3）にすると効き目が無くなったため（core#194）
func defenseLoss(state match.Match, by match.ParticipantIndex) float64 {
	threat := strongestThreat(state, by)
	loss := 0.0
	for _, territory := range state.Territories {
		if owner, ok := state.TerritoryOwner(territory); !ok || owner != by {
			continue
		}
		placed, ok := state.PlacedCreature(territory.Creature)
		if !ok {
			continue
		}
		if placed.HitPoints.Int()+state.TerrainBonus(territory) <= threat {
			loss += float64(state.TerritoryValue(territory))
		}
	}
	return loss
}

func strongestThreat(state match.Match, by match.ParticipantIndex) int {
	specs := state.CardSpecs()
	threat := 0
	for i, participant := range state.Participants {
		if i == by.Int() {
			continue
		}
		weapon := strongestWeapon(specs, participant.Hand)
		for _, ref := range participant.Hand {
			if spec, ok := specs[ref]; ok && spec.Creature != nil {
				threat = max(threat, spec.Creature.Attack.Int()+weapon)
			}
		}
	}
	return threat
}

// 上書き・倍率は数えない（値が範囲や相手の属性で決まり、守る側から一つに決められないため。見本の武器は増減だけ）
func strongestWeapon(specs map[card.Ref]match.CardSpec, hand []card.Ref) int {
	best := 0
	for _, ref := range hand {
		spec, ok := specs[ref]
		if !ok || spec.Item == nil {
			continue
		}
		total := 0
		for _, e := range spec.Item.Effects {
			if e.AttackModifier != nil {
				total += e.AttackModifier.Value.Int()
			}
		}
		best = max(best, total)
	}
	return best
}

// 点。採点基準の値（額）に重みを掛けて足し、上手さの分を引く。0〜1 にそろえない（npcmindset.Criterion の理由）。
// 重みがすべて 0 なら上手さも数えず 0 にする（すべての候補が同じ点になり均等に選ぶ、完全ランダム型を保つため。core#179）
func score(state match.Match, o outcome, weights npcmindset.CriterionWeights) float64 {
	if allZero(weights) {
		return 0
	}
	return float64(weights[npcmindset.CriterionOwnTotalMagic].Int())*o.ownTotalMagic -
		float64(weights[npcmindset.CriterionRivalTotalMagic].Int())*o.rivalTotalMagic +
		float64(weights[npcmindset.CriterionTerritoryCount].Int())*o.territoryCount*float64(state.Rules.BaseLandValue) -
		o.attrition - o.vacated - o.defense
}

func allZero(weights npcmindset.CriterionWeights) bool {
	for _, weight := range weights {
		if weight.Int() != 0 {
			return false
		}
	}
	return true
}

// 候補ごとに、今の状態の写しにコマンドを当てた後の状態で測る（決まりを NPC の側に書き写さないため）。
// match.Evolve は変えるスライスと map を先に Clone してから変えるので、Match の値のまま渡せば元の状態は変わらない（テストで確かめる）。
// Decide に渡す now はイベントの発生時刻にしか使われず、ここで出すイベントは捨てるので、ゼロ値でよい
func scoreCandidates(origin, current match.Match, by match.ParticipantIndex, candidates []match.Command, weights npcmindset.CriterionWeights, random match.Random) ([]float64, error) {
	scores := make([]float64, len(candidates))
	for i, candidate := range candidates {
		measured, err := measure(origin, current, by, candidate, weights, random)
		if err != nil {
			return nil, err
		}
		scores[i] = score(current, measured, weights)
	}
	return scores, nil
}

// 候補 1 つを当てた後の値。戦闘になる候補は、戦闘の乱数の違う先読みを battleSamples 回当てて平均する
func measure(origin, current match.Match, by match.ParticipantIndex, cmd match.Command, weights npcmindset.CriterionWeights, random match.Random) (outcome, error) {
	if !inBattleChoice(current) {
		after, err := apply(current, cmd, rand.New(rand.NewPCG(0, 0)))
		if err != nil {
			return outcome{}, err
		}
		if !inBattleChoice(after) {
			return outcomeOf(origin, after, by), nil
		}
	}
	var total outcome
	for range battleSamples {
		// 選択肢どうしを同じ乱数の列で比べるため、乱数の元から種だけ引く
		seed := uint64(random.IntN(1 << 30))
		after, err := apply(current, cmd, rand.New(rand.NewPCG(seed, seed)))
		if err != nil {
			return outcome{}, err
		}
		settled, err := settle(origin, after, by, weights, seed)
		if err != nil {
			return outcome{}, err
		}
		total = total.plus(outcomeOf(origin, settled, by), 1.0/battleSamples)
	}
	return total, nil
}

func inBattleChoice(current match.Match) bool {
	return current.Phase == match.PhaseAttacking || current.Phase == match.PhaseDefending
}

func apply(current match.Match, cmd match.Command, random match.Random) (match.Match, error) {
	events, err := match.Decide(current, cmd, time.Time{}, random)
	if err != nil {
		return match.Match{}, fmt.Errorf("npcdecision: candidate %T is rejected: %w", cmd, err)
	}
	var state match.State = current
	for _, event := range events {
		state, err = match.Evolve(state, event)
		if err != nil {
			return match.Match{}, fmt.Errorf("npcdecision: candidate %T cannot be applied: %w", cmd, err)
		}
	}
	return state.(match.Match), nil
}

// 戦闘のアイテムを選ぶ段階にいる間、選ぶ人の選べるもの（アイテム・援護・出さない）を当て、戦闘が決まるまで進める。
// 自分が選ぶ番なら自分の点が一番よくなるもの、相手が選ぶ番なら自分の点が一番悪くなるものを選ぶ（相手の手札は見えている前提）
func settle(origin, current match.Match, by match.ParticipantIndex, weights npcmindset.CriterionWeights, seed uint64) (match.Match, error) {
	if !inBattleChoice(current) {
		return current, nil
	}
	chooser, _ := current.AwaitingParticipant()
	var best match.Match
	var bestScore float64
	found := false
	for _, option := range battleOptions(current, chooser) {
		after, err := apply(current, option, rand.New(rand.NewPCG(seed, seed)))
		if err != nil {
			return match.Match{}, err
		}
		settled, err := settle(origin, after, by, weights, seed)
		if err != nil {
			return match.Match{}, err
		}
		value := score(settled, outcomeOf(origin, settled, by), weights)
		if !found || (chooser == by && value > bestScore) || (chooser != by && value < bestScore) {
			best, bestScore, found = settled, value, true
		}
	}
	if !found {
		return match.Match{}, fmt.Errorf("npcdecision: no battle options: %w", match.ErrInvalidState)
	}
	return best, nil
}

func battleOptions(current match.Match, by match.ParticipantIndex) []match.Command {
	var options []match.Command
	attacking := current.Phase == match.PhaseAttacking
	for _, handIndex := range current.ItemChoices() {
		if attacking {
			options = append(options, &match.AttackCommand{By: by, ItemHandIndex: &handIndex})
		} else {
			options = append(options, &match.DefendCommand{By: by, ItemHandIndex: &handIndex})
		}
	}
	for _, handIndex := range current.CreatureAsItemChoices() {
		options = append(options, &match.UseCreatureAsItemCommand{By: by, HandIndex: handIndex})
	}
	if attacking {
		return append(options, &match.AttackCommand{By: by})
	}
	return append(options, &match.DefendCommand{By: by})
}

// 分かれ道の行き先ごとに、止まった後の一番よい行動まで進めた点で比べ、一番よい行き先を選ぶ（先の分かれ道も同じく進める）。
// 先読みは止まったマスの行動まで（高い土地に止まりに行く・何度も削ってから取る調整はしない。2026-11-18 オーナー）
func choosePath(current match.Match, by match.ParticipantIndex, weights npcmindset.CriterionWeights, random match.Random) (match.Command, error) {
	var candidates []match.Command
	var scores []float64
	for _, to := range current.PathChoices() {
		cmd := &match.ChoosePathCommand{By: by, To: to}
		value, err := pathValue(current, current, by, cmd, weights, random)
		if err != nil {
			return nil, err
		}
		candidates, scores = append(candidates, cmd), append(scores, value)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("npcdecision: no path choices: %w", match.ErrInvalidState)
	}
	return pickBest(candidates, scores, random), nil
}

// 行き先を当て、止まった後に自分が選ぶ段階なら、その候補の一番よい点を返す。自分の番が終わっていれば、当てた後の状態の点
func pathValue(origin, current match.Match, by match.ParticipantIndex, cmd match.Command, weights npcmindset.CriterionWeights, random match.Random) (float64, error) {
	after, err := apply(current, cmd, rand.New(rand.NewPCG(0, 0)))
	if err != nil {
		return 0, err
	}
	awaiting, ok := after.AwaitingParticipant()
	// 目標の魔力に届いて拠点に入ると、その行き先で対戦が終わる（終わった後も待っている人は返るが、選べる行動は無い）
	if !ok || awaiting != by || !scored(after.Phase) || after.Phase == match.PhaseEnded {
		return score(after, outcomeOf(origin, after, by), weights), nil
	}
	var candidates []match.Command
	if after.Phase == match.PhaseChoosingPath {
		for _, to := range after.PathChoices() {
			candidates = append(candidates, &match.ChoosePathCommand{By: by, To: to})
		}
	} else if candidates, err = candidatesOf(after, by, after.ActionChoices()); err != nil {
		return 0, err
	}
	best := 0.0
	for i, candidate := range candidates {
		var value float64
		if after.Phase == match.PhaseChoosingPath {
			value, err = pathValue(origin, after, by, candidate, weights, random)
		} else {
			var measured outcome
			measured, err = measure(origin, after, by, candidate, weights, random)
			value = score(after, measured, weights)
		}
		if err != nil {
			return 0, err
		}
		if i == 0 || value > best {
			best = value
		}
	}
	return best, nil
}

// 場面の候補を全部並べる。対象のある種類は、対象のすべての組みを並べる
func candidatesOf(current match.Match, by match.ParticipantIndex, actions []matchaction.Action) ([]match.Command, error) {
	var candidates []match.Command
	for _, action := range actions {
		switch action {
		case matchaction.ActionPlaceCreature:
			for _, handIndex := range current.PlaceChoices() {
				candidates = append(candidates, &match.PlaceCreatureCommand{By: by, HandIndex: handIndex})
			}
		case matchaction.ActionInvadeTerritory:
			for _, handIndex := range current.InvadeChoices() {
				candidates = append(candidates, &match.InvadeCommand{By: by, HandIndex: handIndex})
			}
		case matchaction.ActionPayToll:
			candidates = append(candidates, &match.PayTollCommand{By: by})
		case matchaction.ActionLevelUpLand:
			for _, choice := range current.LevelUpChoices() {
				candidates = append(candidates, &match.LevelUpLandCommand{By: by, Position: choice.Position, Level: choice.Level})
			}
		case matchaction.ActionExchangeCreature:
			for _, choice := range current.ExchangeChoices() {
				candidates = append(candidates, &match.ExchangeCreatureCommand{By: by, Position: choice.Position, HandIndex: choice.HandIndex})
			}
		case matchaction.ActionChangeLandElement:
			for _, choice := range current.ElementChangeChoices() {
				candidates = append(candidates, &match.ChangeLandElementCommand{By: by, Position: choice.Position, Element: choice.Element})
			}
		case matchaction.ActionMoveCreature:
			for _, choice := range current.MoveChoices() {
				candidates = append(candidates, &match.MoveCreatureCommand{By: by, From: choice.From, To: choice.To})
			}
		case matchaction.ActionAttackWithItem:
			for _, handIndex := range current.ItemChoices() {
				candidates = append(candidates, &match.AttackCommand{By: by, ItemHandIndex: &handIndex})
			}
		case matchaction.ActionAttackWithoutItem:
			candidates = append(candidates, &match.AttackCommand{By: by})
		case matchaction.ActionDefendWithItem:
			for _, handIndex := range current.ItemChoices() {
				candidates = append(candidates, &match.DefendCommand{By: by, ItemHandIndex: &handIndex})
			}
		case matchaction.ActionDefendWithoutItem:
			candidates = append(candidates, &match.DefendCommand{By: by})
		case matchaction.ActionUseCreatureAsItem:
			for _, handIndex := range current.CreatureAsItemChoices() {
				candidates = append(candidates, &match.UseCreatureAsItemCommand{By: by, HandIndex: handIndex})
			}
		case matchaction.ActionRelinquishTerritory:
			for _, position := range current.RelinquishChoices() {
				candidates = append(candidates, &match.RelinquishTerritoryCommand{By: by, Position: position})
			}
		case matchaction.ActionEndTurn:
			candidates = append(candidates, &match.EndTurnCommand{By: by})
		default:
			return nil, fmt.Errorf("npcdecision: action %q is not scored: %w", action, match.ErrInvalidState)
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("npcdecision: no candidates: %w", match.ErrInvalidState)
	}
	return candidates, nil
}

// 点の一番高い候補のうち、均等に 1 つ選ぶ。底上げはしない
func pickBest(candidates []match.Command, scores []float64, random match.Random) match.Command {
	const tolerance = 1e-9
	top := slices.Max(scores)
	var best []match.Command
	for i, score := range scores {
		if top-score <= tolerance {
			best = append(best, candidates[i])
		}
	}
	return best[random.IntN(len(best))]
}
