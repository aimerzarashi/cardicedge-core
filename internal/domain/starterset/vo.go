package starterset

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
)

type ID int64

var entityType = domain.NewEntityType("starter_set")

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("starter set id must be positive")
	}
	return ID(value), nil
}

// NPC のアバター（npcavatar.MaxNameLength）と今は同じ長さだが、参照せずに値を持つ。
// どちらかだけ変えたくなったときに分けられなくなるため
const MaxNameLength = 20

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

// NPC のアバターの状態（npcavatar.Status）と移り方は同じだが、型は共有しない。片方だけ移り方を変えたくなったときに
// 分けられなくなるため。準備中は名前・配るデッキ・解放するボードを自由に変えられる。公開中 → 準備中に戻せないのは、
// 配っている間に中身が変わるため（変えたいときは使用不可にして新しく作る）
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusRetired   Status = "retired"
)

func NewStatus(value string) (Status, error) {
	status := Status(value)
	switch status {
	case StatusDraft, StatusPublished, StatusRetired:
		return status, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrStatusUnknown, value)
	}
}

func (s Status) String() string {
	return string(s)
}

// 公開した後の状態と、状態が変わるか。準備中・使用不可からは公開中にでき、公開中にもう一度公開しても変わらない
func (s Status) Publish() (Status, bool, error) {
	switch s {
	case StatusDraft, StatusRetired:
		return StatusPublished, true, nil
	case StatusPublished:
		return StatusPublished, false, nil
	default:
		return s, false, ErrStatusUnknown
	}
}

// 使用不可にした後の状態と、状態が変わるか。一度も公開していない準備中は使用不可にできない。使用不可にもう一度しても変わらない
func (s Status) Retire() (Status, bool, error) {
	switch s {
	case StatusPublished:
		return StatusRetired, true, nil
	case StatusRetired:
		return StatusRetired, false, nil
	case StatusDraft:
		return s, false, ErrInvalidStatusTransition
	default:
		return s, false, ErrStatusUnknown
	}
}
