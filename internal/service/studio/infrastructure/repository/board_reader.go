package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// boards を読む（一覧と詳細）。並び順は索引（sort_name, board_id）と同じにし、続きは行の比べ方で取る。
// sqlboiler を使わない理由は PresetDeckReader.List と同じ
type BoardReader struct{}

func NewBoardReader() *BoardReader {
	return &BoardReader{}
}

func (r *BoardReader) List(ctx context.Context, condition query.ListBoardsCondition) ([]query.BoardListRow, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var where []string
	var args []any
	arg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}
	if condition.After != nil {
		where = append(where, fmt.Sprintf("(sort_name, board_id) > (%s, %s)", arg(condition.After.SortName), arg(condition.After.BoardID)))
	}
	if len(condition.Statuses) > 0 {
		statuses := make([]string, len(condition.Statuses))
		for i, status := range condition.Statuses {
			statuses[i] = string(status)
		}
		where = append(where, fmt.Sprintf("status = ANY(%s)", arg(pq.Array(statuses))))
	}
	if condition.NameContains != "" {
		where = append(where, fmt.Sprintf(`sort_name LIKE '%%' || %s || '%%' ESCAPE '\'`, arg(likeEscaper.Replace(condition.NameContains))))
	}
	sql := "SELECT board_id, sort_name, metadata FROM boards"
	if len(where) > 0 {
		sql += " WHERE " + strings.Join(where, " AND ")
	}
	sql += " ORDER BY sort_name, board_id LIMIT " + arg(condition.Limit)

	rows, err := exec.QueryContext(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []query.BoardListRow
	for rows.Next() {
		var (
			id       uuid.UUID
			sortName string
			raw      []byte
		)
		if err := rows.Scan(&id, &sortName, &raw); err != nil {
			return nil, err
		}
		board, err := toBoard(id, raw)
		if err != nil {
			return nil, err
		}
		cursor := query.BoardListCursor{SortName: sortName, BoardID: id}
		result = append(result, query.BoardListRow{Board: board, Cursor: cursor})
	}
	return result, rows.Err()
}

func (r *BoardReader) Get(ctx context.Context, id uuid.UUID) (query.Board, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.Board{}, err
	}
	row, err := models.FindBoard(ctx, exec, id.String())
	if errors.Is(err, sql.ErrNoRows) {
		return query.Board{}, query.ErrBoardNotFound
	}
	if err != nil {
		return query.Board{}, err
	}
	return toBoard(id, row.Metadata)
}

func toBoard(id uuid.UUID, raw []byte) (query.Board, error) {
	var metadata readmodel.BoardMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return query.Board{}, fmt.Errorf("boards.metadata of %s: %w", id, err)
	}
	// 公開できない理由を持たない行（理由を足す前に投影した行）を、公開できるものとして返さない。投影を作り直せば入る
	if metadata.PublishProblems == nil {
		return query.Board{}, fmt.Errorf("boards.metadata of %s: publish_problems is missing", id)
	}
	return query.Board{
		ID: metadata.ID, Name: metadata.Name, Status: metadata.Status, Width: metadata.Width, Height: metadata.Height,
		Tiles: metadata.Tiles, Connections: metadata.Connections, DefaultRules: metadata.DefaultRules, TileCounts: metadata.TileCounts, PublishProblems: metadata.PublishProblems,
		Revision: metadata.Revision, RegisteredAt: metadata.RegisteredAt, UpdatedAt: metadata.UpdatedAt,
	}, nil
}
