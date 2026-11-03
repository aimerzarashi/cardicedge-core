package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/grantdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 書くときに操作したオペレーターと配るデッキ・解放するボードの内部 ID を公開用の ID に引き直すため、引き方を持つ（AvatarWriter と同じ）
type StarterSetWriter struct {
	entityRefs domain.EntityRefResolver
}

func NewStarterSetWriter(entityRefs domain.EntityRefResolver) *StarterSetWriter {
	return &StarterSetWriter{entityRefs: entityRefs}
}

// すべての列をイベントから作った値で書く
func (w *StarterSetWriter) Upsert(ctx context.Context, starterSet readmodel.StarterSet) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	registeredByRef := domain.NewEntityRef(operator.Types().EntityType, starterSet.RegisteredBy.Int64())
	updatedByRef := domain.NewEntityRef(operator.Types().EntityType, starterSet.UpdatedBy.Int64())
	refs := make([]domain.EntityRef, 0, len(starterSet.GrantDecks)+len(starterSet.UnlockBoards)+2)
	refs = append(refs, registeredByRef, updatedByRef)
	for _, id := range starterSet.GrantDecks {
		refs = append(refs, domain.NewEntityRef(grantdeck.Types().EntityType, id.Int64()))
	}
	for _, id := range starterSet.UnlockBoards {
		refs = append(refs, domain.NewEntityRef(board.Types().EntityType, id.Int64()))
	}
	publicIDs, err := w.entityRefs.PublicIDs(ctx, refs)
	if err != nil {
		return err
	}
	metadata := starterSet.Metadata
	metadata.GrantDecks = make([]uuid.UUID, 0, len(starterSet.GrantDecks))
	for _, id := range starterSet.GrantDecks {
		publicID, ok := publicIDs[domain.NewEntityRef(grantdeck.Types().EntityType, id.Int64())]
		if !ok {
			return errors.New("repository: grant deck of starter set has no public id")
		}
		metadata.GrantDecks = append(metadata.GrantDecks, publicID)
	}
	metadata.UnlockBoards = make([]uuid.UUID, 0, len(starterSet.UnlockBoards))
	for _, id := range starterSet.UnlockBoards {
		publicID, ok := publicIDs[domain.NewEntityRef(board.Types().EntityType, id.Int64())]
		if !ok {
			return errors.New("repository: board of starter set has no public id")
		}
		metadata.UnlockBoards = append(metadata.UnlockBoards, publicID)
	}
	var registeredOK, updatedOK bool
	metadata.RegisteredBy, registeredOK = publicIDs[registeredByRef]
	metadata.UpdatedBy, updatedOK = publicIDs[updatedByRef]
	if !registeredOK || !updatedOK {
		return errors.New("repository: operator of starter set has no public id")
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	row := &models.StarterSet{
		StarterSetID: starterSet.StarterSetID.String(),
		SortName:     starterSet.SortName,
		Status:       string(starterSet.Status),
		Metadata:     raw,
	}
	return row.Upsert(ctx, exec, true,
		[]string{models.StarterSetColumns.StarterSetID},
		boil.Whitelist(models.StarterSetColumns.SortName, models.StarterSetColumns.Status, models.StarterSetColumns.Metadata),
		boil.Infer(),
	)
}

// 行が無くてもエラーにしない（同じ集約を作り直すときに、何度呼ばれてもよいように）
func (w *StarterSetWriter) Delete(ctx context.Context, starterSet readmodel.StarterSet) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = models.StarterSets(models.StarterSetWhere.StarterSetID.EQ(starterSet.StarterSetID.String())).DeleteAll(ctx, exec)
	return err
}
