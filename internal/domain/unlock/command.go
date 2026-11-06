package unlock

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

type Command interface {
	isUnlockCommand()
}

type CreateCommand struct {
	ID     ID
	Player player.ID
}

// 解放するもの。種類ごとに渡し、すでに解放しているものは Decide が飛ばす。コマンドは種類をまとめ（配布の流れが、ボードと NPC を
// 1 回で解放できるように）、イベントは種類ごとに分ける。解放の種類を足すときは、このコマンドに項目を、種類ごとのイベントを足す
type UnlockCommand struct {
	Boards     []board.ID
	NPCAvatars []npcavatar.ID
}

func (*CreateCommand) isUnlockCommand() {}
func (*UnlockCommand) isUnlockCommand() {}
