package card

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
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

func (n Name) String() string { return string(n) }

// 重複の判定にだけ使い、表示には使わない（全角「ＡＢＣ」と半角「ABC」を同じ名前として扱うため）
func (n Name) Normalized() string { return norm.NFKC.String(string(n)) }
