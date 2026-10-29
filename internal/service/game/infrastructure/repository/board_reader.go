package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

// core_game の boards（make publish-master が studio の read model から公開中・使用不可の行を写した表）のうち、公開中だけを読む。
// 使用不可も写してあるのは、スターターデッキと同じく、後で対戦中のボードの元を表示したくなっても表を作り直さずに済むようにするため
type BoardReader struct{}

func NewBoardReader() *BoardReader {
	return &BoardReader{}
}

const boardStatusPublished = "published"

// 写した表の metadata のうち、盤面に要る項目。studio の read model の型を import せず、この JSON の形だけを契約にする
// （StarterDeckReader と同じ）
type boardMetadata struct {
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Tiles  []struct {
		X         int    `json:"x"`
		Y         int    `json:"y"`
		Kind      string `json:"kind"`
		Attribute string `json:"attribute"`
	} `json:"tiles"`
	Connections []struct {
		A boardPosition `json:"a"`
		B boardPosition `json:"b"`
	} `json:"connections"`
}

type boardPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// 並びは索引（sort_name, board_id）と同じ。続きは行の比べ方で取る（sqlboiler の条件の部品に行の比べ方が無いため、SQL を直接書く）
func (r *BoardReader) ListPublished(ctx context.Context, condition query.ListPublishedBoardsCondition) ([]query.BoardListRow, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	sql := "SELECT board_id, sort_name, metadata FROM boards WHERE status = $1"
	args := []any{boardStatusPublished}
	if condition.After != nil {
		sql += " AND (sort_name, board_id) > ($2, $3)"
		args = append(args, condition.After.SortName, condition.After.BoardID)
	}
	args = append(args, condition.Limit)
	sql += fmt.Sprintf(" ORDER BY sort_name, board_id LIMIT $%d", len(args))

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

// 公開中でなければ（使用不可・準備中・行が無い）query.ErrBoardNotFound
func (r *BoardReader) GetPublished(ctx context.Context, id uuid.UUID) (query.Board, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return query.Board{}, err
	}
	row, err := models.Boards(
		models.BoardWhere.BoardID.EQ(id.String()),
		models.BoardWhere.Status.EQ(boardStatusPublished),
		qm.Limit(1),
	).One(ctx, exec)
	if errors.Is(err, sql.ErrNoRows) {
		return query.Board{}, query.ErrBoardNotFound
	}
	if err != nil {
		return query.Board{}, err
	}
	return toBoard(id, row.Metadata)
}

func toBoard(id uuid.UUID, raw []byte) (query.Board, error) {
	var metadata boardMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return query.Board{}, fmt.Errorf("boards.metadata of %s: %w", id, err)
	}
	tiles := make([]query.BoardTile, 0, len(metadata.Tiles))
	for _, tile := range metadata.Tiles {
		kind, err := query.NewBoardTileKind(tile.Kind)
		if err != nil {
			return query.Board{}, fmt.Errorf("boards.metadata of %s: %w", id, err)
		}
		attribute, err := optionalTileAttribute(tile.Attribute)
		if err != nil {
			return query.Board{}, fmt.Errorf("boards.metadata of %s: %w", id, err)
		}
		tiles = append(tiles, query.BoardTile{X: tile.X, Y: tile.Y, Kind: kind, Attribute: attribute})
	}
	connections := make([]query.BoardConnection, 0, len(metadata.Connections))
	for _, connection := range metadata.Connections {
		connections = append(connections, query.BoardConnection{
			A: query.NewBoardPosition(connection.A.X, connection.A.Y),
			B: query.NewBoardPosition(connection.B.X, connection.B.Y),
		})
	}
	return query.Board{ID: id, Name: metadata.Name, Width: metadata.Width, Height: metadata.Height, Tiles: tiles, Connections: connections}, nil
}

// 土地でないマスには属性が無い（空）
func optionalTileAttribute(value string) (query.BoardTileAttribute, error) {
	if value == "" {
		return "", nil
	}
	return query.NewBoardTileAttribute(value)
}
