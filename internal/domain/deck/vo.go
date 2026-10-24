package deck

import (
	"errors"
	"strings"
	"unicode/utf8"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

type ID int64

var entityType = domain.NewEntityType("deck")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("deck id must be positive")
	}
	return ID(value), nil
}

// デッキの名前の文字数の上限。プリセットデッキと同じ値だが型を分けるのは、プレイヤーが名前を付けるときの決まりを、
// studio のプリセットデッキと別に変えられるようにするため
const MaxNameLength = 50

type Name string

// 長さを数えるのは前後の空白を除いた後（card.NewName と同じ）
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
