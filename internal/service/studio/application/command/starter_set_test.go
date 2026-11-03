package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/starterset"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

func restoredStarterSet(t *testing.T, ctx context.Context, publicID uuid.UUID) starterset.StarterSet {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, err := identifier.New(registry, starterset.NewID).Resolve(ctx, publicID)
	require.NoError(t, err)
	state, err := starterSetStore(registry).Load(ctx, id)
	require.NoError(t, err)
	return state.(starterset.StarterSet)
}

func registerStarterSet(t *testing.T, ctx context.Context, commands *command.StarterSetHandler, name string) command.ChangedStarterSet {
	t.Helper()
	registered, err := commands.Register(ctx, name)
	require.NoError(t, err)
	return registered
}

// 公開中の配布デッキ（smallRules で公開できるカード 4 枚）
func publishedGrantDeck(t *testing.T, ctx context.Context) command.ChangedGrantDeck {
	t.Helper()
	commands := newGrantDeckHandlerWithRules(t, ctx, smallRules())
	filled := fullGrantDeck(t, ctx, commands)
	published, err := commands.Publish(ctx, filled.ID, filled.Revision)
	require.NoError(t, err)
	return published
}

func publishedBoard(t *testing.T, ctx context.Context) command.ChangedBoard {
	t.Helper()
	commands := newBoardHandler(t, ctx)
	changed := publishableBoard(t, ctx, commands)
	published, err := commands.Publish(ctx, changed.ID, changed.Revision)
	require.NoError(t, err)
	return published
}

// 公開中の配布デッキを 1 つ指定した、準備中のスターターセット
func publishableStarterSet(t *testing.T, ctx context.Context, commands *command.StarterSetHandler, name string) command.ChangedStarterSet {
	t.Helper()
	registered := registerStarterSet(t, ctx, commands, name)
	assigned, err := commands.SetGrantDecks(ctx, registered.ID, registered.Revision, []uuid.UUID{publishedGrantDeck(t, ctx).ID})
	require.NoError(t, err)
	return assigned
}

// 公開中の NPC のアバター（公開中のデッキを 1 つ使う）
func publishedNPCAvatar(t *testing.T, ctx context.Context) command.ChangedNPCAvatar {
	t.Helper()
	commands := newNPCAvatarHandler(t, ctx)
	assigned := publishableNPCAvatar(t, ctx, commands, "アバター-"+uuid.NewString()[:8])
	published, err := commands.Publish(ctx, assigned.ID, assigned.Revision)
	require.NoError(t, err)
	return published
}

func internalNPCAvatarIDs(current starterset.StarterSet) []int64 {
	ids := make([]int64, 0, len(current.UnlockNPCAvatars))
	for _, id := range current.UnlockNPCAvatars {
		ids = append(ids, id.Int64())
	}
	return ids
}

func internalGrantDeckIDs(current starterset.StarterSet) []int64 {
	ids := make([]int64, 0, len(current.GrantDecks))
	for _, id := range current.GrantDecks {
		ids = append(ids, id.Int64())
	}
	return ids
}

func internalBoardIDs(current starterset.StarterSet) []int64 {
	ids := make([]int64, 0, len(current.UnlockBoards))
	for _, id := range current.UnlockBoards {
		ids = append(ids, id.Int64())
	}
	return ids
}

func TestRegisterStarterSet_ReturnsPublicIDAndMakesDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))

	registered, err := newStarterSetHandler(t, ctx).Register(ctx, "  スターターセット ")

	require.NoError(t, err)
	require.Equal(t, int64(1), registered.Revision)
	current := restoredStarterSet(t, ctx, registered.ID)
	require.Equal(t, "スターターセット", current.Name.String())
	require.Equal(t, starterset.StatusDraft, current.Status)
	require.Empty(t, current.GrantDecks)
	require.Empty(t, current.UnlockBoards)
}

func TestRegisterStarterSet_RejectsNameAlreadyInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registerStarterSet(t, ctx, commands, "ＡＢＣ")

	_, err := commands.Register(ctx, " abc")

	require.ErrorIs(t, err, command.ErrStarterSetNameAlreadyInUse)
}

