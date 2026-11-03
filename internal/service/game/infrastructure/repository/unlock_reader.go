package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

type UnlockReader struct{}

func NewUnlockReader() *UnlockReader {
	return &UnlockReader{}
}

// プレイヤーの解放（プレイヤーに 1 つ）が持つボードの公開 ID（read model は公開 ID だけを持つ）の順に、写した boards から公開中のものを読む。
// ボードの中身は read model に書き写さず、読むときに引く（ボードのレイアウトを直しても、解放したボードに反映されるため）。
// 公開中でないボード（使用不可にした後・写していない）は飛ばす（Query.board と同じく、公開中でないボードを見せない）。解放がまだ無ければ空
func (r *UnlockReader) ListBoardsByPlayer(ctx context.Context, playerID uuid.UUID) ([]query.Board, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	row, err := models.Unlocks(models.UnlockWhere.PlayerID.EQ(playerID.String())).One(ctx, exec)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var metadata readmodel.UnlockMetadata
	if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
		return nil, fmt.Errorf("unlocks.metadata of %s: %w", row.UnlockID, err)
	}
	ids := make([]string, len(metadata.Boards))
	for i, id := range metadata.Boards {
		ids[i] = id.String()
	}
	rows, err := models.Boards(
		models.BoardWhere.BoardID.IN(ids),
		models.BoardWhere.Status.EQ(boardStatusPublished),
	).All(ctx, exec)
	if err != nil {
		return nil, err
	}
	published := make(map[uuid.UUID]*models.Board, len(rows))
	for _, boardRow := range rows {
		id, err := uuid.Parse(boardRow.BoardID)
		if err != nil {
			return nil, fmt.Errorf("boards.board_id %q: %w", boardRow.BoardID, err)
		}
		published[id] = boardRow
	}
	boards := make([]query.Board, 0, len(metadata.Boards))
	for _, id := range metadata.Boards {
		boardRow, ok := published[id]
		if !ok {
			continue
		}
		board, err := toBoard(id, boardRow.Metadata)
		if err != nil {
			return nil, err
		}
		boards = append(boards, board)
	}
	return boards, nil
}
