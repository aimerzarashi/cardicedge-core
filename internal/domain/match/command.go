package match

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
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
type DeployCreatureCommand struct {
	By ParticipantIndex
	// 手札の何番目か（0 始まり）
	HandIndex int
}

func (*DeployCreatureCommand) isMatchCommand() {}

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
type LevelUpTerritoryCommand struct {
	By       ParticipantIndex
	Position board.Position
	Level    int
}

func (*LevelUpTerritoryCommand) isMatchCommand() {}

// 領地コマンドを使わず、止まったマスの行動に進む
type SkipTerritoryCommandCommand struct {
	By ParticipantIndex
}

func (*SkipTerritoryCommandCommand) isMatchCommand() {}

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
