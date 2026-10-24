package collection

import (
	"context"
	"strconv"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

// domain_unique_key_types の名前。プレイヤーの所持カードを 1 つにする
var OwnerKeyType = domain.NewUniqueKeyType("collection.owner")

func UniqueKeyTypes() []domain.UniqueKeyType {
	return []domain.UniqueKeyType{OwnerKeyType}
}

func OwnerKey(owner player.ID) domain.UniqueKey {
	return domain.NewUniqueKey(OwnerKeyType, strconv.FormatInt(owner.Int64(), 10))
}

// そのプレイヤーの所持カードがもうあれば ErrAlreadyCreated
type UpdateOwnerFunc func(ctx context.Context, id ID, owner player.ID) error
