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
	DrawCards *SpellDrawCards
	GainMagic *SpellGainMagic
	RollDice  *SpellRollDice
	// 対象のプレイヤーにかける呪い
	FixedDiceCurse *SpellFixedDiceCurse
	// 対象のクリーチャーにダメージ
	Damage *SpellDamage
}

type SpellDamage struct {
	Amount int
}

type SpellFixedDiceCurse struct {
	Value int
	Turns int
}

type SpellDrawCards struct {
	Count int
}

type SpellGainMagic struct {
	PerLap int
}

type SpellRollDice struct {
	Count int
}

// アイテムだけの性能（写した cards.metadata.item の値）
type ItemSpec struct {
	Category          string
	AttackModifier    int
	HitPointsModifier int
}

// クリーチャーだけの性能（写した cards.metadata.creature の値）
type CreatureSpec struct {
	Elements              []string
	Attack                int
	HitPoints             int
	ItemRestrictions      []string
	PlacementRestrictions []string
	Abilities             []CreatureAbility
}

// カードの性能を、写した cards から 1 回で読む。写していないカードはエラー（デッキのカードは必ず写してあるため）
type GetCardSpecsFunc func(ctx context.Context, refs []card.Ref) (map[card.Ref]CardSpec, error)

// 戦利品の候補にするカード。そのときの公開中のカードすべて（始めるときに一覧を写さず、引くときに読む）。
// 使用不可のカードは含めない（game の cards には使用不可のカードも写してあるが、戦利品にはしない）
type GetPublishedCardsFunc func(ctx context.Context) ([]card.Ref, error)

// クリーチャーの能力の写し。種類ごとの値のどれか 1 つだけを持つ
type CreatureAbility struct {
	Defensive bool
	// 通行料の倍率（百分率）。持っていなければ nil
	TollMultiplierPercent *int
	FirstStrike           bool
	LastStrike            bool
	// 戦闘中の増減。持っていなければ nil
	BattleModifier *CreatureBattleModifier
	// 戦闘中のランダム。持っていなければ nil
	BattleRandomStats *CreatureBattleRandomStats
}

// 戦闘中の AT・HP の範囲（引く値の最小と最大）
type CreatureBattleRandomStats struct {
	Minimum int
	Maximum int
}

// 戦闘中の AT・HP の増減（負も 0 もある）
type CreatureBattleModifier struct {
	AttackModifier    int
	HitPointsModifier int
}
