package collection

import (
	"context"
	"strconv"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

// domain_unique_key_types の名前。プレイヤーの所持カードを 1 つにする
var PlayerKeyType = domain.NewUniqueKeyType("collection.player")

func UniqueKeyTypes() []domain.UniqueKeyType {
	return []domain.UniqueKeyType{PlayerKeyType}
}

func PlayerKey(playerID player.ID) domain.UniqueKey {
	return domain.NewUniqueKey(PlayerKeyType, strconv.FormatInt(playerID.Int64(), 10))
}

// そのプレイヤーの所持カードがもうあれば ErrAlreadyCreated
type UpdatePlayerFunc func(ctx context.Context, id ID, playerID player.ID) error
