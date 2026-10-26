package startergrant

import (
	"context"
	"strconv"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
)

// domain_unique_key_types の名前。プレイヤーへの配布を 1 つにする
var PlayerKeyType = domain.NewUniqueKeyType("starter_grant.player")

func UniqueKeyTypes() []domain.UniqueKeyType {
	return []domain.UniqueKeyType{PlayerKeyType}
}

func PlayerKey(playerID player.ID) domain.UniqueKey {
	return domain.NewUniqueKey(PlayerKeyType, strconv.FormatInt(playerID.Int64(), 10))
}

// そのプレイヤーにほかの配布が始まっていれば ErrAlreadyStarted
type UpdatePlayerFunc func(ctx context.Context, id ID, playerID player.ID) error