func TestRegisterStarterSet_RejectsInvalidName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  input.Error
	}{
		{name: "blank", value: " ", want: &input.Required{Field: "name"}},
		{name: "too_long", value: strings.Repeat("あ", starterset.MaxNameLength+1), want: &input.TooLong{Field: "name", Max: starterset.MaxNameLength}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))

			_, err := newStarterSetHandler(t, ctx).Register(ctx, tt.value)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
		})
	}
}

func TestStarterSetHandler_RejectsCallWithoutOperator(t *testing.T) {
	ctx := isolatedDatabase(t)
	commands := newStarterSetHandler(t, ctx)

	_, err := commands.Register(ctx, "標準")
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.Rename(ctx, uuid.New(), 1, "新標準")
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.SetGrantDecks(ctx, uuid.New(), 1, nil)
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.SetUnlockBoards(ctx, uuid.New(), 1, nil)
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.SetUnlockNPCAvatars(ctx, uuid.New(), 1, nil)
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.Publish(ctx, uuid.New(), 1)
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.Retire(ctx, uuid.New(), 1)
	require.ErrorIs(t, err, command.ErrNotOperator)
	require.ErrorIs(t, commands.Delete(ctx, uuid.New(), 1), command.ErrNotOperator)
}

func TestRenameStarterSet_ChangesNameAndFreesOldName(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registered := registerStarterSet(t, ctx, commands, "標準")

	renamed, err := commands.Rename(ctx, registered.ID, registered.Revision, "新標準")

	require.NoError(t, err)
	require.Equal(t, int64(2), renamed.Revision)
	require.Equal(t, "新標準", restoredStarterSet(t, ctx, registered.ID).Name.String())
	registerStarterSet(t, ctx, commands, "標準")
}

func TestRenameStarterSet_SameNameKeepsRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registered := registerStarterSet(t, ctx, commands, "標準")

	renamed, err := commands.Rename(ctx, registered.ID, registered.Revision, "標準")

	require.NoError(t, err)
	require.Equal(t, registered.Revision, renamed.Revision)
}

func TestRenameStarterSet_RejectsNameUsedByOtherStarterSet(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registerStarterSet(t, ctx, commands, "標準")
	other := registerStarterSet(t, ctx, commands, "新標準")

	_, err := commands.Rename(ctx, other.ID, other.Revision, "標準")

	require.ErrorIs(t, err, command.ErrStarterSetNameAlreadyInUse)
}

func TestStarterSetHandler_RejectsStaleRevisionAndMissingStarterSet(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registered := registerStarterSet(t, ctx, commands, "標準")
	_, err := commands.Rename(ctx, registered.ID, registered.Revision, "新標準")
	require.NoError(t, err)
	operations := []struct {
		name string
		call func(id uuid.UUID, revision int64) error
	}{
		{name: "rename", call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Rename(ctx, id, revision, "別名")
			return err
		}},
		{name: "set_grant_decks", call: func(id uuid.UUID, revision int64) error {
			_, err := commands.SetGrantDecks(ctx, id, revision, nil)
			return err
		}},
		{name: "set_unlock_boards", call: func(id uuid.UUID, revision int64) error {
			_, err := commands.SetUnlockBoards(ctx, id, revision, nil)
			return err
		}},
		{name: "set_unlock_npc_avatars", call: func(id uuid.UUID, revision int64) error {
			_, err := commands.SetUnlockNPCAvatars(ctx, id, revision, nil)
			return err
		}},
		{name: "publish", call: func(id uuid.UUID, revision int64) error { _, err := commands.Publish(ctx, id, revision); return err }},
		{name: "retire", call: func(id uuid.UUID, revision int64) error { _, err := commands.Retire(ctx, id, revision); return err }},
		{name: "delete", call: func(id uuid.UUID, revision int64) error { return commands.Delete(ctx, id, revision) }},
	}
	for _, op := range operations {
		t.Run(op.name+"_stale_revision", func(t *testing.T) {
			require.ErrorIs(t, op.call(registered.ID, registered.Revision), command.ErrRevisionConflict)
		})
		t.Run(op.name+"_missing_starter_set", func(t *testing.T) {
			require.ErrorIs(t, op.call(uuid.New(), 1), command.ErrStarterSetNotFound)
		})
	}
}

