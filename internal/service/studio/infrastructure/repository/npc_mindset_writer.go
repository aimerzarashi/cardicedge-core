package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/aarondl/sqlboiler/v4/boil"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 書くときに操作したオペレーターの内部 ID を公開用の ID に引き直すため、引き方を持つ（AvatarWriter と同じ）
type NPCMindsetWriter struct {
	entityRefs domain.EntityRefResolver
}

func NewNPCMindsetWriter(entityRefs domain.EntityRefResolver) *NPCMindsetWriter {
	return &NPCMindsetWriter{entityRefs: entityRefs}
}

// すべての列をイベントから作った値で書く
func (w *NPCMindsetWriter) Upsert(ctx context.Context, npcMindset readmodel.NPCMindset) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	registeredByRef := domain.NewEntityRef(operator.Types().EntityType, npcMindset.RegisteredBy.Int64())
	updatedByRef := domain.NewEntityRef(operator.Types().EntityType, npcMindset.UpdatedBy.Int64())
	publicIDs, err := w.entityRefs.PublicIDs(ctx, []domain.EntityRef{registeredByRef, updatedByRef})
	if err != nil {
		return err
	}
	metadata := npcMindset.Metadata
	var registeredOK, updatedOK bool
	metadata.RegisteredBy, registeredOK = publicIDs[registeredByRef]
	metadata.UpdatedBy, updatedOK = publicIDs[updatedByRef]
	if !registeredOK || !updatedOK {
		return errors.New("repository: operator of npc mindset has no public id")
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	row := &models.NPCMindset{
		NPCMindsetID: npcMindset.NPCMindsetID.String(),
		SortName:     npcMindset.SortName,
		Status:       string(npcMindset.Status),
		Metadata:     raw,
	}
	return row.Upsert(ctx, exec, true,
		[]string{models.NPCMindsetColumns.NPCMindsetID},
		boil.Whitelist(models.NPCMindsetColumns.SortName, models.NPCMindsetColumns.Status, models.NPCMindsetColumns.Metadata),
		boil.Infer(),
	)
}

// 行が無くてもエラーにしない（同じ集約を作り直すときに、何度呼ばれてもよいように）
func (w *NPCMindsetWriter) Delete(ctx context.Context, npcMindset readmodel.NPCMindset) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = models.NPCMindsets(models.NPCMindsetWhere.NPCMindsetID.EQ(npcMindset.NPCMindsetID.String())).DeleteAll(ctx, exec)
	return err
}
