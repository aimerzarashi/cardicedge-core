package creature

import (
	"errors"
	"fmt"
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
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

// クリーチャーの属性（element）1 つ。火・地・水・風と、どれでもない無。ボードの土地の属性（board.Element）とは名前をそろえるが、
// 型は分ける（こちらは複数持てる・無は単独など決まりが別で、共通にすると片方を変えたくなったときに分けられなくなるため）
type Element string

const (
	ElementFire    Element = "fire"
	ElementEarth   Element = "earth"
	ElementWater   Element = "water"
	ElementWind    Element = "wind"
	ElementNeutral Element = "neutral"
)

// 保存の並び（火・地・水・風）。複数持つときはこの順にそろえ、入力の並びが違うだけの同じ集合を別の値にしない
var canonicalElements = []Element{ElementFire, ElementEarth, ElementWater, ElementWind}

func (e Element) String() string {
	return string(e)
}

// クリーチャーが持つ属性の集合。無だけを単独で持つか、火・地・水・風から 1〜4 つ（重複なし）を持つ。
// 並びは火・地・水・風に固定する。値の比較は Equal（スライスなので == は使えない）
type Elements []Element

// 登録の直後の属性（無。2026-11-07 オーナー）。登録のイベントが値を明示して持つので、変えても過去のカードは変わらない。
// 共有のスライスを変えられないよう、呼ぶたびに作る
func DefaultElements() Elements {
	return Elements{ElementNeutral}
}

func NewElements(values []string) (Elements, error) {
	if len(values) == 0 {
		return nil, ErrElementsEmpty
	}
	selected := map[Element]bool{}
	for _, value := range values {
		element := Element(value)
		if element != ElementNeutral && !slices.Contains(canonicalElements, element) {
			return nil, fmt.Errorf("%w: %q", ErrElementUnknown, value)
		}
		if selected[element] {
			return nil, fmt.Errorf("%w: %q", ErrElementsDuplicated, value)
		}
		selected[element] = true
	}
	if selected[ElementNeutral] {
		if len(values) > 1 {
			return nil, ErrElementsNeutralMixed
		}
		return Elements{ElementNeutral}, nil
	}
	elements := make(Elements, 0, len(values))
	for _, element := range canonicalElements {
		if selected[element] {
			elements = append(elements, element)
		}
	}
	return elements, nil
}

func (e Elements) Strings() []string {
	values := make([]string, len(e))
	for i, element := range e {
		values[i] = element.String()
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

// クリーチャーの配置制限の集合。置けない土地の属性を 0〜4 個（火・地・水・風。無は土地の属性としてあるが制限の値に無いので持てない。
// 重複なし）。並びは火・地・水・風に固定する。Elements と違って空を許し、無を持たない（決まりが別なので型を分ける）。
// 制限が無いときも nil でなく空にする（理由は ItemRestrictions と同じ）
type PlacementRestrictions []Element

// 登録の直後の配置制限（無し）。共有のスライスを変えられないよう、呼ぶたびに作る
func DefaultPlacementRestrictions() PlacementRestrictions {
	return PlacementRestrictions{}
}

func NewPlacementRestrictions(values []string) (PlacementRestrictions, error) {
	selected := map[Element]bool{}
	for _, value := range values {
		element := Element(value)
		if !slices.Contains(canonicalElements, element) {
			return nil, fmt.Errorf("%w: %q", ErrPlacementRestrictionUnknown, value)
		}
		if selected[element] {
			return nil, fmt.Errorf("%w: %q", ErrPlacementRestrictionsDuplicated, value)
		}
		selected[element] = true
	}
	restrictions := make(PlacementRestrictions, 0, len(values))
	for _, element := range canonicalElements {
		if selected[element] {
			restrictions = append(restrictions, element)
		}
	}
	return restrictions, nil
}

func (r PlacementRestrictions) Strings() []string {
	values := make([]string, len(r))
	for i, element := range r {
		values[i] = element.String()
	}
	return values
}

func (r PlacementRestrictions) Equal(other PlacementRestrictions) bool {
	return slices.Equal(r, other)
}

// 文字列で受けるのは、呼ぶ側が持つ board.Element と型が別のため。無・複の土地は値に無いので当たらない
func (r PlacementRestrictions) Forbids(landElement string) bool {
	return slices.Contains(r, Element(landElement))
}

// アイテム制限の 1 つ。持てないアイテムの分類（武器・防具・巻物）。分類は item.Category と名前をそろえるが、型はパッケージごとに持つ
// （creature が item を読むと、カードの種類どうしが依存するため。core#127 で決めた形）
type ItemRestriction string

const (
	ItemRestrictionWeapon ItemRestriction = "weapon"
	ItemRestrictionArmor  ItemRestriction = "armor"
	ItemRestrictionScroll ItemRestriction = "scroll"
)

// 保存の並び（武器・防具・巻物）。入力の並びが違うだけの同じ集合を別の値にしない
var canonicalItemRestrictions = []ItemRestriction{ItemRestrictionWeapon, ItemRestrictionArmor, ItemRestrictionScroll}

func (r ItemRestriction) String() string {
	return string(r)
}

// クリーチャーのアイテム制限の集合。0〜3 個（重複なし）。並びは武器・防具・巻物に固定する。値の比較は Equal
// （スライスなので == は使えない）。制限が無いときも nil でなく空にする（JSON を null でなく [] にし、読む側が項目の欠けと見分けるため）
type ItemRestrictions []ItemRestriction

// 登録の直後のアイテム制限（無し）。共有のスライスを変えられないよう、呼ぶたびに作る
func DefaultItemRestrictions() ItemRestrictions {
	return ItemRestrictions{}
}

func NewItemRestrictions(values []string) (ItemRestrictions, error) {
	selected := map[ItemRestriction]bool{}
	for _, value := range values {
		restriction := ItemRestriction(value)
		if !slices.Contains(canonicalItemRestrictions, restriction) {
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

// 文字列で受けるのは、呼ぶ側が持つ item.Category と型が別のため
func (r ItemRestrictions) Forbids(itemCategory string) bool {
	return slices.Contains(r, ItemRestriction(itemCategory))
}

// 移動。領地コマンドのクリーチャーの移動で進めるマスの数。0 以上（0 は動けない。防御型のクリーチャー）。上限は持たない（2026-11-10 オーナー）
type Movement int

// 登録の直後の移動（2026-11-10 オーナー）
const DefaultMovement Movement = 1

func NewMovement(value int) (Movement, error) {
	if value < 0 {
		return 0, ErrMovementNegative
	}
	return Movement(value), nil
}

func (m Movement) Int() int {
	return int(m)
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
