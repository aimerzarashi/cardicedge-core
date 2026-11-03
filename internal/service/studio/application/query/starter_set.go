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

// 公開 ID に当たる スターターセットが read model に無い（無い・削除した・まだ投影されていない）
var ErrStarterSetNotFound = errors.New("query: starter set not found")

// presentation 層に read model の作りを見せず、表示に要る値だけを返す
type StarterSet struct {
	ID     uuid.UUID
	Name   string
	Status readmodel.StarterSetStatus
	// 集約の版（画面が変える操作に渡す）。投影が遅れているあいだは古いことがある（internal/eventstore/README.md）
	Revision     int64
	RegisteredAt time.Time
	// 操作したオペレーターの公開 ID。GraphQL にはまだ出さない（オペレーターの read model と、何を見せるかが決まっていないため）
	RegisteredBy uuid.UUID
	UpdatedAt    time.Time
	UpdatedBy    uuid.UUID
	// 配るデッキ（配布デッキ）・解放するボードの公開 ID（集約が持つ順）
	GrantDecks   []uuid.UUID
	UnlockBoards []uuid.UUID
}

// Statuses が nil なら絞り込まない。空のリストは入力の誤り（CardFilter と同じ）
type StarterSetFilter struct {
	NameContains *string
	Statuses     []readmodel.StarterSetStatus
}

// 並び順（正規化した名前 → 公開 ID）の位置。項目名を 1 文字にする理由は CardListCursor と同じ
type StarterSetListCursor struct {
	SortName     string    `json:"n"`
	StarterSetID uuid.UUID `json:"i"`
}

func (c StarterSetListCursor) Validate() error {
	if c.SortName == "" || c.StarterSetID == uuid.Nil {
		return errors.New("sort name and starter set id are required")
	}
	return nil
}

// 読み出しの関数に渡す条件。値は application 層で確かめ・正規化した後のもの
type ListStarterSetsCondition struct {
	NameContains string
	Statuses     []readmodel.StarterSetStatus
	After        *StarterSetListCursor
	Limit        int
}

type StarterSetListRow struct {
	StarterSet StarterSet
	Cursor     StarterSetListCursor
}

type ListStarterSetsFunc func(ctx context.Context, condition ListStarterSetsCondition) ([]StarterSetListRow, error)

// 無ければ ErrStarterSetNotFound
type GetStarterSetFunc func(ctx context.Context, id uuid.UUID) (StarterSet, error)

type StarterSetHandler struct {
	listStarterSets ListStarterSetsFunc
	getStarterSet   GetStarterSetFunc
}

func NewStarterSetHandler(listStarterSets ListStarterSetsFunc, getStarterSet GetStarterSetFunc) *StarterSetHandler {
	return &StarterSetHandler{listStarterSets: listStarterSets, getStarterSet: getStarterSet}
}

// 入力の誤りは input.Errors（まとめて返す）。項目名は GraphQL の引数・入力の項目名
func (q *StarterSetHandler) List(ctx context.Context, filter StarterSetFilter, first *int, after *string) (pagination.Connection[StarterSet], error) {
	invalid := input.Errors{}
	page, err := pagination.NewCondition[StarterSetListCursor](first, after)
	if errors.Is(err, pagination.ErrFirstOutOfRange) {
		invalid = append(invalid, &input.OutOfRange{Field: "first", Min: pagination.MinFirst, Max: pagination.MaxFirst})
	}
	if errors.Is(err, pagination.ErrInvalidCursor) {
		invalid = append(invalid, &input.InvalidFormat{Field: "after"})
	}
	if err != nil && len(invalid) == 0 {
		return pagination.Connection[StarterSet]{}, err
	}
	nameContains, nameErr := normalizeNameContains(filter.NameContains)
	if nameErr != nil {
		invalid = append(invalid, nameErr)
	}
	if filter.Statuses != nil && len(filter.Statuses) == 0 {
		invalid = append(invalid, &input.Required{Field: "filter.statuses"})
	}
	if len(invalid) > 0 {
		return pagination.Connection[StarterSet]{}, invalid
	}
	rows, err := q.listStarterSets(ctx, ListStarterSetsCondition{
		NameContains: nameContains, Statuses: filter.Statuses, After: page.After, Limit: page.FetchLimit(),
	})
	if err != nil {
		return pagination.Connection[StarterSet]{}, err
	}
	rowConnection, err := pagination.BuildConnection(rows, page, func(row StarterSetListRow) StarterSetListCursor {
		return row.Cursor
	})
	if err != nil {
		return pagination.Connection[StarterSet]{}, err
	}
	connection := pagination.Connection[StarterSet]{
		Edges:    make([]pagination.Edge[StarterSet], 0, len(rowConnection.Edges)),
		PageInfo: rowConnection.PageInfo,
	}
	for _, edge := range rowConnection.Edges {
		connection.Edges = append(connection.Edges, pagination.Edge[StarterSet]{Node: edge.Node.StarterSet, Cursor: edge.Cursor})
	}
	return connection, nil
}

// 無ければ ErrStarterSetNotFound
func (q *StarterSetHandler) Get(ctx context.Context, id uuid.UUID) (StarterSet, error) {
	return q.getStarterSet(ctx, id)
}
