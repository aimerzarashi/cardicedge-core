package command_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/creature"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/presetdeck"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterdeckset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/eventstore"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

func newStarterDeckSetHandler(t *testing.T, ctx context.Context) *command.StarterDeckSetHandler {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	return command.NewStarterDeckSetHandler(
		eventstore.New(registry, starterdeckset.NewID, starterdeckset.NewEventType, starterdeckset.InitialState, starterdeckset.Evolve),
		identifier.New(registry, starterdeckset.NewID),
		constraint.FindStarterDeckSet,
		constraint.CreateStarterDeckSet,
		constraint.UpdateStarterDeckSetCardReferences,
		presetDeckStore(registry),
		identifier.New(registry, presetdeck.NewID),
		// wiring と同じく、DB に残る精度（マイクロ秒）にそろえる
		func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
	)
}

// 合計がちょうど MaxTotalCount のプリセットデッキを作る（MaxCardCount 枚ずつのカードで埋める）
func fullPresetDeck(t *testing.T, ctx context.Context, name string) command.ChangedPresetDeck {
	t.Helper()
	deck := registerPresetDeck(t, ctx, name)
	cards := make([]command.CardCountInput, 0, presetdeck.MaxTotalCount/presetdeck.MaxCardCount)
	for range presetdeck.MaxTotalCount / presetdeck.MaxCardCount {
		cards = append(cards, command.CardCountInput{CardID: issueCard(t, ctx, creature.NewID), Count: presetdeck.MaxCardCount})
	}
	filled, err := newPresetDeckHandler(t, ctx).SetCardCounts(ctx, deck.ID, deck.Revision, cards)
	require.NoError(t, err)
	return filled
}

// コマンドの結果は revision だけなので、写した中身はイベントストアから集約を読み直して確かめる
func restoredStarterDeckSet(t *testing.T, ctx context.Context) starterdeckset.StarterDeckSet {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, found, err := constraint.FindStarterDeckSet(ctx)
	require.NoError(t, err)
	require.True(t, found)
	state, err := eventstore.New(registry, starterdeckset.NewID, starterdeckset.NewEventType, starterdeckset.InitialState, starterdeckset.Evolve).Load(ctx, id)
	require.NoError(t, err)
	return state.(starterdeckset.StarterDeckSet)
}

// 写しの元（公開 ID と revision）を、並びを問わずに比べられる形にする
func sources(set starterdeckset.StarterDeckSet) map[uuid.UUID]int64 {
	result := map[uuid.UUID]int64{}
	for _, deck := range set.Decks {
		result[deck.Source().PresetDeckID()] = deck.Source().Revision()
	}
	return result
}

func TestSetStarterDeckSet_FirstSetCopiesPresetDecks(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	fireEarth := fullPresetDeck(t, ctx, "火・地属性")
	waterWind := fullPresetDeck(t, ctx, "水・風属性")

	changed, err := newStarterDeckSetHandler(t, ctx).Set(ctx, 0, []uuid.UUID{fireEarth.ID, waterWind.ID})

	require.NoError(t, err)
	require.Equal(t, command.ChangedStarterDeckSet{Revision: 1}, changed)
	set := restoredStarterDeckSet(t, ctx)
	require.Equal(t, map[uuid.UUID]int64{fireEarth.ID: fireEarth.Revision, waterWind.ID: waterWind.Revision}, sources(set))
	for _, deck := range set.Decks {
		total := 0
		for _, card := range deck.Cards() {
			total += card.Count().Int()
		}
		require.Equal(t, presetdeck.MaxTotalCount, total)
	}
}

// 写した後にプリセットデッキを直しても、スターターデッキは変わらない
func TestSetStarterDeckSet_KeepsCopiesWhenPresetDeckChanges(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	fireEarth := fullPresetDeck(t, ctx, "火・地属性")
	waterWind := fullPresetDeck(t, ctx, "水・風属性")
	_, err := newStarterDeckSetHandler(t, ctx).Set(ctx, 0, []uuid.UUID{fireEarth.ID, waterWind.ID})
	require.NoError(t, err)

	_, err = newPresetDeckHandler(t, ctx).Rename(ctx, fireEarth.ID, fireEarth.Revision, "炎・大地属性")

	require.NoError(t, err)
	for _, deck := range restoredStarterDeckSet(t, ctx).Decks {
		require.NotEqual(t, "炎・大地属性", deck.Name().String())
	}
}

