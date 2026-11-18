// NPC の判断。対戦の状態と選べるものから、NPC が送るコマンドを 1 つ選ぶ（選ぶ重みは、NPC の参加者に写した思考パターン npcmindset から読む）。
// 決まり（できるか）は match の Decide と選択肢（ActionChoices 等）だけが持ち、ここは選べるものの中から選ぶだけにする
// （決まりを NPC の側に書き写さないため。設計メモ「NPC の考え方」）
package npcdecision

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"time"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/matchaction"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcmindset"
)

// 引く・捨てる・振る前・行き先は、山札の順とサイコロ（隠れた情報）で結果が変わり、後の状態を当てると先に見ることになるので、採点せず
// 種類ごとの重みで選ぶ。それ以外（配置・侵略・攻める・守る・手放す・領地コマンド・終わり）は、候補ごとに採点基準で点を付けて選ぶ（scoreCandidates）。
// 段階で分けるのは、点の付く段階と付かない段階が混ざらないようにするため
func scored(phase match.Phase) bool {
	switch phase {
	case match.PhaseDrawing, match.PhaseDiscarding, match.PhaseRolling, match.PhaseChoosingPath:
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
	if scored(current.Phase) {
		candidates, err := candidatesOf(current, by, actions)
		if err != nil {
			return nil, err
		}
		scores, err := scoreCandidates(current, by, candidates, mindset.CriterionWeights, random)
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

// 候補ごとに、今の状態の写しにコマンドを当てた後の状態で測る（決まりを NPC の側に書き写さないため）。
// match.Evolve は変えるスライスと map を先に Clone してから変えるので、Match の値のまま渡せば元の状態は変わらない（テストで確かめる）。
// Decide に渡す now はイベントの発生時刻にしか使われず、ここで出すイベントは捨てるので、ゼロ値でよい
func scoreCandidates(current match.Match, by match.ParticipantIndex, candidates []match.Command, weights npcmindset.CriterionWeights, random match.Random) ([]float64, error) {
	floor := 0
	for _, territory := range current.Territories {
		floor = max(floor, current.TerritoryToll(territory))
	}
	values := make([][]float64, len(candidates))
	for i, candidate := range candidates {
		samples, err := measure(current, by, candidate, floor, random)
		if err != nil {
			return nil, err
		}
		values[i] = samples
	}
	scores := make([]float64, len(candidates))
	for c, criterion := range npcmindset.Criteria() {
		low, high := values[0][c], values[0][c]
		for _, v := range values {
			low, high = min(low, v[c]), max(high, v[c])
		}
		if high == low {
			continue
		}
		for i, v := range values {
			scores[i] += float64(weights[criterion].Int()) * (v[c] - low) / (high - low)
		}
	}
	return scores, nil
}

// 候補 1 つの基準ごとの値（Criteria の並び）。戦闘になる候補は、戦闘の乱数の違う先読みを battleSamples 回当てて平均する
func measure(current match.Match, by match.ParticipantIndex, cmd match.Command, floor int, random match.Random) ([]float64, error) {
	battling := inBattleChoice(current)
	if !battling {
		after, err := apply(current, cmd, rand.New(rand.NewPCG(0, 0)))
		if err != nil {
			return nil, err
		}
		battling = inBattleChoice(after)
		if !battling {
			return values(after, by, floor, nil), nil
		}
	}
	total := make([]float64, len(npcmindset.Criteria()))
	for range battleSamples {
		// 選択肢どうしを同じ乱数の列で比べるため、乱数の元から種だけ引く
		seed := uint64(random.IntN(1 << 30))
		after, err := apply(current, cmd, rand.New(rand.NewPCG(seed, seed)))
		if err != nil {
			return nil, err
		}
		position := battlePosition(current, after)
		settled, err := settle(after, by, seed)
		if err != nil {
			return nil, err
		}
		for c, v := range values(settled, by, floor, position) {
			total[c] += v / battleSamples
		}
	}
	return total, nil
}

func inBattleChoice(current match.Match) bool {
	return current.Phase == match.PhaseAttacking || current.Phase == match.PhaseDefending
}

// 戦闘の領地。戦闘の入口の状態（今か、コマンドを当てた後）の侵略が指す
func battlePosition(states ...match.Match) *match.Territory {
	for _, state := range states {
		if state.Invasion != nil {
			return &match.Territory{Position: state.Invasion.Position}
		}
	}
	return nil
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
// 自分が選ぶ番なら自分の総魔力の差が一番よくなるもの、相手が選ぶ番なら自分に一番悪くなるものを選ぶ（相手の手札は見えている前提）
func settle(current match.Match, by match.ParticipantIndex, seed uint64) (match.Match, error) {
	if !inBattleChoice(current) {
		return current, nil
	}
	chooser, _ := current.AwaitingParticipant()
	var best match.Match
	var bestLead float64
	found := false
	for _, option := range battleOptions(current, chooser) {
		after, err := apply(current, option, rand.New(rand.NewPCG(seed, seed)))
		if err != nil {
			return match.Match{}, err
		}
		settled, err := settle(after, by, seed)
		if err != nil {
			return match.Match{}, err
		}
		lead := totalMagicLead(settled, by)
		if !found || (chooser == by && lead > bestLead) || (chooser != by && lead < bestLead) {
			best, bestLead, found = settled, lead, true
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

func totalMagicLead(state match.Match, by match.ParticipantIndex) float64 {
	totals := state.TotalMagics()
	best := 0
	first := true
	for i, total := range totals {
		if i == int(by) {
			continue
		}
		if first || total > best {
			best, first = total, false
		}
	}
	return float64(totals[by] - best)
}

// Criteria の並びで、後の状態の基準の値を出す。position が nil なら戦闘ではなく、battle_hold は 0
func values(state match.Match, by match.ParticipantIndex, floor int, position *match.Territory) []float64 {
	hold := 0.0
	if position != nil {
		if territory, ok := state.TerritoryAt(position.Position); ok {
			if owner, ok := state.TerritoryOwner(territory); ok && owner == by {
				hold = 1
			}
		}
	}
	result := make([]float64, len(npcmindset.Criteria()))
	for c, criterion := range npcmindset.Criteria() {
		switch criterion {
		case npcmindset.CriterionTotalMagicLead:
			result[c] = totalMagicLead(state, by)
		case npcmindset.CriterionTerritoryCount:
			result[c] = float64(state.TerritoryCount(by))
		case npcmindset.CriterionBattleHold:
			result[c] = hold
		case npcmindset.CriterionMagicReserve:
			result[c] = float64(min(state.Participants[by].Magic, floor))
		}
	}
	return result
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
