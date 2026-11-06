package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator/operatortest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/infrastructure/repository"
)

type cardFixture struct {
	presetDeckFixture
	writer *repository.CardWriter
}

func newCardFixture(t *testing.T) cardFixture {
	t.Helper()
	base := newPresetDeckFixture(t)
	return cardFixture{presetDeckFixture: base, writer: repository.NewCardWriter(identifier.NewEntityRefResolver(base.registry))}
}

func (f cardFixture) cardList(cardType readmodel.CardType, sortName, name string) readmodel.Card {
	id := uuid.New()
	at := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	metadata := readmodel.CardMetadata{ID: id, Name: name, Status: readmodel.CardStatusDraft, Revision: 1, RegisteredAt: at, UpdatedAt: at}
	return readmodel.Card{
		CardID: id, CardType: cardType, Status: readmodel.CardStatusDraft, SortName: sortName, Metadata: metadata,
		RegisteredBy: f.registeredBy, UpdatedBy: f.updatedBy,
	}
}

func (f cardFixture) put(t *testing.T, cardType readmodel.CardType, sortName, name string) readmodel.Card {
	t.Helper()
	list := f.cardList(cardType, sortName, name)
	require.NoError(t, f.writer.Upsert(f.ctx, list))
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
	f := newCardFixture(t)
	ctx := f.ctx
	list := f.put(t, readmodel.CardTypeCreature, "dragon", "Ｄragon")

	rows := listAll(t, ctx)

	require.Len(t, rows, 1)
	card := rows[0].Card
	require.Equal(t, list.CardID, card.ID)
	require.Equal(t, readmodel.CardTypeCreature, card.Type)
	require.Equal(t, "Ｄragon", card.Name)
	require.Equal(t, readmodel.CardStatusDraft, card.Status)
	require.EqualValues(t, 1, card.Revision)
	require.Equal(t, list.Metadata.RegisteredAt, card.RegisteredAt.UTC())
	require.Equal(t, list.Metadata.UpdatedAt, card.UpdatedAt.UTC())
	require.Equal(t, f.registeredByPublicID, card.RegisteredBy)
	require.Equal(t, f.updatedByPublicID, card.UpdatedBy)
	require.Equal(t, query.CardListCursor{CardType: readmodel.CardTypeCreature, SortName: "dragon", CardID: list.CardID}, rows[0].Cursor)
}

func TestCard_UpsertOverwritesSameCard(t *testing.T) {
	f := newCardFixture(t)
	ctx := f.ctx
	list := f.put(t, readmodel.CardTypeCreature, "dragon", "Dragon")
	list.SortName = "goblin"
	list.Metadata.Name = "Goblin"

	require.NoError(t, f.writer.Upsert(ctx, list))

	require.Equal(t, []string{"Goblin"}, names(listAll(t, ctx)))
}

func TestCard_DeleteRemovesCardFromList(t *testing.T) {
	f := newCardFixture(t)
	ctx := f.ctx
	deleted := f.put(t, readmodel.CardTypeCreature, "dragon", "Dragon")
	f.put(t, readmodel.CardTypeCreature, "goblin", "Goblin")

	require.NoError(t, f.writer.Delete(ctx, deleted))

	require.Equal(t, []string{"Goblin"}, names(listAll(t, ctx)))
}

// 同じ集約を作り直すときに、何度呼ばれてもよいように
func TestCard_DeleteAcceptsMissingCard(t *testing.T) {
	f := newCardFixture(t)
	ctx := f.ctx

	err := f.writer.Delete(ctx, f.cardList(readmodel.CardTypeCreature, "dragon", "Dragon"))

	require.NoError(t, err)
}

func TestCard_OrdersByCardTypeThenSortName(t *testing.T) {
	f := newCardFixture(t)
	ctx := f.ctx
	f.put(t, readmodel.CardTypeSpell, "a", "Spell A")
	f.put(t, readmodel.CardTypeItem, "a", "Item A")
	f.put(t, readmodel.CardTypeCreature, "b", "Creature B")
	f.put(t, readmodel.CardTypeCreature, "a", "Creature A")

	require.Equal(t, []string{"Creature A", "Creature B", "Item A", "Spell A"}, names(listAll(t, ctx)))
}

func TestCard_ContinuesAfterCursor(t *testing.T) {
	f := newCardFixture(t)
	ctx := f.ctx
	f.put(t, readmodel.CardTypeCreature, "a", "Creature A")
	second := f.put(t, readmodel.CardTypeCreature, "b", "Creature B")
	f.put(t, readmodel.CardTypeItem, "a", "Item A")
	after := query.CardListCursor{CardType: second.CardType, SortName: second.SortName, CardID: second.CardID}

	rows, err := repository.NewCardReader().List(ctx, query.ListCardsCondition{After: &after, Limit: 10})

	require.NoError(t, err)
	require.Equal(t, []string{"Item A"}, names(rows))
}

func TestCard_LimitsRows(t *testing.T) {
	f := newCardFixture(t)
	ctx := f.ctx
	f.put(t, readmodel.CardTypeCreature, "a", "Creature A")
	f.put(t, readmodel.CardTypeCreature, "b", "Creature B")

	rows, err := repository.NewCardReader().List(ctx, query.ListCardsCondition{Limit: 1})

	require.NoError(t, err)
	require.Equal(t, []string{"Creature A"}, names(rows))
}

