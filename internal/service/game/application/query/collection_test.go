package query_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
)

// 渡された持ち主と条件を覚え、決めた行を返す読み出しの関数
type fakeCollectionReader struct {
	called    bool
	playerID  uuid.UUID
	condition query.ListCollectionCondition
	rows      []query.CollectionListRow
}

func (r *fakeCollectionReader) list(_ context.Context, playerID uuid.UUID, condition query.ListCollectionCondition) ([]query.CollectionListRow, error) {
	r.called = true
	r.playerID = playerID
	r.condition = condition
	return r.rows, nil
}

// 項目に入れる値は生成済みの変数で渡す（factory_enforcer）ため、行は変数を経由して作る
func collectionRowOf(id uuid.UUID, name string, cardType query.CardType, count int) query.CollectionListRow {
	card := query.Card{ID: id, Name: name, Type: cardType}
	owned := query.CollectionCard{Card: card, Count: count}
	cursor := query.CollectionListCursor{CardType: cardType, SortName: name, CardID: id}
	return query.CollectionListRow{Card: owned, Cursor: cursor}
}

func collectionRow(name string, count int) query.CollectionListRow {
	return collectionRowOf(uuid.New(), name, query.CardTypeCreature, count)
}

// 次のページがあるかを知るため、1 件多く読む
func TestCollectionHandler_ReadsOneMoreThanFirstForPlayerAndPassesTypes(t *testing.T) {
	reader := &fakeCollectionReader{}
	playerID := uuid.New()

	_, err := query.NewCollectionHandler(reader.list).List(t.Context(), playerID, query.CollectionFilter{Types: []query.CardType{query.CardTypeSpell}}, intPtr(10), nil)

	require.NoError(t, err)
	require.Equal(t, playerID, reader.playerID)
	require.Equal(t, query.ListCollectionCondition{Types: []query.CardType{query.CardTypeSpell}, Limit: 11}, reader.condition)
}

func TestCollectionHandler_UsesDefaultFirstAndNoTypeFilterWhenOmitted(t *testing.T) {
	reader := &fakeCollectionReader{}

	_, err := query.NewCollectionHandler(reader.list).List(t.Context(), uuid.New(), query.CollectionFilter{}, nil, nil)

	require.NoError(t, err)
	require.Equal(t, query.ListCollectionCondition{Limit: pagination.DefaultFirst + 1}, reader.condition)
}

func TestCollectionHandler_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		types []query.CardType
		first *int
		after *string
		want  []input.Error
	}{
		{name: "empty_types", types: []query.CardType{}, want: []input.Error{&input.Required{Field: "filter.types"}}},
		{name: "first_out_of_range", first: intPtr(101), want: []input.Error{&input.OutOfRange{Field: "first", Min: 1, Max: 100}}},
		{name: "after_not_a_cursor", after: stringPtr("not-a-cursor"), want: []input.Error{&input.InvalidFormat{Field: "after"}}},
		{name: "all", types: []query.CardType{}, first: intPtr(0), after: stringPtr("not-a-cursor"), want: []input.Error{
			&input.Required{Field: "filter.types"},
			&input.OutOfRange{Field: "first", Min: 1, Max: 100},
			&input.InvalidFormat{Field: "after"},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeCollectionReader{}

			_, err := query.NewCollectionHandler(reader.list).List(t.Context(), uuid.New(), query.CollectionFilter{Types: tt.types}, tt.first, tt.after)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.ElementsMatch(t, tt.want, []input.Error(invalid))
			require.False(t, reader.called)
		})
	}
}

func TestCollectionHandler_ListPagesWithCursorOfLastCard(t *testing.T) {
	reader := &fakeCollectionReader{rows: []query.CollectionListRow{collectionRow("A", 3), collectionRow("B", 1), collectionRow("C", 2)}}

	connection, err := query.NewCollectionHandler(reader.list).List(t.Context(), uuid.New(), query.CollectionFilter{}, intPtr(2), nil)

	require.NoError(t, err)
	require.Len(t, connection.Edges, 2)
	require.Equal(t, "A", connection.Edges[0].Node.Card.Name)
	require.Equal(t, 3, connection.Edges[0].Node.Count)
	require.Equal(t, "B", connection.Edges[1].Node.Card.Name)
	require.True(t, connection.PageInfo.HasNextPage)
	require.NotNil(t, connection.PageInfo.EndCursor)
	require.Equal(t, connection.Edges[1].Cursor, *connection.PageInfo.EndCursor)
}

func TestCollectionHandler_ListReturnsEmptyEdgesWhenNothingIsOwned(t *testing.T) {
	reader := &fakeCollectionReader{}

	connection, err := query.NewCollectionHandler(reader.list).List(t.Context(), uuid.New(), query.CollectionFilter{}, nil, nil)

	require.NoError(t, err)
	require.NotNil(t, connection.Edges)
	require.Empty(t, connection.Edges)
	require.False(t, connection.PageInfo.HasNextPage)
}

func TestCollectionHandler_PassesDecodedCursorToReader(t *testing.T) {
	id := uuid.New()
	reader := &fakeCollectionReader{rows: []query.CollectionListRow{collectionRowOf(id, "a", query.CardTypeItem, 1), collectionRow("B", 1)}}
	handler := query.NewCollectionHandler(reader.list)
	page, err := handler.List(t.Context(), uuid.New(), query.CollectionFilter{}, intPtr(1), nil)
	require.NoError(t, err)

	_, err = handler.List(t.Context(), uuid.New(), query.CollectionFilter{}, intPtr(1), &page.Edges[0].Cursor)

	require.NoError(t, err)
	require.Equal(t, &query.CollectionListCursor{CardType: query.CardTypeItem, SortName: "a", CardID: id}, reader.condition.After)
}
