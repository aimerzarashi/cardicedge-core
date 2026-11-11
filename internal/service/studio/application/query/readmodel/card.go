// read model の状態の型と、イベントから状態を作る Evolve（投影の宣言は internal/service/studio、DB の読み書きは infrastructure/repository）
package readmodel

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/item"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/spell"
)

// cards.card_type の値。並び順を表す（集約の種別の id の大小に並び順の決まりを隠さないため。migrations/0010）。
// 素の数にしないのは、件数などほかの数と取り違えてもコンパイルが通らないように
type CardType int16

const (
	CardTypeCreature CardType = 1
	CardTypeItem     CardType = 2
	CardTypeSpell    CardType = 3
)

// DB の cards.card_type など、外から来た数を種類にする
func NewCardType(value int16) (CardType, error) {
	cardType := CardType(value)
	if cardType < CardTypeCreature || cardType > CardTypeSpell {
		return 0, fmt.Errorf("readmodel: unknown card type: %d", value)
	}
	return cardType, nil
}

// cards.status の値（準備中・公開中・使用不可）。domain の card.Status を使わないのは、read model の型を application 層の
// 外（presentation 層）に見せるため、ドメインの型への依存を持ち込まないように（CardType と同じ）
type CardStatus string

const (
	CardStatusDraft     CardStatus = "draft"
	CardStatusPublished CardStatus = "published"
	CardStatusRetired   CardStatus = "retired"
)

// DB の cards.status など、外から来た値を状態にする
func NewCardStatus(value string) (CardStatus, error) {
	status := CardStatus(value)
	switch status {
	case CardStatusDraft, CardStatusPublished, CardStatusRetired:
		return status, nil
	default:
		return "", fmt.Errorf("readmodel: unknown card status: %q", value)
	}
}

// cards の 1 行。個別の列（CardType・Status・SortName）は並べ替え・絞り込み用、Metadata は表示用。
// 種類は列だけに持つ（並べ替えに必ず要り、読み出しで毎回取るため。表示用にも持つと、同じ事実が 2 か所になる）。
// 状態は絞り込みに使う列と、表示に使う metadata の両方に持つ（0008 の決まり）
type Card struct {
	CardID   uuid.UUID
	CardType CardType
	Status   CardStatus
	SortName string
	Metadata CardMetadata
	// イベントはオペレーターを内部 ID で持つため、内部 ID で持ち、書くときに公開 ID に引き直す（NPCAvatar と同じ）
	RegisteredBy operator.ID
	UpdatedBy    operator.ID
	// 集約が削除された。DB には持たず、投影が行を消すかを決めるのに使う。削除のイベントを足したら Evolve で立てる
	IsDeleted bool
}

