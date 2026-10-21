package query_test

import (
	"context"
	"encoding/base64"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/pagination"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/input"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query/readmodel"
)

// 渡された条件を覚え、決めた行を返す読み出しの関数
type fakeReader struct {
	called    bool
	condition query.ListCardsCondition
	rows      []query.CardListRow
}

func (r *fakeReader) list(_ context.Context, condition query.ListCardsCondition) ([]query.CardListRow, error) {
	r.called = true
	r.condition = condition
	return r.rows, nil
}

func ptr[T any](value T) *T {
	return &value
}

func row(name string) query.CardListRow {
	id := uuid.New()
	card := query.Card{ID: id, Type: readmodel.CardTypeCreature, Name: name, RegisteredAt: time.Unix(0, 0).UTC(), UpdatedAt: time.Unix(0, 0).UTC()}
	cursor := query.CardListCursor{CardType: readmodel.CardTypeCreature, SortName: strings.ToLower(name), CardID: id}
	return query.CardListRow{Card: card, Cursor: cursor}
}

func encode(t *testing.T, cursor query.CardListCursor) string {
	t.Helper()
	encoded, err := pagination.EncodeCursor(cursor)
	require.NoError(t, err)
	return encoded
}

func TestCardHandler_PassesNormalizedConditionToReader(t *testing.T) {
	reader := &fakeReader{}
	filter := query.CardFilter{NameContains: ptr("  Ｄra "), Types: []readmodel.CardType{readmodel.CardTypeCreature}}

	_, err := query.NewCardHandler(reader.list, nil).List(t.Context(), filter, ptr(10), nil)

	require.NoError(t, err)
	require.Equal(t, query.ListCardsCondition{
		Types:        []readmodel.CardType{readmodel.CardTypeCreature},
		NameContains: "dra",
		// 次のページがあるかを知るため、1 件多く読む
		Limit: 11,
	}, reader.condition)
}

func TestCardHandler_PassesDecodedAfterCursorToReader(t *testing.T) {
	reader := &fakeReader{}
	after := query.CardListCursor{CardType: readmodel.CardTypeItem, SortName: "dragon", CardID: uuid.New()}

	_, err := query.NewCardHandler(reader.list, nil).List(t.Context(), query.CardFilter{}, nil, ptr(encode(t, after)))

	require.NoError(t, err)
	require.Equal(t, &after, reader.condition.After)
	require.Equal(t, pagination.DefaultFirst+1, reader.condition.Limit)
}

func TestCardHandler_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		filter func() query.CardFilter
		first  *int
		after  *string
		want   input.Error
	}{
		{name: "first_out_of_range", first: ptr(0), want: &input.OutOfRange{Field: "first", Min: 1, Max: 100}},
		{name: "after_not_a_cursor", after: ptr("not-a-cursor"), want: &input.InvalidFormat{Field: "after"}},
		{
			name: "blank_name_contains",
			filter: func() query.CardFilter {
				return query.CardFilter{NameContains: ptr("  ")}
			},
			want: &input.Required{Field: "filter.nameContains"},
		},
		{
			name: "too_long_name_contains",
			filter: func() query.CardFilter {
				return query.CardFilter{NameContains: ptr(strings.Repeat("あ", 51))}
			},
			want: &input.TooLong{Field: "filter.nameContains", Max: 50},
		},
		{
			name: "empty_types",
			filter: func() query.CardFilter {
				return query.CardFilter{Types: []readmodel.CardType{}}
			},
			want: &input.Required{Field: "filter.types"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeReader{}
			filter := query.CardFilter{}
			if tt.filter != nil {
				filter = tt.filter()
			}

			_, err := query.NewCardHandler(reader.list, nil).List(t.Context(), filter, tt.first, tt.after)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
			require.False(t, reader.called)
		})
	}
}

func TestCardHandler_RejectsCursorWithUnknownCardType(t *testing.T) {
	reader := &fakeReader{}
	// 種類の型は 1〜3 しか作れないため、JSON から作る
	after := base64.RawURLEncoding.EncodeToString([]byte(`{"t":9,"n":"dragon","i":"` + uuid.NewString() + `"}`))

	_, err := query.NewCardHandler(reader.list, nil).List(t.Context(), query.CardFilter{}, nil, &after)

	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, input.Errors{&input.InvalidFormat{Field: "after"}}, invalid)
}

func TestCardHandler_ReportsAllInvalidInputs(t *testing.T) {
	_, err := query.NewCardHandler((&fakeReader{}).list, nil).List(t.Context(), query.CardFilter{NameContains: ptr("")}, ptr(0), nil)

	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Len(t, invalid, 2)
}

func TestCardHandler_ReturnsCardsWithCursorsAndNextPage(t *testing.T) {
	rows := []query.CardListRow{row("Apple"), row("Banana"), row("Cherry")}
	reader := &fakeReader{rows: rows}

	connection, err := query.NewCardHandler(reader.list, nil).List(t.Context(), query.CardFilter{}, ptr(2), nil)

	require.NoError(t, err)
	require.Len(t, connection.Edges, 2)
	require.Equal(t, rows[0].Card, connection.Edges[0].Node)
	require.Equal(t, rows[1].Card, connection.Edges[1].Node)
	require.Equal(t, encode(t, rows[1].Cursor), connection.Edges[1].Cursor)
	require.True(t, connection.PageInfo.HasNextPage)
	require.False(t, connection.PageInfo.HasPreviousPage)
}

// カードの一覧と同じ並び（種類 → 正規化した名前 → 公開 ID）
func TestCompareCards_OrdersByCardTypeThenNormalizedName(t *testing.T) {
	card := func(cardType readmodel.CardType, name string) query.Card {
		return query.Card{ID: uuid.New(), Type: cardType, Name: name}
	}
	spell := card(readmodel.CardTypeSpell, "Ａ")
	item := card(readmodel.CardTypeItem, "Ｚ")
	goblin := card(readmodel.CardTypeCreature, "goblin")
	dragon := card(readmodel.CardTypeCreature, "Ｄragon")
	cards := []query.Card{spell, item, goblin, dragon}

	slices.SortFunc(cards, query.CompareCards)

	require.Equal(t, []query.Card{dragon, goblin, item, spell}, cards)
}
