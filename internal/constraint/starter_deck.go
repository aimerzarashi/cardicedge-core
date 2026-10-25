package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeck"
)

var (
	_ starterdeck.UpdateNameFunc           = UpdateStarterDeckName
	_ starterdeck.UpdateCardReferencesFunc = UpdateStarterDeckCardReferences
	_ starterdeck.ReserveQuotaFunc         = ReserveStarterDeckQuota
	_ starterdeck.ReleaseQuotaFunc         = ReleaseStarterDeckQuota
)

func UpdateStarterDeckName(ctx context.Context, id starterdeck.ID, name presetdeck.Name) error {
	return updateUniqueKey(ctx, id.Int64(), starterdeck.NameKey(name), starterdeck.ErrNameAlreadyInUse)
}

func UpdateStarterDeckCardReferences(ctx context.Context, id starterdeck.ID, cards []presetdeck.CardRef) error {
	ids := make([]int64, 0, len(cards))
	for _, card := range cards {
		ids = append(ids, card.ID())
	}
	return updateReferences(ctx, starterdeck.CardReferenceType(), id.Int64(), ids)
}

func ReserveStarterDeckQuota(ctx context.Context, quota domain.Quota, limit domain.QuotaLimit) error {
	return reserveQuota(ctx, quota, limit, starterdeck.ErrLimitReached)
}

func ReleaseStarterDeckQuota(ctx context.Context, quota domain.Quota) error {
	return releaseQuota(ctx, quota)
}
