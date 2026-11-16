package creature

import (
	"errors"
	"fmt"
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/effect"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/element"
)

type ID int64

var entityType = card.CreatureEntityType

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("creature id must be positive")
	}
	return ID(value), nil
}

// 保存の並び（火・地・水・風）。複数持つときはこの順にそろえ、入力の並びが違うだけの同じ集合を別の値にしない
var canonicalElements = []element.Creature{element.CreatureFire, element.CreatureEarth, element.CreatureWater, element.CreatureWind}

// クリーチャーが持つ属性の集合。無だけを単独で持つか、火・地・水・風から 1〜4 つ（重複なし）を持つ。
// 並びは火・地・水・風に固定する。値の比較は Equal（スライスなので == は使えない）
type Elements []element.Creature

// 登録の直後の属性（無。2026-11-07 オーナー）。登録のイベントが値を明示して持つので、変えても過去のカードは変わらない。
// 共有のスライスを変えられないよう、呼ぶたびに作る
func DefaultElements() Elements {
	return Elements{element.CreatureNeutral}
}

func NewElements(values []string) (Elements, error) {
	if len(values) == 0 {
		return nil, ErrElementsEmpty
	}
	selected := map[element.Creature]bool{}
	for _, value := range values {
		creatureElement, err := element.NewCreature(value)
		if err != nil {
			return nil, fmt.Errorf("%w: %q", ErrElementUnknown, value)
		}
		if selected[creatureElement] {
			return nil, fmt.Errorf("%w: %q", ErrElementsDuplicated, value)
		}
		selected[creatureElement] = true
	}
	if selected[element.CreatureNeutral] {
		if len(values) > 1 {
			return nil, ErrElementsNeutralMixed
		}
		return Elements{element.CreatureNeutral}, nil
	}
	elements := make(Elements, 0, len(values))
	for _, creatureElement := range canonicalElements {
		if selected[creatureElement] {
			elements = append(elements, creatureElement)
		}
	}
	return elements, nil
}

func (e Elements) Strings() []string {
	values := make([]string, len(e))
	for i, creatureElement := range e {
		values[i] = creatureElement.String()
	}
	return values
}

func (e Elements) Equal(other Elements) bool {
	return slices.Equal(e, other)
}

// 攻撃力（AT）。0 以上（0 のカードがある）。上限は持たない（決まりから導ける値が無いため）
type Attack int

// 登録の直後の攻撃力（2026-11-07 オーナー）
const DefaultAttack Attack = 10

func NewAttack(value int) (Attack, error) {
	if value < 0 {
		return 0, ErrAttackNegative
	}
	return Attack(value), nil
}

func (a Attack) Int() int {
	return int(a)
}

// 効果で増減した後の攻撃力。0 で止める。0 になっても倒れない
func (a Attack) Modified(delta int) Attack {
	return Attack(max(int(a)+delta, 0))
}

// 配置制限。クリーチャーは、制限の属性の土地では活動できない（配置・侵略・交換・移動で入れず、いる土地の属性をその属性に変えられない）。
// match の判定は CreatureSpec.canBeActiveOn。
// 配置制限の集合。活動できない土地の属性を 0〜4 個（火・地・水・風。無・複は土地の属性としてあるが制限の値に無いので持てない。
// 重複なし）。並びは火・地・水・風に固定する。値は土地の属性（クリーチャーの属性ではない。2026-11-15 オーナー。core#178）。
// 制限が無いときも nil でなく空にする（理由は ItemRestrictions と同じ）
type PlacementRestrictions []element.Land

// 配置制限の保存の並び
var canonicalPlacementRestrictions = []element.Land{element.LandFire, element.LandEarth, element.LandWater, element.LandWind}

// 登録の直後の配置制限（無し）。共有のスライスを変えられないよう、呼ぶたびに作る
func DefaultPlacementRestrictions() PlacementRestrictions {
	return PlacementRestrictions{}
}

func NewPlacementRestrictions(values []string) (PlacementRestrictions, error) {
	selected := map[element.Land]bool{}
	for _, value := range values {
		land, err := element.NewLand(value)
		if err != nil || !slices.Contains(canonicalPlacementRestrictions, land) {
			return nil, fmt.Errorf("%w: %q", ErrPlacementRestrictionUnknown, value)
		}
		if selected[land] {
			return nil, fmt.Errorf("%w: %q", ErrPlacementRestrictionsDuplicated, value)
		}
		selected[land] = true
	}
	restrictions := make(PlacementRestrictions, 0, len(values))
	for _, land := range canonicalPlacementRestrictions {
		if selected[land] {
			restrictions = append(restrictions, land)
		}
	}
	return restrictions, nil
}

func (r PlacementRestrictions) Strings() []string {
	values := make([]string, len(r))
	for i, land := range r {
		values[i] = land.String()
	}
	return values
}

func (r PlacementRestrictions) Equal(other PlacementRestrictions) bool {
	return slices.Equal(r, other)
}

// 無・複の土地は値に無いので当たらない
func (r PlacementRestrictions) Forbids(land element.Land) bool {
	return slices.Contains(r, land)
}

