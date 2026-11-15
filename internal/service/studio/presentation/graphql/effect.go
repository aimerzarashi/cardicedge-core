package graphql

import (
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/presentation/graphql/model"
)

// 効果の変換はアイテムとクリーチャーで共有する（core#178）

var effectKinds = map[model.EffectKind]string{
	model.EffectKindAttackModifier:        command.EffectAttackModifier,
	model.EffectKindHitPointsModifier:     command.EffectHitPointsModifier,
	model.EffectKindAttackOverride:        command.EffectAttackOverride,
	model.EffectKindHitPointsOverride:     command.EffectHitPointsOverride,
	model.EffectKindAttackMultiplier:      command.EffectAttackMultiplier,
	model.EffectKindIgnoreTerrainEffect:   command.EffectIgnoreTerrainEffect,
	model.EffectKindStrikeFirst:           command.EffectStrikeFirst,
	model.EffectKindStrikeLast:            command.EffectStrikeLast,
	model.EffectKindNullifyReceivedDamage: command.EffectNullifyReceivedDamage,
	model.EffectKindTollMultiplier:        command.EffectTollMultiplier,
	model.EffectKindForbidInvasion:        command.EffectForbidInvasion,
	model.EffectKindForbidMovement:        command.EffectForbidMovement,
}

func fromEffectKind(kind string) (model.EffectKind, error) {
	for value, name := range effectKinds {
		if name == kind {
			return value, nil
		}
	}
	return "", fmt.Errorf("graphql: unknown effect kind %q", kind)
}

