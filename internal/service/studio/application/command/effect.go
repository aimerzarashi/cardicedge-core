package command

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/effect"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

// 効果の種類（domain の種類の名前と同じ）。アイテムとクリーチャーで共有する（効果の型を共有するため。core#178）
const (
	EffectAttackModifier            = "attack_modifier"
	EffectHitPointsModifier         = "hit_points_modifier"
	EffectAttackOverride            = "attack_override"
	EffectHitPointsOverride         = "hit_points_override"
	EffectAttackMultiplier          = "attack_multiplier"
	EffectIgnoreTerrainEffect       = "ignore_terrain_effect"
	EffectStrikeFirst               = "strike_first"
	EffectStrikeLast                = "strike_last"
	EffectNullifyReceivedDamage     = "nullify_received_damage"
	EffectTollMultiplier            = "toll_multiplier"
	EffectForbidInvasion            = "forbid_invasion"
	EffectForbidMovement            = "forbid_movement"
	EffectReturnToHandOnDestruction = "return_to_hand_on_destruction"
	EffectGainMagicOnDestruction    = "gain_magic_on_destruction"
	EffectUseCreatureAsItem         = "use_creature_as_item"
)

// 効果の制約の種類（domain の種類の名前と同じ）
const EffectConstraintOpponentCreatureElement = "opponent_creature_element"

// 効果の入力と出力。種類と、種類に要る値（増減は Value、上書きは Minimum と Maximum、AT の倍率は Percent と Constraints、
// 通行料の倍率は Percent、受けるダメージを 0 にするは Constraints、破壊されたら手札に戻るは Destructions、
// 破壊されたら魔力を得るは Destructions と Amount、手札のクリーチャーをアイテムとして使えるは Elements）
type Effect struct {
	Kind         string
	Value        *int
	Minimum      *int
	Maximum      *int
	Percent      *int
	Constraints  []EffectConstraint
	Destructions []string
	Amount       *int
	Elements     []string
}

// 効果の制約の入力と出力。種類と、種類に要る値（相手のクリーチャーの属性は Elements）
type EffectConstraint struct {
	Kind     string
	Elements []string
}

// 効果の並びを domain の値にする。種類と値の組み合わせが合わないものは effects.N の InvalidFormat、値の範囲の誤りは値の項目の誤り
// （effects.N.value・minimum・maximum・percent、制約は effects.N.constraints.M・effects.N.constraints.M.elements）。誤りはまとめて返す。
// どの種類を持てるかは持ち主の決まりで、ここでは確かめない
func newEffects(inputs []Effect) (effect.Effects, input.Errors) {
	values := make([]effect.Effect, 0, len(inputs))
	invalid := input.Errors{}
	for i, value := range inputs {
		converted, errs := value.domain(fmt.Sprintf("effects.%d", i))
		invalid = append(invalid, errs...)
		values = append(values, converted)
	}
	if len(invalid) > 0 {
		return nil, invalid
	}
	effects, err := effect.NewEffects(values)
	if err != nil {
		return nil, input.Errors{&input.InvalidFormat{Field: "effects"}}
	}
	return effects, nil
}

// 種類ごとに要る値の組（ほかの値が入っていれば形の誤り）
type effectValues struct {
	value, rangeValues, percent, constraints, destructions, amount, elements bool
}

var effectShapes = map[string]effectValues{
	EffectAttackModifier:            {value: true},
	EffectHitPointsModifier:         {value: true},
	EffectAttackOverride:            {rangeValues: true},
	EffectHitPointsOverride:         {rangeValues: true},
	EffectAttackMultiplier:          {percent: true, constraints: true},
	EffectIgnoreTerrainEffect:       {},
	EffectStrikeFirst:               {},
	EffectStrikeLast:                {},
	EffectNullifyReceivedDamage:     {constraints: true},
	EffectTollMultiplier:            {percent: true},
	EffectForbidInvasion:            {},
	EffectForbidMovement:            {},
	EffectReturnToHandOnDestruction: {destructions: true},
	EffectGainMagicOnDestruction:    {destructions: true, amount: true},
	EffectUseCreatureAsItem:         {elements: true},
}

func (e Effect) matches(shape effectValues) bool {
	return (e.Value != nil) == shape.value && (e.Minimum != nil) == shape.rangeValues && (e.Maximum != nil) == shape.rangeValues &&
		(e.Percent != nil) == shape.percent && (shape.constraints || e.Constraints == nil) &&
		(e.Destructions != nil) == shape.destructions && (e.Amount != nil) == shape.amount && (e.Elements != nil) == shape.elements
}

