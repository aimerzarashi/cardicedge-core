package graphql

import (
	"errors"
	"fmt"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/presentation/graphql/model"
)

// types の null（絞り込まない）と空のリスト（入力の誤り）は、application 層が見分けるため区別して渡す
func toCardFilter(filter *model.CardFilter) (query.CardFilter, error) {
	if filter == nil {
		return query.CardFilter{}, nil
	}
	var types []readmodel.CardType
	if filter.Types != nil {
		types = make([]readmodel.CardType, 0, len(filter.Types))
		for _, cardType := range filter.Types {
			converted, err := toCardType(cardType)
			if err != nil {
				return query.CardFilter{}, err
			}
			types = append(types, converted)
		}
	}
	var statuses []readmodel.CardStatus
	if filter.Statuses != nil {
		statuses = make([]readmodel.CardStatus, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			converted, err := toCardStatus(status)
			if err != nil {
				return query.CardFilter{}, err
			}
			statuses = append(statuses, converted)
		}
	}
	return query.CardFilter{NameContains: filter.NameContains, Types: types, Statuses: statuses}, nil
}

// toCardType と同じく、enum の値と状態の対応を明示する
func toCardStatus(status model.CardStatus) (readmodel.CardStatus, error) {
	switch status {
	case model.CardStatusDraft:
		return readmodel.CardStatusDraft, nil
	case model.CardStatusPublished:
		return readmodel.CardStatusPublished, nil
	case model.CardStatusRetired:
		return readmodel.CardStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown card status: %s", status)
	}
}

func fromCardStatus(status readmodel.CardStatus) (model.CardStatus, error) {
	switch status {
	case readmodel.CardStatusDraft:
		return model.CardStatusDraft, nil
	case readmodel.CardStatusPublished:
		return model.CardStatusPublished, nil
	case readmodel.CardStatusRetired:
		return model.CardStatusRetired, nil
	default:
		return "", fmt.Errorf("graphql: unknown card status: %q", status)
	}
}

// enum の値と種類の対応を明示する（文字列から型を変換すると、対応が名前の一致に隠れるため）
func toCardType(cardType model.CardType) (readmodel.CardType, error) {
	switch cardType {
	case model.CardTypeCreature:
		return readmodel.CardTypeCreature, nil
	case model.CardTypeItem:
		return readmodel.CardTypeItem, nil
	case model.CardTypeSpell:
		return readmodel.CardTypeSpell, nil
	default:
		return 0, fmt.Errorf("graphql: unknown card type: %s", cardType)
	}
}

func toCardConnection(connection pagination.Connection[query.Card]) (*model.CardConnection, error) {
	edges := make([]*model.CardEdge, 0, len(connection.Edges))
	for _, edge := range connection.Edges {
		node, err := toCard(edge.Node)
		if err != nil {
			return nil, err
		}
		edges = append(edges, &model.CardEdge{Cursor: edge.Cursor, Node: node})
	}
	pageInfo := &model.PageInfo{
		HasNextPage:     connection.PageInfo.HasNextPage,
		HasPreviousPage: connection.PageInfo.HasPreviousPage,
		StartCursor:     connection.PageInfo.StartCursor,
		EndCursor:       connection.PageInfo.EndCursor,
	}
	return &model.CardConnection{Edges: edges, PageInfo: pageInfo}, nil
}

