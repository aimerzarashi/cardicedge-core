package constraint_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/constraint"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/player/playertest"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
)

func (f fixture) issuePlayer(t *testing.T) player.ID {
	t.Helper()
	id, _, err := identifier.New(f.registry, player.NewID).Issue(f.ctx)
	require.NoError(t, err)
	return id
}

func (f fixture) updatePlayerSubject(t *testing.T, id player.ID, issuer, subject string) error {
	t.Helper()
	return constraint.UpdatePlayerSubject(f.ctx, id, playertest.Issuer(t, issuer), playertest.Subject(t, subject))
}

func TestUpdatePlayerSubject_RejectsSubjectRegisteredByOtherPlayer(t *testing.T) {
	f := setup(t)
	subject := "user-" + uuid.NewString()
	require.NoError(t, f.updatePlayerSubject(t, f.issuePlayer(t), "diecardomain-game", subject))

	err := f.updatePlayerSubject(t, f.issuePlayer(t), "diecardomain-game", subject)

	require.ErrorIs(t, err, player.ErrAlreadyRegistered)
}

func TestUpdatePlayerSubject_AllowsSameSubjectFromOtherIssuer(t *testing.T) {
	f := setup(t)
	subject := "user-" + uuid.NewString()
	require.NoError(t, f.updatePlayerSubject(t, f.issuePlayer(t), "diecardomain-game", subject))

	err := f.updatePlayerSubject(t, f.issuePlayer(t), "other-issuer", subject)

	require.NoError(t, err)
}

func TestFindPlayerBySubject_ReturnsRegisteredPlayer(t *testing.T) {
	f := setup(t)
	subject := "user-" + uuid.NewString()
	id := f.issuePlayer(t)
	require.NoError(t, f.updatePlayerSubject(t, id, "diecardomain-game", subject))

	got, found, err := constraint.FindPlayerBySubject(f.ctx, playertest.Issuer(t, "diecardomain-game"), playertest.Subject(t, subject))
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, id, got)

	_, found, err = constraint.FindPlayerBySubject(f.ctx, playertest.Issuer(t, "diecardomain-game"), playertest.Subject(t, subject+"-other"))
	require.NoError(t, err)
	require.False(t, found)
}
