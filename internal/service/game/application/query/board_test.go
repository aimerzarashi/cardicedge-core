package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

// 渡された条件を覚え、決めた行を返す読み出しの関数
type fakeBoardReader struct {
	called    bool
	condition query.ListPublishedBoardsCondition
	rows      []query.BoardListRow
}

func (r *fakeBoardReader) list(_ context.Context, condition query.ListPublishedBoardsCondition) ([]query.BoardListRow, error) {
	r.called = true
	r.condition = condition
	return r.rows, nil
}

func boardRow(name string) query.BoardListRow {
	id := uuid.New()
	board := query.Board{ID: id, Name: name}
	cursor := query.BoardListCursor{SortName: name, BoardID: id}
	return query.BoardListRow{Board: board, Cursor: cursor}
}

func intPtr(value int) *int {
	return &value
}

func stringPtr(value string) *string {
	return &value
}

// 次のページがあるかを知るため、1 件多く読む
func TestBoardHandler_ReadsOneMoreThanFirst(t *testing.T) {
	reader := &fakeBoardReader{}

	_, err := query.NewBoardHandler(reader.list, nil).List(t.Context(), intPtr(10), nil)

	require.NoError(t, err)
	require.Equal(t, query.ListPublishedBoardsCondition{Limit: 11}, reader.condition)
}

func TestBoardHandler_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		first *int
		after *string
		want  []input.Error
	}{
		{name: "first_out_of_range", first: intPtr(101), want: []input.Error{&input.OutOfRange{Field: "first", Min: 1, Max: 100}}},
		{name: "after_not_a_cursor", after: stringPtr("not-a-cursor"), want: []input.Error{&input.InvalidFormat{Field: "after"}}},
		{name: "both", first: intPtr(0), after: stringPtr("not-a-cursor"), want: []input.Error{
			&input.OutOfRange{Field: "first", Min: 1, Max: 100},
			&input.InvalidFormat{Field: "after"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeBoardReader{}

			_, err := query.NewBoardHandler(reader.list, nil).List(t.Context(), tt.first, tt.after)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.ElementsMatch(t, tt.want, []input.Error(invalid))
			require.False(t, reader.called)
		})
	}
}

func TestBoardHandler_ListPagesWithCursorOfLastBoard(t *testing.T) {
	reader := &fakeBoardReader{rows: []query.BoardListRow{boardRow("A"), boardRow("B"), boardRow("C")}}

	connection, err := query.NewBoardHandler(reader.list, nil).List(t.Context(), intPtr(2), nil)

	require.NoError(t, err)
	require.Len(t, connection.Edges, 2)
	require.Equal(t, "A", connection.Edges[0].Node.Name)
	require.Equal(t, "B", connection.Edges[1].Node.Name)
	require.True(t, connection.PageInfo.HasNextPage)
}

func TestBoardHandler_ListReturnsEmptyEdgesWhenNothingIsPublished(t *testing.T) {
	reader := &fakeBoardReader{}

	connection, err := query.NewBoardHandler(reader.list, nil).List(t.Context(), nil, nil)

	require.NoError(t, err)
	require.NotNil(t, connection.Edges)
	require.Empty(t, connection.Edges)
	require.False(t, connection.PageInfo.HasNextPage)
}
