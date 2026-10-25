package repository_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
)

func TestAvatarReader_ListsOwnAvatarsInRegistrationOrder(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	other, otherPublicID := f.issuePlayer(t)
	writer := repository.NewAvatarWriter(identifier.NewEntityRefResolver(f.registry))
	base := time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	older := readmodel.Avatar{AvatarID: uuid.New(), Owner: owner, Name: "先", RegisteredAt: base}
	newer := readmodel.Avatar{AvatarID: uuid.New(), Owner: owner, Name: "後", RegisteredAt: base.Add(time.Second)}
	others := readmodel.Avatar{AvatarID: uuid.New(), Owner: other, Name: "他", RegisteredAt: base}
	// 作った順と逆に書いても、作った順に読む
	for _, avatar := range []readmodel.Avatar{newer, others, older} {
		require.NoError(t, writer.Upsert(f.ctx, avatar))
	}

	got, err := repository.NewAvatarReader().ListByPlayer(f.ctx, ownerPublicID)
	require.NoError(t, err)
	gotOther, err := repository.NewAvatarReader().ListByPlayer(f.ctx, otherPublicID)
	require.NoError(t, err)

	require.Equal(t, []query.Avatar{{ID: older.AvatarID, Name: "先"}, {ID: newer.AvatarID, Name: "後"}}, got)
	require.Equal(t, []query.Avatar{{ID: others.AvatarID, Name: "他"}}, gotOther)
}

func TestAvatarWriter_UpsertTwiceKeepsOneRow(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	writer := repository.NewAvatarWriter(identifier.NewEntityRefResolver(f.registry))
	avatar := readmodel.Avatar{AvatarID: uuid.New(), Owner: owner, Name: "アリス", RegisteredAt: time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)}

	require.NoError(t, writer.Upsert(f.ctx, avatar))
	require.NoError(t, writer.Upsert(f.ctx, avatar))
	got, err := repository.NewAvatarReader().ListByPlayer(f.ctx, ownerPublicID)

	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestAvatarWriter_DeleteRemovesRowAndToleratesMissing(t *testing.T) {
	f := newPlayerFixture(t)
	owner, ownerPublicID := f.issuePlayer(t)
	writer := repository.NewAvatarWriter(identifier.NewEntityRefResolver(f.registry))
	avatar := readmodel.Avatar{AvatarID: uuid.New(), Owner: owner, Name: "アリス", RegisteredAt: time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)}
	require.NoError(t, writer.Upsert(f.ctx, avatar))

	require.NoError(t, writer.Delete(f.ctx, avatar))
	require.NoError(t, writer.Delete(f.ctx, avatar))
	got, err := repository.NewAvatarReader().ListByPlayer(f.ctx, ownerPublicID)

	require.NoError(t, err)
	require.Empty(t, got)
}
