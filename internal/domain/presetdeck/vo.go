package presetdeck

import (
	"errors"
	"strings"
	"unicode/utf8"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

type ID int64

var entityType = domain.NewEntityType("preset_deck")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("grant deck id must be positive")
	}
	return ID(value), nil
}

// プリセットデッキの版（集約の版。1 以上）。配布デッキ・NPC デッキが、どの時点から写したかの記録（Source）に使う
type Revision int64

func NewRevision(value int64) (Revision, error) {
	if value <= 0 {
		return 0, ErrRevisionNotPositive
	}
	return Revision(value), nil
}

func (r Revision) Int64() int64 {
	return int64(r)
}

// カード名と同じ上限にそろえる（画面で同じ幅に収まり、決まりを 2 つ覚えずに済むため）
const MaxNameLength = card.MaxNameLength

type Name string

// 長さを正規化する前の値で数える理由は card.NewName と同じ
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
