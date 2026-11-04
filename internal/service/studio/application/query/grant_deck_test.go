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
type fakeGrantDeckReader struct {
	called    bool
	condition query.ListGrantDecksCondition
	rows      []query.GrantDeckListRow
}

func (r *fakeGrantDeckReader) list(_ context.Context, condition query.ListGrantDecksCondition) ([]query.GrantDeckListRow, error) {
	r.called = true
	r.condition = condition
	return r.rows, nil
}

func grantDeckRow(name string) query.GrantDeckListRow {
	id := uuid.New()
	deck := query.GrantDeck{ID: id, Name: name}
	cursor := query.GrantDeckListCursor{SortName: strings.ToLower(name), GrantDeckID: id}
	return query.GrantDeckListRow{GrantDeck: deck, Cursor: cursor}
}

// 名前はカード名と同じ正規化をしてから、状態はそのまま渡す
func TestGrantDeckHandler_PassesNormalizedConditionToReader(t *testing.T) {
	reader := &fakeGrantDeckReader{}
	filter := query.GrantDeckFilter{
		NameContains: ptr(" ＦIRE "),
		Statuses:     []readmodel.GrantDeckStatus{readmodel.GrantDeckStatusPublished},
	}

	_, err := query.NewGrantDeckHandler(reader.list, nil).List(t.Context(), filter, ptr(10), nil)

	require.NoError(t, err)
	require.Equal(t, query.ListGrantDecksCondition{
		NameContains: "fire", Statuses: []readmodel.GrantDeckStatus{readmodel.GrantDeckStatusPublished}, Limit: 11,
	}, reader.condition)
}

func TestGrantDeckHandler_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name         string
		nameContains *string
		statuses     []readmodel.GrantDeckStatus
		first        *int
		after        *string
		want         []input.Error
	}{
		{name: "first_out_of_range", first: ptr(101), want: []input.Error{&input.OutOfRange{Field: "first", Min: 1, Max: 100}}},
		{name: "after_not_a_cursor", after: ptr("not-a-cursor"), want: []input.Error{&input.InvalidFormat{Field: "after"}}},
		{name: "empty_statuses", statuses: []readmodel.GrantDeckStatus{}, want: []input.Error{&input.Required{Field: "filter.statuses"}}},
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
			reader := &fakeGrantDeckReader{}
			filter := query.GrantDeckFilter{NameContains: tt.nameContains, Statuses: tt.statuses}

			_, err := query.NewGrantDeckHandler(reader.list, nil).List(t.Context(), filter, tt.first, tt.after)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.False(t, reader.called)
		})
	}
}

func TestGrantDeckHandler_ReturnsGrantDecksWithCursorsAndNextPage(t *testing.T) {
	rows := []query.GrantDeckListRow{grantDeckRow("A"), grantDeckRow("B"), grantDeckRow("C")}
	reader := &fakeGrantDeckReader{rows: rows}

	connection, err := query.NewGrantDeckHandler(reader.list, nil).List(t.Context(), query.GrantDeckFilter{}, ptr(2), nil)

	require.NoError(t, err)
	require.Len(t, connection.Edges, 2)
	require.Equal(t, rows[0].GrantDeck, connection.Edges[0].Node)
	require.Equal(t, rows[1].GrantDeck, connection.Edges[1].Node)
	require.True(t, connection.PageInfo.HasNextPage)
}

func TestGrantDeckHandler_GetReturnsNotFoundForMissingGrantDeck(t *testing.T) {
	get := func(context.Context, uuid.UUID) (query.GrantDeck, error) {
		return query.GrantDeck{}, query.ErrGrantDeckNotFound
	}

	_, err := query.NewGrantDeckHandler(nil, get).Get(t.Context(), uuid.New())

	require.ErrorIs(t, err, query.ErrGrantDeckNotFound)
}
