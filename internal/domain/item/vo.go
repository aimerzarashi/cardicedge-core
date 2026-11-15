package item

import (
	"errors"
	"fmt"
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
)

type ID int64

var entityType = card.ItemEntityType

func (ID) EntityType() domain.EntityType {
	return entityType
}

func (id ID) Int64() int64 {
	return int64(id)
}

func NewID(value int64) (ID, error) {
	if value <= 0 {
		return 0, errors.New("item id must be positive")
	}
	return ID(value), nil
}

// アイテムの分類（2026-11-07 オーナー。武器・防具・巻物）
type Category string

const (
	CategoryWeapon Category = "weapon"
	CategoryArmor  Category = "armor"
	CategoryScroll Category = "scroll"
)

// 登録の直後の分類（2026-11-07 オーナー）
const DefaultCategory = CategoryWeapon

func NewCategory(value string) (Category, error) {
	switch category := Category(value); category {
	case CategoryWeapon, CategoryArmor, CategoryScroll:
		return category, nil
	default:
		return "", ErrCategoryInvalid
	}
}

func (c Category) String() string {
	return string(c)
}

// 戦闘中に足す AT。下げるアイテムも作れるよう、負も認める（確かめることが無いので、生成関数は誤りを返さない）
type AttackModifier int

func NewAttackModifier(value int) AttackModifier {
	return AttackModifier(value)
}

func (b AttackModifier) Int() int {
	return int(b)
}

// 戦闘中に足す HP。負も認める理由は AttackModifier と同じ
type HitPointsModifier int

func NewHitPointsModifier(value int) HitPointsModifier {
	return HitPointsModifier(value)
}

func (b HitPointsModifier) Int() int {
	return int(b)
}

// 上書きした後の AT。0 以上
type AttackOverride int

func NewAttackOverride(value int) (AttackOverride, error) {
	if value < 0 {
		return 0, ErrAttackOverrideNegative
	}
	return AttackOverride(value), nil
}

func (o AttackOverride) Int() int {
	return int(o)
}

// AT の倍率（百分率。150 で 1.5 倍）。1 以上。上限は持たない（決まりから導ける値が無いため。クリーチャーの通行料の倍率と同じ）
type AttackMultiplierPercent int

func NewAttackMultiplierPercent(value int) (AttackMultiplierPercent, error) {
	if value < 1 {
		return 0, ErrAttackMultiplierPercentTooSmall
	}
	return AttackMultiplierPercent(value), nil
}

func (p AttackMultiplierPercent) Int() int {
	return int(p)
}

// 制約で比べる属性 1 つ。クリーチャーの属性（creature.Element）と名前をそろえるが、型はパッケージごとに持つ
// （item と creature は互いを読まない決まりのため。core#127。クリーチャーの能力を効果の並びに移すときに置き場所を見直す）
type Element string

const (
	ElementFire    Element = "fire"
	ElementEarth   Element = "earth"
	ElementWater   Element = "water"
	ElementWind    Element = "wind"
	ElementNeutral Element = "neutral"
)

// 保存の並び。入力の並びが違うだけの同じ集合を別の値にしない
var canonicalElements = []Element{ElementFire, ElementEarth, ElementWater, ElementWind, ElementNeutral}

func (e Element) String() string {
	return string(e)
}

// 属性の集合。1 つ以上（重複なし）。並びは火・地・水・風・無に固定する。値の比較は Equal
type Elements []Element

func NewElements(values []string) (Elements, error) {
	if len(values) == 0 {
		return nil, ErrElementsEmpty
	}
	selected := map[Element]bool{}
	for _, value := range values {
		element := Element(value)
		if !slices.Contains(canonicalElements, element) {
			return nil, fmt.Errorf("%w: %q", ErrElementUnknown, value)
		}
		if selected[element] {
			return nil, fmt.Errorf("%w: %q", ErrElementsDuplicated, value)
		}
		selected[element] = true
	}
	elements := make(Elements, 0, len(values))
	for _, element := range canonicalElements {
		if selected[element] {
			elements = append(elements, element)
		}
	}
	return elements, nil
}

func (e Elements) Equal(other Elements) bool {
	return slices.Equal(e, other)
}

// 効果が発動する制約 1 つ。種類ごとの値のどれか 1 つだけを持つ（Effect と同じ形）。制約は「誰の × 何を × どう比べるか × 値」で、
// 今は相手の属性だけ（2026-11-14 オーナー）
type Constraint struct {
	// 相手の属性。戦う相手のクリーチャーの属性のどれかが合うとき
	OpponentElement *OpponentElementConstraint `json:"opponent_element,omitempty"`
}

