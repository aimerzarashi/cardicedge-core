// 効果の定義（効果の種類・値・制約と、スペルの効果）。誰が持つかによらない語彙で、クリーチャー・アイテム・スペルが持つ
// （2026-11-15 オーナー。core#178。施設が効果を持つときは board も読む）。board を読まない（board → effect と循環するため）。
// 効果が指すボードの語彙は element の段に降ろす。どの持ち主がどの効果を持てるかの表は持ち主の側（creature・item）に置く。
// 効果の当て方（戦闘・通行料・侵略と移動）は match の決まり
package effect

import (
	"fmt"
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/element"
)

// 戦闘中に足す AT。下げる効果も作れるよう、負も認める（確かめることが無いので、生成関数は誤りを返さない）
type AttackModifier int

func NewAttackModifier(value int) AttackModifier {
	return AttackModifier(value)
}

func (m AttackModifier) Int() int {
	return int(m)
}

// 戦闘中に足す HP。負も認める理由は AttackModifier と同じ
type HitPointsModifier int

func NewHitPointsModifier(value int) HitPointsModifier {
	return HitPointsModifier(value)
}

func (m HitPointsModifier) Int() int {
	return int(m)
}

// 上書きした後の AT（範囲の端）。0 以上
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

// 上書きした後の HP（範囲の端）。1 以上（0 だと戦闘の始めに倒れ、効果の意味が無くなるため）
type HitPointsOverride int

func NewHitPointsOverride(value int) (HitPointsOverride, error) {
	if value < 1 {
		return 0, ErrHitPointsOverrideTooSmall
	}
	return HitPointsOverride(value), nil
}

func (o HitPointsOverride) Int() int {
	return int(o)
}

// AT の倍率（百分率。150 で 1.5 倍）。1 以上。上限は持たない（決まりから導ける値が無いため。通行料の倍率と同じ）
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

// 破壊されたときの効果で得る魔力の額。1 以上
type DestructionMagicAmount int

func NewDestructionMagicAmount(value int) (DestructionMagicAmount, error) {
	if value < 1 {
		return 0, ErrDestructionMagicAmountTooSmall
	}
	return DestructionMagicAmount(value), nil
}

func (a DestructionMagicAmount) Int() int {
	return int(a)
}

// 破壊されたときの効果が発動する破壊。match の 3 つの破壊のイベント（侵略したクリーチャー・守るクリーチャー・ボード）と対応する
// （2026-11-15 オーナー。core#182）
type Destruction string

const (
	// 戦闘で、侵略したクリーチャーとして破壊された（InvadingCreatureDestroyedEvent）
	DestructionInvading Destruction = "invading"
	// 戦闘で、領地を守るクリーチャーとして破壊された（DefendingCreatureDestroyedEvent）
	DestructionDefending Destruction = "defending"
	// 戦闘の外（今はスペルのダメージ）で破壊された（BoardCreatureDestroyedEvent）
	DestructionBoard Destruction = "board"
)

// 保存の並び
var canonicalDestructions = []Destruction{DestructionInvading, DestructionDefending, DestructionBoard}

// 発動する破壊の集合。1 つ以上（選ばないと発動しない効果になるため）で重複なし、並びは侵略・守る・ボードに固定する
type Destructions []Destruction

func NewDestructions(values []string) (Destructions, error) {
	if len(values) == 0 {
		return nil, ErrDestructionsEmpty
	}
	selected := map[Destruction]bool{}
	for _, value := range values {
		destruction := Destruction(value)
		if !slices.Contains(canonicalDestructions, destruction) {
			return nil, fmt.Errorf("%w: %q", ErrDestructionUnknown, value)
		}
		if selected[destruction] {
			return nil, fmt.Errorf("%w: %q", ErrDestructionsDuplicated, value)
		}
		selected[destruction] = true
	}
	destructions := make(Destructions, 0, len(values))
	for _, destruction := range canonicalDestructions {
		if selected[destruction] {
			destructions = append(destructions, destruction)
		}
	}
	return destructions, nil
}

func (d Destructions) Contains(destruction Destruction) bool {
	return slices.Contains(d, destruction)
}

