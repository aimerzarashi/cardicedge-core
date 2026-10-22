package query_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
)

// 渡された条件を覚え、決めた行を返す読み出しの関数
type fakePresetDeckReader struct {
	called    bool
	condition query.ListPresetDecksCondition
	rows      []query.PresetDeckListRow
}

func (r *fakePresetDeckReader) list(_ context.Context, condition query.ListPresetDecksCondition) ([]query.PresetDeckListRow, error) {
	r.called = true
	r.condition = condition
	return r.rows, nil
}

func presetDeckRow(name string) query.PresetDeckListRow {
	id := uuid.New()
	deck := query.PresetDeck{ID: id, Name: name}
	cursor := query.PresetDeckListCursor{SortName: strings.ToLower(name), PresetDeckID: id}
	return query.PresetDeckListRow{PresetDeck: deck, Cursor: cursor}
}

// 名前はカード名と同じ正規化をしてから渡す
func TestPresetDeckHandler_PassesNormalizedConditionToReader(t *testing.T) {
	reader := &fakePresetDeckReader{}

	_, err := query.NewPresetDeckHandler(reader.list, nil).List(t.Context(), query.PresetDeckFilter{NameContains: ptr(" ＦIRE ")}, ptr(10), nil)

	require.NoError(t, err)
	require.Equal(t, query.ListPresetDecksCondition{NameContains: "fire", Limit: 11}, reader.condition)
}

func TestPresetDeckHandler_RejectsInvalidInput(t *testing.T) {
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
			reader := &fakePresetDeckReader{}

			filter := query.PresetDeckFilter{NameContains: tt.nameContains}

			_, err := query.NewPresetDeckHandler(reader.list, nil).List(t.Context(), filter, tt.first, tt.after)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.False(t, reader.called)
		})
	}
}

func TestPresetDeckHandler_ReturnsPresetDecksWithCursorsAndNextPage(t *testing.T) {
	rows := []query.PresetDeckListRow{presetDeckRow("A"), presetDeckRow("B"), presetDeckRow("C")}
	reader := &fakePresetDeckReader{rows: rows}

	connection, err := query.NewPresetDeckHandler(reader.list, nil).List(t.Context(), query.PresetDeckFilter{}, ptr(2), nil)

	require.NoError(t, err)
	require.Len(t, connection.Edges, 2)
	require.Equal(t, rows[0].PresetDeck, connection.Edges[0].Node)
	require.Equal(t, rows[1].PresetDeck, connection.Edges[1].Node)
	require.True(t, connection.PageInfo.HasNextPage)
}
