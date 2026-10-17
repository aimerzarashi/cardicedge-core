// read model の DB の読み書き（app/query/readmodel の型を書き、読み出して app 層の型で返す）
package repository

import (
	"context"
	"encoding/json"

	"github.com/aarondl/sqlboiler/v4/boil"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/cms/app/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
)

// すべての列をイベントから作った値で書く
func UpsertCardList(ctx context.Context, list readmodel.CardList) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	metadata, err := json.Marshal(list.Metadata)
	if err != nil {
		return err
	}
	row := &models.CardList{
		CardID:   list.CardID.String(),
		CardType: int16(list.CardType),
		SortName: list.SortName,
		Metadata: metadata,
	}
	return row.Upsert(ctx, exec, true,
		[]string{models.CardListColumns.CardID},
		boil.Whitelist(models.CardListColumns.CardType, models.CardListColumns.SortName, models.CardListColumns.Metadata),
		boil.Infer(),
	)
}

// 行が無くてもエラーにしない（同じ集約を作り直すときに、何度呼ばれてもよいように）
func DeleteCardList(ctx context.Context, list readmodel.CardList) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = models.CardLists(models.CardListWhere.CardID.EQ(list.CardID.String())).DeleteAll(ctx, exec)
	return err
}