func fromEffectKinds(kinds []string) ([]model.EffectKind, error) {
	values := make([]model.EffectKind, len(kinds))
	for i, kind := range kinds {
		value, err := fromEffectKind(kind)
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	return values, nil
}

// 値はどの種類でもそのまま渡し、種類と値の組み合わせは command が確かめる
func toEffects(effects []*model.EffectInput) ([]command.Effect, error) {
	values := make([]command.Effect, len(effects))
	for i, effect := range effects {
		kind, ok := effectKinds[effect.Kind]
		if !ok {
			return nil, fmt.Errorf("graphql: unknown effect kind %q", effect.Kind)
		}
		var constraints []command.EffectConstraint
		if effect.Constraints != nil {
			constraints = make([]command.EffectConstraint, len(effect.Constraints))
			for j, constraint := range effect.Constraints {
				if constraint.Kind != model.EffectConstraintKindOpponentCreatureElement {
					return nil, fmt.Errorf("graphql: unknown effect constraint kind %q", constraint.Kind)
				}
				elements, err := toElements(constraint.Elements)
				if err != nil {
					return nil, err
				}
				constraints[j] = command.EffectConstraint{Kind: command.EffectConstraintOpponentCreatureElement, Elements: elements}
			}
		}
		values[i] = command.Effect{Kind: kind, Value: effect.Value, Minimum: effect.Minimum, Maximum: effect.Maximum, Percent: effect.Percent, Constraints: constraints}
	}
	return values, nil
}

func fromEffects(effects []command.Effect) ([]model.Effect, error) {
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

func fromEffect(effect command.Effect) (model.Effect, error) {
	kind, err := fromEffectKind(effect.Kind)
	if err != nil {
		return nil, err
	}
	switch effect.Kind {
	case command.EffectAttackModifier:
		return &model.AttackModifierEffect{Kind: kind, Value: *effect.Value}, nil
	case command.EffectHitPointsModifier:
		return &model.HitPointsModifierEffect{Kind: kind, Value: *effect.Value}, nil
	case command.EffectAttackOverride:
		return &model.AttackOverrideEffect{Kind: kind, Minimum: *effect.Minimum, Maximum: *effect.Maximum}, nil
	case command.EffectHitPointsOverride:
		return &model.HitPointsOverrideEffect{Kind: kind, Minimum: *effect.Minimum, Maximum: *effect.Maximum}, nil
	case command.EffectAttackMultiplier:
		constraints, err := fromEffectConstraints(effect.Constraints)
		if err != nil {
			return nil, err
		}
		return &model.AttackMultiplierEffect{Kind: kind, Percent: *effect.Percent, Constraints: constraints}, nil
	case command.EffectIgnoreTerrainEffect:
		return &model.IgnoreTerrainEffect{Kind: kind}, nil
	case command.EffectStrikeFirst:
		return &model.StrikeFirstEffect{Kind: kind}, nil
	case command.EffectStrikeLast:
		return &model.StrikeLastEffect{Kind: kind}, nil
	case command.EffectNullifyReceivedDamage:
		constraints, err := fromEffectConstraints(effect.Constraints)
		if err != nil {
			return nil, err
		}
		return &model.NullifyReceivedDamageEffect{Kind: kind, Constraints: constraints}, nil
	case command.EffectTollMultiplier:
		return &model.TollMultiplierEffect{Kind: kind, Percent: *effect.Percent}, nil
	case command.EffectForbidInvasion:
		return &model.ForbidInvasionEffect{Kind: kind}, nil
	}
	return &model.ForbidMovementEffect{Kind: kind}, nil
}

func fromEffectConstraints(constraints []command.EffectConstraint) ([]model.EffectConstraint, error) {
	values := make([]model.EffectConstraint, len(constraints))
	for i, constraint := range constraints {
		if constraint.Kind != command.EffectConstraintOpponentCreatureElement {
			return nil, fmt.Errorf("graphql: unknown effect constraint kind %q", constraint.Kind)
		}
		elements, err := fromElements(constraint.Elements)
		if err != nil {
			return nil, err
		}
		values[i] = &model.OpponentCreatureElementConstraint{Kind: model.EffectConstraintKindOpponentCreatureElement, Elements: elements}
	}
	return values, nil
}

// read model の効果を、command の効果の形にそろえてから GraphQL の型にする（変換を 1 か所にするため）
func fromEffectsMetadata(effects []readmodel.EffectMetadata) ([]model.Effect, error) {
	values := make([]command.Effect, len(effects))
	for i, metadata := range effects {
		value, err := effectOfMetadata(metadata)
		if err != nil {
			return nil, fmt.Errorf("graphql: effect %d: %w", i, err)
		}
		values[i] = value
	}
	return fromEffects(values)
}

func effectOfMetadata(metadata readmodel.EffectMetadata) (command.Effect, error) {
	switch {
	case metadata.AttackModifier != nil:
		return command.Effect{Kind: command.EffectAttackModifier, Value: &metadata.AttackModifier.Value}, nil
	case metadata.HitPointsModifier != nil:
		return command.Effect{Kind: command.EffectHitPointsModifier, Value: &metadata.HitPointsModifier.Value}, nil
	case metadata.AttackOverride != nil:
		return command.Effect{Kind: command.EffectAttackOverride, Minimum: &metadata.AttackOverride.Minimum, Maximum: &metadata.AttackOverride.Maximum}, nil
	case metadata.HitPointsOverride != nil:
		return command.Effect{Kind: command.EffectHitPointsOverride, Minimum: &metadata.HitPointsOverride.Minimum, Maximum: &metadata.HitPointsOverride.Maximum}, nil
	case metadata.AttackMultiplier != nil:
		return command.Effect{Kind: command.EffectAttackMultiplier, Percent: &metadata.AttackMultiplier.Percent, Constraints: constraintsOfMetadata(metadata.AttackMultiplier.Constraints)}, nil
	case metadata.IgnoreTerrainEffect != nil:
		return command.Effect{Kind: command.EffectIgnoreTerrainEffect}, nil
	case metadata.StrikeFirst != nil:
		return command.Effect{Kind: command.EffectStrikeFirst}, nil
	case metadata.StrikeLast != nil:
		return command.Effect{Kind: command.EffectStrikeLast}, nil
	case metadata.NullifyReceivedDamage != nil:
		return command.Effect{Kind: command.EffectNullifyReceivedDamage, Constraints: constraintsOfMetadata(metadata.NullifyReceivedDamage.Constraints)}, nil
	case metadata.TollMultiplier != nil:
		return command.Effect{Kind: command.EffectTollMultiplier, Percent: &metadata.TollMultiplier.Percent}, nil
	case metadata.ForbidInvasion != nil:
		return command.Effect{Kind: command.EffectForbidInvasion}, nil
	case metadata.ForbidMovement != nil:
		return command.Effect{Kind: command.EffectForbidMovement}, nil
	}
	return command.Effect{}, fmt.Errorf("graphql: effect has no kind")
}

func constraintsOfMetadata(constraints []readmodel.ConstraintMetadata) []command.EffectConstraint {
	values := make([]command.EffectConstraint, 0, len(constraints))
	for _, constraint := range constraints {
		if constraint.OpponentCreatureElement != nil {
			values = append(values, command.EffectConstraint{Kind: command.EffectConstraintOpponentCreatureElement, Elements: constraint.OpponentCreatureElement.Elements})
		}
	}
	return values
}

func allowedItemEffectKinds(category string) ([]model.EffectKind, error) {
	kinds, err := query.ItemAllowedEffectKinds(category)
	if err != nil {
		return nil, err
	}
	return fromEffectKinds(kinds)
}

func allowedCreatureEffectKinds() ([]model.EffectKind, error) {
	return fromEffectKinds(query.CreatureAllowedEffectKinds())
}

// 配置制限（土地の属性。core#178 で creature の属性の型から土地の属性の型に直した）
func fromLandElements(values []string) ([]model.BoardTileElement, error) {
	elements := make([]model.BoardTileElement, len(values))
	for i, value := range values {
		land, err := readmodel.NewBoardTileElement(value)
		if err != nil {
			return nil, err
		}
		element, err := fromBoardTileElement(land)
		if err != nil {
			return nil, err
		}
		elements[i] = element
	}
	return elements, nil
}

func toLandElements(elements []model.BoardTileElement) ([]string, error) {
	values := make([]string, len(elements))
	for i, element := range elements {
		value, err := toBoardTileElement(element)
		if err != nil {
			return nil, err
		}
		values[i] = string(value)
	}
	return values, nil
}
