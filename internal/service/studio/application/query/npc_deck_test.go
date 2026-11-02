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

func npcDeckRow(name string, source *query.NPCDeckSource) query.NPCDeckListRow {
	id := uuid.New()
	npcDeck := query.NPCDeck{ID: id, Name: name, Source: source}
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

	_, err := query.NewNPCDeckHandler(reader.list, nil, noRevisions).List(t.Context(), filter, ptr(10), nil)

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

			_, err := query.NewNPCDeckHandler(reader.list, nil, noRevisions).List(t.Context(), filter, tt.first, tt.after)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.False(t, reader.called)
		})
	}
}

func TestNPCDeckHandler_ReturnsNPCDecksWithCursorsAndNextPage(t *testing.T) {
	rows := []query.NPCDeckListRow{npcDeckRow("A", nil), npcDeckRow("B", nil), npcDeckRow("C", nil)}
	reader := &fakeNPCDeckReader{rows: rows}

	connection, err := query.NewNPCDeckHandler(reader.list, nil, noRevisions).List(t.Context(), query.NPCDeckFilter{}, ptr(2), nil)

	require.NoError(t, err)
	require.Len(t, connection.Edges, 2)
	require.Equal(t, rows[0].NPCDeck, connection.Edges[0].Node)
	require.Equal(t, rows[1].NPCDeck, connection.Edges[1].Node)
	require.True(t, connection.PageInfo.HasNextPage)
}

// 写した後に元が変わっていれば changedSinceCopied が true。同じ revision なら false。元が消されていれば比べる相手が無いので
// changedSinceCopied は false で、presetDeckDeleted が true。空から作ったもの（写し元が無い）は比べない
func TestNPCDeckHandler_ComparesSourceWithCurrentPresetDeck(t *testing.T) {
	unchanged, changed, deleted := uuid.New(), uuid.New(), uuid.New()
	source := func(presetDeckID uuid.UUID) *query.NPCDeckSource {
		return &query.NPCDeckSource{PresetDeckID: presetDeckID, Revision: 5}
	}
	rows := []query.NPCDeckListRow{
		npcDeckRow("unchanged", source(unchanged)),
		npcDeckRow("changed", source(changed)),
		npcDeckRow("deleted", source(deleted)),
		npcDeckRow("empty", nil),
	}
	reader := &fakeNPCDeckReader{rows: rows}
	revisions := func(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]int64, error) {
		require.ElementsMatch(t, []uuid.UUID{unchanged, changed, deleted}, ids)
		return map[uuid.UUID]int64{unchanged: 5, changed: 6}, nil
	}

	connection, err := query.NewNPCDeckHandler(reader.list, nil, revisions).List(t.Context(), query.NPCDeckFilter{}, nil, nil)

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
	require.False(t, rows[1].NPCDeck.Source.ChangedSinceCopied)
}

func TestNPCDeckHandler_GetReturnsNotFoundForMissingNPCDeck(t *testing.T) {
	get := func(context.Context, uuid.UUID) (query.NPCDeck, error) {
		return query.NPCDeck{}, query.ErrNPCDeckNotFound
	}

	_, err := query.NewNPCDeckHandler(nil, get, noRevisions).Get(t.Context(), uuid.New())

	require.ErrorIs(t, err, query.ErrNPCDeckNotFound)
}

func TestNPCDeckHandler_GetShowsWhetherSourceChanged(t *testing.T) {
	presetDeckID := uuid.New()
	get := func(context.Context, uuid.UUID) (query.NPCDeck, error) {
		return query.NPCDeck{Source: &query.NPCDeckSource{PresetDeckID: presetDeckID, Revision: 2}}, nil
	}
	revisions := func(context.Context, []uuid.UUID) (map[uuid.UUID]int64, error) {
		return map[uuid.UUID]int64{presetDeckID: 3}, nil
	}

	npcDeck, err := query.NewNPCDeckHandler(nil, get, revisions).Get(t.Context(), uuid.New())

	require.NoError(t, err)
	require.True(t, npcDeck.Source.ChangedSinceCopied)
	require.False(t, npcDeck.Source.PresetDeckDeleted)
}
