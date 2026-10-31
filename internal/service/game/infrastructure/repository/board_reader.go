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
	// 無い（対を写す前の行）なら、空にせず読み出しの誤りにするため、ポインターで受ける（空の対は [] で写る）
	Pairs *[]struct {
		A boardPosition `json:"a"`
		B boardPosition `json:"b"`
	} `json:"pairs"`
	// 無い（既定のルールを写す前の行）なら、0 にせず読み出しの誤りにするため、ポインターで受ける
	DefaultRules *struct {
		TargetMagic     int `json:"target_magic"`
		StartingMagic   int `json:"starting_magic"`
		LapBonus        int `json:"lap_bonus"`
		CheckpointBonus int `json:"checkpoint_bonus"`
		// ラウンドの上限とサイコロの目を写す前の行を、0 にせず読み出しの誤りにするため、ポインターで受ける
		RoundLimit *int `json:"round_limit"`
		DiceMin    *int `json:"dice_min"`
		DiceMax    *int `json:"dice_max"`
	} `json:"default_rules"`
	// default_rules と同じ理由で、無い行は 0 にせず読み出しの誤りにする
	TileCounts *struct {
		Land       int `json:"land"`
		Base       int `json:"base"`
		Checkpoint int `json:"checkpoint"`
		Fire       int `json:"fire"`
		Earth      int `json:"earth"`
		Water      int `json:"water"`
		Wind       int `json:"wind"`
		Neutral    int `json:"neutral"`
		Multi      int `json:"multi"`
		// 特殊なマスの数を写す前の行を、0 にせず読み出しの誤りにするため、ポインターで受ける
		Warp         *int `json:"warp"`
		Bridge       *int `json:"bridge"`
		Sanctuary    *int `json:"sanctuary"`
		Shrine       *int `json:"shrine"`
		ProphecyShop *int `json:"prophecy_shop"`
		CardShop     *int `json:"card_shop"`
	} `json:"tile_counts"`
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
	if metadata.DefaultRules == nil {
		return query.Board{}, fmt.Errorf("boards.metadata of %s: default_rules is missing", id)
	}
	for name, value := range map[string]*int{
		"round_limit": metadata.DefaultRules.RoundLimit, "dice_min": metadata.DefaultRules.DiceMin, "dice_max": metadata.DefaultRules.DiceMax,
	} {
		if value == nil {
			return query.Board{}, fmt.Errorf("boards.metadata of %s: default_rules.%s is missing", id, name)
		}
	}
	if metadata.TileCounts == nil {
		return query.Board{}, fmt.Errorf("boards.metadata of %s: tile_counts is missing", id)
	}
	if metadata.Pairs == nil {
		return query.Board{}, fmt.Errorf("boards.metadata of %s: pairs is missing", id)
	}
	c := metadata.TileCounts
	for name, count := range map[string]*int{
		"warp": c.Warp, "bridge": c.Bridge, "sanctuary": c.Sanctuary, "shrine": c.Shrine, "prophecy_shop": c.ProphecyShop, "card_shop": c.CardShop,
	} {
		if count == nil {
			return query.Board{}, fmt.Errorf("boards.metadata of %s: tile_counts.%s is missing", id, name)
		}
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
	pairs := make([]query.BoardPair, 0, len(*metadata.Pairs))
	for _, pair := range *metadata.Pairs {
		pairs = append(pairs, query.BoardPair{
			A: query.NewBoardPosition(pair.A.X, pair.A.Y),
			B: query.NewBoardPosition(pair.B.X, pair.B.Y),
		})
	}
	defaultRules := query.BoardDefaultRules{
		TargetMagic: metadata.DefaultRules.TargetMagic, StartingMagic: metadata.DefaultRules.StartingMagic,
		LapBonus: metadata.DefaultRules.LapBonus, CheckpointBonus: metadata.DefaultRules.CheckpointBonus,
		RoundLimit: *metadata.DefaultRules.RoundLimit, DiceMin: *metadata.DefaultRules.DiceMin, DiceMax: *metadata.DefaultRules.DiceMax,
	}
	tileCounts := query.NewBoardTileCounts(c.Land, c.Base, c.Checkpoint, c.Fire, c.Earth, c.Water, c.Wind, c.Neutral, c.Multi,
		*c.Warp, *c.Bridge, *c.Sanctuary, *c.Shrine, *c.ProphecyShop, *c.CardShop)
	return query.Board{ID: id, Name: metadata.Name, Width: metadata.Width, Height: metadata.Height, Tiles: tiles, Connections: connections, Pairs: pairs, DefaultRules: defaultRules, TileCounts: tileCounts}, nil
}

// 土地でないマスには属性が無い（空）
func optionalTileAttribute(value string) (query.BoardTileAttribute, error) {
	if value == "" {
		return "", nil
	}
	return query.NewBoardTileAttribute(value)
}
