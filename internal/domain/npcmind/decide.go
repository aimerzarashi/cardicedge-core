// NPC の思考パターン（考え方）。対戦の状態と選べるものから、NPC が送るコマンドを 1 つ選ぶ。
// 決まり（できるか）は match の Decide と選択肢（ActionChoices 等）だけが持ち、ここは選べるものの中から選ぶだけにする
// （決まりを NPC の側に書き写さないため。設計メモ「NPC の考え方」）
package npcmind

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
)

// 操作を待っている参加者が今選べる行動の種類から、重みに比例して 1 つを選び、その種類の対象は均等に選ぶ（2026-11-17 オーナー。
// core#179・core#187）。選べるものを全部 1 列に並べて引かないのは、候補の多い種類（レベルアップ・移動等）ばかりが選ばれないようにするため。
// 重みがどれも同じなら完全ランダム（DefaultWeights）
func Choose(current match.Match, by match.ParticipantIndex, weights Weights, random match.Random) (match.Command, error) {
	actions := current.ActionChoices()
	if len(actions) == 0 {
		return nil, fmt.Errorf("npcmind: no action choices: %w", match.ErrInvalidState)
	}
	action := chooseAction(actions, weights, random)
	cmd, ok := command(current, by, action, random)
	if !ok {
		return nil, fmt.Errorf("npcmind: no targets for action %q: %w", action, match.ErrInvalidState)
	}
	return cmd, nil
}

// 重みの合計から 1 つ引き、並びの順に重みを積んで当たった種類を選ぶ。選べる種類の重みがすべて 0 なら均等に選ぶ
// （通行料を払うしか選べない場面で払うの重みが 0 でも、止まらないようにするため）
func chooseAction(actions []match.Action, weights Weights, random match.Random) match.Action {
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
func command(current match.Match, by match.ParticipantIndex, action match.Action, random match.Random) (match.Command, bool) {
	switch action {
	case match.ActionDrawCard:
		return &match.DrawCommand{By: by}, true
	case match.ActionDiscardCard:
		// 捨てる手札には選択肢の関数が無い（手札のどれでも捨てられる）
		hand := current.Participants[by].Hand
		if len(hand) == 0 {
			return nil, false
		}
		return &match.DiscardCommand{By: by, HandIndex: random.IntN(len(hand))}, true
	case match.ActionUseSpell:
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
	case match.ActionRollDice:
		return &match.RollCommand{By: by}, true
	case match.ActionChoosePath:
		to, ok := pick(random, current.PathChoices())
		return &match.ChoosePathCommand{By: by, To: to}, ok
	case match.ActionPlaceCreature:
		handIndex, ok := pick(random, current.PlaceChoices())
		return &match.PlaceCreatureCommand{By: by, HandIndex: handIndex}, ok
	case match.ActionInvadeTerritory:
		handIndex, ok := pick(random, current.InvadeChoices())
		return &match.InvadeCommand{By: by, HandIndex: handIndex}, ok
	case match.ActionPayToll:
		return &match.PayTollCommand{By: by}, true
	case match.ActionLevelUpLand:
		choice, ok := pick(random, current.LevelUpChoices())
		return &match.LevelUpLandCommand{By: by, Position: choice.Position, Level: choice.Level}, ok
	case match.ActionExchangeCreature:
		choice, ok := pick(random, current.ExchangeChoices())
		return &match.ExchangeCreatureCommand{By: by, Position: choice.Position, HandIndex: choice.HandIndex}, ok
	case match.ActionChangeLandElement:
		choice, ok := pick(random, current.ElementChangeChoices())
		return &match.ChangeLandElementCommand{By: by, Position: choice.Position, Element: choice.Element}, ok
	case match.ActionMoveCreature:
		choice, ok := pick(random, current.MoveChoices())
		return &match.MoveCreatureCommand{By: by, From: choice.From, To: choice.To}, ok
	case match.ActionAttackWithItem:
		handIndex, ok := pick(random, current.ItemChoices())
		return &match.AttackCommand{By: by, ItemHandIndex: &handIndex}, ok
	case match.ActionAttackWithoutItem:
		return &match.AttackCommand{By: by}, true
	case match.ActionDefendWithItem:
		handIndex, ok := pick(random, current.ItemChoices())
		return &match.DefendCommand{By: by, ItemHandIndex: &handIndex}, ok
	case match.ActionDefendWithoutItem:
		return &match.DefendCommand{By: by}, true
	case match.ActionUseCreatureAsItem:
		handIndex, ok := pick(random, current.CreatureAsItemChoices())
		return &match.UseCreatureAsItemCommand{By: by, HandIndex: handIndex}, ok
	case match.ActionRelinquishTerritory:
		position, ok := pick(random, current.RelinquishChoices())
		return &match.RelinquishTerritoryCommand{By: by, Position: position}, ok
	case match.ActionEndTurn:
		return &match.EndTurnCommand{By: by}, true
	default:
		return nil, false
	}
}
