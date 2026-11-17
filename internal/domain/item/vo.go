package item

import (
	"errors"
	"slices"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/effect"
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

// 登録の直後の分類（2026-11-07 オーナー）
const DefaultCategory = card.ItemCategoryWeapon

// 分類ごとに持てる効果の種類（選べる効果の表）。見本のカードで使う組み合わせだけを入れ、新しい組み合わせは足すときに議論する
// （2026-11-14 オーナー。設計メモ「能力の作りの見直し」）。効果の型は effect に置き、どの持ち主が何を持てるかはここで決める（core#178）
var allowedEffectKinds = map[card.ItemCategory][]effect.Kind{
	card.ItemCategoryWeapon: {effect.KindAttackModifier, effect.KindAttackMultiplier},
	card.ItemCategoryArmor:  {effect.KindHitPointsModifier},
	card.ItemCategoryScroll: {effect.KindAttackOverride, effect.KindAttackMultiplier, effect.KindIgnoreTerrainEffect},
}

// その分類で持てる効果の種類（選べる効果の表の並び）。studio の画面が、選べるものだけを出すのに使う
func AllowedEffectKinds(category card.ItemCategory) []effect.Kind {
	return slices.Clone(allowedEffectKinds[category])
}

// その分類で持てる効果の種類か
func Allows(category card.ItemCategory, kind effect.Kind) bool {
	return slices.Contains(allowedEffectKinds[category], kind)
}
