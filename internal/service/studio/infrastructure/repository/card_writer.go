// read model の DB の読み書き（application/query/readmodel の型を書き、読み出して application 層の型で返す）
package repository

import (
	"context"
	"encoding/json"

	"github.com/aarondl/sqlboiler/v4/boil"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// cards を書く（投影が使う）
type CardWriter struct{}

func NewCardWriter() *CardWriter {
	return &CardWriter{}
}

// すべての列をイベントから作った値で書く
func (w *CardWriter) Upsert(ctx context.Context, card readmodel.Card) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(card.Metadata)
	if err != nil {
		return err
	}
	row := &models.Card{
		CardID:   card.CardID.String(),
		CardType: int16(card.CardType),
		SortName: card.SortName,
		Metadata: metadata,
	}
	return row.Upsert(ctx, exec, true,
		[]string{models.CardColumns.CardID},
		boil.Whitelist(models.CardColumns.CardType, models.CardColumns.SortName, models.CardColumns.Metadata),
		boil.Infer(),
	)
}

// 行が無くてもエラーにしない（同じ集約を作り直すときに、何度呼ばれてもよいように）
func (w *CardWriter) Delete(ctx context.Context, card readmodel.Card) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = models.Cards(models.CardWhere.CardID.EQ(card.CardID.String())).DeleteAll(ctx, exec)
	return err
}
