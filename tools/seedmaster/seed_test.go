package main

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/operator"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/authentication"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/tests/testhelper"
)

// 本番と同じ組み立てのアプリ（studio の投影が動く）を、自分だけの DB で起動し、初期オペレーターとして見本を入れる準備をする
// （道具は投影が read model を書くのを待つため、投影の動くアプリが要る）
func startSeeder(t *testing.T) (context.Context, *seeder) {
	t.Helper()
	if os.Getenv("TEST_DB_DSN") == "" {
		t.Skip("TEST_DB_DSN is not set. run via 'make test'")
	}
	app, err := testhelper.StartApp(t.Context())
	require.NoError(t, err)
	t.Cleanup(app.Close)
	ctx := rdb.WithDB(t.Context(), app.StudioDB)
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	email, err := operator.NewEmail("initial@example.com")
	require.NoError(t, err)
	operatorID, found, err := constraint.FindOperatorByEmail(ctx, email)
	require.NoError(t, err)
	require.True(t, found)
	return authentication.WithOperator(ctx, operatorID), newSeeder(registry)
}

func requireSeeded(t *testing.T, ctx context.Context, s *seeder, m master) {
	t.Helper()
	cards, err := s.listCards(ctx)
	require.NoError(t, err)
	require.Len(t, cards, len(m.Cards))
	for _, c := range cards {
		require.Equal(t, readmodel.CardStatusPublished, c.Status)
	}
	decks, err := s.listPresetDecks(ctx)
	require.NoError(t, err)
	require.Len(t, decks, len(m.PresetDecks))
	for _, deck := range decks {
		require.Equal(t, deckTotalCount, deck.TotalCount)
	}
	set, err := s.starterDeckSet.Get(ctx)
	require.NoError(t, err)
	require.Len(t, set.Decks, starterDeckSize)
}

// 途中で止まった（一部だけ入っている）状態から流し直しても、足りない分だけを入れ、見本どおりにそろう
func TestSeed_CompletesSampleFromPartiallySeededState(t *testing.T) {
	m, err := loadMaster(sampleMasterJSON)
	require.NoError(t, err)
	tests := []struct {
		name    string
		prepare func(t *testing.T, ctx context.Context, s *seeder)
	}{
		{name: "nothing_seeded", prepare: func(*testing.T, context.Context, *seeder) {}},
		{name: "some_cards_registered", prepare: func(t *testing.T, ctx context.Context, s *seeder) {
			for _, c := range m.Cards[:5] {
				_, err := s.registerCard(ctx, c)
				require.NoError(t, err)
			}
		}},
		// 公開してから止まったカードと、登録だけして止まったカードが混ざっていても、公開中にそろう
		{name: "some_cards_published", prepare: func(t *testing.T, ctx context.Context, s *seeder) {
			for _, c := range m.Cards[:5] {
				registered, err := s.registerCard(ctx, c)
				require.NoError(t, err)
				_, err = s.cardStatuses.Publish(ctx, registered.ID, registered.Revision)
				require.NoError(t, err)
			}
			for _, c := range m.Cards[5:10] {
				_, err := s.registerCard(ctx, c)
				require.NoError(t, err)
			}
		}},
		{name: "preset_deck_registered_without_counts", prepare: func(t *testing.T, ctx context.Context, s *seeder) {
			_, err := s.seedCards(ctx, m.Cards)
			require.NoError(t, err)
			_, err = s.presetDecks.Register(ctx, m.PresetDecks[0].Name)
			require.NoError(t, err)
		}},
		{name: "one_preset_deck_completed", prepare: func(t *testing.T, ctx context.Context, s *seeder) {
			cardIDs, err := s.seedCards(ctx, m.Cards)
			require.NoError(t, err)
			_, err = s.seedPresetDecks(ctx, m.PresetDecks[:1], cardIDs)
			require.NoError(t, err)
		}},
		{name: "starter_deck_set_just_set", prepare: func(t *testing.T, ctx context.Context, s *seeder) {
			cardIDs, err := s.seedCards(ctx, m.Cards)
			require.NoError(t, err)
			deckIDs, err := s.seedPresetDecks(ctx, m.PresetDecks, cardIDs)
			require.NoError(t, err)
			ids := make([]uuid.UUID, 0, len(m.StarterDeckSet))
			for _, name := range m.StarterDeckSet {
				ids = append(ids, deckIDs[name])
			}
			_, err = s.starterDeckSets.Set(ctx, 0, ids)
			require.NoError(t, err)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, s := startSeeder(t)
			tt.prepare(t, ctx, s)

			require.NoError(t, s.seed(ctx, m))

			requireSeeded(t, ctx, s, m)
		})
	}
}

// 設定済みなら何もしない（2 回目に流しても、カード・デッキが増えない）
func TestSeed_DoesNothingWhenAlreadySeeded(t *testing.T) {
	m, err := loadMaster(sampleMasterJSON)
	require.NoError(t, err)
	ctx, s := startSeeder(t)
	require.NoError(t, s.seed(ctx, m))

	require.NoError(t, s.seed(ctx, m))

	requireSeeded(t, ctx, s, m)
}
