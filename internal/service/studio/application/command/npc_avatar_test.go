package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint/constrainttest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domaindef"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/command"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/studio/application/input"
)

func restoredNPCAvatar(t *testing.T, ctx context.Context, publicID uuid.UUID) npcavatar.NPCAvatar {
	t.Helper()
	registry, err := domaindef.Load(ctx)
	require.NoError(t, err)
	id, err := identifier.New(registry, npcavatar.NewID).Resolve(ctx, publicID)
	require.NoError(t, err)
	state, err := npcAvatarStore(registry).Load(ctx, id)
	require.NoError(t, err)
	return state.(npcavatar.NPCAvatar)
}

func registerNPCAvatar(t *testing.T, ctx context.Context, commands *command.NPCAvatarHandler, name string) command.ChangedNPCAvatar {
	t.Helper()
	registered, err := commands.Register(ctx, name)
	require.NoError(t, err)
	return registered
}

// 公開中の NPC のデッキ（smallRules で公開できるカード 4 枚）
func publishedNPCDeck(t *testing.T, ctx context.Context) command.ChangedNPCDeck {
	t.Helper()
	commands := newNPCDeckHandlerWithRules(t, ctx, smallRules())
	filled := fullNPCDeck(t, ctx, commands)
	published, err := commands.Publish(ctx, filled.ID, filled.Revision)
	require.NoError(t, err)
	return published
}

// 公開中の NPC のデッキを 1 つ使う、準備中の NPC のアバター
func publishableNPCAvatar(t *testing.T, ctx context.Context, commands *command.NPCAvatarHandler, name string) command.ChangedNPCAvatar {
	t.Helper()
	registered := registerNPCAvatar(t, ctx, commands, name)
	assigned, err := commands.SetDecks(ctx, registered.ID, registered.Revision, []uuid.UUID{publishedNPCDeck(t, ctx).ID})
	require.NoError(t, err)
	return assigned
}

func internalDeckIDs(current npcavatar.NPCAvatar) []int64 {
	ids := make([]int64, 0, len(current.Decks))
	for _, deck := range current.Decks {
		ids = append(ids, deck.Int64())
	}
	return ids
}

func TestRegisterNPCAvatar_ReturnsPublicIDAndMakesDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))

	registered, err := newNPCAvatarHandler(t, ctx).Register(ctx, "  見習いの魔術師 ")

	require.NoError(t, err)
	require.Equal(t, int64(1), registered.Revision)
	current := restoredNPCAvatar(t, ctx, registered.ID)
	require.Equal(t, "見習いの魔術師", current.Name.String())
	require.Equal(t, npcavatar.StatusDraft, current.Status)
}

func TestRegisterNPCAvatar_RejectsNameAlreadyInUse(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registerNPCAvatar(t, ctx, commands, "ＡＢＣ")

	_, err := commands.Register(ctx, " abc")

	require.ErrorIs(t, err, command.ErrNPCAvatarNameAlreadyInUse)
}

func TestRegisterNPCAvatar_AllowsNameUsedByBoard(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	_, err := newBoardHandler(t, ctx).Register(ctx, "草原")
	require.NoError(t, err)

	_, err = newNPCAvatarHandler(t, ctx).Register(ctx, "草原")

	require.NoError(t, err)
}

func TestRegisterNPCAvatar_RejectsInvalidName(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  input.Error
	}{
		{name: "blank", value: " ", want: &input.Required{Field: "name"}},
		{name: "too_long", value: strings.Repeat("あ", npcavatar.MaxNameLength+1), want: &input.TooLong{Field: "name", Max: npcavatar.MaxNameLength}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := signedIn(t, isolatedDatabase(t), registrant(t))

			_, err := newNPCAvatarHandler(t, ctx).Register(ctx, tt.value)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, input.Errors{tt.want}, invalid)
		})
	}
}

func TestNPCAvatarHandler_RejectsCallWithoutOperator(t *testing.T) {
	ctx := isolatedDatabase(t)
	commands := newNPCAvatarHandler(t, ctx)

	_, err := commands.Register(ctx, "森の番人")
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.Rename(ctx, uuid.New(), 1, "炎の剣士")
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.SetDecks(ctx, uuid.New(), 1, nil)
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.Publish(ctx, uuid.New(), 1)
	require.ErrorIs(t, err, command.ErrNotOperator)
	_, err = commands.Retire(ctx, uuid.New(), 1)
	require.ErrorIs(t, err, command.ErrNotOperator)
	require.ErrorIs(t, commands.Delete(ctx, uuid.New(), 1), command.ErrNotOperator)
}

