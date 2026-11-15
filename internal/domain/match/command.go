package match

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/element"
)

type Command interface {
	isMatchCommand()
}

// 対戦を始める。参加者は部屋の席の順。値が使えるか（持っているデッキか等）は部屋で選んだ時に確かめてあり、ここでは見ない
type StartCommand struct {
	ID           ID
	Participants []Participant
	Board        Board
	Rules        Rules
}

func (*StartCommand) isMatchCommand() {}

// 以下のコマンドは、操作した参加者の番号を持つ。プレイヤーか NPC かは見ず、今の番の人でなければ断る
type DrawCommand struct {
	By ParticipantIndex
}

func (*DrawCommand) isMatchCommand() {}

type DiscardCommand struct {
	By ParticipantIndex
	// 手札の何番目か（0 始まり）
	HandIndex int
}

func (*DiscardCommand) isMatchCommand() {}

type RollCommand struct {
	By ParticipantIndex
}

func (*RollCommand) isMatchCommand() {}

type ChoosePathCommand struct {
	By ParticipantIndex
	To board.Position
}

func (*ChoosePathCommand) isMatchCommand() {}

// 止まった空き地に、手札のクリーチャーを配置する。配置したらそのままターンを終える。配置しないときは EndTurnCommand
type PlaceCreatureCommand struct {
	By ParticipantIndex
	// 手札の何番目か（0 始まり）
	HandIndex int
}

func (*PlaceCreatureCommand) isMatchCommand() {}

// サイコロを振る前に、手札のスペルを使う（1 ターンに 1 枚まで。2026-11-10 オーナー。core#149）。コストを払い、効果を当て、スペルは捨て札に入る
type UseSpellCommand struct {
	By ParticipantIndex
	// 手札の何番目か（0 始まり）
	HandIndex int
	// 対象のプレイヤー（対象のプレイヤーを選ぶスペルだけ。ほかのスペルは nil）
	Target *ParticipantIndex
	// 対象のクリーチャーがいる領地の位置（クリーチャーを選ぶスペルだけ。ほかのスペルは nil）
	TargetPosition *board.Position
}

func (*UseSpellCommand) isMatchCommand() {}

// 止まった他人の領地を、手札のクリーチャーで侵略する。選んだ時点で確定し、コストを払う
type InvadeCommand struct {
	By ParticipantIndex
	// 手札の何番目か（0 始まり）
	HandIndex int
}

func (*InvadeCommand) isMatchCommand() {}

// 侵略を確定した人が、アイテムを選ぶ。出さなければ nil
type AttackCommand struct {
	By            ParticipantIndex
	ItemHandIndex *int
}

func (*AttackCommand) isMatchCommand() {}

// 止まった他人の領地の通行料を払う
type PayTollCommand struct {
	By ParticipantIndex
}

func (*PayTollCommand) isMatchCommand() {}

// 侵略された領地の持ち主が、アイテムを選んで戦闘に進める。出さなければ nil
type DefendCommand struct {
	By            ParticipantIndex
	ItemHandIndex *int
}

func (*DefendCommand) isMatchCommand() {}

// 魔力が負の間に、自分の領地を 1 つ手放す
type RelinquishTerritoryCommand struct {
	By       ParticipantIndex
	Position board.Position
}

func (*RelinquishTerritoryCommand) isMatchCommand() {}

// 通った自分の領地の土地のレベルを Level に上げる（飛ばして上げられる）
type LevelUpLandCommand struct {
	By       ParticipantIndex
	Position board.Position
	Level    int
}

func (*LevelUpLandCommand) isMatchCommand() {}

// 通った自分の領地のクリーチャーを、手札のクリーチャーと入れ替える（HandIndex は手札の何番目か。0 始まり）
type ExchangeCreatureCommand struct {
	By        ParticipantIndex
	Position  board.Position
	HandIndex int
}

func (*ExchangeCreatureCommand) isMatchCommand() {}

// 通った自分の領地のクリーチャーを From から To へ動かす（To が他人の領地なら戦闘）
type MoveCreatureCommand struct {
	By   ParticipantIndex
	From board.Position
	To   board.Position
}

func (*MoveCreatureCommand) isMatchCommand() {}

// 通った自分の領地の土地の属性を Element に変える
type ChangeLandElementCommand struct {
	By       ParticipantIndex
	Position board.Position
	Element  element.Land
}

func (*ChangeLandElementCommand) isMatchCommand() {}

type EndTurnCommand struct {
	By ParticipantIndex
}

func (*EndTurnCommand) isMatchCommand() {}

// 終わった後に、プレイヤーが戦利品のパックを選ぶ。番は無い（全員が同時に選べる）。
// 候補はそのときの公開中のカード（呼ぶ側が読んで渡す。始めるときに一覧を写さないため）
type ChooseRewardPackCommand struct {
	By         ParticipantIndex
	Pack       PackKey
	Candidates []card.Ref
}

func (*ChooseRewardPackCommand) isMatchCommand() {}
