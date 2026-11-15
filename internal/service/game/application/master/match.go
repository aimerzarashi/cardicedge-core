package master

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/match"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
)

// 対戦を始めるときに写す、ボード・デッキの中身。部屋が選んだ時点のものではなく、始める時点で game に写してあるものを読む。
// 選んだ後に使用不可になったものも読めなければならない（対戦は始めた時点の写しで進むため）ので、公開中かは見ない。
// 写していないもの（行・カードの内部 ID が無い）は、待てば直る失敗として返す

// ボードの写し（大きさ・マス・つながり・対）
type GetMatchBoardFunc func(ctx context.Context, id board.ID) (match.Board, error)

// プレイヤーのデッキのカード。枚数は DB の値のまま持ち、ルールの上限で確かめるのは呼ぶ側
type GetPlayerDeckCardsFunc func(ctx context.Context, id deck.ID) ([]DeckCard, error)

// NPC のデッキのカード（プレイヤーのデッキと同じ）
type GetNPCDeckCardsFunc func(ctx context.Context, id npcdeck.ID) ([]DeckCard, error)

// 対戦を始めるときに写すカードの性能（写した cards.metadata の値。確かめるのは呼ぶ側で、domain の値にするとき）
type CardSpec struct {
	Cost   int
	Rarity string
	// クリーチャーだけが持つ（クリーチャー以外は nil）
	Creature *CreatureSpec
	// アイテムだけが持つ（アイテム以外は nil）
	Item *ItemSpec
	// スペルだけが持つ（スペル以外は nil）
	Spell *SpellSpec
}

// スペルだけの性能（写した cards.metadata.spell の値）。効果は種類ごとの値のどれか 1 つで、どれも無ければ効果なし
type SpellSpec struct {
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

// アイテムだけの性能（写した cards.metadata.item の値）
type ItemSpec struct {
	Category string
	Effects  []Effect
}

// 効果の写し。種類ごとの値のどれか 1 つだけを持つ（query.Effect と同じ形。アイテムとクリーチャーで共有する。core#178）
type Effect struct {
	AttackModifier        *int
	HitPointsModifier     *int
	AttackOverride        *EffectRange
	HitPointsOverride     *EffectRange
	AttackMultiplier      *EffectAttackMultiplier
	IgnoreTerrainEffect   bool
	StrikeFirst           bool
	StrikeLast            bool
	NullifyReceivedDamage *EffectNullifyReceivedDamage
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

// クリーチャーだけの性能（写した cards.metadata.creature の値）
type CreatureSpec struct {
	Elements              []string
	Attack                int
	HitPoints             int
	ItemRestrictions      []string
	PlacementRestrictions []string
	Effects               []Effect
}

// カードの性能を、写した cards から 1 回で読む。写していないカードはエラー（デッキのカードは必ず写してあるため）
type GetCardSpecsFunc func(ctx context.Context, refs []card.Ref) (map[card.Ref]CardSpec, error)

// 戦利品の候補にするカード。そのときの公開中のカードすべて（始めるときに一覧を写さず、引くときに読む）。
// 使用不可のカードは含めない（game の cards には使用不可のカードも写してあるが、戦利品にはしない）
type GetPublishedCardsFunc func(ctx context.Context) ([]card.Ref, error)
