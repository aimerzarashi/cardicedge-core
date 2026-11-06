package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
)

// read model の登録・更新の人（内部 ID）を公開 ID に引き直す。他の参照を同じ問い合わせで引く writer は extra に足す
// （NPCAvatarWriter・NPCDeckWriter は自前で引く）。引けなければ誤り（読み手に内部 ID を渡さず、空の ID も書かないため）
func operatorPublicIDs(ctx context.Context, resolver domain.EntityRefResolver, registeredBy, updatedBy operator.ID, extra ...domain.EntityRef) (registered, updated uuid.UUID, publicIDs map[domain.EntityRef]uuid.UUID, err error) {
	registeredRef := domain.NewEntityRef(operator.Types().EntityType, registeredBy.Int64())
	updatedRef := domain.NewEntityRef(operator.Types().EntityType, updatedBy.Int64())
	refs := append([]domain.EntityRef{registeredRef, updatedRef}, extra...)
	publicIDs, err = resolver.PublicIDs(ctx, refs)
	if err != nil {
		return uuid.Nil, uuid.Nil, nil, err
	}
	registered, registeredOK := publicIDs[registeredRef]
	updated, updatedOK := publicIDs[updatedRef]
	if !registeredOK || !updatedOK {
		return uuid.Nil, uuid.Nil, nil, errors.New("repository: operator of read model has no public id")
	}
	return registered, updated, publicIDs, nil
}