func (e Effect) domain(field string) (effect.Effect, input.Errors) {
	shape, ok := effectShapes[e.Kind]
	if !ok || !e.matches(shape) {
		return effect.Effect{}, input.Errors{&input.InvalidFormat{Field: field}}
	}
	switch e.Kind {
	case EffectAttackModifier:
		return effect.NewAttackModifierEffect(effect.NewAttackModifier(*e.Value)), nil
	case EffectHitPointsModifier:
		return effect.NewHitPointsModifierEffect(effect.NewHitPointsModifier(*e.Value)), nil
	case EffectAttackOverride:
		return e.attackOverride(field)
	case EffectHitPointsOverride:
		return e.hitPointsOverride(field)
	case EffectAttackMultiplier:
		percent, err := effect.NewAttackMultiplierPercent(*e.Percent)
		invalid := input.Errors{}
		if err != nil {
			invalid = append(invalid, &input.TooSmall{Field: field + ".percent", Min: 1})
		}
		constraints, invalidConstraints := newConstraints(field, e.Constraints)
		if invalid = append(invalid, invalidConstraints...); len(invalid) > 0 {
			return effect.Effect{}, invalid
		}
		return effect.NewAttackMultiplierEffect(percent, constraints), nil
	case EffectNullifyReceivedDamage:
		constraints, invalid := newConstraints(field, e.Constraints)
		if len(invalid) > 0 {
			return effect.Effect{}, invalid
		}
		return effect.NewNullifyReceivedDamageEffect(constraints), nil
	case EffectTollMultiplier:
		percent, err := effect.NewTollMultiplierPercent(*e.Percent)
		if err != nil {
			return effect.Effect{}, input.Errors{&input.TooSmall{Field: field + ".percent", Min: 1}}
		}
		return effect.NewTollMultiplierEffect(percent), nil
	case EffectReturnToHandOnDestruction:
		destructions, err := effect.NewDestructions(e.Destructions)
		if err != nil {
			return effect.Effect{}, input.Errors{&input.InvalidFormat{Field: field + ".destructions"}}
		}
		return effect.NewReturnToHandOnDestructionEffect(destructions), nil
	case EffectGainMagicOnDestruction:
		return e.gainMagicOnDestruction(field)
	case EffectUseCreatureAsItem:
		// 属性の誤り（空・知らない属性・重複）は、制約の属性と同じく elements の 1 つの誤りにする
		elements, err := effect.NewCreatureElements(e.Elements)
		if err != nil {
			return effect.Effect{}, input.Errors{&input.InvalidFormat{Field: field + ".elements"}}
		}
		return effect.NewUseCreatureAsItemEffect(elements), nil
	case EffectIgnoreTerrainEffect:
		return effect.NewIgnoreTerrainEffect(), nil
	case EffectStrikeFirst:
		return effect.NewStrikeFirstEffect(), nil
	case EffectStrikeLast:
		return effect.NewStrikeLastEffect(), nil
	case EffectForbidInvasion:
		return effect.NewForbidInvasionEffect(), nil
	}
	return effect.NewForbidMovementEffect(), nil
}

// 発動する破壊の誤り（空・知らない破壊・重複）は destructions の 1 つの誤り、額の下限の誤りは amount の誤りにし、まとめて返す
func (e Effect) gainMagicOnDestruction(field string) (effect.Effect, input.Errors) {
	invalid := input.Errors{}
	destructions, err := effect.NewDestructions(e.Destructions)
	if err != nil {
		invalid = append(invalid, &input.InvalidFormat{Field: field + ".destructions"})
	}
	amount, err := effect.NewDestructionMagicAmount(*e.Amount)
	if err != nil {
		invalid = append(invalid, &input.TooSmall{Field: field + ".amount", Min: 1})
	}
	if len(invalid) > 0 {
		return effect.Effect{}, invalid
	}
	return effect.NewGainMagicOnDestructionEffect(destructions, amount), nil
}

// 最小の下限の誤りと、最大が最小より小さい誤りは、それぞれの項目に返す（最大の下限は最小）
func (e Effect) attackOverride(field string) (effect.Effect, input.Errors) {
	minimum, err := effect.NewAttackOverride(*e.Minimum)
	if err != nil {
		return effect.Effect{}, input.Errors{&input.TooSmall{Field: field + ".minimum", Min: 0}}
	}
	maximum, err := effect.NewAttackOverride(*e.Maximum)
	if err != nil {
		return effect.Effect{}, input.Errors{&input.TooSmall{Field: field + ".maximum", Min: minimum.Int()}}
	}
	value, err := effect.NewAttackOverrideEffect(minimum, maximum)
	if err != nil {
		return effect.Effect{}, input.Errors{&input.TooSmall{Field: field + ".maximum", Min: minimum.Int()}}
	}
	return value, nil
}

