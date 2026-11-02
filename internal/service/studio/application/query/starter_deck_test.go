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

func starterDeckRow(name string, source *query.StarterDeckSource) query.StarterDeckListRow {
	id := uuid.New()
	deck := query.StarterDeck{ID: id, Name: name, Source: source}
	cursor := query.StarterDeckListCursor{SortName: strings.ToLower(name), StarterDeckID: id}
	return query.StarterDeckListRow{StarterDeck: deck, Cursor: cursor}
}

// 名前はカード名と同じ正規化をしてから、状態はそのまま渡す
func TestStarterDeckHandler_PassesNormalizedConditionToReader(t *testing.T) {
	reader := &fakeStarterDeckReader{}
	filter := query.StarterDeckFilter{
		NameContains: ptr(" ＦIRE "),
		Statuses:     []readmodel.StarterDeckStatus{readmodel.StarterDeckStatusPublished},
	}

	_, err := query.NewStarterDeckHandler(reader.list, nil, noRevisions).List(t.Context(), filter, ptr(10), nil)

	require.NoError(t, err)
	require.Equal(t, query.ListStarterDecksCondition{
		NameContains: "fire", Statuses: []readmodel.StarterDeckStatus{readmodel.StarterDeckStatusPublished}, Limit: 11,
	}, reader.condition)
}

func TestStarterDeckHandler_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name         string
		nameContains *string
		statuses     []readmodel.StarterDeckStatus
		first        *int
		after        *string
		want         []input.Error
	}{
		{name: "first_out_of_range", first: ptr(101), want: []input.Error{&input.OutOfRange{Field: "first", Min: 1, Max: 100}}},
		{name: "after_not_a_cursor", after: ptr("not-a-cursor"), want: []input.Error{&input.InvalidFormat{Field: "after"}}},
		{name: "empty_statuses", statuses: []readmodel.StarterDeckStatus{}, want: []input.Error{&input.Required{Field: "filter.statuses"}}},
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
			filter := query.StarterDeckFilter{NameContains: tt.nameContains, Statuses: tt.statuses}

			_, err := query.NewStarterDeckHandler(reader.list, nil, noRevisions).List(t.Context(), filter, tt.first, tt.after)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.False(t, reader.called)
		})
	}
}

func TestStarterDeckHandler_ReturnsStarterDecksWithCursorsAndNextPage(t *testing.T) {
	rows := []query.StarterDeckListRow{starterDeckRow("A", nil), starterDeckRow("B", nil), starterDeckRow("C", nil)}
	reader := &fakeStarterDeckReader{rows: rows}

	connection, err := query.NewStarterDeckHandler(reader.list, nil, noRevisions).List(t.Context(), query.StarterDeckFilter{}, ptr(2), nil)

	require.NoError(t, err)
	require.Len(t, connection.Edges, 2)
	require.Equal(t, rows[0].StarterDeck, connection.Edges[0].Node)
	require.Equal(t, rows[1].StarterDeck, connection.Edges[1].Node)
	require.True(t, connection.PageInfo.HasNextPage)
}

// 写した後に元が変わっていれば changedSinceCopied が true。同じ revision なら false。元が消されていれば比べる相手が無いので
// changedSinceCopied は false で、presetDeckDeleted が true。空から作ったもの（写し元が無い）は比べない
func TestStarterDeckHandler_ComparesSourceWithCurrentPresetDeck(t *testing.T) {
	unchanged, changed, deleted := uuid.New(), uuid.New(), uuid.New()
	source := func(presetDeckID uuid.UUID) *query.StarterDeckSource {
		return &query.StarterDeckSource{PresetDeckID: presetDeckID, Revision: 5}
	}
	rows := []query.StarterDeckListRow{
		starterDeckRow("unchanged", source(unchanged)),
		starterDeckRow("changed", source(changed)),
		starterDeckRow("deleted", source(deleted)),
		starterDeckRow("empty", nil),
	}
	reader := &fakeStarterDeckReader{rows: rows}
	revisions := func(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]int64, error) {
		require.ElementsMatch(t, []uuid.UUID{unchanged, changed, deleted}, ids)
		return map[uuid.UUID]int64{unchanged: 5, changed: 6}, nil
	}

	connection, err := query.NewStarterDeckHandler(reader.list, nil, revisions).List(t.Context(), query.StarterDeckFilter{}, nil, nil)

	require.NoError(t, err)
	changedSinceCopied := map[string]bool{}
	presetDeckDeleted := map[string]bool{}
	for _, edge := range connection.Edges {
		if edge.Node.Source == nil {
			continue
		}
		changedSinceCopied[edge.Node.Name] = edge.Node.Source.ChangedSinceCopied
		presetDeckDeleted[edge.Node.Name] = edge.Node.Source.PresetDeckDeleted
	}
	require.Equal(t, map[string]bool{"unchanged": false, "changed": true, "deleted": false}, changedSinceCopied)
	require.Equal(t, map[string]bool{"unchanged": false, "changed": false, "deleted": true}, presetDeckDeleted)
	require.Nil(t, connection.Edges[3].Node.Source)
	// 読み出した値を書き換えない
	require.False(t, rows[1].StarterDeck.Source.ChangedSinceCopied)
}

func TestStarterDeckHandler_GetReturnsNotFoundForMissingStarterDeck(t *testing.T) {
	get := func(context.Context, uuid.UUID) (query.StarterDeck, error) {
		return query.StarterDeck{}, query.ErrStarterDeckNotFound
	}

	_, err := query.NewStarterDeckHandler(nil, get, noRevisions).Get(t.Context(), uuid.New())

	require.ErrorIs(t, err, query.ErrStarterDeckNotFound)
}

func TestStarterDeckHandler_GetShowsWhetherSourceChanged(t *testing.T) {
	presetDeckID := uuid.New()
	get := func(context.Context, uuid.UUID) (query.StarterDeck, error) {
		return query.StarterDeck{Source: &query.StarterDeckSource{PresetDeckID: presetDeckID, Revision: 2}}, nil
	}
	revisions := func(context.Context, []uuid.UUID) (map[uuid.UUID]int64, error) {
		return map[uuid.UUID]int64{presetDeckID: 3}, nil
	}

	deck, err := query.NewStarterDeckHandler(nil, get, revisions).Get(t.Context(), uuid.New())

	require.NoError(t, err)
	require.True(t, deck.Source.ChangedSinceCopied)
	require.False(t, deck.Source.PresetDeckDeleted)
}
