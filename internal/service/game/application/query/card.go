package query

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// 公開 ID に当たるカードが、写した cards に無い（無い・カードでない・準備中で写されていない）
var ErrCardNotFound = errors.New("query: card not found")

// カード 1 枚の、見せるのに要る項目。写した cards の行から作る（公開中と使用不可。使用不可はプレイヤーが持っていることがある）
type Card struct {
	ID   uuid.UUID
	Name string
	Type CardType
	Cost int
	// 希少度（写した cards.metadata の値。normal・strange・rare・extra）
	Rarity string
	// クリーチャーだけの性能（クリーチャー以外は nil）
	Creature *CreatureSpec
	// アイテムだけの性能（アイテム以外は nil）
	Item *ItemSpec
	// スペルだけの性能（スペル以外は nil）
	Spell *SpellSpec
}

// スペルだけの性能。写した cards.metadata.spell の値
type SpellSpec struct {
	Effect SpellEffect
}

// スペルの効果。種類ごとの値のどれか 1 つだけを持ち、どれも無ければ効果なし（studio の readmodel.SpellEffectMetadata と同じ形）
type SpellEffect struct {
	DrawCards    *SpellDrawCards
	GainMagic    *SpellGainMagic
	MultipleDice *SpellMultipleDice
	FixedDice    *SpellFixedDice
	Damage       *SpellDamage
}

type SpellDamage struct {
	Amount int
}

type SpellFixedDice struct {
	Value int
	Turns int
}

type SpellDrawCards struct {
	Count int
}

type SpellGainMagic struct {
	PerLap int
}

type SpellMultipleDice struct {
	Count int
	Turns int
}

// アイテムだけの性能。写した cards.metadata.item の値（分類は weapon・armor・scroll、効果は並びの順）
type ItemSpec struct {
	Category string
	Effects  []Effect
}

// 効果。種類ごとの値のどれか 1 つだけを持つ（studio の readmodel.EffectMetadata と同じ形。アイテムとクリーチャーで共有する。core#178）
type Effect struct {
	// AT の増減の値。持っていなければ nil
	AttackModifier *int
	// HP の増減の値。持っていなければ nil
	HitPointsModifier *int
	// AT の上書きの範囲。持っていなければ nil
	AttackOverride *EffectRange
	// HP の上書きの範囲。持っていなければ nil
	HitPointsOverride *EffectRange
	// AT の倍率。持っていなければ nil
	AttackMultiplier    *EffectAttackMultiplier
	IgnoreTerrainEffect bool
	StrikeFirst         bool
	StrikeLast          bool
	// 受けるダメージを 0 にする。持っていなければ nil
	NullifyReceivedDamage *EffectNullifyReceivedDamage
	// 通行料の倍率（百分率）。持っていなければ nil
	TollMultiplierPercent *int
	ForbidInvasion        bool
	ForbidMovement        bool
}

// 上書きの範囲（固定は最小と最大が同じ）
type EffectRange struct {
	Minimum int
	Maximum int
}

// AT の倍率（百分率）と、発動する制約（すべてを満たすときに発動する）
type EffectAttackMultiplier struct {
	Percent     int
	Constraints []EffectConstraint
}

// 受けるダメージを 0 にする制約（すべてを満たすときに発動する）
type EffectNullifyReceivedDamage struct {
	Constraints []EffectConstraint
}

// 効果の制約。種類ごとの値のどれか 1 つだけを持つ
type EffectConstraint struct {
	// 相手のクリーチャーの属性（合えば満たす属性）。この種類でなければ nil
	OpponentCreatureElements []string
}

// クリーチャーだけの性能。写した cards.metadata.creature の値（属性は fire・earth・water・wind・neutral を火・地・水・風の順に並べたもの）
type CreatureSpec struct {
	Elements              []string
	Attack                int
	HitPoints             int
	ItemRestrictions      []string
	PlacementRestrictions []string
	Effects               []Effect
}

// 写した cards から 1 枚読む。無ければ ErrCardNotFound
type GetCardFunc func(ctx context.Context, id uuid.UUID) (Card, error)

type CardHandler struct {
	get GetCardFunc
}

func NewCardHandler(get GetCardFunc) *CardHandler {
	return &CardHandler{get: get}
}

func (q *CardHandler) Get(ctx context.Context, id uuid.UUID) (Card, error) {
	return q.get(ctx, id)
}