func TestSetStarterDeckSet_ResetReplacesDecks(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	fireEarth := fullPresetDeck(t, ctx, "火・地属性")
	waterWind := fullPresetDeck(t, ctx, "水・風属性")
	lightDark := fullPresetDeck(t, ctx, "光・闇属性")
	commands := newStarterDeckSetHandler(t, ctx)
	first, err := commands.Set(ctx, 0, []uuid.UUID{fireEarth.ID, waterWind.ID})
	require.NoError(t, err)

	changed, err := commands.Set(ctx, first.Revision, []uuid.UUID{fireEarth.ID, lightDark.ID})

	require.NoError(t, err)
	require.Equal(t, int64(2), changed.Revision)
	require.Equal(t, map[uuid.UUID]int64{fireEarth.ID: fireEarth.Revision, lightDark.ID: lightDark.Revision}, sources(restoredStarterDeckSet(t, ctx)))
}

// 同じプリセットデッキを選び直すと、直していれば写し直し（反映）、直していなければ revision を進めない
func TestSetStarterDeckSet_RecopiesOnlyChangedPresetDecks(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	fireEarth := fullPresetDeck(t, ctx, "火・地属性")
	waterWind := fullPresetDeck(t, ctx, "水・風属性")
	commands := newStarterDeckSetHandler(t, ctx)
	first, err := commands.Set(ctx, 0, []uuid.UUID{fireEarth.ID, waterWind.ID})
	require.NoError(t, err)
	unchanged, err := commands.Set(ctx, first.Revision, []uuid.UUID{waterWind.ID, fireEarth.ID})
	require.NoError(t, err)
	require.Equal(t, first, unchanged)
	renamed, err := newPresetDeckHandler(t, ctx).Rename(ctx, fireEarth.ID, fireEarth.Revision, "炎・大地属性")
	require.NoError(t, err)

	recopied, err := commands.Set(ctx, first.Revision, []uuid.UUID{fireEarth.ID, waterWind.ID})

	require.NoError(t, err)
	require.Equal(t, int64(2), recopied.Revision)
	require.Equal(t, renamed.Revision, sources(restoredStarterDeckSet(t, ctx))[fireEarth.ID])
}

func TestSetStarterDeckSet_RejectsStaleRevision(t *testing.T) {
	tests := []struct {
		name     string
		revision int64
	}{
		// 未設定を見て操作したが、もう設定があった
		{name: "unset_seen_but_already_set", revision: 0},
		{name: "older_revision", revision: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			fireEarth := fullPresetDeck(t, ctx, "火・地属性")
			waterWind := fullPresetDeck(t, ctx, "水・風属性")
			lightDark := fullPresetDeck(t, ctx, "光・闇属性")
			commands := newStarterDeckSetHandler(t, ctx)
			first, err := commands.Set(ctx, 0, []uuid.UUID{fireEarth.ID, waterWind.ID})
			require.NoError(t, err)
			_, err = commands.Set(ctx, first.Revision, []uuid.UUID{fireEarth.ID, lightDark.ID})
			require.NoError(t, err)

			_, err = commands.Set(ctx, tt.revision, []uuid.UUID{waterWind.ID, lightDark.ID})

			require.ErrorIs(t, err, command.ErrRevisionConflict)
		})
	}
}

