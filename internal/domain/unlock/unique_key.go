package unlock

import (
	"context"
	"strconv"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

// domain_unique_key_types の名前。プレイヤーの解放を 1 つにする
var PlayerKeyType = domain.NewUniqueKeyType("unlock.player")

func UniqueKeyTypes() []domain.UniqueKeyType {
	return []domain.UniqueKeyType{PlayerKeyType}
}

func PlayerKey(playerID player.ID) domain.UniqueKey {
	return domain.NewUniqueKey(PlayerKeyType, strconv.FormatInt(playerID.Int64(), 10))
}

// そのプレイヤーの解放がもうあれば ErrAlreadyCreated
type UpdatePlayerFunc func(ctx context.Context, id ID, playerID player.ID) error
