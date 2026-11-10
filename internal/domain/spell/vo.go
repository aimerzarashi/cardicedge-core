package spell

import (
	"errors"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

type ID int64

var entityType = card.SpellEntityType

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("spell id must be positive")
	}
	return ID(value), nil
}

// スペルの効果。種類ごとの型のどれか 1 つだけを持ち、効果の無いスペルはどれも持たない（2026-11-10 オーナー。種類ごとに型を分ける）。
// 種類は、対戦で使う場面を作るスライスごとに足す（core#116）。JSON はタグだけで、持っている種類の名前の下に値を入れる（{"draw_cards":{"count":1}}。無ければ {}）
type Effect struct {
	DrawCards *DrawCardsEffect `json:"draw_cards,omitempty"`
}

// 効果なし（登録の直後の値）
func NoEffect() Effect {
	return Effect{}
}

func (e Effect) IsNone() bool {
	return e.DrawCards == nil
}

// 中身で比べる（種類ごとの値をポインタで持つため == では比べられない）
func (e Effect) Equal(other Effect) bool {
	switch {
	case e.DrawCards != nil && other.DrawCards != nil:
		return *e.DrawCards == *other.DrawCards
	default:
		return e.IsNone() && other.IsNone()
	}
}

// カードを引く（ホープ）。山札の先頭から Count 枚引く
type DrawCardsEffect struct {
	Count DrawCount `json:"count"`
}

func NewDrawCardsEffect(count DrawCount) Effect {
	return Effect{DrawCards: &DrawCardsEffect{Count: count}}
}

// 引く枚数。1 以上（0 枚引く効果は効果なしと同じため）。上限は持たない
type DrawCount int

func NewDrawCount(value int) (DrawCount, error) {
	if value < 1 {
		return 0, ErrDrawCountTooSmall
	}
	return DrawCount(value), nil
}

func (c DrawCount) Int() int {
	return int(c)
}
