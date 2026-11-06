package studiorules

import (
	"errors"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
)

type ID int64

var entityType = domain.NewEntityType("studio_rules")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("studio rules id must be positive")
	}
	return ID(value), nil
}

// studio でマスタを作るときの決まりの値。何の決まりかで分ける（gamerules.Rules と同じ）。
// game でも使う決まり（カード名の長さ等）は、game の画面に響くため入れない
// 範囲の型と「上限 ≥ 下限」の決まりは board.SizeRange が持つ。Rules は部品を持つだけなので生成関数は無い
type Rules struct {
	Board board.SizeRange `json:"board"`
}

// 作るときの値。値の変更は core#23 で作るまで、これが全体のルールになる。
// 作るとき（とテストの用意）のほかは、この値を直接読まず studio_rules から読む（ここを読むと、値を変えても効かない所ができるため）
func Defaults() Rules {
	// 定数で失敗しないため、呼ぶ側に error を返さず panic にする
	minSize, err := board.NewMinSize(1)
	if err != nil {
		panic(err)
	}
	maxSize, err := board.NewMaxSize(32)
	if err != nil {
		panic(err)
	}
	boardRange, err := board.NewSizeRange(minSize, maxSize)
	if err != nil {
		panic(err)
	}
	return Rules{Board: boardRange}
}