func (e Effect) hitPointsOverride(field string) (effect.Effect, input.Errors) {
	minimum, err := effect.NewHitPointsOverride(*e.Minimum)
	if err != nil {
		return effect.Effect{}, input.Errors{&input.TooSmall{Field: field + ".minimum", Min: 1}}
	}
	maximum, err := effect.NewHitPointsOverride(*e.Maximum)
	if err != nil {
		return effect.Effect{}, input.Errors{&input.TooSmall{Field: field + ".maximum", Min: minimum.Int()}}
	}
	value, err := effect.NewHitPointsOverrideEffect(minimum, maximum)
	if err != nil {
		return effect.Effect{}, input.Errors{&input.TooSmall{Field: field + ".maximum", Min: minimum.Int()}}
	}
	return value, nil
}

func newConstraints(field string, inputs []EffectConstraint) (effect.Constraints, input.Errors) {
	invalid := input.Errors{}
	values := make([]effect.Constraint, 0, len(inputs))
	for i, constraint := range inputs {
		value, errs := constraint.domain(fmt.Sprintf("%s.constraints.%d", field, i))
		invalid = append(invalid, errs...)
		values = append(values, value)
	}
	if len(invalid) > 0 {
		return nil, invalid
	}
	constraints, err := effect.NewConstraints(values)
	if err != nil {
		return nil, input.Errors{&input.InvalidFormat{Field: field}}
	}
	return constraints, nil
}

// 属性の誤り（空・知らない属性・重複）は、どれも elements の 1 つの誤りにする
func (c EffectConstraint) domain(field string) (effect.Constraint, input.Errors) {
	if c.Kind != EffectConstraintOpponentCreatureElement {
		return effect.Constraint{}, input.Errors{&input.InvalidFormat{Field: field}}
	}
	elements, err := effect.NewCreatureElements(c.Elements)
	if err != nil {
		return effect.Constraint{}, input.Errors{&input.InvalidFormat{Field: field + ".elements"}}
	}
	return effect.NewOpponentCreatureElementConstraint(elements), nil
}

func effectsOf(effects effect.Effects) []Effect {
	values := make([]Effect, len(effects))
	for i, value := range effects {
		values[i] = effectOf(value)
	}
	return values
}

func effectOf(value effect.Effect) Effect {
	converted := Effect{Kind: value.Kind().String()}
	switch {
	case value.AttackModifier != nil:
		converted.Value = new(value.AttackModifier.Value.Int())
	case value.HitPointsModifier != nil:
		converted.Value = new(value.HitPointsModifier.Value.Int())
	case value.AttackOverride != nil:
		converted.Minimum, converted.Maximum = new(value.AttackOverride.Minimum.Int()), new(value.AttackOverride.Maximum.Int())
	case value.HitPointsOverride != nil:
		converted.Minimum, converted.Maximum = new(value.HitPointsOverride.Minimum.Int()), new(value.HitPointsOverride.Maximum.Int())
	case value.AttackMultiplier != nil:
		converted.Percent = new(value.AttackMultiplier.Percent.Int())
		converted.Constraints = constraintsOf(value.AttackMultiplier.Constraints)
	case value.NullifyReceivedDamage != nil:
		converted.Constraints = constraintsOf(value.NullifyReceivedDamage.Constraints)
	case value.TollMultiplier != nil:
		converted.Percent = new(value.TollMultiplier.Percent.Int())
	case value.ReturnToHandOnDestruction != nil:
		converted.Destructions = destructionsOf(value.ReturnToHandOnDestruction.Destructions)
	case value.GainMagicOnDestruction != nil:
		converted.Destructions = destructionsOf(value.GainMagicOnDestruction.Destructions)
		converted.Amount = new(value.GainMagicOnDestruction.Amount.Int())
	case value.UseCreatureAsItem != nil:
		converted.Elements = creatureElementsOf(value.UseCreatureAsItem.Elements)
	}
	return converted
}

func destructionsOf(destructions effect.Destructions) []string {
	values := make([]string, len(destructions))
	for i, destruction := range destructions {
		values[i] = string(destruction)
	}
	return values
}

func constraintsOf(constraints effect.Constraints) []EffectConstraint {
	values := make([]EffectConstraint, len(constraints))
	for i, constraint := range constraints {
		values[i] = EffectConstraint{Kind: EffectConstraintOpponentCreatureElement, Elements: creatureElementsOf(constraint.OpponentCreatureElement.Elements)}
	}
	return values
}

func creatureElementsOf(elements effect.CreatureElements) []string {
	values := make([]string, len(elements))
	for i, creatureElement := range elements {
		values[i] = creatureElement.String()
	}
	return values
}