func TestCard_FiltersByCardTypes(t *testing.T) {
	f := newCardFixture(t)
	ctx := f.ctx
	f.put(t, readmodel.CardTypeCreature, "a", "Creature A")
	f.put(t, readmodel.CardTypeItem, "a", "Item A")
	f.put(t, readmodel.CardTypeSpell, "a", "Spell A")

	rows, err := repository.NewCardReader().List(ctx, query.ListCardsCondition{
		Types: []readmodel.CardType{readmodel.CardTypeCreature, readmodel.CardTypeSpell}, Limit: 10,
	})

	require.NoError(t, err)
	require.Equal(t, []string{"Creature A", "Spell A"}, names(rows))
}

func TestCard_FiltersByStatuses(t *testing.T) {
	f := newCardFixture(t)
	ctx := f.ctx
	f.put(t, readmodel.CardTypeCreature, "a", "Draft")
	published := f.cardList(readmodel.CardTypeCreature, "b", "Published")
	published.Status = readmodel.CardStatusPublished
	published.Metadata.Status = readmodel.CardStatusPublished
	published.Metadata.Revision = 2
	require.NoError(t, f.writer.Upsert(ctx, published))
	retired := f.cardList(readmodel.CardTypeCreature, "c", "Retired")
	retired.Status = readmodel.CardStatusRetired
	retired.Metadata.Status = readmodel.CardStatusRetired
	require.NoError(t, f.writer.Upsert(ctx, retired))

	rows, err := repository.NewCardReader().List(ctx, query.ListCardsCondition{
		Statuses: []readmodel.CardStatus{readmodel.CardStatusPublished, readmodel.CardStatusRetired}, Limit: 10,
	})

	require.NoError(t, err)
	require.Equal(t, []string{"Published", "Retired"}, names(rows))
	require.Equal(t, readmodel.CardStatusPublished, rows[0].Card.Status)
	require.EqualValues(t, 2, rows[0].Card.Revision)
}

// 状態の列は CHECK で守る（make publish-master が写すカードの選びの根拠なので、read model でも列だけは守る）
func TestCard_RejectsUnknownStatus(t *testing.T) {
	f := newCardFixture(t)
	ctx := f.ctx
	f.put(t, readmodel.CardTypeCreature, "a", "Card")
	exec, err := rdb.GetExecutor(ctx)
	require.NoError(t, err)

	_, err = exec.ExecContext(ctx, `UPDATE cards SET status = 'archived'`)

	require.Error(t, err)
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
			f := newCardFixture(t)
			ctx := f.ctx
			for _, name := range []string{"50%off", "50xoff", "a_b", "axb"} {
				f.put(t, readmodel.CardTypeCreature, name, name)
			}

			rows, err := repository.NewCardReader().List(ctx, query.ListCardsCondition{NameContains: tt.nameContains, Limit: 10})

			require.NoError(t, err)
			require.Equal(t, tt.want, names(rows))
		})
	}
}

// 無いカードは結果に含めない
func TestCard_GetsCardsByPublicID(t *testing.T) {
	f := newCardFixture(t)
	ctx := f.ctx
	dragon := f.put(t, readmodel.CardTypeCreature, "dragon", "Dragon")
	potion := f.put(t, readmodel.CardTypeItem, "potion", "Potion")
	f.put(t, readmodel.CardTypeSpell, "fire", "Fire")

	cards, err := repository.NewCardReader().Get(ctx, []uuid.UUID{dragon.CardID, potion.CardID, uuid.New()})

	require.NoError(t, err)
	require.Len(t, cards, 2)
	require.Equal(t, "Dragon", cards[dragon.CardID].Name)
	require.Equal(t, readmodel.CardTypeCreature, cards[dragon.CardID].Type)
	require.Equal(t, "Potion", cards[potion.CardID].Name)
	require.Equal(t, readmodel.CardTypeItem, cards[potion.CardID].Type)
}

func TestCard_UpsertFailsForOperatorWithoutPublicID(t *testing.T) {
	f := newCardFixture(t)
	card := f.cardList(readmodel.CardTypeCreature, "dragon", "Dragon")
	card.RegisteredBy = operatortest.ID(t, 999999)

	require.Error(t, f.writer.Upsert(f.ctx, card))
}

// 登録した人・更新した人を持たない前の形で投影した行も、読み出しは失敗せずゼロ値で返す（次のイベントで投影し直すと入る）
func TestCard_ListsRowWithoutOperatorsAsZeroValue(t *testing.T) {
	f := newCardFixture(t)
	f.put(t, readmodel.CardTypeCreature, "dragon", "Dragon")
	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	_, err = exec.ExecContext(f.ctx, `UPDATE cards SET metadata = metadata - 'registered_by' - 'updated_by'`)
	require.NoError(t, err)

	rows := listAll(t, f.ctx)

	require.Len(t, rows, 1)
	require.Equal(t, uuid.Nil, rows[0].Card.RegisteredBy)
	require.Equal(t, uuid.Nil, rows[0].Card.UpdatedBy)
}
