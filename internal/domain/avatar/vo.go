package avatar

import (
	"errors"
	"strings"
	"unicode/utf8"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

type ID int64

var entityType = domain.NewEntityType("avatar")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("avatar id must be positive")
	}
	return ID(value), nil
}

// アバターの名前の文字数の上限。ほかのプレイヤーにも見えるので、狭い画面にも収めやすい長さにする（縮めるのは難しいので短めから）。
// デッキの名前と型を分けるのは、見え方の違うそれぞれの決まりを別々に変えられるようにするため
const MaxNameLength = 20

type Name string

// 長さを数えるのは前後の空白を除いた後（deck.NewName と同じ）
func NewName(value string) (Name, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", ErrNameEmpty
	}
	if utf8.RuneCountInString(trimmed) > MaxNameLength {
		return "", ErrNameTooLong
	}
	return Name(trimmed), nil
}

func (n Name) String() string {
	return string(n)
}
