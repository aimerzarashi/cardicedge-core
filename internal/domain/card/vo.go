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

func (n Name) String() string {
	return string(n)
}

// 重複の判定と、一覧の並び・検索にだけ使い、表示には使わない（全角「ＡＢＣ」と半角「ABC」、「Apple」と「apple」を
// 同じ名前として扱うため）。検索の入力は空や上限超えもあり得るため、Name のメソッドにせず文字列を受け取る。
// 小文字にするのを NFKC の後にするのは、NFKC が「Ⅻ」を「XII」にするように大文字を作ることがあるため
func NormalizeName(value string) string {
	return strings.ToLower(norm.NFKC.String(value))
}
