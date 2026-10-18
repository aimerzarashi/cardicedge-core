package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb/rdbtest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/app/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/studio/inf/repository"
)

// 投影が書いたものを、問い合わせが読む形で確かめる（表の作りをテストに書かない）。
// 件数・順番を確かめるため、ほかの行の無い DB（テンプレートの複製）で動かす
func emptyCardLists(t *testing.T) context.Context {
	t.Helper()
	db, _ := rdbtest.NewDatabase(t)
	return rdb.WithDB(t.Context(), db)
}

func cardList(cardType readmodel.CardType, sortName, name string) readmodel.CardList {
	id := uuid.New()
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	metadata := readmodel.CardListMetadata{ID: id, Name: name, RegisteredAt: at, UpdatedAt: at}
	return readmodel.CardList{CardID: id, CardType: cardType, SortName: sortName, Metadata: metadata}
}

func put(t *testing.T, ctx context.Context, cardType readmodel.CardType, sortName, name string) readmodel.CardList {
	t.Helper()
	list := cardList(cardType, sortName, name)
	require.NoError(t, repository.UpsertCardList(ctx, list))
	return list
}

func listAll(t *testing.T, ctx context.Context) []query.CardListRow {
	t.Helper()
	rows, err := repository.ListCards(ctx, query.ListCardsCondition{Limit: 100})
	require.NoError(t, err)
	return rows
}

func names(rows []query.CardListRow) []string {
	result := make([]string, len(rows))
	for i, row := range rows {
		result[i] = row.Card.Name
	}
	return result
}

func TestCardList_ListsUpsertedCardWithDisplayValuesAndCursor(t *testing.T) {
	ctx := emptyCardLists(t)
	list := put(t, ctx, readmodel.CardTypeCreature, "dragon", "Ｄragon")

	rows := listAll(t, ctx)

	require.Len(t, rows, 1)
	card := rows[0].Card
	require.Equal(t, list.CardID, card.ID)
	require.Equal(t, readmodel.CardTypeCreature, card.Type)
	require.Equal(t, "Ｄragon", card.Name)
	require.Equal(t, list.Metadata.RegisteredAt, card.RegisteredAt.UTC())
	require.Equal(t, list.Metadata.UpdatedAt, card.UpdatedAt.UTC())
	require.Equal(t, query.CardListCursor{CardType: readmodel.CardTypeCreature, SortName: "dragon", CardID: list.CardID}, rows[0].Cursor)
}

func TestCardList_UpsertOverwritesSameCard(t *testing.T) {
	ctx := emptyCardLists(t)
	list := put(t, ctx, readmodel.CardTypeCreature, "dragon", "Dragon")
	list.SortName = "goblin"
	list.Metadata.Name = "Goblin"

	require.NoError(t, repository.UpsertCardList(ctx, list))

	require.Equal(t, []string{"Goblin"}, names(listAll(t, ctx)))
}

func TestCardList_DeleteRemovesCardFromList(t *testing.T) {
	ctx := emptyCardLists(t)
	deleted := put(t, ctx, readmodel.CardTypeCreature, "dragon", "Dragon")
	put(t, ctx, readmodel.CardTypeCreature, "goblin", "Goblin")

	require.NoError(t, repository.DeleteCardList(ctx, deleted))

	require.Equal(t, []string{"Goblin"}, names(listAll(t, ctx)))
}

// 同じ集約を作り直すときに、何度呼ばれてもよいように
func TestCardList_DeleteAcceptsMissingCard(t *testing.T) {
	ctx := emptyCardLists(t)

	err := repository.DeleteCardList(ctx, cardList(readmodel.CardTypeCreature, "dragon", "Dragon"))

	require.NoError(t, err)
}

func TestCardList_OrdersByCardTypeThenSortName(t *testing.T) {
	ctx := emptyCardLists(t)
	put(t, ctx, readmodel.CardTypeSpell, "a", "Spell A")
	put(t, ctx, readmodel.CardTypeItem, "a", "Item A")
	put(t, ctx, readmodel.CardTypeCreature, "b", "Creature B")
	put(t, ctx, readmodel.CardTypeCreature, "a", "Creature A")

	require.Equal(t, []string{"Creature A", "Creature B", "Item A", "Spell A"}, names(listAll(t, ctx)))
}

func TestCardList_ContinuesAfterCursor(t *testing.T) {
	ctx := emptyCardLists(t)
	put(t, ctx, readmodel.CardTypeCreature, "a", "Creature A")
	second := put(t, ctx, readmodel.CardTypeCreature, "b", "Creature B")
	put(t, ctx, readmodel.CardTypeItem, "a", "Item A")
	after := query.CardListCursor{CardType: second.CardType, SortName: second.SortName, CardID: second.CardID}

	rows, err := repository.ListCards(ctx, query.ListCardsCondition{After: &after, Limit: 10})

	require.NoError(t, err)
	require.Equal(t, []string{"Item A"}, names(rows))
}

func TestCardList_LimitsRows(t *testing.T) {
	ctx := emptyCardLists(t)
	put(t, ctx, readmodel.CardTypeCreature, "a", "Creature A")
	put(t, ctx, readmodel.CardTypeCreature, "b", "Creature B")

	rows, err := repository.ListCards(ctx, query.ListCardsCondition{Limit: 1})

	require.NoError(t, err)
	require.Equal(t, []string{"Creature A"}, names(rows))
}

func TestCardList_FiltersByCardTypes(t *testing.T) {
	ctx := emptyCardLists(t)
	put(t, ctx, readmodel.CardTypeCreature, "a", "Creature A")
	put(t, ctx, readmodel.CardTypeItem, "a", "Item A")
	put(t, ctx, readmodel.CardTypeSpell, "a", "Spell A")

	rows, err := repository.ListCards(ctx, query.ListCardsCondition{
		Types: []readmodel.CardType{readmodel.CardTypeCreature, readmodel.CardTypeSpell}, Limit: 10,
	})

	require.NoError(t, err)
	require.Equal(t, []string{"Creature A", "Spell A"}, names(rows))
}

// LIKE の記号（% _）は文字どおりに比べる
func TestCardList_FiltersByNameContainsLiterally(t *testing.T) {
	tests := []struct {
		name         string
		nameContains string
		want         []string
	}{
		{name: "percent", nameContains: "50%", want: []string{"50%off"}},
		{name: "underscore", nameContains: "a_b", want: []string{"a_b"}},
		{name: "partial", nameContains: "off", want: []string{"50%off", "50xoff"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := emptyCardLists(t)
			for _, name := range []string{"50%off", "50xoff", "a_b", "axb"} {
				put(t, ctx, readmodel.CardTypeCreature, name, name)
			}

			rows, err := repository.ListCards(ctx, query.ListCardsCondition{NameContains: tt.nameContains, Limit: 10})

			require.NoError(t, err)
			require.Equal(t, tt.want, names(rows))
		})
	}
}
