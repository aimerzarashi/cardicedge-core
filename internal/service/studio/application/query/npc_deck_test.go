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
type fakeNPCDeckReader struct {
	called    bool
	condition query.ListNPCDecksCondition
	rows      []query.NPCDeckListRow
}

func (r *fakeNPCDeckReader) list(_ context.Context, condition query.ListNPCDecksCondition) ([]query.NPCDeckListRow, error) {
	r.called = true
	r.condition = condition
	return r.rows, nil
}

func npcDeckRow(name string) query.NPCDeckListRow {
	id := uuid.New()
	npcDeck := query.NPCDeck{ID: id, Name: name}
	cursor := query.NPCDeckListCursor{SortName: strings.ToLower(name), NPCDeckID: id}
	return query.NPCDeckListRow{NPCDeck: npcDeck, Cursor: cursor}
}

// 名前はカード名と同じ正規化をしてから、状態はそのまま渡す
func TestNPCDeckHandler_PassesNormalizedConditionToReader(t *testing.T) {
	reader := &fakeNPCDeckReader{}
	filter := query.NPCDeckFilter{
		NameContains: ptr(" ＦIRE "),
		Statuses:     []readmodel.NPCDeckStatus{readmodel.NPCDeckStatusPublished},
	}

	_, err := query.NewNPCDeckHandler(reader.list, nil).List(t.Context(), filter, ptr(10), nil)

	require.NoError(t, err)
	require.Equal(t, query.ListNPCDecksCondition{
		NameContains: "fire", Statuses: []readmodel.NPCDeckStatus{readmodel.NPCDeckStatusPublished}, Limit: 11,
	}, reader.condition)
}

func TestNPCDeckHandler_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name         string
		nameContains *string
		statuses     []readmodel.NPCDeckStatus
		first        *int
		after        *string
		want         []input.Error
	}{
		{name: "first_out_of_range", first: ptr(101), want: []input.Error{&input.OutOfRange{Field: "first", Min: 1, Max: 100}}},
		{name: "after_not_a_cursor", after: ptr("not-a-cursor"), want: []input.Error{&input.InvalidFormat{Field: "after"}}},
		{name: "empty_statuses", statuses: []readmodel.NPCDeckStatus{}, want: []input.Error{&input.Required{Field: "filter.statuses"}}},
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
			reader := &fakeNPCDeckReader{}
			filter := query.NPCDeckFilter{NameContains: tt.nameContains, Statuses: tt.statuses}

			_, err := query.NewNPCDeckHandler(reader.list, nil).List(t.Context(), filter, tt.first, tt.after)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.False(t, reader.called)
		})
	}
}

func TestNPCDeckHandler_ReturnsNPCDecksWithCursorsAndNextPage(t *testing.T) {
	rows := []query.NPCDeckListRow{npcDeckRow("A"), npcDeckRow("B"), npcDeckRow("C")}
	reader := &fakeNPCDeckReader{rows: rows}

	connection, err := query.NewNPCDeckHandler(reader.list, nil).List(t.Context(), query.NPCDeckFilter{}, ptr(2), nil)

	require.NoError(t, err)
	require.Len(t, connection.Edges, 2)
	require.Equal(t, rows[0].NPCDeck, connection.Edges[0].Node)
	require.Equal(t, rows[1].NPCDeck, connection.Edges[1].Node)
	require.True(t, connection.PageInfo.HasNextPage)
}

func TestNPCDeckHandler_GetReturnsNotFoundForMissingNPCDeck(t *testing.T) {
	get := func(context.Context, uuid.UUID) (query.NPCDeck, error) {
		return query.NPCDeck{}, query.ErrNPCDeckNotFound
	}

	_, err := query.NewNPCDeckHandler(nil, get).Get(t.Context(), uuid.New())

	require.ErrorIs(t, err, query.ErrNPCDeckNotFound)
}
