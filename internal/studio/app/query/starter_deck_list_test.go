package query_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query"
)

// 渡された条件を覚え、決めた行を返す読み出しの関数
type fakeStarterDeckReader struct {
	called    bool
	condition query.ListStarterDecksCondition
	rows      []query.StarterDeckListRow
}

func (r *fakeStarterDeckReader) list(_ context.Context, condition query.ListStarterDecksCondition) ([]query.StarterDeckListRow, error) {
	r.called = true
	r.condition = condition
	return r.rows, nil
}

func starterDeckRow(name string) query.StarterDeckListRow {
	id := uuid.New()
	deck := query.StarterDeck{ID: id, Name: name}
	cursor := query.StarterDeckListCursor{SortName: strings.ToLower(name), StarterDeckID: id}
	return query.StarterDeckListRow{StarterDeck: deck, Cursor: cursor}
}

// 名前はカード名と同じ正規化をしてから渡す
func TestStarterDeckList_PassesNormalizedConditionToReader(t *testing.T) {
	reader := &fakeStarterDeckReader{}

	_, err := query.NewStarterDeckList(reader.list, nil).List(t.Context(), query.StarterDeckFilter{NameContains: ptr(" ＦIRE ")}, ptr(10), nil)

	require.NoError(t, err)
	require.Equal(t, query.ListStarterDecksCondition{NameContains: "fire", Limit: 11}, reader.condition)
}

func TestStarterDeckList_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name         string
		nameContains *string
		first        *int
		after        *string
		want         []input.Error
	}{
		{name: "first_out_of_range", first: ptr(101), want: []input.Error{&input.OutOfRange{Field: "first", Min: 1, Max: 100}}},
		{name: "after_not_a_cursor", after: ptr("not-a-cursor"), want: []input.Error{&input.InvalidFormat{Field: "after"}}},
		{
			name:         "blank_name_contains_and_first_out_of_range",
			nameContains: ptr(" "),
			first:        ptr(0),
			want: []input.Error{
				&input.OutOfRange{Field: "first", Min: 1, Max: 100},
				&input.Required{Field: "filter.nameContains"},
			},
		},
		{
			name:         "too_long_name_contains",
			nameContains: ptr(strings.Repeat("あ", 51)),
			want:         []input.Error{&input.TooLong{Field: "filter.nameContains", Max: 50}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeStarterDeckReader{}

			filter := query.StarterDeckFilter{NameContains: tt.nameContains}

			_, err := query.NewStarterDeckList(reader.list, nil).List(t.Context(), filter, tt.first, tt.after)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.False(t, reader.called)
		})
	}
}

func TestStarterDeckList_ReturnsStarterDecksWithCursorsAndNextPage(t *testing.T) {
	rows := []query.StarterDeckListRow{starterDeckRow("A"), starterDeckRow("B"), starterDeckRow("C")}
	reader := &fakeStarterDeckReader{rows: rows}

	connection, err := query.NewStarterDeckList(reader.list, nil).List(t.Context(), query.StarterDeckFilter{}, ptr(2), nil)

	require.NoError(t, err)
	require.Len(t, connection.Edges, 2)
	require.Equal(t, rows[0].StarterDeck, connection.Edges[0].Node)
	require.Equal(t, rows[1].StarterDeck, connection.Edges[1].Node)
	require.True(t, connection.PageInfo.HasNextPage)
}
