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
type fakeNPCAvatarReader struct {
	called    bool
	condition query.ListPublishedNPCAvatarsCondition
	rows      []query.NPCAvatarListRow
	decks     []query.NPCDeck
	// デッキの読み出しに渡された ID の一覧の履歴
	deckCalls [][]uuid.UUID
}

func (r *fakeNPCAvatarReader) list(_ context.Context, condition query.ListPublishedNPCAvatarsCondition) ([]query.NPCAvatarListRow, error) {
	r.called = true
	r.condition = condition
	return r.rows, nil
}

func (r *fakeNPCAvatarReader) listDecks(_ context.Context, ids []uuid.UUID) ([]query.NPCDeck, error) {
	r.deckCalls = append(r.deckCalls, ids)
	return r.decks, nil
}

func npcAvatarRow(name string, deckIDs ...uuid.UUID) query.NPCAvatarListRow {
	id := uuid.New()
	avatar := query.NPCAvatarRecord{ID: id, Name: name, DeckIDs: deckIDs}
	cursor := query.NPCAvatarListCursor{SortName: name, NPCAvatarID: id}
	return query.NPCAvatarListRow{Avatar: avatar, Cursor: cursor}
}

// 次のページがあるかを知るため、1 件多く読む
func TestNPCAvatarHandler_ReadsOneMoreThanFirst(t *testing.T) {
	reader := &fakeNPCAvatarReader{}

	_, err := query.NewNPCAvatarHandler(reader.list, nil, reader.listDecks).List(t.Context(), intPtr(10), nil)

	require.NoError(t, err)
	require.Equal(t, query.ListPublishedNPCAvatarsCondition{Limit: 11}, reader.condition)
}

func TestNPCAvatarHandler_RejectsInvalidInput(t *testing.T) {
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
			reader := &fakeNPCAvatarReader{}

			_, err := query.NewNPCAvatarHandler(reader.list, nil, reader.listDecks).List(t.Context(), tt.first, tt.after)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.ElementsMatch(t, tt.want, []input.Error(invalid))
			require.False(t, reader.called)
		})
	}
}

func TestNPCAvatarHandler_ListPagesWithCursorOfLastAvatar(t *testing.T) {
	reader := &fakeNPCAvatarReader{rows: []query.NPCAvatarListRow{npcAvatarRow("A"), npcAvatarRow("B"), npcAvatarRow("C")}}

	connection, err := query.NewNPCAvatarHandler(reader.list, nil, reader.listDecks).List(t.Context(), intPtr(2), nil)

	require.NoError(t, err)
	require.Len(t, connection.Edges, 2)
	require.True(t, connection.PageInfo.HasNextPage)
	require.Equal(t, "A", connection.Edges[0].Node.Name)
	require.Equal(t, "B", connection.Edges[1].Node.Name)
}

// 1 ページ分のデッキを 1 回の読み出しにまとめ（アバターごとに読まない）、同じデッキは重ねて頼まない
func TestNPCAvatarHandler_ListReadsDecksOfWholePageOnce(t *testing.T) {
	d1, d2 := uuid.New(), uuid.New()
	reader := &fakeNPCAvatarReader{rows: []query.NPCAvatarListRow{npcAvatarRow("A", d1, d2), npcAvatarRow("B", d1)}}

	_, err := query.NewNPCAvatarHandler(reader.list, nil, reader.listDecks).List(t.Context(), nil, nil)

	require.NoError(t, err)
	require.Equal(t, [][]uuid.UUID{{d1, d2}}, reader.deckCalls)
}

func TestNPCAvatarHandler_ListSkipsDeckReadWhenNoDeckIsAssigned(t *testing.T) {
	reader := &fakeNPCAvatarReader{rows: []query.NPCAvatarListRow{npcAvatarRow("A")}}

	connection, err := query.NewNPCAvatarHandler(reader.list, nil, reader.listDecks).List(t.Context(), nil, nil)

	require.NoError(t, err)
	require.Empty(t, reader.deckCalls)
	require.Empty(t, connection.Edges[0].Node.Decks)
}

// 読み出しは公開中のデッキだけを順不同で返す。アバターごとに割り当てた順に並べ、公開中でない（読み出しに無い）デッキは飛ばす
func TestNPCAvatarHandler_ListOrdersDecksByAssignmentAndSkipsUnpublished(t *testing.T) {
	d1, d2, unpublished := uuid.New(), uuid.New(), uuid.New()
	reader := &fakeNPCAvatarReader{
		rows:  []query.NPCAvatarListRow{npcAvatarRow("A", d2, unpublished, d1), npcAvatarRow("B", d1)},
		decks: []query.NPCDeck{{ID: d1, Name: "1"}, {ID: d2, Name: "2"}},
	}

	connection, err := query.NewNPCAvatarHandler(reader.list, nil, reader.listDecks).List(t.Context(), nil, nil)

	require.NoError(t, err)
	require.Equal(t, []query.NPCDeck{{ID: d2, Name: "2"}, {ID: d1, Name: "1"}}, connection.Edges[0].Node.Decks)
	require.Equal(t, []query.NPCDeck{{ID: d1, Name: "1"}}, connection.Edges[1].Node.Decks)
}

func TestNPCAvatarHandler_GetReturnsAvatarWithPublishedDecksInAssignedOrder(t *testing.T) {
	d1, d2, unpublished := uuid.New(), uuid.New(), uuid.New()
	id := uuid.New()
	reader := &fakeNPCAvatarReader{decks: []query.NPCDeck{{ID: d1, Name: "1"}, {ID: d2, Name: "2"}}}
	get := func(context.Context, uuid.UUID) (query.NPCAvatarRecord, error) {
		return query.NPCAvatarRecord{ID: id, Name: "A", DeckIDs: []uuid.UUID{d2, unpublished, d1}}, nil
	}

	got, err := query.NewNPCAvatarHandler(reader.list, get, reader.listDecks).Get(t.Context(), id)

	require.NoError(t, err)
	require.Equal(t, query.NPCAvatar{ID: id, Name: "A", Decks: []query.NPCDeck{{ID: d2, Name: "2"}, {ID: d1, Name: "1"}}}, got)
}

func TestNPCAvatarHandler_GetReturnsNotFoundWithoutReadingDecks(t *testing.T) {
	reader := &fakeNPCAvatarReader{}
	get := func(context.Context, uuid.UUID) (query.NPCAvatarRecord, error) {
		return query.NPCAvatarRecord{}, query.ErrNPCAvatarNotFound
	}

	_, err := query.NewNPCAvatarHandler(reader.list, get, reader.listDecks).Get(t.Context(), uuid.New())

	require.ErrorIs(t, err, query.ErrNPCAvatarNotFound)
	require.Empty(t, reader.deckCalls)
}

func TestNPCDeck_TotalCountSumsCardCounts(t *testing.T) {
	deck := query.NPCDeck{Cards: []query.NPCDeckCard{{Count: 3}, {Count: 2}}}

	require.Equal(t, 5, deck.TotalCount())
}
