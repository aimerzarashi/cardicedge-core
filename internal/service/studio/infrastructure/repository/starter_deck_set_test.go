package repository_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/gamerules"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck/presetdecktest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeckset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeckset/starterdecksettest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/infrastructure/repository"
)

var rules = gamerules.Defaults().Deck

// 合計がルールの枚数（既定は 40 枚）の写し（識別子を発行したカードで作る。書くときにカードの公開 ID を引くため）。カードの公開 ID と枚数も返す
func (f presetDeckFixture) starterDeck(t *testing.T, name string, revision int64) (starterdeckset.Deck, []query.PresetDeckCard) {
	t.Helper()
	cards := map[presetdeck.CardRef]presetdeck.CardCount{}
	var want []query.PresetDeckCard
	for range rules.TotalCardCount / rules.MaxCardDuplicates {
		ref, publicID := f.issueCreature(t)
		cards[ref] = presetdecktest.CardCount(t, rules.MaxCardDuplicates)
		want = append(want, query.PresetDeckCard{CardID: publicID, Count: rules.MaxCardDuplicates})
	}
	deck := presetdeck.PresetDeck{
		ID: presetdecktest.ID(t, 1), Version: revision, PublicID: uuid.New(), Name: presetdecktest.Name(t, name), Cards: cards,
	}
	return starterdecksettest.Deck(t, deck, rules), want
}

func (f presetDeckFixture) starterDeckSetWriter() *repository.StarterDeckSetWriter {
	return repository.NewStarterDeckSetWriter(identifier.NewEntityRefResolver(f.registry))
}

func TestStarterDeckSet_GetsUpsertedStarterDeckSetWithCardPublicIDs(t *testing.T) {
	f := newPresetDeckFixture(t)
	fireEarth, fireEarthCards := f.starterDeck(t, "火・地属性", 7)
	waterWind, _ := f.starterDeck(t, "水・風属性", 3)
	at := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	set := readmodel.StarterDeckSet{StarterDeckSetID: uuid.New(), Revision: 2, UpdatedAt: at, Decks: []starterdeckset.Deck{fireEarth, waterWind}}

	require.NoError(t, f.starterDeckSetWriter().Upsert(f.ctx, set))
	got, found, err := repository.NewStarterDeckSetReader().Get(f.ctx)

	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, int64(2), got.Revision)
	require.Equal(t, at, got.UpdatedAt.UTC())
	require.Len(t, got.Decks, 2)
	require.Equal(t, "火・地属性", got.Decks[0].Name)
	require.ElementsMatch(t, fireEarthCards, got.Decks[0].Cards)
	require.Equal(t, query.StarterDeckSource{PresetDeckID: fireEarth.Source().PresetDeckID(), Revision: 7}, got.Decks[0].Source)
}

func TestStarterDeckSet_GetReturnsNotFoundBeforeSet(t *testing.T) {
	f := newPresetDeckFixture(t)

	_, found, err := repository.NewStarterDeckSetReader().Get(f.ctx)

	require.NoError(t, err)
	require.False(t, found)
}

// 無い（削除した）プリセットデッキは結果に含めない
func TestGetPresetDeckRevisions_ReturnsRevisionsOfExistingPresetDecks(t *testing.T) {
	f := newPresetDeckFixture(t)
	fire := f.put(t, "fire", "Fire")

	revisions, err := repository.NewPresetDeckReader().Revisions(f.ctx, []uuid.UUID{fire.PresetDeckID, uuid.New()})

	require.NoError(t, err)
	require.Equal(t, map[uuid.UUID]int64{fire.PresetDeckID: fire.Metadata.Revision}, revisions)
}
