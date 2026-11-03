package unlock

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

type Command interface {
	isUnlockCommand()
}

type CreateCommand struct {
	ID     ID
	Player player.ID
}

// 解放するボード。すでに解放しているものは Decide が飛ばす。解放の種類を足すときは、種類ごとのコマンドを足し、
// 複数の種類を同時に解放するコマンドは、種類ごとのイベントを 1 つずつ出す
type UnlockBoardsCommand struct {
	Boards []board.ID
}

func (*CreateCommand) isUnlockCommand()       {}
func (*UnlockBoardsCommand) isUnlockCommand() {}