func TestDeleteStarterSet_RemovesStarterSetAndFreesName(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registered := registerStarterSet(t, ctx, commands, "標準")

	require.NoError(t, commands.Delete(ctx, registered.ID, registered.Revision))

	_, err := commands.Rename(ctx, registered.ID, registered.Revision, "新標準")
	require.ErrorIs(t, err, command.ErrStarterSetNotFound)
	registerStarterSet(t, ctx, commands, "標準")
}

func TestSetStarterSetGrantDecks_AssignsPublishedGrantDecksInGivenOrderAndReferences(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registered := registerStarterSet(t, ctx, commands, "標準")
	first, second := publishedGrantDeck(t, ctx), publishedGrantDeck(t, ctx)

	changed, err := commands.SetGrantDecks(ctx, registered.ID, registered.Revision, []uuid.UUID{second.ID, first.ID})

	require.NoError(t, err)
	require.Equal(t, command.ChangedStarterSet{ID: registered.ID, Revision: registered.Revision + 1}, changed)
	current := restoredStarterSet(t, ctx, registered.ID)
	require.Equal(t, []int64{internalID(t, ctx, second.ID), internalID(t, ctx, first.ID)}, internalGrantDeckIDs(current))
	require.ElementsMatch(t, []int64{internalID(t, ctx, first.ID), internalID(t, ctx, second.ID)},
		constrainttest.Referenced(t, ctx, starterset.GrantDeckReferenceType(), current.ID.Int64()))
}

// 外す → 割り当てるの順にイベントを出すので、入れ替えは revision が 2 進む。残したものの順は変わらない
func TestSetStarterSetGrantDecks_ReplacesWholeListAndReferences(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registered := registerStarterSet(t, ctx, commands, "標準")
	first, second := publishedGrantDeck(t, ctx), publishedGrantDeck(t, ctx)
	assigned, err := commands.SetGrantDecks(ctx, registered.ID, registered.Revision, []uuid.UUID{first.ID})
	require.NoError(t, err)

	replaced, err := commands.SetGrantDecks(ctx, assigned.ID, assigned.Revision, []uuid.UUID{second.ID})
	require.NoError(t, err)
	cleared, err := commands.SetGrantDecks(ctx, replaced.ID, replaced.Revision, nil)

	require.NoError(t, err)
	require.Equal(t, assigned.Revision+2, replaced.Revision)
	require.Equal(t, registered.Revision+4, cleared.Revision)
	current := restoredStarterSet(t, ctx, registered.ID)
	require.Empty(t, current.GrantDecks)
	require.Empty(t, constrainttest.Referenced(t, ctx, starterset.GrantDeckReferenceType(), current.ID.Int64()))
}

func TestSetStarterSetGrantDecks_SameGrantDecksKeepRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registered := registerStarterSet(t, ctx, commands, "標準")
	first, second := publishedGrantDeck(t, ctx), publishedGrantDeck(t, ctx)
	assigned, err := commands.SetGrantDecks(ctx, registered.ID, registered.Revision, []uuid.UUID{first.ID, second.ID})
	require.NoError(t, err)

	same, err := commands.SetGrantDecks(ctx, assigned.ID, assigned.Revision, []uuid.UUID{second.ID, first.ID})

	require.NoError(t, err)
	require.Equal(t, assigned, same)
}

