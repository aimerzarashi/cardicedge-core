package card

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const MaxNameLength = 50

// Creature・Item・Spell で共有する。カードという集約は作らず、名前の重複だけを種類をまたいで防ぐ
type Name string

// 長さは正規化する前の値（表示する値）で数える。NFKC は「㍿」を「株式会社」にするように長くなる文字があり、
// 正規化した値で数えると、見た目の文字数と上限が食い違うため
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

// Creature・Item・Spell で共有する、カードの状態。準備中は自由に変えられてデッキに入れられず、公開中はデッキに入れられ、
// 使用不可は新しくデッキに入れられない（プレイヤーが持っているカードは使い続けられる）。
// 公開中 → 準備中に戻せないのは、デッキに入れた・game に写した中身が途中で変わるため。変えたいなら使用不可にして新しく作る
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusRetired   Status = "retired"
)

// DB の cards.status など、外から来た値を状態にする
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

// 使用不可にした後の状態と、状態が変わるか。一度も公開していない準備中のカードは、使用不可（使えなくした）ではなく
// 準備中のままなので、使用不可にできない。使用不可にもう一度しても変わらない
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
