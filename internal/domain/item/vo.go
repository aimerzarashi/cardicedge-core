package item

import (
	"errors"

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

// 戦闘中に足す攻撃力。下げるアイテムも作れるよう、負も認める（確かめることが無いので、生成関数は誤りを返さない）
type AttackBonus int

func NewAttackBonus(value int) AttackBonus {
	return AttackBonus(value)
}

func (b AttackBonus) Int() int {
	return int(b)
}

// 戦闘中に足す体力。負も認める理由は AttackBonus と同じ
type HitPointsBonus int

func NewHitPointsBonus(value int) HitPointsBonus {
	return HitPointsBonus(value)
}

func (b HitPointsBonus) Int() int {
	return int(b)
}