func TestRenameNPCAvatar_ChangesNameAndFreesOldName(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")

	renamed, err := commands.Rename(ctx, registered.ID, registered.Revision, "炎の剣士")

	require.NoError(t, err)
	require.Equal(t, int64(2), renamed.Revision)
	require.Equal(t, "炎の剣士", restoredNPCAvatar(t, ctx, registered.ID).Name.String())
	registerNPCAvatar(t, ctx, commands, "森の番人")
}

func TestRenameNPCAvatar_SameNameKeepsRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")

	renamed, err := commands.Rename(ctx, registered.ID, registered.Revision, "森の番人")

	require.NoError(t, err)
	require.Equal(t, registered.Revision, renamed.Revision)
}

func TestRenameNPCAvatar_RejectsNameUsedByOtherNPCAvatar(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registerNPCAvatar(t, ctx, commands, "森の番人")
	other := registerNPCAvatar(t, ctx, commands, "炎の剣士")

	_, err := commands.Rename(ctx, other.ID, other.Revision, "森の番人")

	require.ErrorIs(t, err, command.ErrNPCAvatarNameAlreadyInUse)
}

func TestNPCAvatarHandler_RejectsStaleRevisionAndMissingNPCAvatar(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")
	_, err := commands.Rename(ctx, registered.ID, registered.Revision, "炎の剣士")
	require.NoError(t, err)
	operations := []struct {
		name string
		call func(id uuid.UUID, revision int64) error
	}{
		{name: "rename", call: func(id uuid.UUID, revision int64) error {
			_, err := commands.Rename(ctx, id, revision, "湖の精霊")
			return err
		}},
		{name: "set_decks", call: func(id uuid.UUID, revision int64) error {
			_, err := commands.SetDecks(ctx, id, revision, nil)
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
		t.Run(op.name+"_missing_npc_avatar", func(t *testing.T) {
			require.ErrorIs(t, op.call(uuid.New(), 1), command.ErrNPCAvatarNotFound)
		})
	}
}

func TestPublishNPCAvatar_MakesPublishedAndSecondCallKeepsRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := publishableNPCAvatar(t, ctx, commands, "森の番人")

	published, err := commands.Publish(ctx, registered.ID, registered.Revision)
	require.NoError(t, err)
	again, err := commands.Publish(ctx, published.ID, published.Revision)

	require.NoError(t, err)
	require.Equal(t, registered.Revision+1, published.Revision)
	require.Equal(t, published.Revision, again.Revision)
	require.Equal(t, npcavatar.StatusPublished, restoredNPCAvatar(t, ctx, registered.ID).Status)
}

func TestRetireNPCAvatar_MakesRetiredOnlyFromPublishedAndCanBePublishedAgain(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := publishableNPCAvatar(t, ctx, commands, "森の番人")

	_, err := commands.Retire(ctx, registered.ID, registered.Revision)
	require.ErrorIs(t, err, command.ErrInvalidNPCAvatarStatusTransition)

	published, err := commands.Publish(ctx, registered.ID, registered.Revision)
	require.NoError(t, err)
	retired, err := commands.Retire(ctx, published.ID, published.Revision)
	require.NoError(t, err)
	require.Equal(t, npcavatar.StatusRetired, restoredNPCAvatar(t, ctx, registered.ID).Status)
	again, err := commands.Publish(ctx, retired.ID, retired.Revision)
	require.NoError(t, err)
	require.Equal(t, npcavatar.StatusPublished, restoredNPCAvatar(t, ctx, again.ID).Status)
}

func TestNPCAvatarHandler_RejectsEditingUnlessDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := publishableNPCAvatar(t, ctx, commands, "森の番人")
	published, err := commands.Publish(ctx, registered.ID, registered.Revision)
	require.NoError(t, err)

	_, err = commands.Rename(ctx, published.ID, published.Revision, "炎の剣士")
	require.ErrorIs(t, err, command.ErrNPCAvatarNotDraft)
	_, err = commands.SetDecks(ctx, published.ID, published.Revision, []uuid.UUID{publishedNPCDeck(t, ctx).ID})
	require.ErrorIs(t, err, command.ErrNPCAvatarNotDraft)
	require.ErrorIs(t, commands.Delete(ctx, published.ID, published.Revision), command.ErrNPCAvatarNotDraft)
}

func TestDeleteNPCAvatar_RemovesNPCAvatarAndFreesName(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")

	require.NoError(t, commands.Delete(ctx, registered.ID, registered.Revision))

	_, err := commands.Rename(ctx, registered.ID, registered.Revision, "炎の剣士")
	require.ErrorIs(t, err, command.ErrNPCAvatarNotFound)
	registerNPCAvatar(t, ctx, commands, "森の番人")
}