func TestSetStarterSetGrantDecks_ReturnsFieldErrorsForDuplicateUnknownAndUnpublished(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registered := registerStarterSet(t, ctx, commands, "標準")
	published := publishedGrantDeck(t, ctx)
	draft := fullGrantDeck(t, ctx, newGrantDeckHandlerWithRules(t, ctx, smallRules()))
	retired := publishedGrantDeck(t, ctx)
	_, err := newGrantDeckHandlerWithRules(t, ctx, smallRules()).Retire(ctx, retired.ID, retired.Revision)
	require.NoError(t, err)
	deleted := registerGrantDeck(t, ctx, newGrantDeckHandler(t, ctx), "消すデッキ")
	require.NoError(t, newGrantDeckHandler(t, ctx).Delete(ctx, deleted.ID, deleted.Revision))
	tests := []struct {
		name  string
		decks []uuid.UUID
		want  []input.Error
	}{
		{name: "duplicated", decks: []uuid.UUID{published.ID, published.ID}, want: []input.Error{&input.Duplicated{Field: "grantDeckIds.1"}}},
		{name: "unknown", decks: []uuid.UUID{published.ID, uuid.New()}, want: []input.Error{&input.NotFound{Field: "grantDeckIds.1"}}},
		{name: "deleted", decks: []uuid.UUID{deleted.ID}, want: []input.Error{&input.NotFound{Field: "grantDeckIds.0"}}},
		{name: "not_grant_deck", decks: []uuid.UUID{registered.ID}, want: []input.Error{&input.NotFound{Field: "grantDeckIds.0"}}},
		{name: "draft_and_retired", decks: []uuid.UUID{draft.ID, published.ID, retired.ID}, want: []input.Error{&input.Unavailable{Field: "grantDeckIds.0"}, &input.Unavailable{Field: "grantDeckIds.2"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := commands.SetGrantDecks(ctx, registered.ID, registered.Revision, tt.decks)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.Empty(t, restoredStarterSet(t, ctx, registered.ID).GrantDecks)
		})
	}
}

// 割り当てた後に使用不可にしたデッキは、ほかのデッキだけを変えるときに外さなくてよい
func TestSetStarterSetGrantDecks_KeepsAssignedGrantDeckThatWasRetiredAfterwards(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registered := registerStarterSet(t, ctx, commands, "標準")
	retired, other := publishedGrantDeck(t, ctx), publishedGrantDeck(t, ctx)
	assigned, err := commands.SetGrantDecks(ctx, registered.ID, registered.Revision, []uuid.UUID{retired.ID})
	require.NoError(t, err)
	_, err = newGrantDeckHandlerWithRules(t, ctx, smallRules()).Retire(ctx, retired.ID, retired.Revision)
	require.NoError(t, err)

	_, err = commands.SetGrantDecks(ctx, assigned.ID, assigned.Revision, []uuid.UUID{retired.ID, other.ID})

	require.NoError(t, err)
	require.Len(t, restoredStarterSet(t, ctx, registered.ID).GrantDecks, 2)
}

func TestSetStarterSetUnlockBoards_AssignsPublishedBoardsInGivenOrderAndReferences(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registered := registerStarterSet(t, ctx, commands, "標準")
	first, second := publishedBoard(t, ctx), publishedBoard(t, ctx)

	changed, err := commands.SetUnlockBoards(ctx, registered.ID, registered.Revision, []uuid.UUID{second.ID, first.ID})

	require.NoError(t, err)
	require.Equal(t, command.ChangedStarterSet{ID: registered.ID, Revision: registered.Revision + 1}, changed)
	current := restoredStarterSet(t, ctx, registered.ID)
	require.Equal(t, []int64{internalID(t, ctx, second.ID), internalID(t, ctx, first.ID)}, internalBoardIDs(current))
	require.ElementsMatch(t, []int64{internalID(t, ctx, first.ID), internalID(t, ctx, second.ID)},
		constrainttest.Referenced(t, ctx, starterset.BoardReferenceType(), current.ID.Int64()))
}

func TestSetStarterSetUnlockBoards_ReplacesWholeListAndSameBoardsKeepRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registered := registerStarterSet(t, ctx, commands, "標準")
	first, second := publishedBoard(t, ctx), publishedBoard(t, ctx)
	assigned, err := commands.SetUnlockBoards(ctx, registered.ID, registered.Revision, []uuid.UUID{first.ID, second.ID})
	require.NoError(t, err)

	same, err := commands.SetUnlockBoards(ctx, assigned.ID, assigned.Revision, []uuid.UUID{second.ID, first.ID})
	require.NoError(t, err)
	replaced, err := commands.SetUnlockBoards(ctx, same.ID, same.Revision, []uuid.UUID{second.ID})

	require.Equal(t, assigned, same)
	require.NoError(t, err)
	require.Equal(t, assigned.Revision+1, replaced.Revision)
	current := restoredStarterSet(t, ctx, registered.ID)
	require.Equal(t, []int64{internalID(t, ctx, second.ID)}, internalBoardIDs(current))
	require.Equal(t, []int64{internalID(t, ctx, second.ID)}, constrainttest.Referenced(t, ctx, starterset.BoardReferenceType(), current.ID.Int64()))
}

