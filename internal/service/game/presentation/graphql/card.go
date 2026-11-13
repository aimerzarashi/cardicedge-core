package graphql

import (
	"fmt"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/presentation/graphql/model"
)

// 種類ごとの型にする（種類が増えて写し忘れたら、黙って落とさずエラーにする）
func fromCard(cardType query.CardType, id uuid.UUID, name string, cost int, rarity string, creature *query.CreatureSpec, item *query.ItemSpec, spell *query.SpellSpec) (model.CardInfo, error) {
	converted, err := fromCardRarity(rarity)
	if err != nil {
		return nil, err
	}
	switch cardType {
	case query.CardTypeCreature:
		// 写した cards.metadata の読み出しが弾くので、無いのはコードの誤り（空の性能を黙って返さない）
		if creature == nil {
			return nil, fmt.Errorf("graphql: creature card %s has no creature spec", id)
		}
		elements, err := fromElements(creature.Elements)
		if err != nil {
			return nil, err
		}
		itemRestrictions := make([]model.ItemCategory, len(creature.ItemRestrictions))
		for i, value := range creature.ItemRestrictions {
			category, err := fromItemCategory(value)
			if err != nil {
				return nil, err
			}
			itemRestrictions[i] = category
		}
		placementRestrictions, err := fromElements(creature.PlacementRestrictions)
		if err != nil {
			return nil, err
		}
		return &model.CreatureCardInfo{
			ID: id, Name: name, Cost: cost, Rarity: converted,
			Elements: elements, Attack: creature.Attack, HitPoints: creature.HitPoints, ItemRestrictions: itemRestrictions, PlacementRestrictions: placementRestrictions, Abilities: fromCreatureAbilities(creature.Abilities),
		}, nil
	case query.CardTypeItem:
		// creature と同じく、読み出しが弾くので、無いのはコードの誤り
		if item == nil {
			return nil, fmt.Errorf("graphql: item card %s has no item spec", id)
		}
		category, err := fromItemCategory(item.Category)
		if err != nil {
			return nil, err
		}
		return &model.ItemCardInfo{ID: id, Name: name, Cost: cost, Rarity: converted, Category: category, AttackModifier: item.AttackModifier, HitPointsModifier: item.HitPointsModifier}, nil
	case query.CardTypeSpell:
		// creature と同じく、読み出しが弾くので、無いのはコードの誤り
		if spell == nil {
			return nil, fmt.Errorf("graphql: spell card %s has no spell spec", id)
		}
		return &model.SpellCardInfo{ID: id, Name: name, Cost: cost, Rarity: converted, Effect: fromSpellEffect(spell.Effect)}, nil
	default:
		return nil, fmt.Errorf("graphql: unknown card type: %d", cardType)
	}
}

// 効果なしは null（種類ごとの型のどれも返さない）
func fromSpellEffect(effect query.SpellEffect) model.SpellEffect {
	switch {
	case effect.DrawCards != nil:
		return &model.DrawCardsSpellEffect{Count: effect.DrawCards.Count}
	case effect.GainMagic != nil:
		return &model.GainMagicSpellEffect{PerLap: effect.GainMagic.PerLap}
	case effect.MultipleDice != nil:
		return &model.MultipleDiceSpellEffect{Count: effect.MultipleDice.Count, Turns: effect.MultipleDice.Turns}
	case effect.FixedDice != nil:
		return &model.FixedDiceSpellEffect{Value: effect.FixedDice.Value, Turns: effect.FixedDice.Turns}
	case effect.Damage != nil:
		return &model.DamageSpellEffect{Amount: effect.Damage.Amount}
	default:
		return nil
	}
}

// 値の対応を明示する（fromCardRarity と同じ理由）
func fromItemCategory(value string) (model.ItemCategory, error) {
	switch value {
	case "weapon":
		return model.ItemCategoryWeapon, nil
	case "armor":
		return model.ItemCategoryArmor, nil
	case "scroll":
		return model.ItemCategoryScroll, nil
	default:
		return "", fmt.Errorf("graphql: unknown item category: %q", value)
	}
}

// 値の対応を明示する（文字列から型を変換すると、対応が名前の一致に隠れるため）
func fromCardRarity(rarity string) (model.CardRarity, error) {
	switch rarity {
	case "normal":
		return model.CardRarityNormal, nil
	case "strange":
		return model.CardRarityStrange, nil
	case "rare":
		return model.CardRarityRare, nil
	case "extra":
		return model.CardRarityExtra, nil
	default:
		return "", fmt.Errorf("graphql: unknown card rarity: %q", rarity)
	}
}

// 値の対応を明示する（fromCardRarity と同じ理由）
func fromElements(values []string) ([]model.CreatureElement, error) {
	elements := make([]model.CreatureElement, len(values))
	for i, value := range values {
		switch value {
		case "fire":
			elements[i] = model.CreatureElementFire
		case "earth":
			elements[i] = model.CreatureElementEarth
		case "water":
			elements[i] = model.CreatureElementWater
		case "wind":
			elements[i] = model.CreatureElementWind
		case "neutral":
			elements[i] = model.CreatureElementNeutral
		default:
			return nil, fmt.Errorf("graphql: unknown element: %q", value)
		}
	}
	return elements, nil
}

func fromCreatureAbilities(abilities []query.CreatureAbility) []model.CreatureAbility {
	values := make([]model.CreatureAbility, len(abilities))
	for i, ability := range abilities {
		switch {
		case ability.Defensive:
			values[i] = &model.CreatureDefensiveAbility{Kind: model.CreatureAbilityKindDefensive}
		case ability.TollMultiplierPercent != nil:
			values[i] = &model.CreatureTollMultiplierAbility{Kind: model.CreatureAbilityKindTollMultiplier, Percent: *ability.TollMultiplierPercent}
		case ability.BattleModifier != nil:
			values[i] = &model.CreatureBattleModifierAbility{Kind: model.CreatureAbilityKindBattleModifier, AttackModifier: ability.BattleModifier.AttackModifier, HitPointsModifier: ability.BattleModifier.HitPointsModifier}
		case ability.BattleRandomStats != nil:
			values[i] = &model.CreatureBattleRandomStatsAbility{Kind: model.CreatureAbilityKindBattleRandomStats, Minimum: ability.BattleRandomStats.Minimum, Maximum: ability.BattleRandomStats.Maximum}
		case ability.FirstStrike:
			values[i] = &model.CreatureFirstStrikeAbility{Kind: model.CreatureAbilityKindFirstStrike}
		case ability.LastStrike:
			values[i] = &model.CreatureLastStrikeAbility{Kind: model.CreatureAbilityKindLastStrike}
		}
	}
	return values
}