// 日時はイベントの発生時刻から作る（DB の現在時刻は使わない）
type CardMetadata struct {
	ID     uuid.UUID  `json:"id"`
	Name   string     `json:"name"`
	Status CardStatus `json:"status"`
	// 3 種類に共通の性能。game には make publish-master がこの JSON をそのまま写し、デッキ・手札の表示と、対戦を始めるときの
	// 性能の写しの元になる（studio と game の約束。core#112）。希少度は domain の card.Rarity の値（normal・strange・rare・extra）
	Cost   int    `json:"cost"`
	Rarity string `json:"rarity"`
	// 種類ごとの性能は、種類の名前の入れ子に置く（フラットに並べると種類をまたいで名前がぶつかり、どの種類の値か読み取れないため。
	// 2026-11-07 オーナー）。クリーチャー以外は持たず、省く
	Creature *CreatureMetadata `json:"creature,omitempty"`
	Item     *ItemMetadata     `json:"item,omitempty"`
	Spell    *SpellMetadata    `json:"spell,omitempty"`
	// 画面が操作（公開する・使用不可にする）の入力に渡す、集約の版
	Revision     int64     `json:"revision"`
	RegisteredAt time.Time `json:"registered_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// 投影では空のまま。書くときに repository が RegisteredBy・UpdatedBy から公開 ID を入れる
	RegisteredBy uuid.UUID `json:"registered_by"`
	UpdatedBy    uuid.UUID `json:"updated_by"`
}

// クリーチャーだけの性能。属性は domain の creature.Element の値（fire・earth・water・wind・neutral）を火・地・水・風の順に並べた配列
type CreatureMetadata struct {
	Elements              []string                  `json:"elements"`
	Attack                int                       `json:"attack"`
	HitPoints             int                       `json:"hit_points"`
	ItemRestrictions      []string                  `json:"item_restrictions"`
	PlacementRestrictions []string                  `json:"placement_restrictions"`
	Abilities             []CreatureAbilityMetadata `json:"abilities"`
}

// アイテムだけの性能。分類は domain の item.Category の値（weapon・armor・scroll）
type ItemMetadata struct {
	Category       string `json:"category"`
	AttackBonus    int    `json:"attack_bonus"`
	HitPointsBonus int    `json:"hit_points_bonus"`
}

// スペルだけの性能
type SpellMetadata struct {
	Effect SpellEffectMetadata `json:"effect"`
}

// スペルの効果。domain の spell.Effect と同じ形（持っている種類の名前の下に値を入れ、効果なしは {}）。種類ごとの型を分けるのも同じ（2026-11-10 オーナー）
type SpellEffectMetadata struct {
	DrawCards *SpellDrawCardsMetadata `json:"draw_cards,omitempty"`
	GainMagic *SpellGainMagicMetadata `json:"gain_magic,omitempty"`
	RollDice  *SpellRollDiceMetadata  `json:"roll_dice,omitempty"`
	// 対象のプレイヤーにかける呪い
	FixedDiceCurse *SpellFixedDiceCurseMetadata `json:"fixed_dice_curse,omitempty"`
	// 対象のクリーチャーにダメージ
	Damage *SpellDamageMetadata `json:"damage,omitempty"`
}

type SpellDamageMetadata struct {
	Amount int `json:"amount"`
}

type SpellFixedDiceCurseMetadata struct {
	Value int `json:"value"`
	Turns int `json:"turns"`
}

type SpellDrawCardsMetadata struct {
	Count int `json:"count"`
}

type SpellGainMagicMetadata struct {
	PerLap int `json:"per_lap"`
}

type SpellRollDiceMetadata struct {
	Count int `json:"count"`
}

// クリーチャーの能力。domain の creature.Ability と同じ形（持っている種類の名前の下に値を置く）
type CreatureAbilityMetadata struct {
	Defensive      *CreatureDefensiveAbilityMetadata      `json:"defensive,omitempty"`
	TollMultiplier *CreatureTollMultiplierAbilityMetadata `json:"toll_multiplier,omitempty"`
}

type CreatureDefensiveAbilityMetadata struct{}

type CreatureTollMultiplierAbilityMetadata struct {
	Percent int `json:"percent"`
}

func creatureAbilitiesMetadata(abilities creature.Abilities) []CreatureAbilityMetadata {
	values := make([]CreatureAbilityMetadata, len(abilities))
	for i, ability := range abilities {
		if ability.IsDefensive() {
			values[i].Defensive = &CreatureDefensiveAbilityMetadata{}
		}
		if ability.TollMultiplier != nil {
			values[i].TollMultiplier = &CreatureTollMultiplierAbilityMetadata{Percent: ability.TollMultiplier.Percent.Int()}
		}
	}
	return values
}

func spellEffectMetadata(effect spell.Effect) SpellEffectMetadata {
	var metadata SpellEffectMetadata
	if effect.DrawCards != nil {
		metadata.DrawCards = &SpellDrawCardsMetadata{Count: effect.DrawCards.Count.Int()}
	}
	if effect.GainMagic != nil {
		metadata.GainMagic = &SpellGainMagicMetadata{PerLap: effect.GainMagic.PerLap.Int()}
	}
	if effect.RollDice != nil {
		metadata.RollDice = &SpellRollDiceMetadata{Count: effect.RollDice.Count.Int()}
	}
	if effect.FixedDiceCurse != nil {
		metadata.FixedDiceCurse = &SpellFixedDiceCurseMetadata{Value: effect.FixedDiceCurse.Value.Int(), Turns: effect.FixedDiceCurse.Turns.Int()}
	}
	if effect.Damage != nil {
		metadata.Damage = &SpellDamageMetadata{Amount: effect.Damage.Amount.Int()}
	}
	return metadata
}

// Creature・Item・Spell のイベントだけを扱い、ほかは状態をそのまま返す
func (state Card) Evolve(event any) Card {
	switch e := event.(type) {
	case *creature.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.CardID = e.PublicID
		state.CardType = CardTypeCreature
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Status = CardStatusDraft
		state.Metadata = CardMetadata{
			ID: e.PublicID, Name: e.Name.String(), Status: CardStatusDraft, Revision: e.GetMeta().AggregateVersion,
			Cost: e.Cost.Int(), Rarity: e.Rarity.String(),
			Creature:     &CreatureMetadata{Elements: e.Elements.Strings(), Attack: e.Attack.Int(), HitPoints: e.HitPoints.Int(), ItemRestrictions: e.ItemRestrictions.Strings(), PlacementRestrictions: e.PlacementRestrictions.Strings(), Abilities: creatureAbilitiesMetadata(e.Abilities)},
			RegisteredAt: occurredAt, UpdatedAt: occurredAt,
		}
		state.RegisteredBy = e.RegisteredBy
		state.UpdatedBy = e.RegisteredBy
	case *item.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.CardID = e.PublicID
		state.CardType = CardTypeItem
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Status = CardStatusDraft
		state.Metadata = CardMetadata{
			ID: e.PublicID, Name: e.Name.String(), Status: CardStatusDraft, Revision: e.GetMeta().AggregateVersion,
			Cost: e.Cost.Int(), Rarity: e.Rarity.String(),
			Item:         &ItemMetadata{Category: e.Category.String(), AttackBonus: e.AttackBonus.Int(), HitPointsBonus: e.HitPointsBonus.Int()},
			RegisteredAt: occurredAt, UpdatedAt: occurredAt,
		}
		state.RegisteredBy = e.RegisteredBy
		state.UpdatedBy = e.RegisteredBy
	case *spell.RegisteredEvent:
		occurredAt := e.GetMeta().OccurredAt
		state.CardID = e.PublicID
		state.CardType = CardTypeSpell
		state.SortName = domain.NormalizeName(e.Name.String())
		state.Status = CardStatusDraft
		state.Metadata = CardMetadata{
			ID: e.PublicID, Name: e.Name.String(), Status: CardStatusDraft, Revision: e.GetMeta().AggregateVersion,
			Cost: e.Cost.Int(), Rarity: e.Rarity.String(),
			Spell:        &SpellMetadata{Effect: spellEffectMetadata(e.Effect)},
			RegisteredAt: occurredAt, UpdatedAt: occurredAt,
		}
		state.RegisteredBy = e.RegisteredBy
		state.UpdatedBy = e.RegisteredBy
	case *creature.PublishedEvent:
		state = state.changeStatus(CardStatusPublished, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.PublishedBy)
	case *creature.RetiredEvent:
		state = state.changeStatus(CardStatusRetired, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RetiredBy)
	case *item.PublishedEvent:
		state = state.changeStatus(CardStatusPublished, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.PublishedBy)
	case *item.RetiredEvent:
		state = state.changeStatus(CardStatusRetired, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RetiredBy)
	case *spell.PublishedEvent:
		state = state.changeStatus(CardStatusPublished, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.PublishedBy)
	case *spell.RetiredEvent:
		state = state.changeStatus(CardStatusRetired, e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.RetiredBy)
	case *creature.CostChangedEvent:
		state.Metadata.Cost = e.Cost.Int()
		state = state.changed(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *item.CostChangedEvent:
		state.Metadata.Cost = e.Cost.Int()
		state = state.changed(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *spell.CostChangedEvent:
		state.Metadata.Cost = e.Cost.Int()
		state = state.changed(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *creature.RarityChangedEvent:
		state.Metadata.Rarity = e.Rarity.String()
		state = state.changed(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *creature.ElementsChangedEvent:
		changed := *state.Metadata.Creature
		changed.Elements = e.Elements.Strings()
		state.Metadata.Creature = &changed
		state = state.changed(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *creature.StatsChangedEvent:
		changed := *state.Metadata.Creature
		changed.Attack = e.Attack.Int()
		changed.HitPoints = e.HitPoints.Int()
		state.Metadata.Creature = &changed
		state = state.changed(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *spell.EffectChangedEvent:
		state.Metadata.Spell = &SpellMetadata{Effect: spellEffectMetadata(e.Effect)}
		state = state.changed(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *creature.AbilitiesChangedEvent:
		changed := *state.Metadata.Creature
		changed.Abilities = creatureAbilitiesMetadata(e.Abilities)
		state.Metadata.Creature = &changed
		state = state.changed(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *creature.ItemRestrictionsChangedEvent:
		changed := *state.Metadata.Creature
		changed.ItemRestrictions = e.ItemRestrictions.Strings()
		state.Metadata.Creature = &changed
		state = state.changed(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *creature.PlacementRestrictionsChangedEvent:
		changed := *state.Metadata.Creature
		changed.PlacementRestrictions = e.PlacementRestrictions.Strings()
		state.Metadata.Creature = &changed
		state = state.changed(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *item.RarityChangedEvent:
		state.Metadata.Rarity = e.Rarity.String()
		state = state.changed(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *item.CategoryChangedEvent:
		changed := *state.Metadata.Item
		changed.Category = e.Category.String()
		state.Metadata.Item = &changed
		state = state.changed(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *item.StatBonusesChangedEvent:
		changed := *state.Metadata.Item
		changed.AttackBonus = e.AttackBonus.Int()
		changed.HitPointsBonus = e.HitPointsBonus.Int()
		state.Metadata.Item = &changed
		state = state.changed(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	case *spell.RarityChangedEvent:
		state.Metadata.Rarity = e.Rarity.String()
		state = state.changed(e.GetMeta().AggregateVersion, e.GetMeta().OccurredAt, e.ChangedBy)
	}
	return state
}

// 3 種類のイベントの EventMeta は型が別（集約の ID の型が違う）なので、使う値（版・発生時刻・操作した人）だけ受け取る
func (state Card) changeStatus(status CardStatus, revision int64, occurredAt time.Time, by operator.ID) Card {
	state.Status = status
	state.Metadata.Status = status
	return state.changed(revision, occurredAt, by)
}

// 変えた操作の版・日時・人を残す（状態・性能の変更で共通）
func (state Card) changed(revision int64, occurredAt time.Time, by operator.ID) Card {
	state.Metadata.Revision = revision
	state.Metadata.UpdatedAt = occurredAt
	state.UpdatedBy = by
	return state
}

func (state Card) IsTerminal() bool {
	return state.IsDeleted
}
