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
type fakeNPCAvatarReader struct {
	called    bool
	condition query.ListNPCAvatarsCondition
	rows      []query.NPCAvatarListRow
}

func (r *fakeNPCAvatarReader) list(_ context.Context, condition query.ListNPCAvatarsCondition) ([]query.NPCAvatarListRow, error) {
	r.called = true
	r.condition = condition
	return r.rows, nil
}

func npcAvatarRow(name string) query.NPCAvatarListRow {
	id := uuid.New()
	npcAvatar := query.NPCAvatar{ID: id, Name: name}
	cursor := query.NPCAvatarListCursor{SortName: strings.ToLower(name), NPCAvatarID: id}
	return query.NPCAvatarListRow{NPCAvatar: npcAvatar, Cursor: cursor}
}

// 名前はカード名と同じ正規化をしてから、状態はそのまま渡す
func TestNPCAvatarHandler_PassesNormalizedConditionToReader(t *testing.T) {
	reader := &fakeNPCAvatarReader{}
	filter := query.NPCAvatarFilter{
		NameContains: ptr(" ＦIRE "),
		Statuses:     []readmodel.NPCAvatarStatus{readmodel.NPCAvatarStatusPublished},
	}

	_, err := query.NewNPCAvatarHandler(reader.list, nil).List(t.Context(), filter, ptr(10), nil)

	require.NoError(t, err)
	require.Equal(t, query.ListNPCAvatarsCondition{
		NameContains: "fire", Statuses: []readmodel.NPCAvatarStatus{readmodel.NPCAvatarStatusPublished}, Limit: 11,
	}, reader.condition)
}

func TestNPCAvatarHandler_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name         string
		nameContains *string
		statuses     []readmodel.NPCAvatarStatus
		first        *int
		after        *string
		want         []input.Error
	}{
		{name: "first_out_of_range", first: ptr(101), want: []input.Error{&input.OutOfRange{Field: "first", Min: 1, Max: 100}}},
		{name: "after_not_a_cursor", after: ptr("not-a-cursor"), want: []input.Error{&input.InvalidFormat{Field: "after"}}},
		{name: "empty_statuses", statuses: []readmodel.NPCAvatarStatus{}, want: []input.Error{&input.Required{Field: "filter.statuses"}}},
		{name: "blank_name_contains", nameContains: ptr(" "), want: []input.Error{&input.Required{Field: "filter.nameContains"}}},
		{
			name:         "too_long_name_contains",
			nameContains: ptr(strings.Repeat("あ", 51)),
			want:         []input.Error{&input.TooLong{Field: "filter.nameContains", Max: 50}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeNPCAvatarReader{}
			filter := query.NPCAvatarFilter{NameContains: tt.nameContains, Statuses: tt.statuses}

			_, err := query.NewNPCAvatarHandler(reader.list, nil).List(t.Context(), filter, tt.first, tt.after)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.ElementsMatch(t, tt.want, []input.Error(invalid))
			require.False(t, reader.called)
		})
	}
}

func TestNPCAvatarHandler_ListPagesWithCursorOfLastNPCAvatar(t *testing.T) {
	reader := &fakeNPCAvatarReader{rows: []query.NPCAvatarListRow{npcAvatarRow("A"), npcAvatarRow("B"), npcAvatarRow("C")}}

	connection, err := query.NewNPCAvatarHandler(reader.list, nil).List(t.Context(), query.NPCAvatarFilter{}, ptr(2), nil)

	require.NoError(t, err)
	require.Len(t, connection.Edges, 2)
	require.Equal(t, "A", connection.Edges[0].Node.Name)
	require.Equal(t, "B", connection.Edges[1].Node.Name)
	require.True(t, connection.PageInfo.HasNextPage)
}