func TestSetStarterSetUnlockBoards_ReturnsFieldErrorsForDuplicateUnknownAndUnpublished(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registered := registerStarterSet(t, ctx, commands, "標準")
	published := publishedBoard(t, ctx)
	draft := publishableBoard(t, ctx, newBoardHandler(t, ctx))
	retired := publishedBoard(t, ctx)
	_, err := newBoardHandler(t, ctx).Retire(ctx, retired.ID, retired.Revision)
	require.NoError(t, err)
	tests := []struct {
		name   string
		boards []uuid.UUID
		want   []input.Error
	}{
		{name: "duplicated", boards: []uuid.UUID{published.ID, published.ID}, want: []input.Error{&input.Duplicated{Field: "unlockBoardIds.1"}}},
		{name: "unknown", boards: []uuid.UUID{published.ID, uuid.New()}, want: []input.Error{&input.NotFound{Field: "unlockBoardIds.1"}}},
		{name: "not_board", boards: []uuid.UUID{registered.ID}, want: []input.Error{&input.NotFound{Field: "unlockBoardIds.0"}}},
		{name: "draft_and_retired", boards: []uuid.UUID{draft.ID, published.ID, retired.ID}, want: []input.Error{&input.Unavailable{Field: "unlockBoardIds.0"}, &input.Unavailable{Field: "unlockBoardIds.2"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := commands.SetUnlockBoards(ctx, registered.ID, registered.Revision, tt.boards)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.Empty(t, restoredStarterSet(t, ctx, registered.ID).UnlockBoards)
		})
	}
}

func TestPublishStarterSet_MakesPublishedWithoutBoardsAndSecondCallKeepsRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	assigned := publishableStarterSet(t, ctx, commands, "標準")

	published, err := commands.Publish(ctx, assigned.ID, assigned.Revision)
	require.NoError(t, err)
	again, err := commands.Publish(ctx, published.ID, published.Revision)

	require.NoError(t, err)
	require.Equal(t, assigned.Revision+1, published.Revision)
	require.Equal(t, published, again)
	require.Equal(t, starterset.StatusPublished, restoredStarterSet(t, ctx, assigned.ID).Status)
}

func TestPublishStarterSet_RequiresGrantDeckEvenWithBoards(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registered := registerStarterSet(t, ctx, commands, "標準")
	_, withoutAnything := commands.Publish(ctx, registered.ID, registered.Revision)
	withBoard, err := commands.SetUnlockBoards(ctx, registered.ID, registered.Revision, []uuid.UUID{publishedBoard(t, ctx).ID})
	require.NoError(t, err)
	_, withBoardOnly := commands.Publish(ctx, withBoard.ID, withBoard.Revision)

	for _, err := range []error{withoutAnything, withBoardOnly} {
		invalid, ok := input.AsErrors(err)
		require.True(t, ok)
		require.Equal(t, input.Errors{&input.Required{Field: "grantDecks"}}, invalid)
	}
	require.Equal(t, starterset.StatusDraft, restoredStarterSet(t, ctx, registered.ID).Status)
}