func TestSetNPCAvatarDecks_AssignsPublishedDecksAndReferences(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")
	first, second := publishedNPCDeck(t, ctx), publishedNPCDeck(t, ctx)

	changed, err := commands.SetDecks(ctx, registered.ID, registered.Revision, []uuid.UUID{second.ID, first.ID})

	require.NoError(t, err)
	require.Equal(t, command.ChangedNPCAvatar{ID: registered.ID, Revision: registered.Revision + 1}, changed)
	current := restoredNPCAvatar(t, ctx, registered.ID)
	require.Equal(t, []int64{internalID(t, ctx, second.ID), internalID(t, ctx, first.ID)}, internalDeckIDs(current))
	require.ElementsMatch(t, []int64{internalID(t, ctx, first.ID), internalID(t, ctx, second.ID)},
		constrainttest.Referenced(t, ctx, npcavatar.DeckReferenceType(), current.ID.Int64()))
}

func TestSetNPCAvatarDecks_ReplacesWholeListAndReferences(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")
	first, second := publishedNPCDeck(t, ctx), publishedNPCDeck(t, ctx)
	assigned, err := commands.SetDecks(ctx, registered.ID, registered.Revision, []uuid.UUID{first.ID})
	require.NoError(t, err)

	replaced, err := commands.SetDecks(ctx, assigned.ID, assigned.Revision, []uuid.UUID{second.ID})
	require.NoError(t, err)
	cleared, err := commands.SetDecks(ctx, replaced.ID, replaced.Revision, nil)

	require.NoError(t, err)
	// 割り当て（+1）→ 入れ替え（外す +1・割り当てる +1）→ すべて外す（+1）
	require.Equal(t, assigned.Revision+2, replaced.Revision)
	require.Equal(t, registered.Revision+4, cleared.Revision)
	current := restoredNPCAvatar(t, ctx, registered.ID)
	require.Empty(t, current.Decks)
	require.Empty(t, constrainttest.Referenced(t, ctx, npcavatar.DeckReferenceType(), current.ID.Int64()))
}

// 同じ NPC のデッキを、複数の NPC のアバターが使える
func TestSetNPCAvatarDecks_AllowsDeckSharedByNPCAvatars(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	deck := publishedNPCDeck(t, ctx)

	for _, name := range []string{"森の番人", "炎の剣士"} {
		registered := registerNPCAvatar(t, ctx, commands, name)
		_, err := commands.SetDecks(ctx, registered.ID, registered.Revision, []uuid.UUID{deck.ID})
		require.NoError(t, err)
	}
}

func TestSetNPCAvatarDecks_SameDecksKeepRevision(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")
	first, second := publishedNPCDeck(t, ctx), publishedNPCDeck(t, ctx)
	assigned, err := commands.SetDecks(ctx, registered.ID, registered.Revision, []uuid.UUID{first.ID, second.ID})
	require.NoError(t, err)

	same, err := commands.SetDecks(ctx, assigned.ID, assigned.Revision, []uuid.UUID{second.ID, first.ID})

	require.NoError(t, err)
	require.Equal(t, assigned, same)
}

