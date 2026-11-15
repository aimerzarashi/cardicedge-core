package graphql

import (
	"errors"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/presentation/graphql/model"
)

// 効果の変換はアイテムとクリーチャーで共有する（core#178）

func fromEffects(effects []query.Effect) ([]model.Effect, error) {
	values := make([]model.Effect, len(effects))
	for i, effect := range effects {
		value, err := fromEffect(effect)
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	return values, nil
}

func fromEffect(effect query.Effect) (model.Effect, error) {
	switch {
	case effect.AttackModifier != nil:
		return &model.AttackModifierEffect{Kind: model.EffectKindAttackModifier, Value: *effect.AttackModifier}, nil
	case effect.HitPointsModifier != nil:
		return &model.HitPointsModifierEffect{Kind: model.EffectKindHitPointsModifier, Value: *effect.HitPointsModifier}, nil
	case effect.AttackOverride != nil:
		return &model.AttackOverrideEffect{Kind: model.EffectKindAttackOverride, Minimum: effect.AttackOverride.Minimum, Maximum: effect.AttackOverride.Maximum}, nil
	case effect.HitPointsOverride != nil:
		return &model.HitPointsOverrideEffect{Kind: model.EffectKindHitPointsOverride, Minimum: effect.HitPointsOverride.Minimum, Maximum: effect.HitPointsOverride.Maximum}, nil
	case effect.AttackMultiplier != nil:
		constraints, err := fromEffectConstraints(effect.AttackMultiplier.Constraints)
		if err != nil {
			return nil, err
		}
		return &model.AttackMultiplierEffect{Kind: model.EffectKindAttackMultiplier, Percent: effect.AttackMultiplier.Percent, Constraints: constraints}, nil
	case effect.IgnoreTerrainEffect:
		return &model.IgnoreTerrainEffect{Kind: model.EffectKindIgnoreTerrainEffect}, nil
	case effect.StrikeFirst:
		return &model.StrikeFirstEffect{Kind: model.EffectKindStrikeFirst}, nil
	case effect.StrikeLast:
		return &model.StrikeLastEffect{Kind: model.EffectKindStrikeLast}, nil
	case effect.NullifyReceivedDamage != nil:
		constraints, err := fromEffectConstraints(effect.NullifyReceivedDamage.Constraints)
		if err != nil {
			return nil, err
		}
		return &model.NullifyReceivedDamageEffect{Kind: model.EffectKindNullifyReceivedDamage, Constraints: constraints}, nil
	case effect.TollMultiplierPercent != nil:
		return &model.TollMultiplierEffect{Kind: model.EffectKindTollMultiplier, Percent: *effect.TollMultiplierPercent}, nil
	case effect.ForbidInvasion:
		return &model.ForbidInvasionEffect{Kind: model.EffectKindForbidInvasion}, nil
	case effect.ForbidMovement:
		return &model.ForbidMovementEffect{Kind: model.EffectKindForbidMovement}, nil
	}
	return nil, errors.New("graphql: effect has no kind")
}

func fromEffectConstraints(constraints []query.EffectConstraint) ([]model.EffectConstraint, error) {
	values := make([]model.EffectConstraint, len(constraints))
	for i, constraint := range constraints {
		if constraint.OpponentCreatureElements == nil {
			return nil, fmt.Errorf("graphql: effect constraint %d has no kind", i)
		}
		elements, err := fromElements(constraint.OpponentCreatureElements)
		if err != nil {
			return nil, err
		}
		values[i] = &model.OpponentCreatureElementConstraint{Kind: model.EffectConstraintKindOpponentCreatureElement, Elements: elements}
	}
	return values, nil
}

// 配置制限（土地の属性。core#178 でクリーチャーの属性の型から土地の属性の型に直した）
func fromLandElements(values []string) ([]model.BoardTileElement, error) {
	elements := make([]model.BoardTileElement, len(values))
	for i, value := range values {
		land, err := query.NewBoardTileElement(value)
		if err != nil {
			return nil, err
		}
		if elements[i], err = fromBoardTileElement(land); err != nil {
			return nil, err
		}
	}
	return elements, nil
}
