package matchaction

import "fmt"

// 今選べる行動の種類。対象（手札・領地・行き先）は持たず、種類ごとの選択肢（PlaceChoices 等）から選ぶ。
// 種類は操作（Command）と 1 対 1 にし、攻める・守るのアイテムを出す・出さないも分ける（画面と NPC が段階を見ずに、どの操作を送るかを決められるようにするため。2026-11-17 オーナー。core#186）
type Action string

const (
	ActionDrawCard            Action = "draw_card"
	ActionDiscardCard         Action = "discard_card"
	ActionUseSpell            Action = "use_spell"
	ActionRollDice            Action = "roll_dice"
	ActionChoosePath          Action = "choose_path"
	ActionPlaceCreature       Action = "place_creature"
	ActionInvadeTerritory     Action = "invade_territory"
	ActionPayToll             Action = "pay_toll"
	ActionLevelUpLand         Action = "level_up_land"
	ActionExchangeCreature    Action = "exchange_creature"
	ActionChangeLandElement   Action = "change_land_element"
	ActionMoveCreature        Action = "move_creature"
	ActionAttackWithItem      Action = "attack_with_item"
	ActionAttackWithoutItem   Action = "attack_without_item"
	ActionDefendWithItem      Action = "defend_with_item"
	ActionDefendWithoutItem   Action = "defend_without_item"
	ActionUseCreatureAsItem   Action = "use_creature_as_item"
	ActionRelinquishTerritory Action = "relinquish_territory"
	ActionEndTurn             Action = "end_turn"
)

// 行動の種類のすべて（ActionChoices が返しうるもの。種類ごとに値を持たせる側（NPC の思考パターンの重み等）が、足し忘れずに並べるため）
func Actions() []Action {
	return []Action{
		ActionDrawCard, ActionDiscardCard, ActionUseSpell, ActionRollDice, ActionChoosePath, ActionPlaceCreature,
		ActionInvadeTerritory, ActionPayToll, ActionLevelUpLand, ActionExchangeCreature, ActionChangeLandElement,
		ActionMoveCreature, ActionAttackWithItem, ActionAttackWithoutItem, ActionDefendWithItem, ActionDefendWithoutItem,
		ActionUseCreatureAsItem, ActionRelinquishTerritory, ActionEndTurn,
	}
}

// DB や GraphQL の入力等、外から来た値を行動の種類にする。種類のすべて（Actions）にない値は誤り
func NewAction(value string) (Action, error) {
	action := Action(value)
	for _, known := range Actions() {
		if action == known {
			return action, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrUnknownAction, value)
}
