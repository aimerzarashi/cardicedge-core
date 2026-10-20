package card

import "gitlab.com/aimerzarashi/diecardomain/core/internal/domain"

// domain_unique_key_types の名前。Creature・Item・Spell で 1 つの種別を共有し、3 種類をまたいで名前を一意にする
var nameKeyType = domain.NewUniqueKeyType("card.name")

func UniqueKeyTypes() []domain.UniqueKeyType {
	return []domain.UniqueKeyType{nameKeyType}
}

func NameKey(name Name) domain.UniqueKey {
	return domain.NewUniqueKey(nameKeyType, NormalizeName(name.String()))
}
