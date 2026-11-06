package repository

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/aarondl/sqlboiler/v4/boil"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 書くときにカードとオペレーターの内部 ID を公開 ID に引き直すため、引き方を持つ（PresetDeckWriter・NPCAvatarWriter と同じ）
type NPCDeckWriter struct {
	entityRefs domain.EntityRefResolver
}

func NewNPCDeckWriter(entityRefs domain.EntityRefResolver) *NPCDeckWriter {
	return &NPCDeckWriter{entityRefs: entityRefs}
}

// すべての列をイベントから作った値で書く。カードは公開 ID の順に並べる（同じ状態から同じ行を作るためだけに並べる）
func (w *NPCDeckWriter) Upsert(ctx context.Context, npcDeck readmodel.NPCDeck) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	registeredByRef := domain.NewEntityRef(operator.Types().EntityType, npcDeck.RegisteredBy.Int64())
	updatedByRef := domain.NewEntityRef(operator.Types().EntityType, npcDeck.UpdatedBy.Int64())
	refs := make([]domain.EntityRef, 0, len(npcDeck.Cards)+2)
	refs = append(refs, registeredByRef, updatedByRef)
	for card := range npcDeck.Cards {
		refs = append(refs, domain.NewEntityRef(card.EntityType, card.ID))
	}
	publicIDs, err := w.entityRefs.PublicIDs(ctx, refs)
	if err != nil {
		return err
	}
	cards := make([]readmodel.DeckCard, 0, len(npcDeck.Cards))
	for card, count := range npcDeck.Cards {
		publicID, ok := publicIDs[domain.NewEntityRef(card.EntityType, card.ID)]
		if !ok {
			return errors.New("repository: card in npc deck has no public id")
		}
		cards = append(cards, readmodel.DeckCard{CardID: publicID, Count: count})
	}
	slices.SortFunc(cards, func(a, b readmodel.DeckCard) int {
		return strings.Compare(a.CardID.String(), b.CardID.String())
	})
	metadata := npcDeck.Metadata
	metadata.Cards = cards
	var registeredOK, updatedOK bool
	metadata.RegisteredBy, registeredOK = publicIDs[registeredByRef]
	metadata.UpdatedBy, updatedOK = publicIDs[updatedByRef]
	if !registeredOK || !updatedOK {
		return errors.New("repository: operator of npc deck has no public id")
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	row := &models.NPCDeck{
		NPCDeckID:  npcDeck.NPCDeckID.String(),
		SortName:   npcDeck.SortName,
		Status:     string(npcDeck.Status),
		TotalCount: npcDeck.TotalCount(),
		Metadata:   raw,
	}
	return row.Upsert(ctx, exec, true,
		[]string{models.NPCDeckColumns.NPCDeckID},
		boil.Whitelist(models.NPCDeckColumns.SortName, models.NPCDeckColumns.Status, models.NPCDeckColumns.TotalCount, models.NPCDeckColumns.Metadata),
		boil.Infer(),
	)
}

// 行が無くてもエラーにしない（同じ集約を作り直すときに、何度呼ばれてもよいように）
func (w *NPCDeckWriter) Delete(ctx context.Context, npcDeck readmodel.NPCDeck) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = models.NPCDecks(models.NPCDeckWhere.NPCDeckID.EQ(npcDeck.NPCDeckID.String())).DeleteAll(ctx, exec)
	return err
}