func (d Destructions) Equal(other Destructions) bool {
	return slices.Equal(d, other)
}

// 制約で比べるクリーチャーの属性の集合。1 つ以上（重複なし）で、無とほかを一緒に選べる（クリーチャー自身の属性の
// 「無は無だけ」の決まりは creature のもので、ここには当てない）。並びは火・地・水・風・無に固定する。値の比較は Equal
type CreatureElements []element.Creature

// 保存の並び。入力の並びが違うだけの同じ集合を別の値にしない
var canonicalCreatureElements = []element.Creature{element.CreatureFire, element.CreatureEarth, element.CreatureWater, element.CreatureWind, element.CreatureNeutral}

func NewCreatureElements(values []string) (CreatureElements, error) {
	if len(values) == 0 {
		return nil, ErrCreatureElementsEmpty
	}
	selected := map[element.Creature]bool{}
	for _, value := range values {
		creatureElement, err := element.NewCreature(value)
		if err != nil {
			return nil, fmt.Errorf("%w: %q", ErrCreatureElementUnknown, value)
		}
		if selected[creatureElement] {
			return nil, fmt.Errorf("%w: %q", ErrCreatureElementsDuplicated, value)
		}
		selected[creatureElement] = true
	}
	elements := make(CreatureElements, 0, len(values))
	for _, creatureElement := range canonicalCreatureElements {
		if selected[creatureElement] {
			elements = append(elements, creatureElement)
		}
	}
	return elements, nil
}

func (e CreatureElements) Equal(other CreatureElements) bool {
	return slices.Equal(e, other)
}

// 効果が発動する制約 1 つ。種類ごとの値のどれか 1 つだけを持つ（Effect と同じ形）。制約は「誰の × 何を × どう比べるか × 値」で、
// 今は相手のクリーチャーの属性だけ（2026-11-14 オーナー）
type Constraint struct {
	// 相手のクリーチャーの属性。戦う相手のクリーチャーの属性のどれかが合うとき（「属性」には土地の属性もあり、ボードの効果では相手が
	// プレイヤーにもなるため、何の属性かを名前に入れる。2026-11-15 オーナー）
	OpponentCreatureElement *OpponentCreatureElementConstraint `json:"opponent_creature_element,omitempty"`
}

type OpponentCreatureElementConstraint struct {
	Elements CreatureElements `json:"elements"`
}

func NewOpponentCreatureElementConstraint(elements CreatureElements) Constraint {
	return Constraint{OpponentCreatureElement: &OpponentCreatureElementConstraint{Elements: slices.Clone(elements)}}
}

func (c Constraint) valid() bool {
	return c.OpponentCreatureElement != nil
}

func (c Constraint) equal(other Constraint) bool {
	if (c.OpponentCreatureElement == nil) != (other.OpponentCreatureElement == nil) {
		return false
	}
	return c.OpponentCreatureElement == nil || c.OpponentCreatureElement.Elements.Equal(other.OpponentCreatureElement.Elements)
}

