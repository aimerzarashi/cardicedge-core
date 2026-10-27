package studiorules

import (
	"errors"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
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
type Rules struct {
	Board Board `json:"board"`
}

// 幅と高さで範囲を分けないのは、縦長・横長のボードを同じ決まりで作れるようにするため
type Board struct {
	MinSize int `json:"min_size"`
	MaxSize int `json:"max_size"`
}

func (b Board) Contains(size int) bool {
	return size >= b.MinSize && size <= b.MaxSize
}

func (b Board) Clamp(size int) int {
	return min(max(size, b.MinSize), b.MaxSize)
}

var ErrInvalidRules = errors.New("studio rules: invalid value")

// 値の形として確かめるのは、下限が 1 以上（0 以下の大きさの格子は作れない）と、上限が下限以上（範囲が空にならない）だけ
func New(board Board) (Rules, error) {
	if board.MinSize < 1 {
		return Rules{}, fmt.Errorf("%w: board.min_size is %d", ErrInvalidRules, board.MinSize)
	}
	if board.MaxSize < board.MinSize {
		return Rules{}, fmt.Errorf("%w: board.max_size %d is below board.min_size %d", ErrInvalidRules, board.MaxSize, board.MinSize)
	}
	return Rules{Board: board}, nil
}

// 作るときの値。値の変更は core#23 で作るまで、これが全体のルールになる。
// 作るとき（とテストの用意）のほかは、この値を直接読まず studio_rules から読む（ここを読むと、値を変えても効かない所ができるため）
func Defaults() Rules {
	return Rules{Board: Board{MinSize: 1, MaxSize: 32}}
}
