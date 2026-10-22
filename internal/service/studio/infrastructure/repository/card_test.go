package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/infrastructure/repository"
)

func cardList(cardType readmodel.CardType, sortName, name string) readmodel.Card {
	id := uuid.New()
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	metadata := readmodel.CardMetadata{ID: id, Name: name, RegisteredAt: at, UpdatedAt: at}
	return readmodel.Card{CardID: id, CardType: cardType, SortName: sortName, Metadata: metadata}
}

func put(t *testing.T, ctx context.Context, cardType readmodel.CardType, sortName, name string) readmodel.Card {
	t.Helper()
	list := cardList(cardType, sortName, name)
	require.NoError(t, repository.NewCardWriter().Upsert(ctx, list))
	return list
}

func listAll(t *testing.T, ctx context.Context) []query.CardListRow {
	t.Helper()
	rows, err := repository.NewCardReader().List(ctx, query.ListCardsCondition{Limit: 100})
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

func TestCard_ListsUpsertedCardWithDisplayValuesAndCursor(t *testing.T) {
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

func TestCard_UpsertOverwritesSameCard(t *testing.T) {
	ctx := emptyCardLists(t)
	list := put(t, ctx, readmodel.CardTypeCreature, "dragon", "Dragon")
	list.SortName = "goblin"
	list.Metadata.Name = "Goblin"

	require.NoError(t, repository.NewCardWriter().Upsert(ctx, list))

	require.Equal(t, []string{"Goblin"}, names(listAll(t, ctx)))
}

func TestCard_DeleteRemovesCardFromList(t *testing.T) {
	ctx := emptyCardLists(t)
	deleted := put(t, ctx, readmodel.CardTypeCreature, "dragon", "Dragon")
	put(t, ctx, readmodel.CardTypeCreature, "goblin", "Goblin")

	require.NoError(t, repository.NewCardWriter().Delete(ctx, deleted))

	require.Equal(t, []string{"Goblin"}, names(listAll(t, ctx)))
}

// 同じ集約を作り直すときに、何度呼ばれてもよいように
func TestCard_DeleteAcceptsMissingCard(t *testing.T) {
	ctx := emptyCardLists(t)

	err := repository.NewCardWriter().Delete(ctx, cardList(readmodel.CardTypeCreature, "dragon", "Dragon"))

	require.NoError(t, err)
}

func TestCard_OrdersByCardTypeThenSortName(t *testing.T) {
	ctx := emptyCardLists(t)
	put(t, ctx, readmodel.CardTypeSpell, "a", "Spell A")
	put(t, ctx, readmodel.CardTypeItem, "a", "Item A")
	put(t, ctx, readmodel.CardTypeCreature, "b", "Creature B")
	put(t, ctx, readmodel.CardTypeCreature, "a", "Creature A")

	require.Equal(t, []string{"Creature A", "Creature B", "Item A", "Spell A"}, names(listAll(t, ctx)))
}

func TestCard_ContinuesAfterCursor(t *testing.T) {
	ctx := emptyCardLists(t)
	put(t, ctx, readmodel.CardTypeCreature, "a", "Creature A")
	second := put(t, ctx, readmodel.CardTypeCreature, "b", "Creature B")
	put(t, ctx, readmodel.CardTypeItem, "a", "Item A")
	after := query.CardListCursor{CardType: second.CardType, SortName: second.SortName, CardID: second.CardID}

	rows, err := repository.NewCardReader().List(ctx, query.ListCardsCondition{After: &after, Limit: 10})

	require.NoError(t, err)
	require.Equal(t, []string{"Item A"}, names(rows))
}

func TestCard_LimitsRows(t *testing.T) {
	ctx := emptyCardLists(t)
	put(t, ctx, readmodel.CardTypeCreature, "a", "Creature A")
	put(t, ctx, readmodel.CardTypeCreature, "b", "Creature B")

	rows, err := repository.NewCardReader().List(ctx, query.ListCardsCondition{Limit: 1})

	require.NoError(t, err)
	require.Equal(t, []string{"Creature A"}, names(rows))
}

func TestCard_FiltersByCardTypes(t *testing.T) {
	ctx := emptyCardLists(t)
	put(t, ctx, readmodel.CardTypeCreature, "a", "Creature A")
	put(t, ctx, readmodel.CardTypeItem, "a", "Item A")
	put(t, ctx, readmodel.CardTypeSpell, "a", "Spell A")

	rows, err := repository.NewCardReader().List(ctx, query.ListCardsCondition{
		Types: []readmodel.CardType{readmodel.CardTypeCreature, readmodel.CardTypeSpell}, Limit: 10,
	})

	require.NoError(t, err)
	require.Equal(t, []string{"Creature A", "Spell A"}, names(rows))
}

// LIKE の記号（% _）は文字どおりに比べる
func TestCard_FiltersByNameContainsLiterally(t *testing.T) {
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

			rows, err := repository.NewCardReader().List(ctx, query.ListCardsCondition{NameContains: tt.nameContains, Limit: 10})

			require.NoError(t, err)
			require.Equal(t, tt.want, names(rows))
		})
	}
}

// 無いカードは結果に含めない
func TestCard_GetsCardsByPublicID(t *testing.T) {
	ctx := emptyCardLists(t)
	dragon := put(t, ctx, readmodel.CardTypeCreature, "dragon", "Dragon")
	potion := put(t, ctx, readmodel.CardTypeItem, "potion", "Potion")
	put(t, ctx, readmodel.CardTypeSpell, "fire", "Fire")

	cards, err := repository.NewCardReader().Get(ctx, []uuid.UUID{dragon.CardID, potion.CardID, uuid.New()})

	require.NoError(t, err)
	require.Len(t, cards, 2)
	require.Equal(t, "Dragon", cards[dragon.CardID].Name)
	require.Equal(t, readmodel.CardTypeCreature, cards[dragon.CardID].Type)
	require.Equal(t, "Potion", cards[potion.CardID].Name)
	require.Equal(t, readmodel.CardTypeItem, cards[potion.CardID].Type)
}