func TestSetStarterDeckSet_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name string
		ids  func(t *testing.T, ctx context.Context) []uuid.UUID
		want []input.Error
	}{
		{
			name: "one_deck",
			ids: func(t *testing.T, ctx context.Context) []uuid.UUID {
				return []uuid.UUID{fullPresetDeck(t, ctx, "火・地属性").ID}
			},
			want: []input.Error{&input.OutOfRange{Field: "presetDeckIds", Min: 2, Max: 2}},
		},
		{
			name: "three_decks",
			ids: func(t *testing.T, ctx context.Context) []uuid.UUID {
				return []uuid.UUID{uuid.New(), uuid.New(), uuid.New()}
			},
			want: []input.Error{&input.OutOfRange{Field: "presetDeckIds", Min: 2, Max: 2}},
		},
		{
			name: "same_preset_deck_twice",
			ids: func(t *testing.T, ctx context.Context) []uuid.UUID {
				id := fullPresetDeck(t, ctx, "火・地属性").ID
				return []uuid.UUID{id, id}
			},
			want: []input.Error{&input.Duplicated{Field: "presetDeckIds.1"}},
		},
		{
			name: "unknown_preset_deck",
			ids: func(t *testing.T, ctx context.Context) []uuid.UUID {
				return []uuid.UUID{fullPresetDeck(t, ctx, "火・地属性").ID, uuid.New()}
			},
			want: []input.Error{&input.NotFound{Field: "presetDeckIds.1"}},
		},
		{
			name: "deleted_preset_deck",
			ids: func(t *testing.T, ctx context.Context) []uuid.UUID {
				deleted := fullPresetDeck(t, ctx, "消したデッキ")
				require.NoError(t, newPresetDeckHandler(t, ctx).Delete(ctx, deleted.ID, deleted.Revision))
				return []uuid.UUID{deleted.ID, fullPresetDeck(t, ctx, "火・地属性").ID}
			},
			want: []input.Error{&input.NotFound{Field: "presetDeckIds.0"}},
		},
		{
			name: "not_exactly_max_total",
			ids: func(t *testing.T, ctx context.Context) []uuid.UUID {
				return []uuid.UUID{fullPresetDeck(t, ctx, "火・地属性").ID, registerPresetDeck(t, ctx, "作りかけ").ID}
			},
			want: []input.Error{&input.TotalMismatch{Field: "presetDeckIds.1", Required: presetdeck.MaxTotalCount}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))
			ids := tt.ids(t, ctx)

			_, err := newStarterDeckSetHandler(t, ctx).Set(ctx, 0, ids)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
		})
	}
}

func TestSetStarterDeckSet_RejectsWithoutOperator(t *testing.T) {
	ctx := isolatedDatabase(t)

	_, err := newStarterDeckSetHandler(t, ctx).Set(ctx, 0, []uuid.UUID{uuid.New(), uuid.New()})

	require.ErrorIs(t, err, command.ErrNotOperator)
}

// 写したカードを参照し、参照されている間はカードを消せない（プリセットデッキを消しても、スターターデッキのカードは残る）
func TestSetStarterDeckSet_ReferencesCopiedCards(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	fireEarth := fullPresetDeck(t, ctx, "火・地属性")
	waterWind := fullPresetDeck(t, ctx, "水・風属性")

	_, err := newStarterDeckSetHandler(t, ctx).Set(ctx, 0, []uuid.UUID{fireEarth.ID, waterWind.ID})

	require.NoError(t, err)
	set := restoredStarterDeckSet(t, ctx)
	want := []int64{}
	for _, card := range set.Cards() {
		want = append(want, card.ID())
	}
	require.ElementsMatch(t, want, constrainttest.Referenced(t, ctx, starterdeckset.CardReferenceType(), set.ID.Int64()))
	require.Len(t, want, 2*presetdeck.MaxTotalCount/presetdeck.MaxCardCount)
}

// 初めての設定が同時に来ても、スターターデッキセットは 1 つだけで、ほかは先に変えられたになる
func TestSetStarterDeckSet_CreatesOnlyOneWhenFirstSetsAreConcurrent(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	decks := make([]uuid.UUID, 0, 4)
	for i := range 4 {
		decks = append(decks, fullPresetDeck(t, ctx, fmt.Sprintf("デッキ%d", i)).ID)
	}
	commands := newStarterDeckSetHandler(t, ctx)

	const callers = 20
	errs := make([]error, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Go(func() {
			_, errs[i] = commands.Set(ctx, 0, []uuid.UUID{decks[i%2], decks[2+i%2]})
		})
	}
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		require.ErrorIs(t, err, command.ErrRevisionConflict)
	}
	require.Equal(t, 1, succeeded)
	require.Equal(t, int64(1), restoredStarterDeckSet(t, ctx).Version)
}
