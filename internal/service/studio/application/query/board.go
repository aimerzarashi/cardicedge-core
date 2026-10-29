package query

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 公開 ID に当たるボードが read model に無い（無い・削除した・まだ投影されていない）
var ErrBoardNotFound = errors.New("query: board not found")

// presentation 層に read model の作りを見せず、表示に要る値だけを返す。マスは y → x、つながりは a → b の順。
// 一覧の要素にもマスとつながりを持たせる（スターターデッキの一覧が入れたカードを持つのと同じ。最大 32×32 でも、
// 1 ページ 100 件までなので、別に引き直す仕組みは作らなかった）
type Board struct {
	ID     uuid.UUID
	Name   string
	Status readmodel.BoardStatus
	Width  int
	Height int
	// 置いてあるマスだけ（置いていない座標は含まない）
	Tiles        []readmodel.BoardTile
	Connections  []readmodel.BoardConnection
	Pairs        []readmodel.BoardPair
	DefaultRules readmodel.BoardDefaultRules
	TileCounts   readmodel.BoardTileCounts
	// 公開できない理由（空なら公開できる）。投影が計算して持つ値をそのまま返す
	PublishProblems []readmodel.BoardPublishProblem
	// 集約の版（画面が変える操作に渡す）。投影が遅れているあいだは古いことがある（internal/eventstore/README.md）
	Revision     int64
	RegisteredAt time.Time
	UpdatedAt    time.Time
}

// Statuses が nil なら絞り込まない。空のリストは入力の誤り（CardFilter と同じ）
type BoardFilter struct {
	NameContains *string
	Statuses     []readmodel.BoardStatus
}

// 並び順（正規化した名前 → 公開 ID）の位置。項目名を 1 文字にする理由は CardListCursor と同じ
type BoardListCursor struct {
	SortName string    `json:"n"`
	BoardID  uuid.UUID `json:"i"`
}

func (c BoardListCursor) Validate() error {
	if c.SortName == "" || c.BoardID == uuid.Nil {
		return errors.New("sort name and board id are required")
	}
	return nil
}

// 読み出しの関数に渡す条件。値は application 層で確かめ・正規化した後のもの
type ListBoardsCondition struct {
	NameContains string
	Statuses     []readmodel.BoardStatus
	After        *BoardListCursor
	Limit        int
}

type BoardListRow struct {
	Board  Board
	Cursor BoardListCursor
}

type ListBoardsFunc func(ctx context.Context, condition ListBoardsCondition) ([]BoardListRow, error)

// 無ければ ErrBoardNotFound
type GetBoardFunc func(ctx context.Context, id uuid.UUID) (Board, error)

type BoardHandler struct {
	listBoards ListBoardsFunc
	getBoard   GetBoardFunc
}

func NewBoardHandler(listBoards ListBoardsFunc, getBoard GetBoardFunc) *BoardHandler {
	return &BoardHandler{listBoards: listBoards, getBoard: getBoard}
}

// 入力の誤りは input.Errors（まとめて返す）。項目名は GraphQL の引数・入力の項目名
func (q *BoardHandler) List(ctx context.Context, filter BoardFilter, first *int, after *string) (pagination.Connection[Board], error) {
	invalid := input.Errors{}
	page, err := pagination.NewCondition[BoardListCursor](first, after)
	if errors.Is(err, pagination.ErrFirstOutOfRange) {
		invalid = append(invalid, &input.OutOfRange{Field: "first", Min: pagination.MinFirst, Max: pagination.MaxFirst})
	}
	if errors.Is(err, pagination.ErrInvalidCursor) {
		invalid = append(invalid, &input.InvalidFormat{Field: "after"})
	}
	if err != nil && len(invalid) == 0 {
		return pagination.Connection[Board]{}, err
	}
	nameContains, nameErr := normalizeNameContains(filter.NameContains)
	if nameErr != nil {
		invalid = append(invalid, nameErr)
	}
	if filter.Statuses != nil && len(filter.Statuses) == 0 {
		invalid = append(invalid, &input.Required{Field: "filter.statuses"})
	}
	if len(invalid) > 0 {
		return pagination.Connection[Board]{}, invalid
	}
	rows, err := q.listBoards(ctx, ListBoardsCondition{
		NameContains: nameContains, Statuses: filter.Statuses, After: page.After, Limit: page.FetchLimit(),
	})
	if err != nil {
		return pagination.Connection[Board]{}, err
	}
	rowConnection, err := pagination.BuildConnection(rows, page, func(row BoardListRow) BoardListCursor {
		return row.Cursor
	})
	if err != nil {
		return pagination.Connection[Board]{}, err
	}
	connection := pagination.Connection[Board]{
		Edges:    make([]pagination.Edge[Board], 0, len(rowConnection.Edges)),
		PageInfo: rowConnection.PageInfo,
	}
	for _, edge := range rowConnection.Edges {
		connection.Edges = append(connection.Edges, pagination.Edge[Board]{Node: edge.Node.Board, Cursor: edge.Cursor})
	}
	return connection, nil
}

// 無ければ ErrBoardNotFound
func (q *BoardHandler) Get(ctx context.Context, id uuid.UUID) (Board, error) {
	return q.getBoard(ctx, id)
}