// 戦う相手のクリーチャーの属性で、制約を満たすか
func (c Constraint) SatisfiedByOpponentCreatureElements(opponentElements []element.Creature) bool {
	if c.OpponentCreatureElement == nil {
		return false
	}
	return slices.ContainsFunc(c.OpponentCreatureElement.Elements, func(creatureElement element.Creature) bool {
		return slices.Contains(opponentElements, creatureElement)
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

func (c Constraints) SatisfiedByOpponentCreatureElements(opponentElements []element.Creature) bool {
	for _, constraint := range c {
		if !constraint.SatisfiedByOpponentCreatureElements(opponentElements) {
			return false
		}
	}
	return true
}

// 効果 1 つ。種類ごとの値のどれか 1 つだけを持つ。能力は「呼び名」でなく効果を単位にし、タイミングは効果の種類で決まる
// （2026-11-14 オーナー。core#177）。名前は効果を言葉にした形（2026-11-15 オーナー。core#178）。「強打」「先制」のような言葉は画面のラベルだけ。
// 項目のコメントが効果の一覧で、用語・定義（持ち主のクリーチャーが主語）・match 側の発動を書く。持ち主がクリーチャーのときの発動は
// BattleCreatureEffectActivatedEvent、アイテムのときは BattleItemEffectActivatedEvent（どちらも側・効果の種類・増減）
type Effect struct {
	// AT の増減。戦闘中、AT が決まった値だけ増減する。発動は持ち主の位置（クリーチャーは能力の位置、アイテムはアイテムの位置）で、増減が 0 でも出す
	AttackModifier *AttackModifierEffect `json:"attack_modifier,omitempty"`
	// HP の増減。戦闘中、HP が決まった値だけ増減する。発動は AT の増減と同じ
	HitPointsModifier *HitPointsModifierEffect `json:"hit_points_modifier,omitempty"`
	// AT の上書き。戦闘中、AT が最小〜最大のランダムな値になる（1 刻み。固定は最小と最大が同じ）。発動は持ち主の位置（今の値との差を増減に持つ）。
	// 乱数は並びの順に引く
	AttackOverride *AttackOverrideEffect `json:"attack_override,omitempty"`
	// HP の上書き。戦闘中、HP が最小〜最大のランダムな値になる。発動は AT の上書きと同じ
	HitPointsOverride *HitPointsOverrideEffect `json:"hit_points_override,omitempty"`
	// AT の倍率（画面のラベルは強打）。制約を満たすとき、増減・上書きがすべて済んだ後に AT に倍率を掛ける（切り捨て）。
	// 発動したうち一番高い倍率を 1 回だけ掛け、掛けたものだけが発動のイベントを出す（2026-11-14 オーナー）
	AttackMultiplier *AttackMultiplierEffect `json:"attack_multiplier,omitempty"`
	// 地形効果を無視する。持ち主が攻撃する直前に発動し、相手の地形効果を無視してダメージを与える（相手に地形効果が無ければ発動しない）
	IgnoreTerrainEffect *IgnoreTerrainEffect `json:"ignore_terrain_effect,omitempty"`
	// 先に攻撃する（画面のラベルは先制）。戦闘で、先に攻撃する効果の無い相手より先に攻撃する。発動は攻撃の前（持っていれば効き目が無くても出す）
	StrikeFirst *StrikeFirstEffect `json:"strike_first,omitempty"`
	// 後に攻撃する（画面のラベルは後手）。戦闘で、後に攻撃する効果の無い相手より後に攻撃する。発動は先に攻撃すると同じ
	StrikeLast *StrikeLastEffect `json:"strike_last,omitempty"`
	// 受けるダメージを 0 にする（画面のラベルは攻撃無効）。制約を満たす相手から攻撃を受けたとき、ダメージが 0 になる。
	// 発動は攻撃を受ける直前（満たさなければ出さない）
	NullifyReceivedDamage *NullifyReceivedDamageEffect `json:"nullify_received_damage,omitempty"`
	// 通行料の倍率（戦闘の外）。いる領地の通行料を払うときに倍率を掛ける。発動は払うときの TollCreatureEffectActivatedEvent
	TollMultiplier *TollMultiplierEffect `json:"toll_multiplier,omitempty"`
	// 侵略できない（戦闘の外）。手札から侵略できず、移動で他人の領地にも入れない（守ることはできる）。match の判定は CreatureSpec.canInvade
	ForbidInvasion *ForbidInvasionEffect `json:"forbid_invasion,omitempty"`
	// 移動できない（戦闘の外）。領地コマンドで動けない。match の判定は CreatureSpec.canMove
	ForbidMovement *ForbidMovementEffect `json:"forbid_movement,omitempty"`
	// 破壊されたら手札に戻る。選んだ破壊のとき、捨て札に入ったカードが持ち主の手札に戻る（手札の上限はいつもどおり持ち主のターンの終わりに合わせる）。
	// 発動は破壊のイベントの直後の DestructionCreatureEffectActivatedEvent で、結果は CardReturnedToHandEvent
	ReturnToHandOnDestruction *ReturnToHandOnDestructionEffect `json:"return_to_hand_on_destruction,omitempty"`
	// 破壊されたら魔力を得る。選んだ破壊のとき、持ち主が額の魔力を受け取る。発動は手札に戻ると同じで、結果は MagicReceivedEvent
	GainMagicOnDestruction *GainMagicOnDestructionEffect `json:"gain_magic_on_destruction,omitempty"`
}

type AttackModifierEffect struct {
	Value AttackModifier `json:"value"`
}

type HitPointsModifierEffect struct {
	Value HitPointsModifier `json:"value"`
}

type AttackOverrideEffect struct {
	Minimum AttackOverride `json:"minimum"`
	Maximum AttackOverride `json:"maximum"`
}

type HitPointsOverrideEffect struct {
	Minimum HitPointsOverride `json:"minimum"`
	Maximum HitPointsOverride `json:"maximum"`
}

type AttackMultiplierEffect struct {
	Percent     AttackMultiplierPercent `json:"percent"`
	Constraints Constraints             `json:"constraints"`
}

type IgnoreTerrainEffect struct{}

type StrikeFirstEffect struct{}

type StrikeLastEffect struct{}

type NullifyReceivedDamageEffect struct {
	Constraints Constraints `json:"constraints"`
}

type TollMultiplierEffect struct {
	Percent TollMultiplierPercent `json:"percent"`
}

type ForbidInvasionEffect struct{}

type ForbidMovementEffect struct{}

type ReturnToHandOnDestructionEffect struct {
	Destructions Destructions `json:"destructions"`
}

type GainMagicOnDestructionEffect struct {
	Destructions Destructions           `json:"destructions"`
	Amount       DestructionMagicAmount `json:"amount"`
}

func NewAttackModifierEffect(value AttackModifier) Effect {
	return Effect{AttackModifier: &AttackModifierEffect{Value: value}}
}

func NewHitPointsModifierEffect(value HitPointsModifier) Effect {
	return Effect{HitPointsModifier: &HitPointsModifierEffect{Value: value}}
}

// 範囲の両端を比べる決まりなので、生成関数は効果に 1 つだけ置く
func NewAttackOverrideEffect(minimum, maximum AttackOverride) (Effect, error) {
	if maximum < minimum {
		return Effect{}, ErrOverrideMaximumLessThanMinimum
	}
	return Effect{AttackOverride: &AttackOverrideEffect{Minimum: minimum, Maximum: maximum}}, nil
}

func NewHitPointsOverrideEffect(minimum, maximum HitPointsOverride) (Effect, error) {
	if maximum < minimum {
		return Effect{}, ErrOverrideMaximumLessThanMinimum
	}
	return Effect{HitPointsOverride: &HitPointsOverrideEffect{Minimum: minimum, Maximum: maximum}}, nil
}

func NewAttackMultiplierEffect(percent AttackMultiplierPercent, constraints Constraints) Effect {
	return Effect{AttackMultiplier: &AttackMultiplierEffect{Percent: percent, Constraints: append(Constraints{}, constraints...)}}
}

func NewIgnoreTerrainEffect() Effect {
	return Effect{IgnoreTerrainEffect: &IgnoreTerrainEffect{}}
}

func NewStrikeFirstEffect() Effect {
	return Effect{StrikeFirst: &StrikeFirstEffect{}}
}

func NewStrikeLastEffect() Effect {
	return Effect{StrikeLast: &StrikeLastEffect{}}
}

func NewNullifyReceivedDamageEffect(constraints Constraints) Effect {
	return Effect{NullifyReceivedDamage: &NullifyReceivedDamageEffect{Constraints: append(Constraints{}, constraints...)}}
}

func NewTollMultiplierEffect(percent TollMultiplierPercent) Effect {
	return Effect{TollMultiplier: &TollMultiplierEffect{Percent: percent}}
}

func NewForbidInvasionEffect() Effect {
	return Effect{ForbidInvasion: &ForbidInvasionEffect{}}
}

func NewReturnToHandOnDestructionEffect(destructions Destructions) Effect {
	return Effect{ReturnToHandOnDestruction: &ReturnToHandOnDestructionEffect{Destructions: destructions}}
}

func NewGainMagicOnDestructionEffect(destructions Destructions, amount DestructionMagicAmount) Effect {
	return Effect{GainMagicOnDestruction: &GainMagicOnDestructionEffect{Destructions: destructions, Amount: amount}}
}

// 破壊されたときの効果が、その破壊で発動するか
func (e Effect) ActivatesOn(destruction Destruction) bool {
	switch {
	case e.ReturnToHandOnDestruction != nil:
		return e.ReturnToHandOnDestruction.Destructions.Contains(destruction)
	case e.GainMagicOnDestruction != nil:
		return e.GainMagicOnDestruction.Destructions.Contains(destruction)
	}
	return false
}

func NewForbidMovementEffect() Effect {
	return Effect{ForbidMovement: &ForbidMovementEffect{}}
}

// 効果の種類の名前
type Kind string

const (
	KindAttackModifier            Kind = "attack_modifier"
	KindHitPointsModifier         Kind = "hit_points_modifier"
	KindAttackOverride            Kind = "attack_override"
	KindHitPointsOverride         Kind = "hit_points_override"
	KindAttackMultiplier          Kind = "attack_multiplier"
	KindIgnoreTerrainEffect       Kind = "ignore_terrain_effect"
	KindStrikeFirst               Kind = "strike_first"
	KindStrikeLast                Kind = "strike_last"
	KindNullifyReceivedDamage     Kind = "nullify_received_damage"
	KindTollMultiplier            Kind = "toll_multiplier"
	KindForbidInvasion            Kind = "forbid_invasion"
	KindForbidMovement            Kind = "forbid_movement"
	KindReturnToHandOnDestruction Kind = "return_to_hand_on_destruction"
	KindGainMagicOnDestruction    Kind = "gain_magic_on_destruction"
)

// 一覧の並び（Effect の項目の順）
var kinds = []Kind{
	KindAttackModifier, KindHitPointsModifier, KindAttackOverride, KindHitPointsOverride, KindAttackMultiplier, KindIgnoreTerrainEffect,
	KindStrikeFirst, KindStrikeLast, KindNullifyReceivedDamage, KindTollMultiplier, KindForbidInvasion, KindForbidMovement,
	KindReturnToHandOnDestruction, KindGainMagicOnDestruction,
}

func NewKind(value string) (Kind, error) {
	if kind := Kind(value); slices.Contains(kinds, kind) {
		return kind, nil
	}
	return "", ErrKindUnknown
}

func (k Kind) String() string {
	return string(k)
}

// 種類の名前。種類が無い・2 つ以上なら空
func (e Effect) Kind() Kind {
	present := []bool{
		e.AttackModifier != nil, e.HitPointsModifier != nil, e.AttackOverride != nil, e.HitPointsOverride != nil, e.AttackMultiplier != nil,
		e.IgnoreTerrainEffect != nil, e.StrikeFirst != nil, e.StrikeLast != nil, e.NullifyReceivedDamage != nil, e.TollMultiplier != nil,
		e.ForbidInvasion != nil, e.ForbidMovement != nil, e.ReturnToHandOnDestruction != nil, e.GainMagicOnDestruction != nil,
	}
	var found []Kind
	for i, ok := range present {
		if ok {
			found = append(found, kinds[i])
		}
	}
	if len(found) != 1 {
		return ""
	}
	return found[0]
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
	case e.HitPointsOverride != nil:
		return *e.HitPointsOverride == *other.HitPointsOverride
	case e.AttackMultiplier != nil:
		return e.AttackMultiplier.Percent == other.AttackMultiplier.Percent && e.AttackMultiplier.Constraints.Equal(other.AttackMultiplier.Constraints)
	case e.NullifyReceivedDamage != nil:
		return e.NullifyReceivedDamage.Constraints.Equal(other.NullifyReceivedDamage.Constraints)
	case e.TollMultiplier != nil:
		return *e.TollMultiplier == *other.TollMultiplier
	case e.ReturnToHandOnDestruction != nil:
		return e.ReturnToHandOnDestruction.Destructions.Equal(other.ReturnToHandOnDestruction.Destructions)
	case e.GainMagicOnDestruction != nil:
		return e.GainMagicOnDestruction.Amount == other.GainMagicOnDestruction.Amount &&
			e.GainMagicOnDestruction.Destructions.Equal(other.GainMagicOnDestruction.Destructions)
	}
	return true
}

// 効果の並び。並びは入力のまま持つ（効果を当てる順・乱数を引く順になるため）。同じ種類を重ねて持ってよい（制約の「または」を
// 効果を並べて表すため）。どの種類を持てるか・同時に持てない組み合わせは持ち主の決まり。値の比較は Equal。無いときも nil でなく空にする
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

// その種類の効果を持っているか
func (e Effects) Has(kind Kind) bool {
	return slices.ContainsFunc(e, func(effect Effect) bool { return effect.Kind() == kind })
}

// 渡した種類のうち、持てない種類（最初の 1 つ）。すべて持てれば false
func (e Effects) NotAllowed(allowed []Kind) (Kind, bool) {
	for _, effect := range e {
		if !slices.Contains(allowed, effect.Kind()) {
			return effect.Kind(), true
		}
	}
	return "", false
}

// スペルの効果（core#178 で spell から移した。効果の置き場所をカードの種類でそろえるため。2 つ以上の効果を持つスペルを作るときに、
// 効果の並びにするかを決める）。種類ごとの型のどれか 1 つだけを持ち、効果の無いスペルはどれも持たない（2026-11-10 オーナー。種類ごとに型を分ける）。
// 種類は、対戦で使う場面を作るスライスごとに足す（core#116）。JSON はタグだけで、持っている種類の名前の下に値を入れる（{"draw_cards":{"count":1}}。無ければ {}）。
// スペルは使ったときに発動する。ターンをまたいで効き続ける効果（サイコロを複数振る・サイコロの目を固定する）も、使ったときに対象にかかる。
// 「呪い」という言葉は使わない（2026-11-13 オーナー。core#173）
type SpellEffect struct {
	DrawCards    *DrawCardsSpellEffect    `json:"draw_cards,omitempty"`
	GainMagic    *GainMagicSpellEffect    `json:"gain_magic,omitempty"`
	MultipleDice *MultipleDiceSpellEffect `json:"multiple_dice,omitempty"`
	FixedDice    *FixedDiceSpellEffect    `json:"fixed_dice,omitempty"`
	Damage       *DamageSpellEffect       `json:"damage,omitempty"`
}

// 効果なし（登録の直後の値）
func NoSpellEffect() SpellEffect {
	return SpellEffect{}
}

func (e SpellEffect) IsNone() bool {
	return e.DrawCards == nil && e.GainMagic == nil && e.MultipleDice == nil && e.FixedDice == nil && e.Damage == nil
}

// 対象のプレイヤー（自分を含む全員）を選ぶ効果か
func (e SpellEffect) TargetsPlayer() bool {
	return e.MultipleDice != nil || e.FixedDice != nil
}

// 対象のクリーチャー（自分の領地を含む、盤面の領地のクリーチャー。2026-11-11 オーナー）を選ぶ効果か
func (e SpellEffect) TargetsCreature() bool {
	return e.Damage != nil
}

// 中身で比べる（種類ごとの値をポインタで持つため == では比べられない）
func (e SpellEffect) Equal(other SpellEffect) bool {
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

func NewDrawCardsSpellEffect(count DrawCount) SpellEffect {
	return SpellEffect{DrawCards: &DrawCardsSpellEffect{Count: count}}
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

func NewGainMagicSpellEffect(perLap MagicPerLap) SpellEffect {
	return SpellEffect{GainMagic: &GainMagicSpellEffect{PerLap: perLap}}
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

func NewMultipleDiceSpellEffect(count DiceCount, turns EffectTurns) SpellEffect {
	return SpellEffect{MultipleDice: &MultipleDiceSpellEffect{Count: count, Turns: turns}}
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

func NewFixedDiceSpellEffect(value DiceFace, turns EffectTurns) SpellEffect {
	return SpellEffect{FixedDice: &FixedDiceSpellEffect{Value: value, Turns: turns}}
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

func NewDamageSpellEffect(amount DamageAmount) SpellEffect {
	return SpellEffect{Damage: &DamageSpellEffect{Amount: amount}}
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
