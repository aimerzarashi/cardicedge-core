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

// どれか 1 つでも同じ属性があるか（属性で効くものは、どれか 1 つでも合えば効く。2026-11-07 オーナー）
func (e Elements) Overlaps(other Elements) bool {
	return slices.ContainsFunc(e, func(element Element) bool { return slices.Contains(other, element) })
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

// 配置制限。クリーチャーは、制限の属性のマスでは活動できない（配置・侵略・交換・移動で入れず、いる土地の属性をその属性に変えられない）。
// match の判定は CreatureSpec.canBeActiveOn。
// 配置制限の集合。活動できない土地の属性を 0〜4 個（火・地・水・風。無は土地の属性としてあるが制限の値に無いので持てない。
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

// アイテム制限。クリーチャーは、制限の分類のアイテムを使えない。match の判定は CreatureSpec.canUseItem。
// アイテム制限の集合。0〜3 個（重複なし）。並びは武器・防具・巻物に固定する。値の比較は Equal
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

// クリーチャーの能力 1 つ。種類ごとの値のどれか 1 つだけを持つ（spell.Effect と同じ形。種類ごとに型を分ける。2026-11-10 オーナー）。
// 項目のコメントが能力の一覧で、用語・定義（クリーチャーが主語）・match 側の判定や発動の名前を書く。判定の名前は使う場面の言葉で
// なく能力の名前から決める（場面の言葉はほかの場面に合わず、英語の単語もぶれるため。2026-11-12 オーナー。core#160）
type Ability struct {
	// 防御型。侵略できない。移動できない（守ることはできる）。match の判定は CreatureSpec.canInvade・canMove
	Defensive *DefensiveAbility `json:"defensive,omitempty"`
	// 通行料の倍率。いる領地の通行料を払うときに倍率を掛ける。match の発動は tollMultiplier → TollMultiplierActivatedEvent（core#157・core#159）
	TollMultiplier *TollMultiplierAbility `json:"toll_multiplier,omitempty"`
	// 先制。戦闘で、先制の無い相手より先に攻撃する。match の発動は BattleCreatureAbilityActivatedEvent で、得た攻撃の順の強さを戦闘の中の値に持つ（core#158・core#175）
	FirstStrike *FirstStrikeAbility `json:"first_strike,omitempty"`
	// 後手。戦闘で、後手の無い相手より後に攻撃する。match の判定・発動は先制と同じ（core#158）
	LastStrike *LastStrikeAbility `json:"last_strike,omitempty"`
	// 戦闘中の増減。戦闘中、AT・HP が決まった値だけ増減する（負の値もある）。match の発動は ResolveBattle の能力の位置（持っている能力の並びの順）の BattleCreatureAbilityActivatedEvent（増減を持ち、増減が 0 でも出す）（core#163・core#175）
	BattleModifier *BattleModifierAbility `json:"battle_modifier,omitempty"`
	// 戦闘中のランダム。戦闘中、AT・HP が最小〜最大のランダムな値になる（別々に引く。1 刻み）。match の発動は ResolveBattle の能力の位置（持っている能力の並びの順）の BattleCreatureAbilityActivatedEvent（引いた値と今の値の差を増減に持つ）（core#169・core#175）
	BattleRandomStats *BattleRandomStatsAbility `json:"battle_random_stats,omitempty"`
	// 攻撃無効。戦闘で、属性のどれかが合うクリーチャーから攻撃を受けたとき、ダメージが 0 になる。match の判定は CreatureSpec.isImmuneToAttackFrom、発動は攻撃を受ける直前の BattleCreatureAbilityActivatedEvent（属性が合わなければ出さない）（core#174）
	AttackImmunity *AttackImmunityAbility `json:"attack_immunity,omitempty"`
}

type DefensiveAbility struct{}

type FirstStrikeAbility struct{}

type LastStrikeAbility struct{}

func NewFirstStrikeAbility() Ability {
	return Ability{FirstStrike: &FirstStrikeAbility{}}
}

func NewLastStrikeAbility() Ability {
	return Ability{LastStrike: &LastStrikeAbility{}}
}

type BattleModifierAbility struct {
	AttackModifier    AttackModifier    `json:"attack_modifier"`
	HitPointsModifier HitPointsModifier `json:"hit_points_modifier"`
}

func NewBattleModifierAbility(attackModifier AttackModifier, hitPointsModifier HitPointsModifier) Ability {
	return Ability{BattleModifier: &BattleModifierAbility{AttackModifier: attackModifier, HitPointsModifier: hitPointsModifier}}
}

// 戦闘中の AT の増減。負も 0 も認める（アイテムの増減と同じ。範囲の決まりが無いため生成関数は確かめない）
type AttackModifier int

func NewAttackModifier(value int) AttackModifier {
	return AttackModifier(value)
}

func (b AttackModifier) Int() int {
	return int(b)
}

// 戦闘中の HP の増減。AT の増減と同じ
type HitPointsModifier int

func NewHitPointsModifier(value int) HitPointsModifier {
	return HitPointsModifier(value)
}

func (b HitPointsModifier) Int() int {
	return int(b)
}

type BattleRandomStatsAbility struct {
	Minimum BattleRandomStatsBound `json:"minimum"`
	Maximum BattleRandomStatsBound `json:"maximum"`
}

// 最小が 1 以上なのは、引いた HP が 0 だと戦闘の始めに倒れ、能力の意味が無くなるため。AT と HP は同じ範囲から引く
func NewBattleRandomStatsAbility(minimum, maximum int) (Ability, error) {
	if minimum < 1 {
		return Ability{}, ErrBattleRandomStatsMinimumTooSmall
	}
	if maximum < minimum {
		return Ability{}, ErrBattleRandomStatsMaximumLessThanMinimum
	}
	return Ability{BattleRandomStats: &BattleRandomStatsAbility{Minimum: BattleRandomStatsBound(minimum), Maximum: BattleRandomStatsBound(maximum)}}, nil
}

// 攻撃無効が効く相手の属性。火・地・水・風から 1 つ以上（2026-11-13。無は選べない）
type AttackImmunityAbility struct {
	Elements Elements `json:"elements"`
}

func NewAttackImmunityAbility(elements Elements) (Ability, error) {
	if len(elements) == 0 {
		return Ability{}, ErrElementsEmpty
	}
	if slices.Contains(elements, ElementNeutral) {
		return Ability{}, ErrAttackImmunityNeutral
	}
	return Ability{AttackImmunity: &AttackImmunityAbility{Elements: slices.Clone(elements)}}, nil
}

// 戦闘中のランダムの範囲の端（最小・最大）。範囲の決まりは両端を比べるので、生成関数は NewBattleRandomStatsAbility に 1 つだけ置く
type BattleRandomStatsBound int

func (b BattleRandomStatsBound) Int() int {
	return int(b)
}

type TollMultiplierAbility struct {
	Percent TollMultiplierPercent `json:"percent"`
}

func NewTollMultiplierAbility(percent TollMultiplierPercent) Ability {
	return Ability{TollMultiplier: &TollMultiplierAbility{Percent: percent}}
}

// 通行料の倍率（百分率。150 で 1.5 倍）。1 以上。上限は持たない（決まりから導ける値が無いため）
type TollMultiplierPercent int

func NewTollMultiplierPercent(value int) (TollMultiplierPercent, error) {
	if value < 1 {
		return 0, ErrTollMultiplierPercentTooSmall
	}
	return TollMultiplierPercent(value), nil
}

func (p TollMultiplierPercent) Int() int {
	return int(p)
}

func NewDefensiveAbility() Ability {
	return Ability{Defensive: &DefensiveAbility{}}
}

// 種類の名前。同じ種類を重ねて持てないことを確かめるのに使う
func (a Ability) kind() string {
	var kinds []string
	if a.Defensive != nil {
		kinds = append(kinds, "defensive")
	}
	if a.TollMultiplier != nil {
		kinds = append(kinds, "toll_multiplier")
	}
	if a.FirstStrike != nil {
		kinds = append(kinds, "first_strike")
	}
	if a.LastStrike != nil {
		kinds = append(kinds, "last_strike")
	}
	if a.BattleModifier != nil {
		kinds = append(kinds, "battle_modifier")
	}
	if a.BattleRandomStats != nil {
		kinds = append(kinds, "battle_random_stats")
	}
	if a.AttackImmunity != nil {
		kinds = append(kinds, "attack_immunity")
	}
	if len(kinds) != 1 {
		return ""
	}
	return kinds[0]
}

// 種類と値が同じか（ポインタの先を比べる）
func (a Ability) equal(other Ability) bool {
	if a.kind() != other.kind() {
		return false
	}
	if a.TollMultiplier != nil && a.TollMultiplier.Percent != other.TollMultiplier.Percent {
		return false
	}
	if a.BattleModifier != nil && *a.BattleModifier != *other.BattleModifier {
		return false
	}
	if a.BattleRandomStats != nil && *a.BattleRandomStats != *other.BattleRandomStats {
		return false
	}
	return a.AttackImmunity == nil || a.AttackImmunity.Elements.Equal(other.AttackImmunity.Elements)
}

func (a Ability) IsDefensive() bool {
	return a.Defensive != nil
}

// クリーチャーの能力の並び。同じ種類は 1 つまで。並びは入力のまま持つ（core#127。効果を当てる順になりうるため）。値の比較は Equal。
// 無いときも nil でなく空にする（理由は ItemRestrictions と同じ）
type Abilities []Ability

// 登録の直後の能力（無し）。共有のスライスを変えられないよう、呼ぶたびに作る
func DefaultAbilities() Abilities {
	return Abilities{}
}

func NewAbilities(values []Ability) (Abilities, error) {
	seen := map[string]bool{}
	abilities := make(Abilities, 0, len(values))
	for _, value := range values {
		kind := value.kind()
		if kind == "" {
			return nil, ErrAbilityEmpty
		}
		if seen[kind] {
			return nil, fmt.Errorf("%w: %q", ErrAbilitiesDuplicated, kind)
		}
		seen[kind] = true
		abilities = append(abilities, value)
	}
	// 先制と後手は同時に持てない（2026-11-11 オーナー。core#158）
	if seen["first_strike"] && seen["last_strike"] {
		return nil, ErrAbilitiesConflicted
	}
	return abilities, nil
}

func (a Abilities) Equal(other Abilities) bool {
	return slices.EqualFunc(a, other, Ability.equal)
}

func (a Abilities) IsDefensive() bool {
	return slices.ContainsFunc(a, Ability.IsDefensive)
}

func (a Abilities) HasFirstStrike() bool {
	return slices.ContainsFunc(a, func(ability Ability) bool { return ability.FirstStrike != nil })
}

func (a Abilities) HasLastStrike() bool {
	return slices.ContainsFunc(a, func(ability Ability) bool { return ability.LastStrike != nil })
}

// 通行料の倍率。持っていなければ false
func (a Abilities) TollMultiplier() (TollMultiplierPercent, bool) {
	for _, ability := range a {
		if ability.TollMultiplier != nil {
			return ability.TollMultiplier.Percent, true
		}
	}
	return 0, false
}

// 戦闘中の増減。持っていなければ false
func (a Abilities) BattleModifier() (BattleModifierAbility, bool) {
	for _, ability := range a {
		if ability.BattleModifier != nil {
			return *ability.BattleModifier, true
		}
	}
	return BattleModifierAbility{}, false
}

// 戦闘中のランダム。持っていなければ false
func (a Abilities) BattleRandomStats() (BattleRandomStatsAbility, bool) {
	for _, ability := range a {
		if ability.BattleRandomStats != nil {
			return *ability.BattleRandomStats, true
		}
	}
	return BattleRandomStatsAbility{}, false
}

// 攻撃無効。持っていなければ false
func (a Abilities) AttackImmunity() (AttackImmunityAbility, bool) {
	for _, ability := range a {
		if ability.AttackImmunity != nil {
			return *ability.AttackImmunity, true
		}
	}
	return AttackImmunityAbility{}, false
}
