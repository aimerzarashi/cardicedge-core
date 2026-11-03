package unlock

import (
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

type State interface {
	isUnlockState()
}

type UncreatedUnlock struct{}

func (UncreatedUnlock) isUnlockState() {}

// 解放したものを種類ごとに持つ（いまはボードだけ）。Decide が「すでに解放しているものは飛ばす」のに使う。
// 持ち主は、配布の流れが unlock のイベントから配布を引くためにコマンドが読む
type Unlock struct {
	ID      ID
	Version int64
	Player  player.ID
	// 解放した順
	Boards []board.ID
}

func (Unlock) isUnlockState() {}

func InitialState() State {
	return UncreatedUnlock{}
}
