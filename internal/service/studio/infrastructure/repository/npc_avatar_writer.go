package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/aarondl/sqlboiler/v4/boil"
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 書くときに操作したオペレーターと使うデッキの内部 ID を公開用の ID に引き直すため、引き方を持つ（AvatarWriter と同じ）
type NPCAvatarWriter struct {
	entityRefs domain.EntityRefResolver
}

func NewNPCAvatarWriter(entityRefs domain.EntityRefResolver) *NPCAvatarWriter {
	return &NPCAvatarWriter{entityRefs: entityRefs}
}

// すべての列をイベントから作った値で書く
func (w *NPCAvatarWriter) Upsert(ctx context.Context, npcAvatar readmodel.NPCAvatar) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	registeredByRef := domain.NewEntityRef(operator.Types().EntityType, npcAvatar.RegisteredBy.Int64())
	updatedByRef := domain.NewEntityRef(operator.Types().EntityType, npcAvatar.UpdatedBy.Int64())
	refs := make([]domain.EntityRef, 0, len(npcAvatar.Decks)+2)
	refs = append(refs, registeredByRef, updatedByRef)
	for _, deck := range npcAvatar.Decks {
		refs = append(refs, domain.NewEntityRef(npcdeck.Types().EntityType, deck.Int64()))
	}
	publicIDs, err := w.entityRefs.PublicIDs(ctx, refs)
	if err != nil {
		return err
	}
	metadata := npcAvatar.Metadata
	metadata.Decks = make([]uuid.UUID, 0, len(npcAvatar.Decks))
	for _, deck := range npcAvatar.Decks {
		publicID, ok := publicIDs[domain.NewEntityRef(npcdeck.Types().EntityType, deck.Int64())]
		if !ok {
			return errors.New("repository: deck of npc avatar has no public id")
		}
		metadata.Decks = append(metadata.Decks, publicID)
	}
	var registeredOK, updatedOK bool
	metadata.RegisteredBy, registeredOK = publicIDs[registeredByRef]
	metadata.UpdatedBy, updatedOK = publicIDs[updatedByRef]
	if !registeredOK || !updatedOK {
		return errors.New("repository: operator of npc avatar has no public id")
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	row := &models.NPCAvatar{
		NPCAvatarID: npcAvatar.NPCAvatarID.String(),
		SortName:    npcAvatar.SortName,
		Status:      string(npcAvatar.Status),
		Metadata:    raw,
	}
	return row.Upsert(ctx, exec, true,
		[]string{models.NPCAvatarColumns.NPCAvatarID},
		boil.Whitelist(models.NPCAvatarColumns.SortName, models.NPCAvatarColumns.Status, models.NPCAvatarColumns.Metadata),
		boil.Infer(),
	)
}

// 行が無くてもエラーにしない（同じ集約を作り直すときに、何度呼ばれてもよいように）
func (w *NPCAvatarWriter) Delete(ctx context.Context, npcAvatar readmodel.NPCAvatar) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = models.NPCAvatars(models.NPCAvatarWhere.NPCAvatarID.EQ(npcAvatar.NPCAvatarID.String())).DeleteAll(ctx, exec)
	return err
}
