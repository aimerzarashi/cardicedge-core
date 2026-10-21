package identifier

import (
	"context"
	"database/sql"
	"errors"
	"slices"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

type entityRefResolver struct {
	registry *domaindef.Registry
}

func NewEntityRefResolver(registry *domaindef.Registry) domain.EntityRefResolver {
	return &entityRefResolver{registry: registry}
}

func (r *entityRefResolver) Resolve(ctx context.Context, publicID uuid.UUID) (domain.EntityRef, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return domain.EntityRef{}, err
	}
	row, err := models.DomainEntities(models.DomainEntityWhere.PublicID.EQ(publicID.String())).One(ctx, exec)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.EntityRef{}, domain.ErrEntityNotFound
	}
	if err != nil {
		return domain.EntityRef{}, err
	}
	entityType, err := r.registry.EntityTypeName(domaindef.NewEntityTypeID(row.EntityTypeID))
	if err != nil {
		return domain.EntityRef{}, err
	}
	return domain.NewEntityRef(entityType, row.ID), nil
}

func (r *entityRefResolver) ResolveAll(ctx context.Context, publicIDs []uuid.UUID) (map[uuid.UUID]domain.EntityRef, error) {
	refs := make(map[uuid.UUID]domain.EntityRef, len(publicIDs))
	if len(publicIDs) == 0 {
		return refs, nil
	}
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(publicIDs))
	for _, publicID := range publicIDs {
		keys = append(keys, publicID.String())
	}
	rows, err := models.DomainEntities(models.DomainEntityWhere.PublicID.IN(keys)).All(ctx, exec)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		entityType, err := r.registry.EntityTypeName(domaindef.NewEntityTypeID(row.EntityTypeID))
		if err != nil {
			return nil, err
		}
		publicID, err := uuid.Parse(row.PublicID)
		if err != nil {
			return nil, err
		}
		refs[publicID] = domain.NewEntityRef(entityType, row.ID)
	}
	return refs, nil
}

// 内部 ID は種類をまたいで一意（domain_entities の主キー）なので ID で引き、種類が違えば無いものとする
// （呼ぶ側の持つ種類が誤っていれば、別の集約の公開 ID を返さないため）
func (r *entityRefResolver) PublicIDs(ctx context.Context, refs []domain.EntityRef) (map[domain.EntityRef]uuid.UUID, error) {
	publicIDs := make(map[domain.EntityRef]uuid.UUID, len(refs))
	if len(refs) == 0 {
		return publicIDs, nil
	}
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, ref.ID())
	}
	rows, err := models.DomainEntities(models.DomainEntityWhere.ID.IN(ids)).All(ctx, exec)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		entityType, err := r.registry.EntityTypeName(domaindef.NewEntityTypeID(row.EntityTypeID))
		if err != nil {
			return nil, err
		}
		publicID, err := uuid.Parse(row.PublicID)
		if err != nil {
			return nil, err
		}
		publicIDs[domain.NewEntityRef(entityType, row.ID)] = publicID
	}
	for ref := range publicIDs {
		if !slices.Contains(refs, ref) {
			delete(publicIDs, ref)
		}
	}
	return publicIDs, nil
}