// 公開中は同時に 1 つだけ。2 つ目の公開は上限のエラーで、今のセットを使用不可にすると公開できる。使用不可から公開に戻すときも上限を確かめる
func TestPublishStarterSet_AllowsOnlyOnePublishedAtATime(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	first := publishableStarterSet(t, ctx, commands, "一")
	second := publishableStarterSet(t, ctx, commands, "二")
	firstPublished, err := commands.Publish(ctx, first.ID, first.Revision)
	require.NoError(t, err)

	_, atLimit := commands.Publish(ctx, second.ID, second.Revision)
	retired, err := commands.Retire(ctx, firstPublished.ID, firstPublished.Revision)
	require.NoError(t, err)
	secondPublished, afterRetire := commands.Publish(ctx, second.ID, second.Revision)
	_, republishAtLimit := commands.Publish(ctx, retired.ID, retired.Revision)

	require.ErrorIs(t, atLimit, command.ErrStarterSetLimitReached)
	var limitReached *command.LimitReachedError
	require.ErrorAs(t, atLimit, &limitReached)
	require.Equal(t, starterset.MaxPublished, limitReached.Max)
	require.NoError(t, afterRetire)
	require.Equal(t, starterset.StatusPublished, restoredStarterSet(t, ctx, secondPublished.ID).Status)
	require.ErrorIs(t, republishAtLimit, command.ErrStarterSetLimitReached)
	require.Equal(t, starterset.StatusRetired, restoredStarterSet(t, ctx, first.ID).Status)
}

func TestRetireStarterSet_MakesRetiredOnlyFromPublishedAndCanBePublishedAgain(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	assigned := publishableStarterSet(t, ctx, commands, "標準")

	_, err := commands.Retire(ctx, assigned.ID, assigned.Revision)
	require.ErrorIs(t, err, command.ErrInvalidStarterSetStatusTransition)

	published, err := commands.Publish(ctx, assigned.ID, assigned.Revision)
	require.NoError(t, err)
	retired, err := commands.Retire(ctx, published.ID, published.Revision)
	require.NoError(t, err)
	require.Equal(t, starterset.StatusRetired, restoredStarterSet(t, ctx, assigned.ID).Status)
	sameRetired, err := commands.Retire(ctx, retired.ID, retired.Revision)
	require.NoError(t, err)
	require.Equal(t, retired, sameRetired)
	again, err := commands.Publish(ctx, retired.ID, retired.Revision)
	require.NoError(t, err)
	require.Equal(t, starterset.StatusPublished, restoredStarterSet(t, ctx, again.ID).Status)
}

// 公開した後は、名前・配るデッキ・解放するボードを変えられず、消せない（変えたいときは使用不可にして新しく作る）
func TestStarterSetHandler_RejectsEditingUnlessDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	assigned := publishableStarterSet(t, ctx, commands, "標準")
	published, err := commands.Publish(ctx, assigned.ID, assigned.Revision)
	require.NoError(t, err)

	_, err = commands.Rename(ctx, published.ID, published.Revision, "新標準")
	require.ErrorIs(t, err, command.ErrStarterSetNotDraft)
	_, err = commands.SetGrantDecks(ctx, published.ID, published.Revision, []uuid.UUID{publishedGrantDeck(t, ctx).ID})
	require.ErrorIs(t, err, command.ErrStarterSetNotDraft)
	_, err = commands.SetUnlockBoards(ctx, published.ID, published.Revision, []uuid.UUID{publishedBoard(t, ctx).ID})
	require.ErrorIs(t, err, command.ErrStarterSetNotDraft)
	require.ErrorIs(t, commands.Delete(ctx, published.ID, published.Revision), command.ErrStarterSetNotDraft)
}

func TestSetStarterSetUnlockNPCAvatars_AssignsPublishedNPCAvatarsInGivenOrderAndReferences(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registered := registerStarterSet(t, ctx, commands, "標準")
	first, second := publishedNPCAvatar(t, ctx), publishedNPCAvatar(t, ctx)

	changed, err := commands.SetUnlockNPCAvatars(ctx, registered.ID, registered.Revision, []uuid.UUID{second.ID, first.ID})

	require.NoError(t, err)
	require.Equal(t, command.ChangedStarterSet{ID: registered.ID, Revision: registered.Revision + 1}, changed)
	current := restoredStarterSet(t, ctx, registered.ID)
	require.Equal(t, []int64{internalID(t, ctx, second.ID), internalID(t, ctx, first.ID)}, internalNPCAvatarIDs(current))
	require.ElementsMatch(t, []int64{internalID(t, ctx, first.ID), internalID(t, ctx, second.ID)},
		constrainttest.Referenced(t, ctx, starterset.NPCAvatarReferenceType(), current.ID.Int64()))
}

