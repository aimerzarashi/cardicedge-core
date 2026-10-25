package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aarondl/sqlboiler/v4/queries/qm"
	"github.com/google/uuid"

	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/orm/sqlboiler/models"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/platform/rdb"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query"
	"gitlab.com/aimerzarashi/diecardomain/core/internal/service/game/application/query/readmodel"
)

type AvatarReader struct{}

func NewAvatarReader() *AvatarReader {
	return &AvatarReader{}
}

// プレイヤーのアバターを作った順に読む（同じ日時は公開 ID の順）。1 人の数に上限があるため、ページ分けしない
func (r *AvatarReader) ListByPlayer(ctx context.Context, playerID uuid.UUID) ([]query.Avatar, error) {
	exec, err := rdb.GetExecutor(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := models.Avatars(
		models.AvatarWhere.PlayerID.EQ(playerID.String()),
		qm.OrderBy(models.AvatarColumns.RegisteredAt+", "+models.AvatarColumns.AvatarID),
	).All(ctx, exec)
	if err != nil {
		return nil, err
	}
	avatars := make([]query.Avatar, len(rows))
	for i, row := range rows {
		var metadata readmodel.AvatarMetadata
		if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
			return nil, fmt.Errorf("avatars.metadata of %s: %w", row.AvatarID, err)
		}
		avatars[i] = query.Avatar{ID: metadata.ID, Name: metadata.Name}
	}
	return avatars, nil
}
