package constraint

import (
	"context"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterset"
)

var (
	_ starterset.UpdateNameFunc                = UpdateStarterSetName
	_ starterset.UpdateGrantDeckReferencesFunc = UpdateStarterSetGrantDeckReferences
	_ starterset.UpdateBoardReferencesFunc     = UpdateStarterSetBoardReferences
	_ starterset.UpdateNPCAvatarReferencesFunc = UpdateStarterSetNPCAvatarReferences
	_ starterset.ReserveQuotaFunc              = ReserveStarterSetQuota
	_ starterset.ReleaseQuotaFunc              = ReleaseStarterSetQuota
)

func UpdateStarterSetName(ctx context.Context, id starterset.ID, name starterset.Name) error {
	return updateUniqueKey(ctx, id.Int64(), starterset.NameKey(name), starterset.ErrNameAlreadyInUse)
}

func UpdateStarterSetGrantDeckReferences(ctx context.Context, id starterset.ID, grantDecks []grantdeck.ID) error {
	ids := make([]int64, 0, len(grantDecks))
	for _, grantDeck := range grantDecks {
		ids = append(ids, grantDeck.Int64())
	}
	return updateReferences(ctx, starterset.GrantDeckReferenceType(), id.Int64(), ids)
}

func UpdateStarterSetBoardReferences(ctx context.Context, id starterset.ID, boards []board.ID) error {
	ids := make([]int64, 0, len(boards))
	for _, b := range boards {
		ids = append(ids, b.Int64())
	}
	return updateReferences(ctx, starterset.BoardReferenceType(), id.Int64(), ids)
}

func UpdateStarterSetNPCAvatarReferences(ctx context.Context, id starterset.ID, npcAvatars []npcavatar.ID) error {
	ids := make([]int64, 0, len(npcAvatars))
	for _, npcAvatar := range npcAvatars {
		ids = append(ids, npcAvatar.Int64())
	}
	return updateReferences(ctx, starterset.NPCAvatarReferenceType(), id.Int64(), ids)
}

func ReserveStarterSetQuota(ctx context.Context, quota domain.Quota, limit domain.QuotaLimit) error {
	return reserveQuota(ctx, quota, limit, starterset.ErrLimitReached)
}

func ReleaseStarterSetQuota(ctx context.Context, quota domain.Quota) error {
	return releaseQuota(ctx, quota)
}
