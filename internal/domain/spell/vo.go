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
// 種類は、対戦で使う場面を作るスライスごとに足す（core#116）。JSON はタグだけで、持っている種類の名前の下に値を入れる（{"draw_cards":{"count":1}}。無ければ {}）。
// スペルは使ったときに発動する。ターンをまたいで効き続ける効果（サイコロを複数振る・サイコロの目を固定する）も、使ったときに対象にかかる。
// 「呪い」という言葉は使わない（2026-11-13 オーナー。core#173）
type Effect struct {
	DrawCards    *DrawCardsSpellEffect    `json:"draw_cards,omitempty"`
	GainMagic    *GainMagicSpellEffect    `json:"gain_magic,omitempty"`
	MultipleDice *MultipleDiceSpellEffect `json:"multiple_dice,omitempty"`
	FixedDice    *FixedDiceSpellEffect    `json:"fixed_dice,omitempty"`
	Damage       *DamageSpellEffect       `json:"damage,omitempty"`
}

// 効果なし（登録の直後の値）
func NoEffect() Effect {
	return Effect{}
}

func (e Effect) IsNone() bool {
	return e.DrawCards == nil && e.GainMagic == nil && e.MultipleDice == nil && e.FixedDice == nil && e.Damage == nil
}

// 対象のプレイヤー（自分を含む全員）を選ぶ効果か
func (e Effect) TargetsPlayer() bool {
	return e.MultipleDice != nil || e.FixedDice != nil
}

// 対象のクリーチャー（自分の領地を含む、盤面の領地のクリーチャー。2026-11-11 オーナー）を選ぶ効果か
func (e Effect) TargetsCreature() bool {
	return e.Damage != nil
}

// 中身で比べる（種類ごとの値をポインタで持つため == では比べられない）
func (e Effect) Equal(other Effect) bool {
	return equalPointee(e.DrawCards, other.DrawCards) && equalPointee(e.GainMagic, other.GainMagic) && equalPointee(e.MultipleDice, other.MultipleDice) &&
		equalPointee(e.FixedDice, other.FixedDice) && equalPointee(e.Damage, other.Damage)
}

func equalPointee[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// カードを引く（ホープ）。山札の先頭から Count 枚引く
type DrawCardsSpellEffect struct {
	Count DrawCount `json:"count"`
}

func NewDrawCardsSpellEffect(count DrawCount) Effect {
	return Effect{DrawCards: &DrawCardsSpellEffect{Count: count}}
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
type GainMagicSpellEffect struct {
	PerLap MagicPerLap `json:"per_lap"`
}

func NewGainMagicSpellEffect(perLap MagicPerLap) Effect {
	return Effect{GainMagic: &GainMagicSpellEffect{PerLap: perLap}}
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

// サイコロを複数振る（フライ）。対象のプレイヤーが振るとき、Count 個を同時に振り、合計の分だけ進む。Turns は効き続けるターン数（2026-11-13 オーナー。core#173）
type MultipleDiceSpellEffect struct {
	Count DiceCount   `json:"count"`
	Turns EffectTurns `json:"turns"`
}

func NewMultipleDiceSpellEffect(count DiceCount, turns EffectTurns) Effect {
	return Effect{MultipleDice: &MultipleDiceSpellEffect{Count: count, Turns: turns}}
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

// サイコロの目を固定する（ホーリーワード）。対象のプレイヤーが振るとき、目は Value になる。Turns は効き続けるターン数
// （かかった人が自分のターンを終えるたびに 1 減り、0 で終わる。終わる条件は今はターン数だけで、ほかの条件は要るときに足す。2026-11-10 オーナー）
type FixedDiceSpellEffect struct {
	Value DiceFace    `json:"value"`
	Turns EffectTurns `json:"turns"`
}

func NewFixedDiceSpellEffect(value DiceFace, turns EffectTurns) Effect {
	return Effect{FixedDice: &FixedDiceSpellEffect{Value: value, Turns: turns}}
}

// 固定するサイコロの目。1 以上（ボードのサイコロの目の範囲は確かめない。範囲はボードごとに違い、マスタの値は対戦の外で決まるため）
type DiceFace int

func NewDiceFace(value int) (DiceFace, error) {
	if value < 1 {
		return 0, ErrDiceFaceTooSmall
	}
	return DiceFace(value), nil
}

func (f DiceFace) Int() int {
	return int(f)
}

// ターンをまたいで効き続ける効果のターン数。1 以上
type EffectTurns int

func NewEffectTurns(value int) (EffectTurns, error) {
	if value < 1 {
		return 0, ErrEffectTurnsTooSmall
	}
	return EffectTurns(value), nil
}

func (t EffectTurns) Int() int {
	return int(t)
}

// 対象のクリーチャーにダメージ（マジックボルト。自分のクリーチャーにも使える）。今の HP から Amount を引き、0 になれば破壊する（2026-11-10 オーナー）
type DamageSpellEffect struct {
	Amount DamageAmount `json:"amount"`
}

func NewDamageSpellEffect(amount DamageAmount) Effect {
	return Effect{Damage: &DamageSpellEffect{Amount: amount}}
}

// ダメージの値。1 以上（0 は効果なしと同じため）。上限は持たない
type DamageAmount int

func NewDamageAmount(value int) (DamageAmount, error) {
	if value < 1 {
		return 0, ErrDamageAmountTooSmall
	}
	return DamageAmount(value), nil
}

func (a DamageAmount) Int() int {
	return int(a)
}