func TestSetNPCAvatarDecks_ReturnsFieldErrorsForDuplicateUnknownAndUnpublished(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")
	published := publishedNPCDeck(t, ctx)
	draft := fullNPCDeck(t, ctx, newNPCDeckHandlerWithRules(t, ctx, smallRules()))
	retired := publishedNPCDeck(t, ctx)
	_, err := newNPCDeckHandlerWithRules(t, ctx, smallRules()).Retire(ctx, retired.ID, retired.Revision)
	require.NoError(t, err)
	deleted := registerNPCDeck(t, ctx, newNPCDeckHandler(t, ctx), "消すデッキ")
	require.NoError(t, newNPCDeckHandler(t, ctx).Delete(ctx, deleted.ID, deleted.Revision))
	tests := []struct {
		name  string
		decks []uuid.UUID
		want  []input.Error
	}{
		{name: "duplicated", decks: []uuid.UUID{published.ID, published.ID}, want: []input.Error{&input.Duplicated{Field: "deckIds.1"}}},
		{name: "unknown", decks: []uuid.UUID{published.ID, uuid.New()}, want: []input.Error{&input.NotFound{Field: "deckIds.1"}}},
		{name: "deleted", decks: []uuid.UUID{deleted.ID}, want: []input.Error{&input.NotFound{Field: "deckIds.0"}}},
		{name: "not_npc_deck", decks: []uuid.UUID{registered.ID}, want: []input.Error{&input.NotFound{Field: "deckIds.0"}}},
		{name: "draft_and_retired", decks: []uuid.UUID{draft.ID, published.ID, retired.ID}, want: []input.Error{&input.Unavailable{Field: "deckIds.0"}, &input.Unavailable{Field: "deckIds.2"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := commands.SetDecks(ctx, registered.ID, registered.Revision, tt.decks)

			invalid, ok := input.AsErrors(err)
			require.True(t, ok)
			require.Equal(t, tt.want, []input.Error(invalid))
			require.Empty(t, restoredNPCAvatar(t, ctx, registered.ID).Decks)
		})
	}
}

// 割り当てた後に使用不可にしたデッキは、ほかのデッキだけを変えるときに外さなくてよい
func TestSetNPCAvatarDecks_KeepsAssignedDeckThatWasRetiredAfterwards(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")
	retired, other := publishedNPCDeck(t, ctx), publishedNPCDeck(t, ctx)
	assigned, err := commands.SetDecks(ctx, registered.ID, registered.Revision, []uuid.UUID{retired.ID})
	require.NoError(t, err)
	_, err = newNPCDeckHandlerWithRules(t, ctx, smallRules()).Retire(ctx, retired.ID, retired.Revision)
	require.NoError(t, err)

	_, err = commands.SetDecks(ctx, assigned.ID, assigned.Revision, []uuid.UUID{retired.ID, other.ID})

	require.NoError(t, err)
	require.Len(t, restoredNPCAvatar(t, ctx, registered.ID).Decks, 2)
}

func TestPublishNPCAvatar_RequiresAssignedDeck(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")

	_, err := commands.Publish(ctx, registered.ID, registered.Revision)

	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, input.Errors{&input.Required{Field: "decks"}}, invalid)
	require.Equal(t, npcavatar.StatusDraft, restoredNPCAvatar(t, ctx, registered.ID).Status)
}

func TestPublishNPCAvatar_RequiresAtLeastOnePublishedDeck(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")
	retiredDeck, otherDeck := publishedNPCDeck(t, ctx), publishedNPCDeck(t, ctx)
	assigned, err := commands.SetDecks(ctx, registered.ID, registered.Revision, []uuid.UUID{retiredDeck.ID, otherDeck.ID})
	require.NoError(t, err)
	decks := newNPCDeckHandlerWithRules(t, ctx, smallRules())
	_, err = decks.Retire(ctx, retiredDeck.ID, retiredDeck.Revision)
	require.NoError(t, err)

	published, err := commands.Publish(ctx, assigned.ID, assigned.Revision)
	require.NoError(t, err)
	require.Equal(t, npcavatar.StatusPublished, restoredNPCAvatar(t, ctx, published.ID).Status)
}

func TestPublishNPCAvatar_RejectsWhenAllAssignedDecksAreRetired(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")
	deck := publishedNPCDeck(t, ctx)
	assigned, err := commands.SetDecks(ctx, registered.ID, registered.Revision, []uuid.UUID{deck.ID})
	require.NoError(t, err)
	_, err = newNPCDeckHandlerWithRules(t, ctx, smallRules()).Retire(ctx, deck.ID, deck.Revision)
	require.NoError(t, err)

	_, err = commands.Publish(ctx, assigned.ID, assigned.Revision)

	invalid, ok := input.AsErrors(err)
	require.True(t, ok)
	require.Equal(t, input.Errors{&input.Unavailable{Field: "decks"}}, invalid)
}

// 使用不可から公開に戻すときも、公開のときと同じ条件を確かめる。すでに公開中なら確かめずに成功する
func TestPublishNPCAvatar_ChecksDecksAgainWhenPublishingRetiredButNotWhenAlreadyPublished(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")
	deck := publishedNPCDeck(t, ctx)
	assigned, err := commands.SetDecks(ctx, registered.ID, registered.Revision, []uuid.UUID{deck.ID})
	require.NoError(t, err)
	published, err := commands.Publish(ctx, assigned.ID, assigned.Revision)
	require.NoError(t, err)
	_, err = newNPCDeckHandlerWithRules(t, ctx, smallRules()).Retire(ctx, deck.ID, deck.Revision)
	require.NoError(t, err)

	again, againErr := commands.Publish(ctx, published.ID, published.Revision)
	retired, err := commands.Retire(ctx, published.ID, published.Revision)
	require.NoError(t, err)
	_, republishErr := commands.Publish(ctx, retired.ID, retired.Revision)

	require.NoError(t, againErr)
	require.Equal(t, published, again)
	invalid, ok := input.AsErrors(republishErr)
	require.True(t, ok)
	require.Equal(t, input.Errors{&input.Unavailable{Field: "decks"}}, invalid)
}
