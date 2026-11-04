package repository_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/card"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/infrastructure/repository"
)

func (f presetDeckFixture) listAll(t *testing.T) []query.PresetDeckListRow {
	t.Helper()
	rows, err := repository.NewPresetDeckReader().List(f.ctx, query.ListPresetDecksCondition{Limit: 100})
	require.NoError(t, err)
	return rows
}

func presetDeckNames(rows []query.PresetDeckListRow) []string {
	result := make([]string, len(rows))
	for i, row := range rows {
		result[i] = row.PresetDeck.Name
	}
	return result
}

// カードは内部 ID から公開 ID に引き直して書き、読むときは公開 ID と枚数で返す
func TestPresetDeck_GetsUpsertedPresetDeckWithCardPublicIDs(t *testing.T) {
	f := newPresetDeckFixture(t)
	dragon, dragonID := f.issueCreature(t)
	potion, potionID := f.issueItem(t)
	list := f.presetDeckList("fire", "Ｆire", map[card.Ref]int{dragon: 3, potion: 1})

	require.NoError(t, f.writer.Upsert(f.ctx, list))
	deck, err := repository.NewPresetDeckReader().Get(f.ctx, list.PresetDeckID)

	require.NoError(t, err)
	require.Equal(t, list.PresetDeckID, deck.ID)
	require.Equal(t, "Ｆire", deck.Name)
	require.ElementsMatch(t, []query.PresetDeckCard{{CardID: dragonID, Count: 3}, {CardID: potionID, Count: 1}}, deck.Cards)
	require.Equal(t, 4, deck.TotalCount)
	require.Equal(t, int64(3), deck.Revision)
	require.Equal(t, list.Metadata.RegisteredAt, deck.RegisteredAt.UTC())
	require.Equal(t, list.Metadata.UpdatedAt, deck.UpdatedAt.UTC())
	require.Equal(t, f.registeredByPublicID, deck.RegisteredBy)
	require.Equal(t, f.updatedByPublicID, deck.UpdatedBy)
}

func TestPresetDeck_GetFailsForMissingPresetDeck(t *testing.T) {
	f := newPresetDeckFixture(t)

	_, err := repository.NewPresetDeckReader().Get(f.ctx, uuid.New())

	require.ErrorIs(t, err, query.ErrPresetDeckNotFound)
}

func TestPresetDeck_UpsertOverwritesSamePresetDeck(t *testing.T) {
	f := newPresetDeckFixture(t)
	dragon, dragonID := f.issueCreature(t)
	list := f.put(t, "fire", "Fire")
	list.SortName = "water"
	list.Metadata.Name = "Water"
	list.Cards = map[card.Ref]int{dragon: 2}

	require.NoError(t, f.writer.Upsert(f.ctx, list))

	rows := f.listAll(t)
	require.Equal(t, []string{"Water"}, presetDeckNames(rows))
	require.Equal(t, []query.PresetDeckCard{{CardID: dragonID, Count: 2}}, rows[0].PresetDeck.Cards)
}

func TestPresetDeck_DeleteRemovesPresetDeck(t *testing.T) {
	f := newPresetDeckFixture(t)
	deleted := f.put(t, "fire", "Fire")
	f.put(t, "water", "Water")

	require.NoError(t, f.writer.Delete(f.ctx, deleted))

	require.Equal(t, []string{"Water"}, presetDeckNames(f.listAll(t)))
}

// 同じ集約を作り直すときに、何度呼ばれてもよいように
func TestPresetDeck_DeleteAcceptsMissingPresetDeck(t *testing.T) {
	f := newPresetDeckFixture(t)

	err := f.writer.Delete(f.ctx, f.presetDeckList("fire", "Fire", map[card.Ref]int{}))

	require.NoError(t, err)
}

func TestPresetDeck_ListsBySortNameWithCursor(t *testing.T) {
	f := newPresetDeckFixture(t)
	f.put(t, "b", "B")
	first := f.put(t, "a", "A")
	f.put(t, "c", "C")
	after := query.PresetDeckListCursor{SortName: first.SortName, PresetDeckID: first.PresetDeckID}

	rows, err := repository.NewPresetDeckReader().List(f.ctx, query.ListPresetDecksCondition{After: &after, Limit: 1})

	require.NoError(t, err)
	require.Equal(t, []string{"A", "B", "C"}, presetDeckNames(f.listAll(t)))
	require.Equal(t, []string{"B"}, presetDeckNames(rows))
	require.Equal(t, query.PresetDeckListCursor{SortName: "b", PresetDeckID: rows[0].PresetDeck.ID}, rows[0].Cursor)
}

// LIKE の記号（% _）は文字どおりに比べる（カードの一覧と同じ）
func TestPresetDeck_FiltersByNameContainsLiterally(t *testing.T) {
	f := newPresetDeckFixture(t)
	for _, name := range []string{"50%off", "50xoff"} {
		f.put(t, name, name)
	}

	rows, err := repository.NewPresetDeckReader().List(f.ctx, query.ListPresetDecksCondition{NameContains: "50%", Limit: 10})

	require.NoError(t, err)
	require.Equal(t, []string{"50%off"}, presetDeckNames(rows))
}