type OpponentElementConstraint struct {
	Elements Elements `json:"elements"`
}

func NewOpponentElementConstraint(elements Elements) Constraint {
	return Constraint{OpponentElement: &OpponentElementConstraint{Elements: slices.Clone(elements)}}
}

func (c Constraint) valid() bool {
	return c.OpponentElement != nil
}

func (c Constraint) equal(other Constraint) bool {
	if (c.OpponentElement == nil) != (other.OpponentElement == nil) {
		return false
	}
	return c.OpponentElement == nil || c.OpponentElement.Elements.Equal(other.OpponentElement.Elements)
}

// 相手の属性（文字列）で、制約を満たすか。属性の型を creature から読まないため文字列で受け取る
func (c Constraint) SatisfiedByOpponentElements(opponentElements []string) bool {
	if c.OpponentElement == nil {
		return false
	}
	return slices.ContainsFunc(c.OpponentElement.Elements, func(element Element) bool {
		return slices.Contains(opponentElements, element.String())
	})
}

// 制約の並び。すべてを満たすときに発動する（「または」は効果を並べて表す。2026-11-14 オーナー）。無いときも nil でなく空にする
type Constraints []Constraint

func NewConstraints(values []Constraint) (Constraints, error) {
	for _, value := range values {
		if !value.valid() {
			return nil, ErrConstraintEmpty
		}
	}
	return append(Constraints{}, values...), nil
}

func (c Constraints) Equal(other Constraints) bool {
	return slices.EqualFunc(c, other, Constraint.equal)
}

func (c Constraints) SatisfiedByOpponentElements(opponentElements []string) bool {
	for _, constraint := range c {
		if !constraint.SatisfiedByOpponentElements(opponentElements) {
			return false
		}
	}
	return true
}

// アイテムの効果 1 つ。種類ごとの値のどれか 1 つだけを持つ（spell.Effect・creature.Ability と同じ形）。能力は「呼び名」でなく
// 効果を単位にし、タイミングは効果の種類で決まる（2026-11-14 オーナー。core#177）。「強打」のような言葉は画面のラベルだけ。
// 項目のコメントが効果の一覧で、用語・定義・match 側の発動を書く
type Effect struct {
	// AT の増減。戦闘中、AT が決まった値だけ増減する。match の発動はアイテムの位置の BattleItemEffectActivatedEvent（増減が 0 でも出す）
	AttackModifier *AttackModifierEffect `json:"attack_modifier,omitempty"`
	// HP の増減。戦闘中、HP が決まった値だけ増減する。発動は AT の増減と同じ
	HitPointsModifier *HitPointsModifierEffect `json:"hit_points_modifier,omitempty"`
	// AT の上書き。戦闘中、AT が決まった値になる。発動はアイテムの位置（今の値との差を増減に持つ）
	AttackOverride *AttackOverrideEffect `json:"attack_override,omitempty"`
	// AT の倍率（画面のラベルは強打）。制約を満たすとき、増減・上書きがすべて済んだ後に AT に倍率を掛ける（切り捨て）。
	// 発動したうち一番高い倍率を 1 回だけ掛け、掛けたものだけが発動のイベントを出す（2026-11-14 オーナー）
	AttackMultiplier *AttackMultiplierEffect `json:"attack_multiplier,omitempty"`
	// 地形効果を無視する。持ち主が攻撃する直前に発動し、相手の地形効果を無視してダメージを与える（相手に地形効果が無ければ発動しない）
	IgnoreTerrainEffect *IgnoreTerrainEffect `json:"ignore_terrain_effect,omitempty"`
}

type AttackModifierEffect struct {
	Value AttackModifier `json:"value"`
}

type HitPointsModifierEffect struct {
	Value HitPointsModifier `json:"value"`
}

type AttackOverrideEffect struct {
	Value AttackOverride `json:"value"`
}

type AttackMultiplierEffect struct {
	Percent     AttackMultiplierPercent `json:"percent"`
	Constraints Constraints             `json:"constraints"`
}

type IgnoreTerrainEffect struct{}

func NewAttackModifierEffect(value AttackModifier) Effect {
	return Effect{AttackModifier: &AttackModifierEffect{Value: value}}
}

func NewHitPointsModifierEffect(value HitPointsModifier) Effect {
	return Effect{HitPointsModifier: &HitPointsModifierEffect{Value: value}}
}

func NewAttackOverrideEffect(value AttackOverride) Effect {
	return Effect{AttackOverride: &AttackOverrideEffect{Value: value}}
}

func NewAttackMultiplierEffect(percent AttackMultiplierPercent, constraints Constraints) Effect {
	return Effect{AttackMultiplier: &AttackMultiplierEffect{Percent: percent, Constraints: append(Constraints{}, constraints...)}}
}

