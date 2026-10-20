package repository

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/aarondl/sqlboiler/v4/boil"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query/readmodel"
)

// 書くときにカードの内部 ID を公開 ID に引き直すため、引き方を持つ（カードの一覧の投影は関数だけで済むが、こちらは DB の識別子を読む）
type StarterDeckListWriter struct {
	entityRefs domain.EntityRefResolver
}

func NewStarterDeckListWriter(entityRefs domain.EntityRefResolver) *StarterDeckListWriter {
	return &StarterDeckListWriter{entityRefs: entityRefs}
}

// すべての列をイベントから作った値で書く。カードは公開 ID の順に並べる（並べる決まりは読む側にあり、ここは同じ状態から
// 同じ行を作るためだけに並べる）
func (w *StarterDeckListWriter) Upsert(ctx context.Context, list readmodel.StarterDeckList) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	refs := make([]domain.EntityRef, 0, len(list.Cards))
	for card := range list.Cards {
		refs = append(refs, domain.NewEntityRef(card.EntityType(), card.ID()))
	}
	publicIDs, err := w.entityRefs.PublicIDs(ctx, refs)
	if err != nil {
		return err
	}
	cards := make([]readmodel.StarterDeckListCard, 0, len(list.Cards))
	for card, count := range list.Cards {
		publicID, ok := publicIDs[domain.NewEntityRef(card.EntityType(), card.ID())]
		if !ok {
			return errors.New("repository: card in starter deck has no public id")
		}
		cards = append(cards, readmodel.StarterDeckListCard{CardID: publicID, Count: count})
	}
	slices.SortFunc(cards, func(a, b readmodel.StarterDeckListCard) int {
		return strings.Compare(a.CardID.String(), b.CardID.String())
	})
	metadata := list.Metadata
	metadata.Cards = cards
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	row := &models.StarterDeckList{
		StarterDeckID: list.StarterDeckID.String(),
		SortName:      list.SortName,
		TotalCount:    list.TotalCount(),
		Metadata:      raw,
	}
	return row.Upsert(ctx, exec, true,
		[]string{models.StarterDeckListColumns.StarterDeckID},
		boil.Whitelist(models.StarterDeckListColumns.SortName, models.StarterDeckListColumns.TotalCount, models.StarterDeckListColumns.Metadata),
		boil.Infer(),
	)
}

// 行が無くてもエラーにしない（同じ集約を作り直すときに、何度呼ばれてもよいように）
func (w *StarterDeckListWriter) Delete(ctx context.Context, list readmodel.StarterDeckList) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = models.StarterDeckLists(models.StarterDeckListWhere.StarterDeckID.EQ(list.StarterDeckID.String())).DeleteAll(ctx, exec)
	return err
}