// 3 種類すべてに GraphQL の型があるが、CardType は整数で、switch の漏れをコンパイラが見つけない。
// default を黙って nil にすると、種類を足したときの足し忘れが一覧から消えるだけで気づけないため、エラーにする
func toCard(card query.Card) (model.Card, error) {
	status, err := fromCardStatus(card.Status)
	if err != nil {
		return nil, err
	}
	rarity, err := fromCardRarity(card.Rarity)
	if err != nil {
		return nil, err
	}
	revision := int(card.Revision)
	switch card.Type {
	case readmodel.CardTypeCreature:
		// 投影が必ず書く値なので、無ければ read model の不整合（黙って空にしない）
		if card.Creature == nil {
			return nil, fmt.Errorf("graphql: creature %s has no creature spec", card.ID)
		}
		elements, err := fromElements(card.Creature.Elements)
		if err != nil {
			return nil, err
		}
		itemRestrictions, err := fromItemCategories(card.Creature.ItemRestrictions)
		if err != nil {
			return nil, err
		}
		placementRestrictions, err := fromElements(card.Creature.PlacementRestrictions)
		if err != nil {
			return nil, err
		}
		return &model.Creature{
			ID: card.ID, Name: card.Name, Status: status, Cost: card.Cost, Rarity: rarity,
			Elements: elements, Attack: card.Creature.Attack, HitPoints: card.Creature.HitPoints, ItemRestrictions: itemRestrictions, PlacementRestrictions: placementRestrictions, Abilities: fromCreatureAbilitiesMetadata(card.Creature.Abilities), Revision: revision,
			RegisteredAt: card.RegisteredAt, UpdatedAt: card.UpdatedAt,
		}, nil
	case readmodel.CardTypeItem:
		// 投影が必ず書く値なので、無ければ read model の不整合（黙って空にしない）
		if card.Item == nil {
			return nil, fmt.Errorf("graphql: item %s has no item spec", card.ID)
		}
		category, err := fromItemCategory(card.Item.Category)
		if err != nil {
			return nil, err
		}
		return &model.Item{
			ID: card.ID, Name: card.Name, Status: status, Cost: card.Cost, Rarity: rarity, Revision: revision,
			Category: category, AttackBonus: card.Item.AttackBonus, HitPointsBonus: card.Item.HitPointsBonus,
			RegisteredAt: card.RegisteredAt, UpdatedAt: card.UpdatedAt,
		}, nil
	case readmodel.CardTypeSpell:
		// 投影が必ず書く値なので、無ければ read model の不整合（黙って空にしない）
		if card.Spell == nil {
			return nil, fmt.Errorf("graphql: spell %s has no spell spec", card.ID)
		}
		return &model.Spell{
			ID: card.ID, Name: card.Name, Status: status, Cost: card.Cost, Rarity: rarity, Revision: revision,
			Effect:       fromSpellEffect(card.Spell.Effect),
			RegisteredAt: card.RegisteredAt, UpdatedAt: card.UpdatedAt,
		}, nil
	default:
		return nil, fmt.Errorf("graphql: card type %d has no GraphQL type", card.Type)
	}
}

// 性能の変更の結果の型のうち、性能の変更（コスト・希少度・属性・攻撃力と体力）で共通のエラーの型。該当しなければ nil（入力の誤りは別に見る）
func cardChangeError(err error) cardChangeResult {
	switch {
	case errors.Is(err, command.ErrCardNotFound):
		return &model.CardNotFoundError{Message: "card not found"}
	case errors.Is(err, command.ErrRevisionConflict):
		return &model.RevisionConflictError{Message: "revision conflict"}
	case errors.Is(err, command.ErrCardNotDraft):
		return &model.CardNotDraftError{Message: "card is not draft"}
	default:
		return nil
	}
}

// 性能の変更の結果の union のどれにも入るエラーの型（gqlgen は union ごとに別の interface を作るため、共通の部分をまとめる）
type cardChangeResult interface {
	model.ChangeCardCostResult
	model.ChangeCardRarityResult
	model.ChangeCreatureElementsResult
	model.ChangeCreatureStatsResult
	model.ChangeCreatureAbilitiesResult
	model.ChangeCreatureItemRestrictionsResult
	model.ChangeCreaturePlacementRestrictionsResult
	model.ChangeItemCategoryResult
	model.ChangeItemStatBonusesResult
	model.ChangeSpellEffectResult
}

// 効果なしは null（種類ごとの型のどれも返さない）
func fromSpellEffect(effect readmodel.SpellEffectMetadata) model.SpellEffect {
	switch {
	case effect.DrawCards != nil:
		return &model.SpellDrawCardsEffect{Count: effect.DrawCards.Count}
	case effect.GainMagic != nil:
		return &model.SpellGainMagicEffect{PerLap: effect.GainMagic.PerLap}
	case effect.RollDice != nil:
		return &model.SpellRollDiceEffect{Count: effect.RollDice.Count}
	case effect.FixedDiceCurse != nil:
		return &model.SpellFixedDiceCurseEffect{Value: effect.FixedDiceCurse.Value, Turns: effect.FixedDiceCurse.Turns}
	case effect.Damage != nil:
		return &model.SpellDamageEffect{Amount: effect.Damage.Amount}
	default:
		return nil
	}
}