func TestSetStarterSetUnlockNPCAvatars_ReplacesWholeListAndSameNPCAvatarsKeepRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registered := registerStarterSet(t, ctx, commands, "標準")
	first, second := publishedNPCAvatar(t, ctx), publishedNPCAvatar(t, ctx)
	assigned, err := commands.SetUnlockNPCAvatars(ctx, registered.ID, registered.Revision, []uuid.UUID{first.ID, second.ID})
	require.NoError(t, err)

	same, err := commands.SetUnlockNPCAvatars(ctx, assigned.ID, assigned.Revision, []uuid.UUID{second.ID, first.ID})
	require.NoError(t, err)
	replaced, err := commands.SetUnlockNPCAvatars(ctx, same.ID, same.Revision, []uuid.UUID{second.ID})

	require.Equal(t, assigned, same)
	require.NoError(t, err)
	require.Equal(t, assigned.Revision+1, replaced.Revision)
	current := restoredStarterSet(t, ctx, registered.ID)
	require.Equal(t, []int64{internalID(t, ctx, second.ID)}, internalNPCAvatarIDs(current))
	require.Equal(t, []int64{internalID(t, ctx, second.ID)}, constrainttest.Referenced(t, ctx, starterset.NPCAvatarReferenceType(), current.ID.Int64()))
}

func TestSetStarterSetUnlockNPCAvatars_ReturnsFieldErrorsForDuplicateUnknownAndUnpublished(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	registered := registerStarterSet(t, ctx, commands, "標準")
	published := publishedNPCAvatar(t, ctx)
	draft := publishableNPCAvatar(t, ctx, newNPCAvatarHandler(t, ctx), "準備中-"+uuid.NewString()[:8])
	retired := publishedNPCAvatar(t, ctx)
	_, err := newNPCAvatarHandler(t, ctx).Retire(ctx, retired.ID, retired.Revision)
	require.NoError(t, err)
	tests := []struct {
		name string
		ids  []uuid.UUID
		want []input.Error
	}{
		{name: "duplicated", ids: []uuid.UUID{published.ID, published.ID}, want: []input.Error{&input.Duplicated{Field: "unlockNpcAvatarIds.1"}}},
		{name: "unknown", ids: []uuid.UUID{published.ID, uuid.New()}, want: []input.Error{&input.NotFound{Field: "unlockNpcAvatarIds.1"}}},
		{name: "not_npc_avatar", ids: []uuid.UUID{registered.ID}, want: []input.Error{&input.NotFound{Field: "unlockNpcAvatarIds.0"}}},
		{name: "draft_and_retired", ids: []uuid.UUID{draft.ID, published.ID, retired.ID}, want: []input.Error{&input.Unavailable{Field: "unlockNpcAvatarIds.0"}, &input.Unavailable{Field: "unlockNpcAvatarIds.2"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := commands.SetUnlockNPCAvatars(ctx, registered.ID, registered.Revision, tt.ids)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.Empty(t, restoredStarterSet(t, ctx, registered.ID).UnlockNPCAvatars)
		})
	}
}

// 解放する NPC のアバターは 0 件でも公開できる。公開した後は変えられない
func TestStarterSetHandler_RejectsSettingUnlockNPCAvatarsUnlessDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newStarterSetHandler(t, ctx)
	assigned := publishableStarterSet(t, ctx, commands, "標準")
	published, err := commands.Publish(ctx, assigned.ID, assigned.Revision)
	require.NoError(t, err)

	_, err = commands.SetUnlockNPCAvatars(ctx, published.ID, published.Revision, []uuid.UUID{publishedNPCAvatar(t, ctx).ID})

	require.ErrorIs(t, err, command.ErrStarterSetNotDraft)
}
