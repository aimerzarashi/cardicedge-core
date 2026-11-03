package repository_test

import (
	"encoding/json"
	"github.com/google/uuid"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/board"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/domain/npcavatar"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/identifier"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/infrastructure/repository"
)

func TestUnlockWriter_WritesBoardsWithPublicIDsOfUnlockAndPlayer(t *testing.T) {
	f := newPlayerFixture(t)
	playerID, playerPublicID := f.issuePlayer(t)
	unlockID, unlockPublicID := f.issueUnlock(t)
	// 解放したボードは game の内部 ID で持ち、read model には公開 ID（publish-master が domain_entities に作った行）で書く
	boardB, boardBPublicID := issueBoardEntity(t, f.ctx)
	boardA, boardAPublicID := issueBoardEntity(t, f.ctx)
	npcAvatar, npcAvatarPublicID := issueNPCAvatarEntity(t, f.ctx)
	registeredAt := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)

	require.NoError(t, repository.NewUnlockWriter(identifier.NewEntityRefResolver(f.registry)).Upsert(f.ctx, readmodel.Unlock{
		Unlock: unlockID, Player: playerID, Boards: []board.ID{boardB, boardA}, NPCAvatars: []npcavatar.ID{npcAvatar}, RegisteredAt: registeredAt, UpdatedAt: registeredAt,
	}))

	exec, err := rdb.GetExecutor(f.ctx)
	require.NoError(t, err)
	row, err := models.FindUnlock(f.ctx, exec, unlockPublicID.String())
	require.NoError(t, err)
	require.Equal(t, playerPublicID.String(), row.PlayerID)
	require.True(t, registeredAt.Equal(row.RegisteredAt))
	var metadata readmodel.UnlockMetadata
	require.NoError(t, json.Unmarshal(row.Metadata, &metadata))
	require.Equal(t, readmodel.UnlockMetadata{
		ID: unlockPublicID, Boards: []uuid.UUID{boardBPublicID, boardAPublicID}, NPCAvatars: []uuid.UUID{npcAvatarPublicID}, RegisteredAt: registeredAt, UpdatedAt: registeredAt,
		RegisteredBy: playerPublicID, UpdatedBy: playerPublicID,
	}, metadata)
}
