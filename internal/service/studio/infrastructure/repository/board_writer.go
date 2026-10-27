package repository

import (
	"context"
	"encoding/json"

	"github.com/aarondl/sqlboiler/v4/boil"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

type BoardWriter struct{}

func NewBoardWriter() *BoardWriter {
	return &BoardWriter{}
}

// すべての列をイベントから作った値で書く
func (w *BoardWriter) Upsert(ctx context.Context, board readmodel.Board) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(board.Metadata)
	if err != nil {
		return err
	}
	row := &models.Board{
		BoardID:  board.BoardID.String(),
		SortName: board.SortName,
		Status:   string(board.Status),
		Metadata: raw,
	}
	return row.Upsert(ctx, exec, true,
		[]string{models.BoardColumns.BoardID},
		boil.Whitelist(models.BoardColumns.SortName, models.BoardColumns.Status, models.BoardColumns.Metadata),
		boil.Infer(),
	)
}

// 行が無くてもエラーにしない（同じ集約を作り直すときに、何度呼ばれてもよいように）
func (w *BoardWriter) Delete(ctx context.Context, board readmodel.Board) error {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = models.Boards(models.BoardWhere.BoardID.EQ(board.BoardID.String())).DeleteAll(ctx, exec)
	return err
}
