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
	GainMagic *GainMagicEffect `json:"gain_magic,omitempty"`
	RollDice  *RollDiceEffect  `json:"roll_dice,omitempty"`
}

// 効果なし（登録の直後の値）
func NoEffect() Effect {
	return Effect{}
}

func (e Effect) IsNone() bool {
	return e.DrawCards == nil && e.GainMagic == nil && e.RollDice == nil
}

// 中身で比べる（種類ごとの値をポインタで持つため == では比べられない）
func (e Effect) Equal(other Effect) bool {
	return equalPointee(e.DrawCards, other.DrawCards) && equalPointee(e.GainMagic, other.GainMagic) && equalPointee(e.RollDice, other.RollDice)
}

func equalPointee[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
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

// 魔力を得る（マナ）。使った人が（周回数 + 1）× PerLap の魔力を得る（周回数はこの対戦で周回を終えた回数。2026-11-10 オーナー）
type GainMagicEffect struct {
	PerLap MagicPerLap `json:"per_lap"`
}

func NewGainMagicEffect(perLap MagicPerLap) Effect {
	return Effect{GainMagic: &GainMagicEffect{PerLap: perLap}}
}

// 周回数 1 つあたりの魔力。1 以上（0 は効果なしと同じため）。上限は持たない
type MagicPerLap int

func NewMagicPerLap(value int) (MagicPerLap, error) {
	if value < 1 {
		return 0, ErrMagicPerLapTooSmall
	}
	return MagicPerLap(value), nil
}

func (m MagicPerLap) Int() int {
	return int(m)
}

// サイコロを複数振る（フライ）。使った人は、このターンのサイコロを Count 個同時に振り、合計の分だけ進む（2026-11-10 オーナー）
type RollDiceEffect struct {
	Count DiceCount `json:"count"`
}

func NewRollDiceEffect(count DiceCount) Effect {
	return Effect{RollDice: &RollDiceEffect{Count: count}}
}

// 振るサイコロの個数。2 以上（1 個はふだんと同じで、効果なしと変わらないため）。上限は持たない
type DiceCount int

func NewDiceCount(value int) (DiceCount, error) {
	if value < 2 {
		return 0, ErrDiceCountTooSmall
	}
	return DiceCount(value), nil
}

func (c DiceCount) Int() int {
	return int(c)
}