// アイテム制限の保存の並び（武器・防具・巻物）。入力の並びが違うだけの同じ集合を別の値にしない
var canonicalItemRestrictions = []card.ItemCategory{card.ItemCategoryWeapon, card.ItemCategoryArmor, card.ItemCategoryScroll}

// アイテム制限。クリーチャーは、制限の分類のアイテムを使えない。match の判定は CreatureSpec.canUseItem。
// アイテム制限の集合。使えないカードの分類（card.ItemCategory）を 0〜3 個（重複なし）。並びは武器・防具・巻物に固定する。値の比較は Equal
// （スライスなので == は使えない）。制限が無いときも nil でなく空にする（JSON を null でなく [] にし、読む側が項目の欠けと見分けるため）
type ItemRestrictions []card.ItemCategory

// 登録の直後のアイテム制限（無し）。共有のスライスを変えられないよう、呼ぶたびに作る
func DefaultItemRestrictions() ItemRestrictions {
	return ItemRestrictions{}
}

func NewItemRestrictions(values []string) (ItemRestrictions, error) {
	selected := map[card.ItemCategory]bool{}
	for _, value := range values {
		restriction, err := card.NewItemCategory(value)
		if err != nil {
			return nil, fmt.Errorf("%w: %q", ErrItemRestrictionUnknown, value)
		}
		if selected[restriction] {
			return nil, fmt.Errorf("%w: %q", ErrItemRestrictionsDuplicated, value)
		}
		selected[restriction] = true
	}
	restrictions := make(ItemRestrictions, 0, len(values))
	for _, restriction := range canonicalItemRestrictions {
		if selected[restriction] {
			restrictions = append(restrictions, restriction)
		}
	}
	return restrictions, nil
}

func (r ItemRestrictions) Strings() []string {
	values := make([]string, len(r))
	for i, restriction := range r {
		values[i] = restriction.String()
	}
	return values
}

func (r ItemRestrictions) Equal(other ItemRestrictions) bool {
	return slices.Equal(r, other)
}

func (r ItemRestrictions) Forbids(category card.ItemCategory) bool {
	return slices.Contains(r, category)
}

// 体力（HP）。1 以上（0 だと出した直後に倒れるため）。上限は持たない
type HitPoints int

// 登録の直後の体力（2026-11-07 オーナー）
const DefaultHitPoints HitPoints = 10

func NewHitPoints(value int) (HitPoints, error) {
	if value < 1 {
		return 0, ErrHitPointsTooSmall
	}
	return HitPoints(value), nil
}

func (h HitPoints) Int() int {
	return int(h)
}

// 攻撃を受けた後の体力。0 で止める。生成関数の下限（1 以上）はカードの性能の決まりで、戦闘の中の 0 は倒れたことを表す
func (h HitPoints) Damaged(attack Attack) HitPoints {
	return HitPoints(max(int(h)-attack.Int(), 0))
}

// 効果で増減した後の体力。0 で止める（0 は Damaged と同じく倒れたことを表す）
func (h HitPoints) Modified(delta int) HitPoints {
	return HitPoints(max(int(h)+delta, 0))
}

// クリーチャーが持てる効果の種類（選べる効果の表）。見本のカードで使う種類だけを入れ、新しい種類は足すときに議論する
// （2026-11-15 オーナー。core#178。能力を効果の並びに移した。効果の型と定義は effect、どの持ち主が何を持てるかは持ち主の側）。
// 能力の言葉との対応: 戦闘中の増減 → AT・HP の増減、戦闘中のランダム → AT・HP の上書き（範囲）、先制・後手 → 先に・後に攻撃する、
// 攻撃無効 → 受けるダメージを 0 にする、防御型 → 侵略できない・移動できない
var allowedEffectKinds = []effect.Kind{
	effect.KindAttackModifier, effect.KindHitPointsModifier, effect.KindAttackOverride, effect.KindHitPointsOverride,
	effect.KindStrikeFirst, effect.KindStrikeLast, effect.KindNullifyReceivedDamage, effect.KindTollMultiplier,
	effect.KindForbidInvasion, effect.KindForbidMovement, effect.KindReturnToHandOnDestruction, effect.KindGainMagicOnDestruction,
}

// クリーチャーが持てる効果の種類（選べる効果の表の並び）。studio の画面が、選べるものだけを出すのに使う
func AllowedEffectKinds() []effect.Kind {
	return slices.Clone(allowedEffectKinds)
}

// 持てない効果の種類は ErrEffectNotAllowed、先に攻撃すると後に攻撃するを一緒に持つのは ErrEffectsConflicted（2026-11-11 オーナー。core#158）。
// 同じ種類を重ねて持つのは認める（アイテムと同じ。制約の「または」を効果を並べて表すため）
func checkEffects(effects effect.Effects) error {
	if kind, found := effects.NotAllowed(allowedEffectKinds); found {
		return fmt.Errorf("%w: %q", ErrEffectNotAllowed, kind)
	}
	if effects.Has(effect.KindStrikeFirst) && effects.Has(effect.KindStrikeLast) {
		return ErrEffectsConflicted
	}
	return nil
}
