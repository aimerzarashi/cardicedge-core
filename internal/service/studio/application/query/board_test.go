package query_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
)

// 渡された条件を覚え、決めた行を返す読み出しの関数
type fakeBoardReader struct {
	called    bool
	condition query.ListBoardsCondition
	rows      []query.BoardListRow
}

func (r *fakeBoardReader) list(_ context.Context, condition query.ListBoardsCondition) ([]query.BoardListRow, error) {
	r.called = true
	r.condition = condition
	return r.rows, nil
}

func boardRow(name string) query.BoardListRow {
	id := uuid.New()
	board := query.Board{ID: id, Name: name}
	cursor := query.BoardListCursor{SortName: strings.ToLower(name), BoardID: id}
	return query.BoardListRow{Board: board, Cursor: cursor}
}

// 名前はカード名と同じ正規化をしてから、状態はそのまま渡す
func TestBoardHandler_PassesNormalizedConditionToReader(t *testing.T) {
	reader := &fakeBoardReader{}
	filter := query.BoardFilter{
		NameContains: ptr(" ＦIRE "),
		Statuses:     []readmodel.BoardStatus{readmodel.BoardStatusPublished},
	}

	_, err := query.NewBoardHandler(reader.list, nil).List(t.Context(), filter, ptr(10), nil)

	require.NoError(t, err)
	require.Equal(t, query.ListBoardsCondition{
		NameContains: "fire", Statuses: []readmodel.BoardStatus{readmodel.BoardStatusPublished}, Limit: 11,
	}, reader.condition)
}

func TestBoardHandler_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name         string
		nameContains *string
		statuses     []readmodel.BoardStatus
		first        *int
		after        *string
		want         []input.Error
	}{
		{name: "first_out_of_range", first: ptr(101), want: []input.Error{&input.OutOfRange{Field: "first", Min: 1, Max: 100}}},
		{name: "after_not_a_cursor", after: ptr("not-a-cursor"), want: []input.Error{&input.InvalidFormat{Field: "after"}}},
		{name: "empty_statuses", statuses: []readmodel.BoardStatus{}, want: []input.Error{&input.Required{Field: "filter.statuses"}}},
		{name: "blank_name_contains", nameContains: ptr(" "), want: []input.Error{&input.Required{Field: "filter.nameContains"}}},
		{
			name:         "too_long_name_contains",
			nameContains: ptr(strings.Repeat("あ", 51)),
			want:         []input.Error{&input.TooLong{Field: "filter.nameContains", Max: 50}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeBoardReader{}
			filter := query.BoardFilter{NameContains: tt.nameContains, Statuses: tt.statuses}

			_, err := query.NewBoardHandler(reader.list, nil).List(t.Context(), filter, tt.first, tt.after)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.ElementsMatch(t, tt.want, []input.Error(invalid))
			require.False(t, reader.called)
		})
	}
}

func TestBoardHandler_ListPagesWithCursorOfLastBoard(t *testing.T) {
	reader := &fakeBoardReader{rows: []query.BoardListRow{boardRow("A"), boardRow("B"), boardRow("C")}}

	connection, err := query.NewBoardHandler(reader.list, nil).List(t.Context(), query.BoardFilter{}, ptr(2), nil)

	require.NoError(t, err)
	require.Len(t, connection.Edges, 2)
	require.Equal(t, "A", connection.Edges[0].Node.Name)
	require.Equal(t, "B", connection.Edges[1].Node.Name)
	require.True(t, connection.PageInfo.HasNextPage)
}
