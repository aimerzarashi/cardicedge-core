package identifier

import (
	"context"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

type identifier[I domain.ID] struct {
	registry *domaindef.Registry
	newID    domain.NewIDFunc[I]
}

func New[I domain.ID](registry *domaindef.Registry, newID domain.NewIDFunc[I]) domain.Identifier[I] {
	return &identifier[I]{registry: registry, newID: newID}
}

func (i *identifier[I]) Issue(ctx context.Context) (I, uuid.UUID, error) {
	var zero I
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return zero, uuid.UUID{}, err
	}
	entityTypeID, err := i.registry.EntityTypeID(zero.EntityType())
	if err != nil {
		return zero, uuid.UUID{}, err
	}

	row := &models.DomainIdentifier{EntityTypeID: entityTypeID.Int16()}
	if err := row.Insert(ctx, exec, boil.Whitelist(models.DomainIdentifierColumns.EntityTypeID)); err != nil {
		return zero, uuid.UUID{}, err
	}
	id, err := i.newID(row.ID)
	if err != nil {
		return zero, uuid.UUID{}, err
	}
	publicID, err := uuid.Parse(row.PublicID)
	if err != nil {
		return zero, uuid.UUID{}, err
	}
	return id, publicID, nil
}

func (i *identifier[I]) Resolve(ctx context.Context, publicID uuid.UUID) (I, error) {
	var zero I
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return zero, err
	}
	entityTypeID, err := i.registry.EntityTypeID(zero.EntityType())
	if err != nil {
		return zero, err
	}
	row, err := models.DomainIdentifiers(
		models.DomainIdentifierWhere.EntityTypeID.EQ(entityTypeID.Int16()),
		models.DomainIdentifierWhere.PublicID.EQ(publicID.String()),
	).One(ctx, exec)
	if err != nil {
		return zero, err
	}
	return i.newID(row.ID)
}