// 種類のどれか 1 つだけを受け付ける（@oneOf の「ちょうど 1 つ」は変数の値だと GraphQL の層が確かめないため。toBoardTileInput と同じ）。
// null は効果なし。どれも無いのは入力の誤り（ok が false）
func toSpellEffect(effect *model.SpellEffectInput) (command.SpellEffect, bool) {
	if effect == nil {
		return command.SpellEffect{}, true
	}
	var converted command.SpellEffect
	kinds := 0
	if effect.DrawCards != nil {
		converted.DrawCards = &command.SpellDrawCards{Count: effect.DrawCards.Count}
		kinds++
	}
	if effect.GainMagic != nil {
		converted.GainMagic = &command.SpellGainMagic{PerLap: effect.GainMagic.PerLap}
		kinds++
	}
	if effect.RollDice != nil {
		converted.RollDice = &command.SpellRollDice{Count: effect.RollDice.Count}
		kinds++
	}
	if effect.FixedDiceCurse != nil {
		converted.FixedDiceCurse = &command.SpellFixedDiceCurse{Value: effect.FixedDiceCurse.Value, Turns: effect.FixedDiceCurse.Turns}
		kinds++
	}
	if effect.Damage != nil {
		converted.Damage = &command.SpellDamage{Amount: effect.Damage.Amount}
		kinds++
	}
	return converted, kinds == 1
}

// 種類がどれも無い効果の入力の誤り（resolver の引数の名前 input がパッケージの input と重なるため、ここで作る）
func invalidSpellEffect() *model.ValidationErrors {
	return validationErrors(input.Errors{&input.InvalidFormat{Field: "effect"}})
}

// enum の値と希少度の対応を明示する（toCardType と同じ理由）
func toCardRarity(rarity model.CardRarity) (string, error) {
	switch rarity {
	case model.CardRarityNormal:
		return "normal", nil
	case model.CardRarityStrange:
		return "strange", nil
	case model.CardRarityRare:
		return "rare", nil
	case model.CardRarityExtra:
		return "extra", nil
	default:
		return "", fmt.Errorf("graphql: unknown card rarity: %s", rarity)
	}
}

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

// enum の値と属性の対応を明示する（toCardType と同じ理由）
// 値の対応を明示する（fromCardRarity と同じ理由）
func toItemCategory(category model.ItemCategory) (string, error) {
	switch category {
	case model.ItemCategoryWeapon:
		return "weapon", nil
	case model.ItemCategoryArmor:
		return "armor", nil
	case model.ItemCategoryScroll:
		return "scroll", nil
	default:
		return "", fmt.Errorf("graphql: unknown item category: %q", category)
	}
}

func fromItemCategories(values []string) ([]model.ItemCategory, error) {
	categories := make([]model.ItemCategory, len(values))
	for i, value := range values {
		category, err := fromItemCategory(value)
		if err != nil {
			return nil, err
		}
		categories[i] = category
	}
	return categories, nil
}

func toItemCategories(categories []model.ItemCategory) ([]string, error) {
	values := make([]string, len(categories))
	for i, category := range categories {
		value, err := toItemCategory(category)
		if err != nil {
			return nil, err
		}
		values[i] = value
	}
	return values, nil
}

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

func toElements(elements []model.CreatureElement) ([]string, error) {
	values := make([]string, len(elements))
	for i, element := range elements {
		switch element {
		case model.CreatureElementFire:
			values[i] = "fire"
		case model.CreatureElementEarth:
			values[i] = "earth"
		case model.CreatureElementWater:
			values[i] = "water"
		case model.CreatureElementWind:
			values[i] = "wind"
		case model.CreatureElementNeutral:
			values[i] = "neutral"
		default:
			return nil, fmt.Errorf("graphql: unknown element: %s", element)
		}
	}
	return values, nil
}

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

func fromCreatureAbilitiesMetadata(abilities []readmodel.CreatureAbilityMetadata) []model.CreatureAbility {
	values := make([]model.CreatureAbility, len(abilities))
	for i, ability := range abilities {
		if ability.Defensive != nil {
			values[i] = &model.CreatureDefensiveAbility{Kind: model.CreatureAbilityKindDefensive}
		}
	}
	return values
}

func fromCreatureAbilities(abilities []command.CreatureAbility) []model.CreatureAbility {
	values := make([]model.CreatureAbility, len(abilities))
	for i, ability := range abilities {
		if ability.Defensive {
			values[i] = &model.CreatureDefensiveAbility{Kind: model.CreatureAbilityKindDefensive}
		}
	}
	return values
}

func toCreatureAbilities(abilities []*model.CreatureAbilityInput) ([]command.CreatureAbility, error) {
	values := make([]command.CreatureAbility, len(abilities))
	for i, ability := range abilities {
		switch ability.Kind {
		case model.CreatureAbilityKindDefensive:
			values[i] = command.CreatureAbility{Defensive: true}
		default:
			return nil, fmt.Errorf("graphql: unknown creature ability kind %q", ability.Kind)
		}
	}
	return values, nil
}
