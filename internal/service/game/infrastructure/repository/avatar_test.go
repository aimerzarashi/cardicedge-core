package repository_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
)

func TestAvatarReader_ListsOwnAvatarsInRegistrationOrder(t *testing.T) {
	f := newPlayerFixture(t)
	playerID, ownerPublicID := f.issuePlayer(t)
	other, otherPublicID := f.issuePlayer(t)
	writer := repository.NewAvatarWriter(identifier.NewEntityRefResolver(f.registry))
	base := time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	older := readmodel.Avatar{AvatarID: uuid.New(), Player: playerID, Name: "先", RegisteredAt: base}
	newer := readmodel.Avatar{AvatarID: uuid.New(), Player: playerID, Name: "後", RegisteredAt: base.Add(time.Second)}
	others := readmodel.Avatar{AvatarID: uuid.New(), Player: other, Name: "他", RegisteredAt: base}
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
	playerID, ownerPublicID := f.issuePlayer(t)
	writer := repository.NewAvatarWriter(identifier.NewEntityRefResolver(f.registry))
	avatar := readmodel.Avatar{AvatarID: uuid.New(), Player: playerID, Name: "アリス", RegisteredAt: time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)}

	require.NoError(t, writer.Upsert(f.ctx, avatar))
	require.NoError(t, writer.Upsert(f.ctx, avatar))
	got, err := repository.NewAvatarReader().ListByPlayer(f.ctx, ownerPublicID)

	require.NoError(t, err)
	require.Len(t, got, 1)
}

func TestAvatarWriter_DeleteRemovesRowAndToleratesMissing(t *testing.T) {
	f := newPlayerFixture(t)
	playerID, ownerPublicID := f.issuePlayer(t)
	writer := repository.NewAvatarWriter(identifier.NewEntityRefResolver(f.registry))
	avatar := readmodel.Avatar{AvatarID: uuid.New(), Player: playerID, Name: "アリス", RegisteredAt: time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)}
	require.NoError(t, writer.Upsert(f.ctx, avatar))

	require.NoError(t, writer.Delete(f.ctx, avatar))
	require.NoError(t, writer.Delete(f.ctx, avatar))
	got, err := repository.NewAvatarReader().ListByPlayer(f.ctx, ownerPublicID)

	require.NoError(t, err)
	require.Empty(t, got)
}

// アバターを作るのは持ち主のプレイヤー自身で、更新の操作は無いため、登録も更新も持ち主の公開 ID になる
func TestAvatarWriter_RecordsOwnerPublicIDAsRegisteredAndUpdatedBy(t *testing.T) {
	f := newPlayerFixture(t)
	playerID, ownerPublicID := f.issuePlayer(t)
	writer := repository.NewAvatarWriter(identifier.NewEntityRefResolver(f.registry))
	registeredAt := time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)
	avatar := readmodel.Avatar{AvatarID: uuid.New(), Player: playerID, Name: "アリス", RegisteredAt: registeredAt}
	require.NoError(t, writer.Upsert(f.ctx, avatar))

	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	row, err := models.FindAvatar(f.ctx, exec, avatar.AvatarID.String())
	require.NoError(t, err)
	var metadata readmodel.AvatarMetadata
	require.NoError(t, json.Unmarshal(row.Metadata, &metadata))

	require.Equal(t, ownerPublicID, metadata.RegisteredBy)
	require.Equal(t, ownerPublicID, metadata.UpdatedBy)
	require.Equal(t, registeredAt, metadata.RegisteredAt.UTC())
	require.Equal(t, registeredAt, metadata.UpdatedAt.UTC())
}
