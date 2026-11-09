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
