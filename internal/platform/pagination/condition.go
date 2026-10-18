// Relay のカーソルによるページ分け。前向き（first・after）だけを扱う（studio の一覧は前へ進むだけで、後ろ向きは使わないため）
package pagination

import (
	"errors"
)

const (
	MinFirst     = 1
	MaxFirst     = 100
	DefaultFirst = 50
)

var ErrFirstOutOfRange = errors.New("pagination: first out of range")

type Condition[C Cursor] struct {
	First int
	After *C
}

// first・after の両方が誤っていれば、両方のエラーを返す（入力の誤りをまとめて知らせるため。errors.Is で見分ける）
func NewCondition[C Cursor](first *int, after *string) (Condition[C], error) {
	condition := Condition[C]{First: DefaultFirst}
	var errs []error
	if first != nil {
		if *first < MinFirst || *first > MaxFirst {
			errs = append(errs, ErrFirstOutOfRange)
		}
		condition.First = *first
	}
	if after != nil {
		cursor, err := DecodeCursor[C](*after)
		if err != nil {
			errs = append(errs, err)
		}
		condition.After = &cursor
	}
	if len(errs) > 0 {
		return Condition[C]{}, errors.Join(errs...)
	}
	return condition, nil
}

// 次のページがあるかを知るため、1 件多く取る。多く取るのはここだけで決め、BuildConnection が 1 件を落とす
func (c Condition[C]) FetchLimit() int {
	return c.First + 1
}
