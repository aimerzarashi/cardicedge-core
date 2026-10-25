package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/deck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/startergrant"
)

var (
	_ deck.UpdateStarterFunc = UpdateDeckStarter
	_ deck.UpdateNameFunc    = UpdateDeckName
	_ deck.ReserveQuotaFunc  = ReserveDeckQuota
	_ deck.ReleaseQuotaFunc  = ReleaseDeckQuota
)

func UpdateDeckStarter(ctx context.Context, id deck.ID, grant startergrant.ID, number startergrant.DeckNumber) error {
	return updateUniqueKey(ctx, id.Int64(), deck.StarterKey(grant, number), deck.ErrAlreadyCreated)
}

func UpdateDeckName(ctx context.Context, id deck.ID, owner player.ID, name deck.Name) error {
	return updateUniqueKey(ctx, id.Int64(), deck.NameKey(owner, name), deck.ErrNameAlreadyInUse)
}

func ReserveDeckQuota(ctx context.Context, quota domain.Quota, limit domain.QuotaLimit) error {
	return reserveQuota(ctx, quota, limit, deck.ErrLimitReached)
}

func ReleaseDeckQuota(ctx context.Context, quota domain.Quota) error {
	return releaseQuota(ctx, quota)
}
