package command_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

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
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")

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
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")

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

func TestNPCAvatarHandler_RejectsRenameAndDeleteUnlessDraft(t *testing.T) {
	ctx := signedIn(t, isolatedDatabase(t), registrant(t))
	commands := newNPCAvatarHandler(t, ctx)
	registered := registerNPCAvatar(t, ctx, commands, "森の番人")
	published, err := commands.Publish(ctx, registered.ID, registered.Revision)
	require.NoError(t, err)

	_, err = commands.Rename(ctx, published.ID, published.Revision, "炎の剣士")
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