func NewIgnoreTerrainEffect() Effect {
	return Effect{IgnoreTerrainEffect: &IgnoreTerrainEffect{}}
}

// 効果の種類の名前
type EffectKind string

const (
	EffectKindAttackModifier      EffectKind = "attack_modifier"
	EffectKindHitPointsModifier   EffectKind = "hit_points_modifier"
	EffectKindAttackOverride      EffectKind = "attack_override"
	EffectKindAttackMultiplier    EffectKind = "attack_multiplier"
	EffectKindIgnoreTerrainEffect EffectKind = "ignore_terrain_effect"
)

func NewEffectKind(value string) (EffectKind, error) {
	switch kind := EffectKind(value); kind {
	case EffectKindAttackModifier, EffectKindHitPointsModifier, EffectKindAttackOverride, EffectKindAttackMultiplier, EffectKindIgnoreTerrainEffect:
		return kind, nil
	}
	return "", ErrEffectKindUnknown
}

// 種類の名前。種類が無い・2 つ以上なら空
func (e Effect) Kind() EffectKind {
	var kinds []EffectKind
	if e.AttackModifier != nil {
		kinds = append(kinds, EffectKindAttackModifier)
	}
	if e.HitPointsModifier != nil {
		kinds = append(kinds, EffectKindHitPointsModifier)
	}
	if e.AttackOverride != nil {
		kinds = append(kinds, EffectKindAttackOverride)
	}
	if e.AttackMultiplier != nil {
		kinds = append(kinds, EffectKindAttackMultiplier)
	}
	if e.IgnoreTerrainEffect != nil {
		kinds = append(kinds, EffectKindIgnoreTerrainEffect)
	}
	if len(kinds) != 1 {
		return ""
	}
	return kinds[0]
}

// 種類と値が同じか（ポインタの先を比べる）
func (e Effect) equal(other Effect) bool {
	if e.Kind() != other.Kind() {
		return false
	}
	switch {
	case e.AttackModifier != nil:
		return *e.AttackModifier == *other.AttackModifier
	case e.HitPointsModifier != nil:
		return *e.HitPointsModifier == *other.HitPointsModifier
	case e.AttackOverride != nil:
		return *e.AttackOverride == *other.AttackOverride
	case e.AttackMultiplier != nil:
		return e.AttackMultiplier.Percent == other.AttackMultiplier.Percent && e.AttackMultiplier.Constraints.Equal(other.AttackMultiplier.Constraints)
	}
	return true
}

// 分類ごとに持てる効果の種類（選べる効果の表）。見本のカードで使う組み合わせだけを入れ、新しい組み合わせは足すときに議論する
// （2026-11-14 オーナー。設計メモ「能力の作りの見直し」）
var allowedEffectKinds = map[Category][]EffectKind{
	CategoryWeapon: {EffectKindAttackModifier, EffectKindAttackMultiplier},
	CategoryArmor:  {EffectKindHitPointsModifier},
	CategoryScroll: {EffectKindAttackOverride, EffectKindAttackMultiplier, EffectKindIgnoreTerrainEffect},
}

// その分類で持てる効果の種類（選べる効果の表の並び）。studio の画面が、選べるものだけを出すのに使う
func (c Category) AllowedEffectKinds() []EffectKind {
	return slices.Clone(allowedEffectKinds[c])
}

// その分類で持てる効果の種類か
func (c Category) Allows(kind EffectKind) bool {
	return slices.Contains(allowedEffectKinds[c], kind)
}

// アイテムの効果の並び。並びは入力のまま持つ（効果を当てる順になるため）。同じ種類を重ねて持ってよい（制約の「または」を
// 効果を並べて表すため）。値の比較は Equal。無いときも nil でなく空にする
type Effects []Effect

// 登録の直後の効果（無し）。共有のスライスを変えられないよう、呼ぶたびに作る
func DefaultEffects() Effects {
	return Effects{}
}

func NewEffects(values []Effect) (Effects, error) {
	for _, value := range values {
		if value.Kind() == "" {
			return nil, ErrEffectEmpty
		}
	}
	return append(Effects{}, values...), nil
}

func (e Effects) Equal(other Effects) bool {
	return slices.EqualFunc(e, other, Effect.equal)
}

// その分類で持てない効果の種類（最初の 1 つ）。すべて持てれば false
func (e Effects) NotAllowedIn(category Category) (EffectKind, bool) {
	for _, effect := range e {
		if !category.Allows(effect.Kind()) {
			return effect.Kind(), true
		}
	}
	return "", false
}
