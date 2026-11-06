package room

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

type Command interface {
	isRoomCommand()
}

// 部屋を作る。作った人が持ち主で、プレイヤーの席に座る。席の並びを決めるまでは席が 0 の部屋
type CreateCommand struct {
	ID    ID
	Owner player.ID
	Name  Name
}

// 席の並びを決める（公開前だけ）。席の並びを変えたら、席ごとの選択はすべて外す
type ChangeSeatsCommand struct {
	ChangedBy player.ID
	Seats     SeatKinds
}

// 対戦ルールを決める（公開前、または公開前に決めていなければ準備中）
type ChangeMatchRulesCommand struct {
	ChangedBy  player.ID
	MatchRules MatchRules
}

// 席のアバターとデッキを選ぶ（公開前、または公開前に決めていなければ準備中）
type ChangeSeatChoiceCommand struct {
	ChangedBy player.ID
	Seat      SeatIndex
	Choice    SeatChoice
}

type PublishCommand struct {
	PublishedBy player.ID
}

// 最後の準備完了なら対戦が始まる。対戦の ID は、Room.StartsMatchOnReady が true のときに呼ぶ側が取って入れる
// （ID を取るのは DB の仕事で、Decide の中ではできないため。最後でなければ 0 のまま）
type DeclareReadyCommand struct {
	Player player.ID
	Match  match.ID
}

// 名前を変える（作った人だけ。閉じる前まで）。名前は対戦の条件ではないので、公開の後にも変えられる
type RenameCommand struct {
	RenamedBy player.ID
	Name      Name
}

// 部屋の今の対戦が終わった（全員が戦利品を受け取り終えた）ことを知らせ、次を決める状態に進める。
// 対戦中でなければ、またはその部屋の今の対戦でなければ何もしない（知らせを受けるポリシーは二度動くことがあるため）
type FinishMatchCommand struct {
	Match match.ID
}

// 対戦の後に、続けるか終えるかを意思表示する（1 人 1 回。変え直しは無い）。誰かが終えるなら休止中に、全員が続けるなら準備中に戻る
type DeclareNextCommand struct {
	Player   player.ID
	Continue bool
}

// 休止中の部屋を再開し、準備中に戻す（作った人だけ。閉じる・対戦ルールを決めるのと同じ人）
type ResumeCommand struct {
	ResumedBy player.ID
}

type CloseCommand struct {
	ClosedBy player.ID
}

func (*CreateCommand) isRoomCommand()           {}
func (*ChangeSeatsCommand) isRoomCommand()      {}
func (*ChangeMatchRulesCommand) isRoomCommand() {}
func (*ChangeSeatChoiceCommand) isRoomCommand() {}
func (*PublishCommand) isRoomCommand()          {}
func (*DeclareReadyCommand) isRoomCommand()     {}
func (*FinishMatchCommand) isRoomCommand()      {}
func (*DeclareNextCommand) isRoomCommand()      {}
func (*ResumeCommand) isRoomCommand()           {}
func (*CloseCommand) isRoomCommand()            {}
func (*RenameCommand) isRoomCommand()           {}
