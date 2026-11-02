package main

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain"
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
	starterDecks, err := s.listStarterDecks(ctx)
	require.NoError(t, err)
	require.Len(t, starterDecks, len(m.StarterDecks))
	for _, name := range m.StarterDecks {
		deck := starterDecks[domain.NormalizeName(name)]
		require.Equal(t, readmodel.StarterDeckStatusPublished, deck.Status)
		require.Equal(t, deckTotalCount, deck.TotalCount)
		require.NotNil(t, deck.Source)
	}
	boards, err := s.listBoards(ctx)
	require.NoError(t, err)
	require.Len(t, boards, len(m.Boards))
	for _, b := range m.Boards {
		board := boards[domain.NormalizeName(b.Name)]
		require.Equal(t, readmodel.BoardStatusPublished, board.Status)
		require.Len(t, board.Tiles, len(b.Tiles))
		attributes := map[masterBoardPosition]string{}
		for _, tile := range b.Tiles {
			attributes[masterBoardPosition{X: tile.X, Y: tile.Y}] = tile.Attribute
		}
		for _, tile := range board.Tiles {
			require.Equal(t, attributes[masterBoardPosition{X: tile.X, Y: tile.Y}], string(tile.Attribute))
		}
		require.Len(t, board.Connections, len(b.Connections))
		require.Len(t, board.Pairs, len(b.Pairs))
		require.Equal(t, readmodel.BoardDefaultRules{
			TargetMagic: b.DefaultRules.TargetMagic, StartingMagic: b.DefaultRules.StartingMagic,
			LapBonus: b.DefaultRules.LapBonus, CheckpointBonus: b.DefaultRules.CheckpointBonus,
			RoundLimit: b.DefaultRules.RoundLimit, DiceMin: b.DefaultRules.DiceMin, DiceMax: b.DefaultRules.DiceMax,
		}, board.DefaultRules)
	}
	npcAvatars, err := s.listNPCAvatars(ctx)
	require.NoError(t, err)
	require.Len(t, npcAvatars, len(m.NPCAvatars))
	npcDecks, err := s.listNPCDecks(ctx)
	require.NoError(t, err)
	for _, avatar := range m.NPCAvatars {
		npcAvatar := npcAvatars[domain.NormalizeName(avatar.Name)]
		require.Equal(t, readmodel.NPCAvatarStatusPublished, npcAvatar.Status)
		wantDecks := make([]uuid.UUID, 0, len(avatar.Decks))
		for _, deck := range avatar.Decks {
			wantDecks = append(wantDecks, npcDecks[domain.NormalizeName(deck)].ID)
		}
		require.Equal(t, wantDecks, npcAvatar.Decks)
	}
	require.Len(t, npcDecks, len(m.NPCDecks))
	for _, name := range m.NPCDecks {
		npcDeck := npcDecks[domain.NormalizeName(name)]
		require.Equal(t, readmodel.NPCDeckStatusPublished, npcDeck.Status)
		require.Equal(t, deckTotalCount, npcDeck.TotalCount)
		require.NotNil(t, npcDeck.Source)
	}
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
		{name: "starter_deck_registered_without_publish", prepare: func(t *testing.T, ctx context.Context, s *seeder) {
			cardIDs, err := s.seedCards(ctx, m.Cards)
			require.NoError(t, err)
			deckIDs, err := s.seedPresetDecks(ctx, m.PresetDecks, cardIDs)
			require.NoError(t, err)
			_, err = s.starterDecks.RegisterFromPresetDeck(ctx, deckIDs[m.StarterDecks[0]], m.StarterDecks[0])
			require.NoError(t, err)
		}},
		{name: "starter_decks_just_published", prepare: func(t *testing.T, ctx context.Context, s *seeder) {
			cardIDs, err := s.seedCards(ctx, m.Cards)
			require.NoError(t, err)
			deckIDs, err := s.seedPresetDecks(ctx, m.PresetDecks, cardIDs)
			require.NoError(t, err)
			for _, name := range m.StarterDecks {
				copied, err := s.starterDecks.RegisterFromPresetDeck(ctx, deckIDs[name], name)
				require.NoError(t, err)
				_, err = s.starterDecks.Publish(ctx, copied.ID, copied.Revision)
				require.NoError(t, err)
			}
		}},
		{name: "board_registered_without_layout", prepare: func(t *testing.T, ctx context.Context, s *seeder) {
			_, err := s.boards.Register(ctx, m.Boards[0].Name)
			require.NoError(t, err)
		}},
		{name: "board_layout_changed_without_publish", prepare: func(t *testing.T, ctx context.Context, s *seeder) {
			registered, err := s.boards.Register(ctx, m.Boards[0].Name)
			require.NoError(t, err)
			_, err = s.boards.ChangeLayout(ctx, registered.ID, registered.Revision, m.Boards[0].layoutInput())
			require.NoError(t, err)
		}},
		{name: "board_default_rules_changed_without_publish", prepare: func(t *testing.T, ctx context.Context, s *seeder) {
			registered, err := s.boards.Register(ctx, m.Boards[0].Name)
			require.NoError(t, err)
			changed, err := s.boards.ChangeLayout(ctx, registered.ID, registered.Revision, m.Boards[0].layoutInput())
			require.NoError(t, err)
			_, err = s.boards.ChangeDefaultRules(ctx, registered.ID, changed.Revision, m.Boards[0].defaultRulesInput())
			require.NoError(t, err)
		}},
		{name: "npc_avatar_registered_without_decks", prepare: func(t *testing.T, ctx context.Context, s *seeder) {
			_, err := s.npcAvatars.Register(ctx, m.NPCAvatars[0].Name)
			require.NoError(t, err)
		}},
		{name: "npc_avatar_decks_assigned_without_publish", prepare: func(t *testing.T, ctx context.Context, s *seeder) {
			cardIDs, err := s.seedCards(ctx, m.Cards)
			require.NoError(t, err)
			deckIDs, err := s.seedPresetDecks(ctx, m.PresetDecks, cardIDs)
			require.NoError(t, err)
			npcDeckIDs, err := s.seedNPCDecks(ctx, m.NPCDecks, deckIDs)
			require.NoError(t, err)
			registered, err := s.npcAvatars.Register(ctx, m.NPCAvatars[0].Name)
			require.NoError(t, err)
			_, err = s.npcAvatars.SetDecks(ctx, registered.ID, registered.Revision, []uuid.UUID{npcDeckIDs[m.NPCAvatars[0].Decks[0]]})
			require.NoError(t, err)
		}},
		{name: "npc_deck_registered_without_publish", prepare: func(t *testing.T, ctx context.Context, s *seeder) {
			cardIDs, err := s.seedCards(ctx, m.Cards)
			require.NoError(t, err)
			deckIDs, err := s.seedPresetDecks(ctx, m.PresetDecks, cardIDs)
			require.NoError(t, err)
			_, err = s.npcDecks.RegisterFromPresetDeck(ctx, deckIDs[m.NPCDecks[0]], m.NPCDecks[0])
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

// 2 回目に流しても、カード・デッキが増えず、公開中のままそろう
func TestSeed_DoesNothingWhenAlreadySeeded(t *testing.T) {
	m, err := loadMaster(sampleMasterJSON)
	require.NoError(t, err)
	ctx, s := startSeeder(t)
	require.NoError(t, s.seed(ctx, m))

	require.NoError(t, s.seed(ctx, m))

	requireSeeded(t, ctx, s, m)
}
